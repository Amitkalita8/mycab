package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	db          *sql.DB
	staticDir   string // built React app to serve; empty = API only
	allowOrigin string // CORS origin; empty or "*" allows any
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})

	// auth
	mux.HandleFunc("POST /api/auth/register/rider", s.registerRider)
	mux.HandleFunc("POST /api/auth/register/driver", s.registerDriver)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.auth("", s.logout))
	mux.HandleFunc("GET /api/me", s.auth("", s.me))

	// public
	mux.HandleFunc("POST /api/fare-estimate", s.fareEstimate)

	// driver
	mux.HandleFunc("PATCH /api/driver/availability", s.auth(RoleDriver, s.setAvailability))
	mux.HandleFunc("PUT /api/driver/location", s.auth(RoleDriver, s.updateLocation))

	// rider
	mux.HandleFunc("GET /api/drivers/nearby", s.auth(RoleRider, s.nearbyDrivers))
	mux.HandleFunc("POST /api/rides", s.auth(RoleRider, s.createRide))
	mux.HandleFunc("POST /api/rides/{id}/pay", s.auth(RoleRider, s.payRide))
	mux.HandleFunc("POST /api/rides/{id}/rate", s.auth(RoleRider, s.rateRide))

	// shared (scoped to the caller inside)
	mux.HandleFunc("GET /api/rides", s.auth("", s.listRides))
	mux.HandleFunc("GET /api/rides/{id}", s.auth("", s.getRide))
	mux.HandleFunc("POST /api/rides/{id}/cancel", s.auth("", s.statusAction(StatusCancelled)))

	// driver ride actions
	mux.HandleFunc("POST /api/rides/{id}/accept", s.auth(RoleDriver, s.acceptRide))
	mux.HandleFunc("POST /api/rides/{id}/start", s.auth(RoleDriver, s.statusAction(StatusStarted)))
	mux.HandleFunc("POST /api/rides/{id}/complete", s.auth(RoleDriver, s.statusAction(StatusCompleted)))

	// unknown /api paths get JSON 404s; everything else is the React app (if configured)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "not found")
	})
	if s.staticDir != "" {
		mux.Handle("/", spaHandler(s.staticDir))
	}

	return s.cors(mux)
}

// spaHandler serves files from dir, falling back to index.html so client-side routes work.
func spaHandler(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if st, err := os.Stat(p); err != nil || st.IsDir() && r.URL.Path != "/" {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		files.ServeHTTP(w, r)
	})
}

// cors lets a separately hosted frontend call the API. Not needed when the Go server serves the frontend.
func (s *Server) cors(next http.Handler) http.Handler {
	origin := s.allowOrigin
	if origin == "" {
		origin = "*"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---------- Accounts ----------

func (s *Server) fetchUser(id int64) (*User, error) {
	var u User
	err := s.db.QueryRow(`SELECT id, name, phone, created_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Name, &u.Phone, &u.CreatedAt)
	return &u, err
}

const driverCols = `id, name, phone, vehicle_model, vehicle_plate, available, lat, lng, location_updated_at,
	CASE WHEN rating_count > 0 THEN rating_sum * 1.0 / rating_count END, rating_count, created_at`

func (s *Server) fetchDriver(id int64) (*Driver, error) {
	var d Driver
	err := s.db.QueryRow(`SELECT `+driverCols+` FROM drivers WHERE id = ?`, id).
		Scan(&d.ID, &d.Name, &d.Phone, &d.VehicleModel, &d.VehiclePlate, &d.Available,
			&d.Lat, &d.Lng, &d.LocationUpdatedAt, &d.Rating, &d.RatingCount, &d.CreatedAt)
	return &d, err
}

// ---------- Drivers ----------

func (s *Server) hasActiveRide(col string, id int64) (bool, error) { // col is a fixed literal
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM rides WHERE `+col+` = ? AND status IN ('requested','accepted','started')`, id).Scan(&n)
	return n > 0, err
}

func (s *Server) setAvailability(w http.ResponseWriter, r *http.Request, a Actor) {
	var in struct {
		Available *bool `json:"available"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Available == nil {
		writeErr(w, 400, "available is required")
		return
	}
	if *in.Available {
		busy, err := s.hasActiveRide("driver_id", a.ID)
		if err != nil {
			writeErr(w, 500, "database error")
			return
		}
		if busy {
			writeErr(w, 409, "finish your current ride first")
			return
		}
	}
	if _, err := s.db.Exec(`UPDATE drivers SET available = ? WHERE id = ?`, *in.Available, a.ID); err != nil {
		writeErr(w, 500, "database error")
		return
	}
	d, _ := s.fetchDriver(a.ID)
	writeJSON(w, 200, d)
}

func validLatLng(lat, lng float64) bool {
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}

func (s *Server) updateLocation(w http.ResponseWriter, r *http.Request, a Actor) {
	var in struct{ Lat, Lng *float64 }
	if !decode(w, r, &in) {
		return
	}
	if in.Lat == nil || in.Lng == nil || !validLatLng(*in.Lat, *in.Lng) {
		writeErr(w, 400, "valid lat and lng are required")
		return
	}
	if _, err := s.db.Exec(`UPDATE drivers SET lat = ?, lng = ?, location_updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		*in.Lat, *in.Lng, a.ID); err != nil {
		writeErr(w, 500, "database error")
		return
	}
	d, _ := s.fetchDriver(a.ID)
	writeJSON(w, 200, d)
}

// nearbyDrivers lists available drivers with a recent (<10 min) location within radius_km.
func (s *Server) nearbyDrivers(w http.ResponseWriter, r *http.Request, _ Actor) {
	q := r.URL.Query()
	lat, err1 := strconv.ParseFloat(q.Get("lat"), 64)
	lng, err2 := strconv.ParseFloat(q.Get("lng"), 64)
	if err1 != nil || err2 != nil || !validLatLng(lat, lng) {
		writeErr(w, 400, "valid lat and lng query params are required")
		return
	}
	radius := 5.0
	if v, err := strconv.ParseFloat(q.Get("radius_km"), 64); err == nil && v > 0 && v <= 50 {
		radius = v
	}
	rows, err := s.db.Query(`SELECT id, name, vehicle_model, vehicle_plate, lat, lng,
		CASE WHEN rating_count > 0 THEN rating_sum * 1.0 / rating_count END
		FROM drivers WHERE available = 1 AND lat IS NOT NULL AND lng IS NOT NULL
		AND CAST(strftime('%s', location_updated_at) AS INTEGER) >= ?`, time.Now().Add(-10*time.Minute).Unix())
	if err != nil {
		writeErr(w, 500, "database error")
		return
	}
	defer rows.Close()
	out := []NearbyDriver{}
	for rows.Next() {
		var d NearbyDriver
		if err := rows.Scan(&d.ID, &d.Name, &d.VehicleModel, &d.VehiclePlate, &d.Lat, &d.Lng, &d.Rating); err != nil {
			writeErr(w, 500, "database error")
			return
		}
		d.DistanceKm = haversineKm(lat, lng, d.Lat, d.Lng)
		if d.DistanceKm <= radius {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DistanceKm < out[j].DistanceKm })
	if len(out) > 20 {
		out = out[:20]
	}
	writeJSON(w, 200, out)
}

// ---------- Rides ----------

const rideSelect = `SELECT r.id, r.user_id, u.name, r.driver_id, d.name, d.vehicle_model, d.vehicle_plate, d.phone,
	d.lat, d.lng, r.pickup, r.dropoff, r.pickup_lat, r.pickup_lng, r.dropoff_lat, r.dropoff_lng,
	r.distance_km, r.fare, r.status, r.payment_method, r.payment_status, r.rating, r.review, r.created_at, r.updated_at
	FROM rides r JOIN users u ON u.id = r.user_id LEFT JOIN drivers d ON d.id = r.driver_id`

func scanRide(sc interface{ Scan(...any) error }) (*Ride, error) {
	var x Ride
	err := sc.Scan(&x.ID, &x.UserID, &x.RiderName, &x.DriverID, &x.DriverName, &x.DriverVehicle, &x.DriverPlate,
		&x.DriverPhone, &x.DriverLat, &x.DriverLng, &x.Pickup, &x.Dropoff, &x.PickupLat, &x.PickupLng,
		&x.DropoffLat, &x.DropoffLng, &x.DistanceKm, &x.Fare, &x.Status, &x.PaymentMethod, &x.PaymentStatus,
		&x.Rating, &x.Review, &x.CreatedAt, &x.UpdatedAt)
	return &x, err
}

func (s *Server) loadRide(id int64) (*Ride, error) {
	return scanRide(s.db.QueryRow(rideSelect+` WHERE r.id = ?`, id))
}

// canView: a rider sees their own rides; a driver sees rides assigned to them and open requests.
func canView(a Actor, x *Ride) bool {
	if a.Role == RoleRider {
		return x.UserID == a.ID
	}
	return x.Status == StatusRequested || (x.DriverID != nil && *x.DriverID == a.ID)
}

// ride loads the ride from the path and enforces visibility. Not-visible is reported as 404.
func (s *Server) ride(w http.ResponseWriter, r *http.Request, a Actor) (*Ride, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return nil, false
	}
	x, err := s.loadRide(id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !canView(a, x)) {
		writeErr(w, 404, "ride not found")
		return nil, false
	} else if err != nil {
		writeErr(w, 500, "database error")
		return nil, false
	}
	return x, true
}

type coords struct {
	PickupLat  float64 `json:"pickup_lat"`
	PickupLng  float64 `json:"pickup_lng"`
	DropoffLat float64 `json:"dropoff_lat"`
	DropoffLng float64 `json:"dropoff_lng"`
}

func (c coords) valid() bool {
	return validLatLng(c.PickupLat, c.PickupLng) && validLatLng(c.DropoffLat, c.DropoffLng)
}

func (s *Server) fareEstimate(w http.ResponseWriter, r *http.Request) {
	var in coords
	if !decode(w, r, &in) {
		return
	}
	if !in.valid() {
		writeErr(w, 400, "invalid coordinates")
		return
	}
	km := haversineKm(in.PickupLat, in.PickupLng, in.DropoffLat, in.DropoffLng)
	writeJSON(w, 200, map[string]float64{"distance_km": km, "fare": calcFare(km)})
}

func (s *Server) createRide(w http.ResponseWriter, r *http.Request, a Actor) {
	var in struct {
		Pickup        string `json:"pickup"`
		Dropoff       string `json:"dropoff"`
		PaymentMethod string `json:"payment_method"`
		coords
	}
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Pickup) == "" || strings.TrimSpace(in.Dropoff) == "" {
		writeErr(w, 400, "pickup and dropoff are required")
		return
	}
	if !in.valid() {
		writeErr(w, 400, "invalid coordinates")
		return
	}
	if in.PaymentMethod == "" {
		in.PaymentMethod = "cash"
	}
	if !paymentMethods[in.PaymentMethod] {
		writeErr(w, 400, "payment_method must be cash, card or wallet")
		return
	}
	if busy, err := s.hasActiveRide("user_id", a.ID); err != nil {
		writeErr(w, 500, "database error")
		return
	} else if busy {
		writeErr(w, 409, "you already have an active ride")
		return
	}
	km := haversineKm(in.PickupLat, in.PickupLng, in.DropoffLat, in.DropoffLng)
	res, err := s.db.Exec(`INSERT INTO rides(user_id, pickup, dropoff, pickup_lat, pickup_lng,
		dropoff_lat, dropoff_lng, distance_km, fare, payment_method) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		a.ID, in.Pickup, in.Dropoff, in.PickupLat, in.PickupLng,
		in.DropoffLat, in.DropoffLng, km, calcFare(km), in.PaymentMethod)
	if err != nil {
		writeErr(w, 500, "could not create ride")
		return
	}
	id, _ := res.LastInsertId()
	ride, _ := s.loadRide(id)
	writeJSON(w, 201, ride)
}

// listRides returns the caller's own rides. Drivers can pass scope=open to see unassigned requests.
func (s *Server) listRides(w http.ResponseWriter, r *http.Request, a Actor) {
	q := rideSelect + ` WHERE `
	var args []any
	switch {
	case a.Role == RoleDriver && r.URL.Query().Get("scope") == "open":
		q += `r.status = 'requested'`
	case a.Role == RoleDriver:
		q += `r.driver_id = ?`
		args = append(args, a.ID)
	default:
		q += `r.user_id = ?`
		args = append(args, a.ID)
	}
	if st := r.URL.Query().Get("status"); st != "" {
		q += ` AND r.status = ?`
		args = append(args, st)
	}
	rows, err := s.db.Query(q+` ORDER BY r.id DESC LIMIT 100`, args...)
	if err != nil {
		writeErr(w, 500, "database error")
		return
	}
	defer rows.Close()
	out := []*Ride{}
	for rows.Next() {
		x, err := scanRide(rows)
		if err != nil {
			writeErr(w, 500, "database error")
			return
		}
		out = append(out, x)
	}
	writeJSON(w, 200, out)
}

func (s *Server) getRide(w http.ResponseWriter, r *http.Request, a Actor) {
	if x, ok := s.ride(w, r, a); ok {
		writeJSON(w, 200, x)
	}
}

// acceptRide assigns the calling driver to a requested ride, atomically.
func (s *Server) acceptRide(w http.ResponseWriter, r *http.Request, a Actor) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		writeErr(w, 500, "database error")
		return
	}
	defer tx.Rollback()

	var status string
	if err := tx.QueryRow(`SELECT status FROM rides WHERE id = ?`, id).Scan(&status); errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "ride not found")
		return
	} else if err != nil {
		writeErr(w, 500, "database error")
		return
	}
	if !canTransition(status, StatusAccepted) {
		writeErr(w, 409, "ride cannot be accepted from status "+status)
		return
	}
	res, err := tx.Exec(`UPDATE drivers SET available = 0 WHERE id = ? AND available = 1`, a.ID)
	if err != nil {
		writeErr(w, 500, "database error")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, 409, "go online (available) before accepting rides")
		return
	}
	if _, err := tx.Exec(`UPDATE rides SET driver_id = ?, status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		a.ID, StatusAccepted, id); err != nil {
		writeErr(w, 500, "database error")
		return
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, 500, "database error")
		return
	}
	x, _ := s.loadRide(id)
	writeJSON(w, 200, x)
}

// statusAction moves a ride to the target status, enforcing who may do it and
// freeing the driver when the ride ends.
func (s *Server) statusAction(target string) func(http.ResponseWriter, *http.Request, Actor) {
	return func(w http.ResponseWriter, r *http.Request, a Actor) {
		x, ok := s.ride(w, r, a)
		if !ok {
			return
		}
		// start/complete: assigned driver only. cancel: the rider, or the assigned driver.
		isOwnerRider := a.Role == RoleRider && x.UserID == a.ID
		isAssigned := a.Role == RoleDriver && x.DriverID != nil && *x.DriverID == a.ID
		if !isOwnerRider && !isAssigned {
			writeErr(w, 403, "you are not allowed to do this on this ride")
			return
		}
		tx, err := s.db.Begin()
		if err != nil {
			writeErr(w, 500, "database error")
			return
		}
		defer tx.Rollback()

		// re-read status inside the transaction to avoid racing another request
		var status string
		var driverID *int64
		if err := tx.QueryRow(`SELECT status, driver_id FROM rides WHERE id = ?`, x.ID).Scan(&status, &driverID); err != nil {
			writeErr(w, 500, "database error")
			return
		}
		if !canTransition(status, target) {
			writeErr(w, 409, "cannot move ride from "+status+" to "+target)
			return
		}
		if _, err := tx.Exec(`UPDATE rides SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, target, x.ID); err != nil {
			writeErr(w, 500, "database error")
			return
		}
		if driverID != nil && (target == StatusCompleted || target == StatusCancelled) {
			if _, err := tx.Exec(`UPDATE drivers SET available = 1 WHERE id = ?`, *driverID); err != nil {
				writeErr(w, 500, "database error")
				return
			}
		}
		if err := tx.Commit(); err != nil {
			writeErr(w, 500, "database error")
			return
		}
		x, _ = s.loadRide(x.ID)
		writeJSON(w, 200, x)
	}
}

// ---------- Payments (simulated: records the payment, no real gateway) ----------

func (s *Server) payRide(w http.ResponseWriter, r *http.Request, a Actor) {
	x, ok := s.ride(w, r, a)
	if !ok {
		return
	}
	var in struct {
		Method string `json:"method"`
	}
	if r.ContentLength != 0 && !decode(w, r, &in) {
		return
	}
	if in.Method == "" {
		in.Method = x.PaymentMethod
	}
	if !paymentMethods[in.Method] {
		writeErr(w, 400, "method must be cash, card or wallet")
		return
	}
	if x.Status != StatusCompleted {
		writeErr(w, 409, "only completed rides can be paid")
		return
	}
	res, err := s.db.Exec(`UPDATE rides SET payment_method = ?, payment_status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND payment_status = ?`, in.Method, PayPaid, x.ID, PayPending)
	if err != nil {
		writeErr(w, 500, "database error")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, 409, "ride is already paid")
		return
	}
	x, _ = s.loadRide(x.ID)
	writeJSON(w, 200, x)
}

// ---------- Ratings ----------

func (s *Server) rateRide(w http.ResponseWriter, r *http.Request, a Actor) {
	x, ok := s.ride(w, r, a)
	if !ok {
		return
	}
	var in struct {
		Rating int    `json:"rating"`
		Review string `json:"review"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Rating < 1 || in.Rating > 5 {
		writeErr(w, 400, "rating must be between 1 and 5")
		return
	}
	if len(in.Review) > 500 {
		writeErr(w, 400, "review is limited to 500 characters")
		return
	}
	if x.Status != StatusCompleted || x.DriverID == nil {
		writeErr(w, 409, "only completed rides can be rated")
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		writeErr(w, 500, "database error")
		return
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE rides SET rating = ?, review = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND rating IS NULL`, in.Rating, in.Review, x.ID)
	if err != nil {
		writeErr(w, 500, "database error")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, 409, "ride is already rated")
		return
	}
	if _, err := tx.Exec(`UPDATE drivers SET rating_sum = rating_sum + ?, rating_count = rating_count + 1 WHERE id = ?`,
		in.Rating, *x.DriverID); err != nil {
		writeErr(w, 500, "database error")
		return
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, 500, "database error")
		return
	}
	x, _ = s.loadRide(x.ID)
	writeJSON(w, 200, x)
}

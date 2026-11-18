package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

type Server struct {
	db *sql.DB
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

	mux.HandleFunc("POST /api/users", s.createUser)
	mux.HandleFunc("GET /api/users/{id}", s.getUser)

	mux.HandleFunc("POST /api/drivers", s.createDriver)
	mux.HandleFunc("GET /api/drivers", s.listDrivers)
	mux.HandleFunc("GET /api/drivers/{id}", s.getDriver)
	mux.HandleFunc("PATCH /api/drivers/{id}/availability", s.setAvailability)

	mux.HandleFunc("POST /api/fare-estimate", s.fareEstimate)
	mux.HandleFunc("POST /api/rides", s.createRide)
	mux.HandleFunc("GET /api/rides", s.listRides)
	mux.HandleFunc("GET /api/rides/{id}", s.getRide)
	mux.HandleFunc("POST /api/rides/{id}/accept", s.acceptRide)
	mux.HandleFunc("POST /api/rides/{id}/start", s.statusAction(StatusStarted))
	mux.HandleFunc("POST /api/rides/{id}/complete", s.statusAction(StatusCompleted))
	mux.HandleFunc("POST /api/rides/{id}/cancel", s.statusAction(StatusCancelled))

	return cors(mux)
}

// cors allows the React dev server (localhost:3000) to call the API.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---------- Users ----------

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name, Phone string }
	if !decode(w, r, &in) {
		return
	}
	in.Name, in.Phone = strings.TrimSpace(in.Name), strings.TrimSpace(in.Phone)
	if in.Name == "" || in.Phone == "" {
		writeErr(w, 400, "name and phone are required")
		return
	}
	res, err := s.db.Exec(`INSERT INTO users(name, phone) VALUES(?, ?)`, in.Name, in.Phone)
	if isUnique(err) {
		writeErr(w, 409, "phone already registered")
		return
	} else if err != nil {
		writeErr(w, 500, "could not create user")
		return
	}
	id, _ := res.LastInsertId()
	u, _ := s.fetchUser(id)
	writeJSON(w, 201, u)
}

func (s *Server) fetchUser(id int64) (*User, error) {
	var u User
	err := s.db.QueryRow(`SELECT id, name, phone, created_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Name, &u.Phone, &u.CreatedAt)
	return &u, err
}

func (s *Server) getUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	u, err := s.fetchUser(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "user not found")
		return
	} else if err != nil {
		writeErr(w, 500, "database error")
		return
	}
	writeJSON(w, 200, u)
}

// ---------- Drivers ----------

const driverCols = `id, name, phone, vehicle_model, vehicle_plate, available, created_at`

func scanDriver(sc interface{ Scan(...any) error }) (*Driver, error) {
	var d Driver
	err := sc.Scan(&d.ID, &d.Name, &d.Phone, &d.VehicleModel, &d.VehiclePlate, &d.Available, &d.CreatedAt)
	return &d, err
}

func (s *Server) createDriver(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name         string `json:"name"`
		Phone        string `json:"phone"`
		VehicleModel string `json:"vehicle_model"`
		VehiclePlate string `json:"vehicle_plate"`
	}
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Phone) == "" ||
		strings.TrimSpace(in.VehicleModel) == "" || strings.TrimSpace(in.VehiclePlate) == "" {
		writeErr(w, 400, "name, phone, vehicle_model and vehicle_plate are required")
		return
	}
	res, err := s.db.Exec(
		`INSERT INTO drivers(name, phone, vehicle_model, vehicle_plate) VALUES(?,?,?,?)`,
		in.Name, in.Phone, in.VehicleModel, in.VehiclePlate)
	if isUnique(err) {
		writeErr(w, 409, "phone or vehicle plate already registered")
		return
	} else if err != nil {
		writeErr(w, 500, "could not create driver")
		return
	}
	id, _ := res.LastInsertId()
	d, _ := scanDriver(s.db.QueryRow(`SELECT `+driverCols+` FROM drivers WHERE id = ?`, id))
	writeJSON(w, 201, d)
}

func (s *Server) listDrivers(w http.ResponseWriter, r *http.Request) {
	q := `SELECT ` + driverCols + ` FROM drivers`
	if r.URL.Query().Get("available") == "true" {
		q += ` WHERE available = 1`
	}
	rows, err := s.db.Query(q + ` ORDER BY id`)
	if err != nil {
		writeErr(w, 500, "database error")
		return
	}
	defer rows.Close()
	out := []*Driver{}
	for rows.Next() {
		d, err := scanDriver(rows)
		if err != nil {
			writeErr(w, 500, "database error")
			return
		}
		out = append(out, d)
	}
	writeJSON(w, 200, out)
}

func (s *Server) getDriver(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	d, err := scanDriver(s.db.QueryRow(`SELECT `+driverCols+` FROM drivers WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "driver not found")
		return
	} else if err != nil {
		writeErr(w, 500, "database error")
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) setAvailability(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
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
	res, err := s.db.Exec(`UPDATE drivers SET available = ? WHERE id = ?`, *in.Available, id)
	if err != nil {
		writeErr(w, 500, "database error")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, 404, "driver not found")
		return
	}
	s.getDriver(w, r)
}

// ---------- Rides ----------

const rideCols = `id, user_id, driver_id, pickup, dropoff, pickup_lat, pickup_lng,
	dropoff_lat, dropoff_lng, distance_km, fare, status, created_at, updated_at`

func scanRide(sc interface{ Scan(...any) error }) (*Ride, error) {
	var x Ride
	err := sc.Scan(&x.ID, &x.UserID, &x.DriverID, &x.Pickup, &x.Dropoff, &x.PickupLat, &x.PickupLng,
		&x.DropoffLat, &x.DropoffLng, &x.DistanceKm, &x.Fare, &x.Status, &x.CreatedAt, &x.UpdatedAt)
	return &x, err
}

type coords struct {
	PickupLat  float64 `json:"pickup_lat"`
	PickupLng  float64 `json:"pickup_lng"`
	DropoffLat float64 `json:"dropoff_lat"`
	DropoffLng float64 `json:"dropoff_lng"`
}

func (c coords) valid() bool {
	ok := func(lat, lng float64) bool { return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180 }
	return ok(c.PickupLat, c.PickupLng) && ok(c.DropoffLat, c.DropoffLng)
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

func (s *Server) createRide(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UserID  int64  `json:"user_id"`
		Pickup  string `json:"pickup"`
		Dropoff string `json:"dropoff"`
		coords
	}
	if !decode(w, r, &in) {
		return
	}
	if in.UserID <= 0 || strings.TrimSpace(in.Pickup) == "" || strings.TrimSpace(in.Dropoff) == "" {
		writeErr(w, 400, "user_id, pickup and dropoff are required")
		return
	}
	if !in.valid() {
		writeErr(w, 400, "invalid coordinates")
		return
	}
	if _, err := s.fetchUser(in.UserID); errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "user not found")
		return
	}
	km := haversineKm(in.PickupLat, in.PickupLng, in.DropoffLat, in.DropoffLng)
	res, err := s.db.Exec(`INSERT INTO rides(user_id, pickup, dropoff, pickup_lat, pickup_lng,
		dropoff_lat, dropoff_lng, distance_km, fare) VALUES(?,?,?,?,?,?,?,?,?)`,
		in.UserID, in.Pickup, in.Dropoff, in.PickupLat, in.PickupLng,
		in.DropoffLat, in.DropoffLng, km, calcFare(km))
	if err != nil {
		writeErr(w, 500, "could not create ride")
		return
	}
	id, _ := res.LastInsertId()
	ride, _ := scanRide(s.db.QueryRow(`SELECT `+rideCols+` FROM rides WHERE id = ?`, id))
	writeJSON(w, 201, ride)
}

func (s *Server) listRides(w http.ResponseWriter, r *http.Request) {
	q := `SELECT ` + rideCols + ` FROM rides WHERE 1=1`
	var args []any
	for _, f := range []string{"user_id", "driver_id", "status"} {
		if v := r.URL.Query().Get(f); v != "" {
			q += ` AND ` + f + ` = ?` // f comes from a fixed list, not user input
			args = append(args, v)
		}
	}
	rows, err := s.db.Query(q+` ORDER BY id DESC`, args...)
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

func (s *Server) getRide(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	x, err := scanRide(s.db.QueryRow(`SELECT `+rideCols+` FROM rides WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "ride not found")
		return
	} else if err != nil {
		writeErr(w, 500, "database error")
		return
	}
	writeJSON(w, 200, x)
}

// acceptRide assigns an available driver to a requested ride, atomically.
func (s *Server) acceptRide(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		DriverID int64 `json:"driver_id"`
	}
	if !decode(w, r, &in) {
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
	res, err := tx.Exec(`UPDATE drivers SET available = 0 WHERE id = ? AND available = 1`, in.DriverID)
	if err != nil {
		writeErr(w, 500, "database error")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, 409, "driver not found or not available")
		return
	}
	if _, err := tx.Exec(`UPDATE rides SET driver_id = ?, status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		in.DriverID, StatusAccepted, id); err != nil {
		writeErr(w, 500, "database error")
		return
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, 500, "database error")
		return
	}
	s.getRide(w, r)
}

// statusAction moves a ride to the target status, freeing the driver when the ride ends.
func (s *Server) statusAction(target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		var driverID *int64
		if err := tx.QueryRow(`SELECT status, driver_id FROM rides WHERE id = ?`, id).Scan(&status, &driverID); errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "ride not found")
			return
		} else if err != nil {
			writeErr(w, 500, "database error")
			return
		}
		if !canTransition(status, target) {
			writeErr(w, 409, "cannot move ride from "+status+" to "+target)
			return
		}
		if _, err := tx.Exec(`UPDATE rides SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, target, id); err != nil {
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
		s.getRide(w, r)
	}
}

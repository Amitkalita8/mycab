package main

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

type env struct {
	t   *testing.T
	url string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db, err := openDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer((&Server{db: db}).routes())
	t.Cleanup(func() { ts.Close(); db.Close() })
	return &env{t, ts.URL}
}

// call performs a request and returns the status code and decoded JSON body.
func (e *env) call(method, path, token string, body any) (int, map[string]any) {
	e.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, e.url+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	json.NewDecoder(res.Body).Decode(&out) // list endpoints decode to nil; use callList for those
	return res.StatusCode, out
}

func (e *env) callList(path, token string) []map[string]any {
	e.t.Helper()
	req, _ := http.NewRequest("GET", e.url+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		e.t.Fatalf("GET %s: status %d", path, res.StatusCode)
	}
	var out []map[string]any
	json.NewDecoder(res.Body).Decode(&out)
	return out
}

func (e *env) expect(want int, method, path, token string, body any) map[string]any {
	e.t.Helper()
	code, out := e.call(method, path, token, body)
	if code != want {
		e.t.Fatalf("%s %s: got %d want %d (%v)", method, path, code, want, out)
	}
	return out
}

func (e *env) rider(name, phone string) string {
	out := e.expect(201, "POST", "/api/auth/register/rider", "", map[string]any{"name": name, "phone": phone, "password": "secret123"})
	return out["token"].(string)
}

func (e *env) driver(name, phone, plate string) (token string, id int64) {
	out := e.expect(201, "POST", "/api/auth/register/driver", "", map[string]any{
		"name": name, "phone": phone, "password": "secret123", "vehicle_model": "Swift", "vehicle_plate": plate})
	return out["token"].(string), int64(out["profile"].(map[string]any)["id"].(float64))
}

var tripBody = map[string]any{
	"pickup": "A", "dropoff": "B", "pickup_lat": 12.97, "pickup_lng": 77.59,
	"dropoff_lat": 13.03, "dropoff_lng": 77.64, "payment_method": "card",
}

func TestFare(t *testing.T) {
	if got := calcFare(0); got != minimumFee {
		t.Errorf("minimum fare: got %v", got)
	}
	if got := calcFare(10); got != 150 {
		t.Errorf("10km fare: got %v want 150", got)
	}
	// ~111.19 km per degree of latitude
	if d := haversineKm(0, 0, 1, 0); math.Abs(d-111.19) > 0.1 {
		t.Errorf("haversine: got %v", d)
	}
}

func TestAuth(t *testing.T) {
	e := newEnv(t)
	tok := e.rider("Asha", "111")

	e.expect(409, "POST", "/api/auth/register/rider", "", map[string]any{"name": "X", "phone": "111", "password": "secret123"})
	e.expect(400, "POST", "/api/auth/register/rider", "", map[string]any{"name": "X", "phone": "222", "password": "123"})

	e.expect(401, "GET", "/api/me", "", nil)
	e.expect(401, "GET", "/api/me", "garbage", nil)
	if me := e.expect(200, "GET", "/api/me", tok, nil); me["role"] != "rider" {
		t.Errorf("me: %v", me)
	}

	e.expect(401, "POST", "/api/auth/login", "", map[string]any{"role": "rider", "phone": "111", "password": "wrong-pass"})
	e.expect(401, "POST", "/api/auth/login", "", map[string]any{"role": "rider", "phone": "nope", "password": "secret123"})
	e.expect(401, "POST", "/api/auth/login", "", map[string]any{"role": "driver", "phone": "111", "password": "secret123"}) // roles are separate
	login := e.expect(200, "POST", "/api/auth/login", "", map[string]any{"role": "rider", "phone": "111", "password": "secret123"})

	e.expect(204, "POST", "/api/auth/logout", login["token"].(string), nil)
	e.expect(401, "GET", "/api/me", login["token"].(string), nil)
	e.expect(200, "GET", "/api/me", tok, nil) // other session unaffected
}

func TestRoleAndOwnershipChecks(t *testing.T) {
	e := newEnv(t)
	rider1, rider2 := e.rider("A", "1"), e.rider("B", "2")
	drv, _ := e.driver("D", "3", "KA01")
	drv2, _ := e.driver("D2", "4", "KA02")

	e.expect(403, "POST", "/api/rides", drv, tripBody)        // driver can't book
	e.expect(403, "POST", "/api/rides/1/accept", rider1, nil) // rider can't accept
	e.expect(403, "PATCH", "/api/driver/availability", rider1, map[string]any{"available": true})

	ride := e.expect(201, "POST", "/api/rides", rider1, tripBody)
	id := "/api/rides/" + itoa(ride["id"])

	e.expect(404, "GET", id, rider2, nil) // someone else's ride is invisible
	e.expect(404, "POST", id+"/cancel", rider2, nil)
	e.expect(409, "POST", "/api/rides", rider1, tripBody) // one active ride at a time

	e.expect(200, "PATCH", "/api/driver/availability", drv, map[string]any{"available": true})
	e.expect(200, "PATCH", "/api/driver/availability", drv2, map[string]any{"available": true})
	e.expect(200, "POST", id+"/accept", drv, nil)
	e.expect(409, "POST", id+"/accept", drv2, nil) // already taken

	e.expect(404, "GET", id, drv2, nil)                                                        // not open any more, not theirs
	e.expect(404, "POST", id+"/start", drv2, nil)                                              // can't act on others' rides
	e.expect(409, "PATCH", "/api/driver/availability", drv, map[string]any{"available": true}) // busy
}

func TestFullRideLifecycle(t *testing.T) {
	e := newEnv(t)
	rider := e.rider("Asha", "111")
	drv, drvID := e.driver("Ravi", "222", "ka01 ab 1234")

	// driver is offline by default and can't accept
	ride := e.expect(201, "POST", "/api/rides", rider, tripBody)
	id := "/api/rides/" + itoa(ride["id"])
	if ride["status"] != "requested" || ride["payment_status"] != "pending" || ride["payment_method"] != "card" {
		t.Fatalf("new ride: %v", ride)
	}
	e.expect(409, "POST", id+"/accept", drv, nil)

	// go online + share location; the rider can find them
	e.expect(200, "PATCH", "/api/driver/availability", drv, map[string]any{"available": true})
	e.expect(400, "PUT", "/api/driver/location", drv, map[string]any{"lat": 999, "lng": 0})
	e.expect(200, "PUT", "/api/driver/location", drv, map[string]any{"lat": 12.971, "lng": 77.591})
	near := e.callList("/api/drivers/nearby?lat=12.97&lng=77.59", rider)
	if len(near) != 1 || int64(near[0]["id"].(float64)) != drvID {
		t.Fatalf("nearby: %v", near)
	}
	if _, leaked := near[0]["phone"]; leaked {
		t.Error("nearby should not expose driver phone")
	}
	if far := e.callList("/api/drivers/nearby?lat=28.6&lng=77.2", rider); len(far) != 0 {
		t.Errorf("far search should be empty: %v", far)
	}

	// open requests are visible to the driver
	if open := e.callList("/api/rides?scope=open", drv); len(open) != 1 {
		t.Fatalf("open rides: %v", open)
	}

	// can't skip steps
	e.expect(403, "POST", id+"/start", drv, nil) // not yet the assigned driver
	e.expect(409, "POST", id+"/pay", rider, nil)
	e.expect(409, "POST", id+"/rate", rider, map[string]any{"rating": 5})

	got := e.expect(200, "POST", id+"/accept", drv, nil)
	if got["status"] != "accepted" || got["driver_plate"] != "KA01 AB 1234" {
		t.Fatalf("accepted: %v", got)
	}
	// driver is no longer listed as nearby (busy)
	if n := e.callList("/api/drivers/nearby?lat=12.97&lng=77.59", rider); len(n) != 0 {
		t.Errorf("busy driver listed: %v", n)
	}
	// live tracking: rider sees the driver's location on the ride
	e.expect(200, "PUT", "/api/driver/location", drv, map[string]any{"lat": 12.98, "lng": 77.6})
	if r := e.expect(200, "GET", id, rider, nil); r["driver_lat"] != 12.98 {
		t.Errorf("tracking: %v", r["driver_lat"])
	}

	e.expect(200, "POST", id+"/start", drv, nil)
	e.expect(409, "POST", id+"/cancel", rider, nil) // too late
	e.expect(200, "POST", id+"/complete", drv, nil)

	// pay
	e.expect(400, "POST", id+"/pay", rider, map[string]any{"method": "bitcoin"})
	paid := e.expect(200, "POST", id+"/pay", rider, map[string]any{"method": "wallet"})
	if paid["payment_status"] != "paid" || paid["payment_method"] != "wallet" {
		t.Fatalf("paid: %v", paid)
	}
	e.expect(409, "POST", id+"/pay", rider, nil)

	// rate
	e.expect(400, "POST", id+"/rate", rider, map[string]any{"rating": 6})
	e.expect(403, "POST", id+"/rate", drv, map[string]any{"rating": 5})
	e.expect(200, "POST", id+"/rate", rider, map[string]any{"rating": 4, "review": "smooth"})
	e.expect(409, "POST", id+"/rate", rider, map[string]any{"rating": 5})
	me := e.expect(200, "GET", "/api/me", drv, nil)["profile"].(map[string]any)
	if me["rating"] != 4.0 || me["rating_count"] != 1.0 || me["available"] != true {
		t.Errorf("driver profile after ride: %v", me)
	}

	// a new ride can be booked again, and cancelling frees the driver
	r2 := e.expect(201, "POST", "/api/rides", rider, tripBody)
	id2 := "/api/rides/" + itoa(r2["id"])
	e.expect(200, "POST", id2+"/accept", drv, nil)
	e.expect(200, "POST", id2+"/cancel", rider, nil)
	if me := e.expect(200, "GET", "/api/me", drv, nil)["profile"].(map[string]any); me["available"] != true {
		t.Errorf("driver should be free after cancel: %v", me)
	}
	if hist := e.callList("/api/rides", rider); len(hist) != 2 {
		t.Errorf("history: %d rides", len(hist))
	}
}

func TestValidation(t *testing.T) {
	e := newEnv(t)
	rider := e.rider("A", "1")
	bad := map[string]any{"pickup": "A", "dropoff": "B", "pickup_lat": 100, "pickup_lng": 0, "dropoff_lat": 0, "dropoff_lng": 0}
	e.expect(400, "POST", "/api/rides", rider, bad)
	e.expect(400, "POST", "/api/rides", rider, map[string]any{"pickup": "", "dropoff": "B"})
	e.expect(400, "POST", "/api/fare-estimate", "", bad)
	fare := e.expect(200, "POST", "/api/fare-estimate", "", tripBody)
	if fare["fare"].(float64) <= 0 {
		t.Errorf("fare estimate: %v", fare)
	}
	e.expect(400, "GET", "/api/drivers/nearby", rider, nil)
}

func itoa(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

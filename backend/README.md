# mycab backend (Go + SQLite)

Run: `go run .`  (env: `PORT` default 8080, `DB_PATH` default `mycab.db`)
Test: `go test ./...`
Uses pure-Go SQLite (`modernc.org/sqlite`) — no gcc/cgo required. Tables are auto-created on start.
Frontend dev server (`npm start` in `../frontend`) proxies `/api` to this server.

## Auth
Riders and drivers are separate account types. Register/login returns a bearer token
(`Authorization: Bearer <token>`), valid 7 days. Passwords are bcrypt-hashed; only a SHA-256 of the token is stored.

| Method | Path | Notes |
|---|---|---|
| POST | /api/auth/register/rider | `{name, phone, password}` → `{token, role, profile}` |
| POST | /api/auth/register/driver | `{name, phone, password, vehicle_model, vehicle_plate}` |
| POST | /api/auth/login | `{role, phone, password}` |
| POST | /api/auth/logout | |
| GET | /api/me | |

## Public
| POST | /api/fare-estimate | `{pickup_lat, pickup_lng, dropoff_lat, dropoff_lng}` |
|---|---|---|

## Driver (role: driver)
| Method | Path | Notes |
|---|---|---|
| PATCH | /api/driver/availability | `{available}` — go online/offline (not while on a ride) |
| PUT | /api/driver/location | `{lat, lng}` — live location |
| GET | /api/rides?scope=open | unassigned requests |
| POST | /api/rides/{id}/accept, /start, /complete | |

## Rider (role: rider)
| Method | Path | Notes |
|---|---|---|
| GET | /api/drivers/nearby?lat=&lng=[&radius_km=5] | available drivers seen in the last 10 min |
| POST | /api/rides | `{pickup, dropoff, + 4 coords, payment_method: cash\|card\|wallet}`; one active ride at a time |
| POST | /api/rides/{id}/pay | `{method}` — **simulated**, records payment only (no gateway) |
| POST | /api/rides/{id}/rate | `{rating 1-5, review}` once per completed ride |

## Shared
| GET | /api/rides[?status=] | the caller's own rides |
|---|---|---|
| GET | /api/rides/{id} | participants only (others get 404); includes the driver's live `driver_lat/lng` |
| POST | /api/rides/{id}/cancel | rider, or assigned driver, before the trip starts |

Ride flow: requested → accepted → started → completed (cancel allowed before start).
Fare = 30 + 12/km (straight-line), minimum 50. Driver is freed when a ride completes or is cancelled.

# mycab backend (Go + SQLite)

Run: `go run .`  (env: `PORT` default 8080, `DB_PATH` default `mycab.db`)
Uses pure-Go SQLite (`modernc.org/sqlite`) — no gcc/cgo required. Tables are auto-created on start.

| Method | Path | Notes |
|---|---|---|
| GET | /api/health | |
| POST | /api/users | `{name, phone}` |
| GET | /api/users/{id} | |
| POST | /api/drivers | `{name, phone, vehicle_model, vehicle_plate}` |
| GET | /api/drivers[?available=true] | |
| GET | /api/drivers/{id} | |
| PATCH | /api/drivers/{id}/availability | `{available}` |
| POST | /api/fare-estimate | `{pickup_lat, pickup_lng, dropoff_lat, dropoff_lng}` |
| POST | /api/rides | `{user_id, pickup, dropoff, + coords}` |
| GET | /api/rides[?user_id=&driver_id=&status=] | |
| GET | /api/rides/{id} | |
| POST | /api/rides/{id}/accept | `{driver_id}` |
| POST | /api/rides/{id}/start, /complete, /cancel | |

Ride flow: requested → accepted → started → completed (cancel allowed before start).
Fare = 30 + 12/km, minimum 50.

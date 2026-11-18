package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionTTL = 7 * 24 * time.Hour

// dummyHash is compared against when a phone is unknown, to equalise login timing.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcrypt.DefaultCost)

// Actor is the authenticated caller.
type Actor struct {
	Role string
	ID   int64
}

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// newSession issues a random opaque token; only its hash is stored.
func (s *Server) newSession(role string, id int64) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(b)
	now := time.Now()
	if _, err := s.db.Exec(`INSERT INTO sessions(token_hash, role, account_id, expires_at) VALUES(?,?,?,?)`,
		hashToken(tok), role, id, now.Add(sessionTTL).Unix()); err != nil {
		return "", err
	}
	s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, now.Unix()) // opportunistic cleanup
	return tok, nil
}

func (s *Server) identify(r *http.Request) (Actor, bool) {
	tok := bearer(r)
	if tok == "" {
		return Actor{}, false
	}
	var a Actor
	err := s.db.QueryRow(`SELECT role, account_id FROM sessions WHERE token_hash = ? AND expires_at > ?`,
		hashToken(tok), time.Now().Unix()).Scan(&a.Role, &a.ID)
	return a, err == nil
}

// auth wraps a handler: requires a valid session and, if role != "", that role.
func (s *Server) auth(role string, h func(http.ResponseWriter, *http.Request, Actor)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, ok := s.identify(r)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if role != "" && a.Role != role {
			writeErr(w, http.StatusForbidden, "this action is for "+role+"s only")
			return
		}
		h(w, r, a)
	}
}

func validPassword(p string) bool { return len(p) >= 6 && len(p) <= 72 } // bcrypt limit is 72 bytes

func (s *Server) profile(a Actor) (any, error) {
	if a.Role == RoleDriver {
		return s.fetchDriver(a.ID)
	}
	return s.fetchUser(a.ID)
}

func (s *Server) respondWithSession(w http.ResponseWriter, code int, a Actor) {
	tok, err := s.newSession(a.Role, a.ID)
	if err != nil {
		writeErr(w, 500, "could not create session")
		return
	}
	p, err := s.profile(a)
	if err != nil {
		writeErr(w, 500, "database error")
		return
	}
	writeJSON(w, code, map[string]any{"token": tok, "role": a.Role, "profile": p})
}

func (s *Server) registerRider(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name, Phone, Password string }
	if !decode(w, r, &in) {
		return
	}
	in.Name, in.Phone = strings.TrimSpace(in.Name), strings.TrimSpace(in.Phone)
	if in.Name == "" || in.Phone == "" || !validPassword(in.Password) {
		writeErr(w, 400, "name, phone and a password of 6-72 characters are required")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, "could not hash password")
		return
	}
	res, err := s.db.Exec(`INSERT INTO users(name, phone, password_hash) VALUES(?,?,?)`, in.Name, in.Phone, string(hash))
	if isUnique(err) {
		writeErr(w, 409, "phone already registered")
		return
	} else if err != nil {
		writeErr(w, 500, "could not create account")
		return
	}
	id, _ := res.LastInsertId()
	s.respondWithSession(w, 201, Actor{RoleRider, id})
}

func (s *Server) registerDriver(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name         string `json:"name"`
		Phone        string `json:"phone"`
		Password     string `json:"password"`
		VehicleModel string `json:"vehicle_model"`
		VehiclePlate string `json:"vehicle_plate"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name, in.Phone = strings.TrimSpace(in.Name), strings.TrimSpace(in.Phone)
	in.VehicleModel, in.VehiclePlate = strings.TrimSpace(in.VehicleModel), strings.ToUpper(strings.TrimSpace(in.VehiclePlate))
	if in.Name == "" || in.Phone == "" || in.VehicleModel == "" || in.VehiclePlate == "" || !validPassword(in.Password) {
		writeErr(w, 400, "name, phone, vehicle_model, vehicle_plate and a password of 6-72 characters are required")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, "could not hash password")
		return
	}
	res, err := s.db.Exec(`INSERT INTO drivers(name, phone, password_hash, vehicle_model, vehicle_plate) VALUES(?,?,?,?,?)`,
		in.Name, in.Phone, string(hash), in.VehicleModel, in.VehiclePlate)
	if isUnique(err) {
		writeErr(w, 409, "phone or vehicle plate already registered")
		return
	} else if err != nil {
		writeErr(w, 500, "could not create account")
		return
	}
	id, _ := res.LastInsertId()
	s.respondWithSession(w, 201, Actor{RoleDriver, id})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Role, Phone, Password string }
	if !decode(w, r, &in) {
		return
	}
	table := map[string]string{RoleRider: "users", RoleDriver: "drivers"}[in.Role]
	if table == "" {
		writeErr(w, 400, "role must be rider or driver")
		return
	}
	var id int64
	var hash string
	err := s.db.QueryRow(`SELECT id, password_hash FROM `+table+` WHERE phone = ?`, strings.TrimSpace(in.Phone)).Scan(&id, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		// burn comparable time so unknown phones aren't distinguishable by latency
		bcrypt.CompareHashAndPassword(dummyHash, []byte(in.Password))
		writeErr(w, 401, "invalid phone or password")
		return
	} else if err != nil {
		writeErr(w, 500, "database error")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		writeErr(w, 401, "invalid phone or password")
		return
	}
	s.respondWithSession(w, 200, Actor{in.Role, id})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, _ Actor) {
	s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, hashToken(bearer(r)))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request, a Actor) {
	p, err := s.profile(a)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 401, "account no longer exists")
		return
	} else if err != nil {
		writeErr(w, 500, "database error")
		return
	}
	writeJSON(w, 200, map[string]any{"role": a.Role, "profile": p})
}

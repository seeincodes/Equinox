package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"equinox/store"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionDuration  = 24 * time.Hour
	slidingThreshold = 12 * time.Hour
	CookieName       = "equinox_session"
)

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type Session struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
}

type Auth struct {
	db *store.DB
}

type contextKey string

const userContextKey contextKey = "user"

func New(db *store.DB) *Auth {
	return &Auth{db: db}
}

func (a *Auth) CreateUser(email, password, role string) (*User, error) {
	validRoles := map[string]bool{"viewer": true, "analyst": true, "admin": true}
	if !validRoles[role] {
		return nil, fmt.Errorf("invalid role: %s (must be viewer, analyst, or admin)", role)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	id := generateID("usr")
	_, err = a.db.Conn().Exec(
		"INSERT INTO users (id, email, password_hash, role) VALUES (?, ?, ?, ?)",
		id, email, string(hash), role,
	)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return &User{ID: id, Email: email, Role: role, CreatedAt: time.Now()}, nil
}

func (a *Auth) Login(email, password string) (*Session, error) {
	var id, hash string
	err := a.db.Conn().QueryRow(
		"SELECT id, password_hash FROM users WHERE email = ?", email,
	).Scan(&id, &hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("invalid credentials")
		}
		return nil, fmt.Errorf("query user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	// Clean up expired sessions for this user
	a.db.Conn().Exec("DELETE FROM sessions WHERE user_id = ? AND expires_at < ?", id, time.Now())

	sessionID := generateID("ses")
	expiresAt := time.Now().Add(sessionDuration)
	_, err = a.db.Conn().Exec(
		"INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)",
		sessionID, id, expiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &Session{ID: sessionID, UserID: id, ExpiresAt: expiresAt}, nil
}

func (a *Auth) ValidateSession(sessionID string) (*User, error) {
	var userID string
	var expiresAt time.Time
	err := a.db.Conn().QueryRow(
		"SELECT user_id, expires_at FROM sessions WHERE id = ?", sessionID,
	).Scan(&userID, &expiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("session not found")
		}
		return nil, fmt.Errorf("query session: %w", err)
	}

	if time.Now().After(expiresAt) {
		a.db.Conn().Exec("DELETE FROM sessions WHERE id = ?", sessionID)
		return nil, fmt.Errorf("session expired")
	}

	// Sliding expiry: extend if less than threshold remaining
	if time.Until(expiresAt) < slidingThreshold {
		newExpiry := time.Now().Add(sessionDuration)
		a.db.Conn().Exec("UPDATE sessions SET expires_at = ? WHERE id = ?", newExpiry, sessionID)
	}

	var user User
	err = a.db.Conn().QueryRow(
		"SELECT id, email, role, created_at FROM users WHERE id = ?", userID,
	).Scan(&user.ID, &user.Email, &user.Role, &user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}

	return &user, nil
}

func (a *Auth) Logout(sessionID string) error {
	_, err := a.db.Conn().Exec("DELETE FROM sessions WHERE id = ?", sessionID)
	return err
}

func (a *Auth) ListUsers() ([]User, error) {
	rows, err := a.db.Conn().Query("SELECT id, email, role, created_at FROM users ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Role, &u.CreatedAt); err != nil {
			continue
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// RequireAuth is middleware that checks for a valid session cookie.
func (a *Auth) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(CookieName)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		user, err := a.ValidateSession(cookie.Value)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole is middleware that checks the user's role. Must be used after RequireAuth.
func (a *Auth) RequireRole(role string, next http.Handler) http.Handler {
	roleHierarchy := map[string]int{"viewer": 0, "analyst": 1, "admin": 2}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		if user == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		if roleHierarchy[user.Role] < roleHierarchy[role] {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// UserFromContext extracts the authenticated user from the request context.
func UserFromContext(ctx context.Context) *User {
	user, _ := ctx.Value(userContextKey).(*User)
	return user
}

func generateID(prefix string) string {
	b := make([]byte, 16)
	rand.Read(b)
	return prefix + "-" + hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

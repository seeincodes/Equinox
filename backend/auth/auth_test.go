package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"equinox/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestAuth(t *testing.T) (*Auth, *store.DB) {
	t.Helper()
	db, err := store.New(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	a := New(db)
	return a, db
}

func TestCreateUser(t *testing.T) {
	a, _ := setupTestAuth(t)
	user, err := a.CreateUser("test@example.com", "password123", "analyst")
	require.NoError(t, err)
	assert.Equal(t, "test@example.com", user.Email)
	assert.Equal(t, "analyst", user.Role)
	assert.NotEmpty(t, user.ID)
}

func TestCreateUser_DuplicateEmail(t *testing.T) {
	a, _ := setupTestAuth(t)
	_, err := a.CreateUser("test@example.com", "pass1", "viewer")
	require.NoError(t, err)
	_, err = a.CreateUser("test@example.com", "pass2", "admin")
	assert.Error(t, err)
}

func TestCreateUser_InvalidRole(t *testing.T) {
	a, _ := setupTestAuth(t)
	_, err := a.CreateUser("test@example.com", "pass", "superadmin")
	assert.Error(t, err)
}

func TestLogin_Success(t *testing.T) {
	a, _ := setupTestAuth(t)
	_, err := a.CreateUser("test@example.com", "password123", "analyst")
	require.NoError(t, err)

	session, err := a.Login("test@example.com", "password123")
	require.NoError(t, err)
	assert.NotEmpty(t, session.ID)
	assert.True(t, session.ExpiresAt.After(time.Now()))
}

func TestLogin_WrongPassword(t *testing.T) {
	a, _ := setupTestAuth(t)
	_, err := a.CreateUser("test@example.com", "password123", "analyst")
	require.NoError(t, err)

	_, err = a.Login("test@example.com", "wrongpassword")
	assert.Error(t, err)
}

func TestLogin_UnknownEmail(t *testing.T) {
	a, _ := setupTestAuth(t)
	_, err := a.Login("nobody@example.com", "password123")
	assert.Error(t, err)
}

func TestValidateSession(t *testing.T) {
	a, _ := setupTestAuth(t)
	_, err := a.CreateUser("test@example.com", "password123", "analyst")
	require.NoError(t, err)
	session, err := a.Login("test@example.com", "password123")
	require.NoError(t, err)

	user, err := a.ValidateSession(session.ID)
	require.NoError(t, err)
	assert.Equal(t, "test@example.com", user.Email)
	assert.Equal(t, "analyst", user.Role)
}

func TestValidateSession_Expired(t *testing.T) {
	a, db := setupTestAuth(t)
	_, err := a.CreateUser("test@example.com", "password123", "analyst")
	require.NoError(t, err)
	session, err := a.Login("test@example.com", "password123")
	require.NoError(t, err)

	// Expire the session manually
	_, err = db.Conn().Exec("UPDATE sessions SET expires_at = ? WHERE id = ?",
		time.Now().Add(-1*time.Hour), session.ID)
	require.NoError(t, err)

	_, err = a.ValidateSession(session.ID)
	assert.Error(t, err)
}

func TestLogout(t *testing.T) {
	a, _ := setupTestAuth(t)
	_, err := a.CreateUser("test@example.com", "password123", "analyst")
	require.NoError(t, err)
	session, err := a.Login("test@example.com", "password123")
	require.NoError(t, err)

	err = a.Logout(session.ID)
	require.NoError(t, err)

	_, err = a.ValidateSession(session.ID)
	assert.Error(t, err)
}

func TestListUsers(t *testing.T) {
	a, _ := setupTestAuth(t)
	_, _ = a.CreateUser("one@example.com", "pass1", "viewer")
	_, _ = a.CreateUser("two@example.com", "pass2", "admin")

	users, err := a.ListUsers()
	require.NoError(t, err)
	assert.Len(t, users, 2)
}

func TestMiddleware_NoSession(t *testing.T) {
	a, _ := setupTestAuth(t)
	handler := a.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMiddleware_ValidSession(t *testing.T) {
	a, _ := setupTestAuth(t)
	_, _ = a.CreateUser("test@example.com", "pass", "analyst")
	session, _ := a.Login("test@example.com", "pass")

	handler := a.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		assert.Equal(t, "test@example.com", user.Email)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: session.ID})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireRole_Authorized(t *testing.T) {
	a, _ := setupTestAuth(t)
	_, _ = a.CreateUser("admin@example.com", "pass", "admin")
	session, _ := a.Login("admin@example.com", "pass")

	handler := a.RequireAuth(a.RequireRole("admin", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: session.ID})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireRole_Forbidden(t *testing.T) {
	a, _ := setupTestAuth(t)
	_, _ = a.CreateUser("viewer@example.com", "pass", "viewer")
	session, _ := a.Login("viewer@example.com", "pass")

	handler := a.RequireAuth(a.RequireRole("admin", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: session.ID})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireRole_AnalystCanRoute(t *testing.T) {
	a, _ := setupTestAuth(t)
	_, _ = a.CreateUser("analyst@example.com", "pass", "analyst")
	session, _ := a.Login("analyst@example.com", "pass")

	handler := a.RequireAuth(a.RequireRole("analyst", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: session.ID})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

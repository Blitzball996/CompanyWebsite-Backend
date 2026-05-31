// Package auth implements the customer account system: registration, email
// verification, login/logout via opaque session cookies, password reset, and
// account endpoints (purchased licenses, receipts). Passwords are bcrypt-hashed.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"blitzball-analytics/internal/captcha"
	"blitzball-analytics/internal/mailer"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const (
	cookieName    = "bb_session"
	sessionTTL    = 30 * 24 * time.Hour // 30 days
	resetTTL      = time.Hour
	bcryptCost    = 12
	ctxUserKey    = ctxKey("user")
)

type ctxKey string

// Handler holds dependencies for all auth/account endpoints.
type Handler struct {
	pool    *pgxpool.Pool
	mail    *mailer.Mailer
	cap     *captcha.GeeTest
	baseURL string // e.g. https://blitzball.lol — used in email links
	secure  bool   // set Secure cookie flag (true in production/https)
}

func New(pool *pgxpool.Pool, mail *mailer.Mailer, cap *captcha.GeeTest, baseURL string, secure bool) *Handler {
	return &Handler{pool: pool, mail: mail, cap: cap, baseURL: strings.TrimRight(baseURL, "/"), secure: secure}
}

// CaptchaConfig tells the frontend whether to show the GeeTest widget and its id.
func (h *Handler) CaptchaConfig(w http.ResponseWriter, r *http.Request) {
	id := ""
	if h.cap != nil && h.cap.Enabled() {
		id = h.cap.ID()
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"enabled": id != "", "captcha_id": id})
}

// User is the authenticated user injected into request context.
type User struct {
	ID    int64
	Email string
	Name  string
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, code int, errCode string) {
	writeJSON(w, code, map[string]interface{}{"ok": false, "error": errCode})
}

func token() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func normEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func validEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	return at > 0 && at < len(s)-1 && strings.IndexByte(s[at+1:], '.') >= 0 && len(s) <= 254
}

// newSession creates a session row and returns its token.
func (h *Handler) newSession(ctx context.Context, userID int64, ip, ua string) (string, time.Time, error) {
	t := token()
	exp := time.Now().Add(sessionTTL)
	_, err := h.pool.Exec(ctx,
		`INSERT INTO sessions (token, user_id, expires_at, ip, ua) VALUES ($1,$2,$3,$4,$5)`,
		t, userID, exp, ip, ua)
	return t, exp, err
}

func (h *Handler) setCookie(w http.ResponseWriter, tok string, exp time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: tok, Path: "/", Expires: exp,
		HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode,
	})
}
func (h *Handler) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode,
	})
}

// userFromRequest resolves the session cookie to a User (or nil).
func (h *Handler) userFromRequest(r *http.Request) *User {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return nil
	}
	var u User
	err = h.pool.QueryRow(r.Context(), `
		SELECT u.id, u.email, u.name FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token=$1 AND s.expires_at > now()`, c.Value).Scan(&u.ID, &u.Email, &u.Name)
	if err != nil {
		return nil
	}
	return &u
}

// RequireAuth is middleware that 401s unless a valid session is present.
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := h.userFromRequest(r)
		if u == nil {
			fail(w, http.StatusUnauthorized, "NOT_LOGGED_IN")
			return
		}
		ctx := context.WithValue(r.Context(), ctxUserKey, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func userOf(r *http.Request) *User {
	u, _ := r.Context().Value(ctxUserKey).(*User)
	return u
}

var _ = pgx.ErrNoRows
var _ = bcrypt.DefaultCost

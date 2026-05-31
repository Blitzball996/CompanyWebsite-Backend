package auth

import (
	"net/http"
	"time"

	"blitzball-analytics/internal/middleware"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type credReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Token    string `json:"token"`
}

func decode(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := jsonDecode(r, v); err != nil {
		fail(w, http.StatusBadRequest, "BAD_REQUEST")
		return false
	}
	return true
}

// Register creates an unverified account and emails a verification link.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req credReq
	if !decode(w, r, &req) {
		return
	}
	email := normEmail(req.Email)
	if !validEmail(email) {
		fail(w, http.StatusBadRequest, "BAD_EMAIL")
		return
	}
	if len(req.Password) < 8 {
		fail(w, http.StatusBadRequest, "WEAK_PASSWORD")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		fail(w, http.StatusInternalServerError, "HASH_ERROR")
		return
	}
	vtok := token()
	ctx := r.Context()
	_, err = h.pool.Exec(ctx,
		`INSERT INTO users (email, pass_hash, name, verify_token) VALUES ($1,$2,$3,$4)`,
		email, string(hash), req.Name, vtok)
	if err != nil {
		// unique violation → email already registered
		fail(w, http.StatusConflict, "EMAIL_TAKEN")
		return
	}
	h.sendVerify(email, vtok)
	writeJSON(w, http.StatusCreated, map[string]interface{}{"ok": true})
}

// Verify marks an email as verified given a token.
func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	tok := r.URL.Query().Get("token")
	if tok == "" {
		fail(w, http.StatusBadRequest, "NO_TOKEN")
		return
	}
	ct, err := h.pool.Exec(r.Context(),
		`UPDATE users SET email_verified=true, verify_token=NULL WHERE verify_token=$1`, tok)
	if err != nil || ct.RowsAffected() == 0 {
		fail(w, http.StatusBadRequest, "BAD_TOKEN")
		return
	}
	// friendly HTML so a click in the email lands somewhere nice
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<!doctype html><meta charset=utf-8><body style="font-family:system-ui;background:#0b0e14;color:#e6edf3;text-align:center;padding:80px">
	<h2>✅ 邮箱已验证 / Email verified</h2><p>你现在可以登录了。You can now log in.</p>
	<p><a style="color:#4f8cff" href="/login.html">→ 登录 / Log in</a></p></body>`))
}

// Login verifies credentials and starts a session.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req credReq
	if !decode(w, r, &req) {
		return
	}
	email := normEmail(req.Email)
	var (
		id       int64
		hash     string
		verified bool
		name     string
	)
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, pass_hash, email_verified, name FROM users WHERE email=$1`, email,
	).Scan(&id, &hash, &verified, &name)
	if err == pgx.ErrNoRows {
		fail(w, http.StatusUnauthorized, "BAD_CREDENTIALS")
		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, "DB_ERROR")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		fail(w, http.StatusUnauthorized, "BAD_CREDENTIALS")
		return
	}
	if !verified {
		fail(w, http.StatusForbidden, "EMAIL_NOT_VERIFIED")
		return
	}
	tok, exp, err := h.newSession(r.Context(), id, middleware.ClientIP(r), r.UserAgent())
	if err != nil {
		fail(w, http.StatusInternalServerError, "SESSION_ERROR")
		return
	}
	h.setCookie(w, tok, exp)
	_, _ = h.pool.Exec(r.Context(), `UPDATE users SET last_login_at=now() WHERE id=$1`, id)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "email": email, "name": name})
}

// Logout destroys the current session.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
		_, _ = h.pool.Exec(r.Context(), `DELETE FROM sessions WHERE token=$1`, c.Value)
	}
	h.clearCookie(w)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// Me returns the current logged-in user (or 401).
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	u := h.userFromRequest(r)
	if u == nil {
		fail(w, http.StatusUnauthorized, "NOT_LOGGED_IN")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "email": u.Email, "name": u.Name})
}

// Forgot issues a reset token and emails it. Always returns ok (no email enumeration).
func (h *Handler) Forgot(w http.ResponseWriter, r *http.Request) {
	var req credReq
	if !decode(w, r, &req) {
		return
	}
	email := normEmail(req.Email)
	rtok := token()
	ct, _ := h.pool.Exec(r.Context(),
		`UPDATE users SET reset_token=$2, reset_expires=$3 WHERE email=$1`,
		email, rtok, time.Now().Add(resetTTL))
	if ct.RowsAffected() == 1 {
		h.sendReset(email, rtok)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// Reset sets a new password given a valid (unexpired) reset token.
func (h *Handler) Reset(w http.ResponseWriter, r *http.Request) {
	var req credReq
	if !decode(w, r, &req) {
		return
	}
	if len(req.Password) < 8 {
		fail(w, http.StatusBadRequest, "WEAK_PASSWORD")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		fail(w, http.StatusInternalServerError, "HASH_ERROR")
		return
	}
	ct, err := h.pool.Exec(r.Context(),
		`UPDATE users SET pass_hash=$2, reset_token=NULL, reset_expires=NULL
		 WHERE reset_token=$1 AND reset_expires > now()`, req.Token, string(hash))
	if err != nil || ct.RowsAffected() == 0 {
		fail(w, http.StatusBadRequest, "BAD_OR_EXPIRED_TOKEN")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

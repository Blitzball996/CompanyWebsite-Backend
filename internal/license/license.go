package license

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler serves activation / verification / admin endpoints.
type Handler struct {
	pool *pgxpool.Pool
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
}

func New(pool *pgxpool.Pool, priv ed25519.PrivateKey) *Handler {
	return &Handler{pool: pool, priv: priv, pub: priv.Public().(ed25519.PublicKey)}
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// activateReq is what the app POSTs.
type activateReq struct {
	Key        string `json:"key"`
	DeviceID   string `json:"device_id"`
	Product    string `json:"product"`     // optional cross-check
	AppVersion string `json:"app_version"` // optional, for logging
}

// token is the signed payload the app stores and verifies offline thereafter.
type token struct {
	Key      string `json:"key"`
	Product  string `json:"product"`
	Edition  string `json:"edition"`
	DeviceID string `json:"device_id"`
	IssuedAt string `json:"issued_at"`
	V        int    `json:"v"`
}

func (h *Handler) sign(t token) (tokenB64, sigB64 string) {
	raw, _ := json.Marshal(t)
	sig := ed25519.Sign(h.priv, raw)
	return base64.RawURLEncoding.EncodeToString(raw), base64.RawURLEncoding.EncodeToString(sig)
}

func fail(w http.ResponseWriter, code int, errCode string) {
	writeJSON(w, code, map[string]interface{}{"ok": false, "error": errCode})
}

// Activate binds a key to a device (one-key-one-device) and returns a signed token.
func (h *Handler) Activate(w http.ResponseWriter, r *http.Request) {
	var req activateReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(req.DeviceID) == "" {
		fail(w, http.StatusBadRequest, "NO_DEVICE_ID")
		return
	}
	canon, product, code := Parse(req.Key)
	if code != "" {
		fail(w, http.StatusBadRequest, code) // BAD_FORMAT / BAD_CHECKSUM / WRONG_PRODUCT
		return
	}
	if req.Product != "" && req.Product != product {
		fail(w, http.StatusBadRequest, "WRONG_PRODUCT")
		return
	}

	ctx := r.Context()
	var (
		dbProduct, dbEdition, status string
		boundDevice                  *string
	)
	err := h.pool.QueryRow(ctx,
		`SELECT product, edition, status, device_id FROM licenses WHERE key=$1`, canon,
	).Scan(&dbProduct, &dbEdition, &status, &boundDevice)
	if err == pgx.ErrNoRows {
		fail(w, http.StatusNotFound, "NOT_FOUND")
		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, "DB_ERROR")
		return
	}
	if status != "active" {
		fail(w, http.StatusForbidden, "REVOKED")
		return
	}
	if boundDevice != nil && *boundDevice != "" && *boundDevice != req.DeviceID {
		fail(w, http.StatusForbidden, "ALREADY_ACTIVATED")
		return
	}

	// bind device (first activation) or refresh last_seen (same device reinstall)
	if _, err := h.pool.Exec(ctx,
		`UPDATE licenses SET device_id=$2,
		   activated_at=COALESCE(activated_at, now()), last_seen_at=now()
		 WHERE key=$1`, canon, req.DeviceID); err != nil {
		fail(w, http.StatusInternalServerError, "DB_ERROR")
		return
	}

	tk := token{Key: canon, Product: dbProduct, Edition: dbEdition,
		DeviceID: req.DeviceID, IssuedAt: time.Now().UTC().Format(time.RFC3339), V: 1}
	tb, sb := h.sign(tk)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "error": "OK", "key": canon, "product": dbProduct, "edition": dbEdition,
		"token": tb, "sig": sb,
	})
}

// Verify re-checks an activation is still valid (called periodically / at startup).
func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	var req activateReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "BAD_REQUEST")
		return
	}
	canon, _, code := Parse(req.Key)
	if code != "" {
		fail(w, http.StatusBadRequest, code)
		return
	}
	ctx := r.Context()
	var status string
	var boundDevice *string
	err := h.pool.QueryRow(ctx,
		`SELECT status, device_id FROM licenses WHERE key=$1`, canon,
	).Scan(&status, &boundDevice)
	if err == pgx.ErrNoRows {
		fail(w, http.StatusNotFound, "NOT_FOUND")
		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, "DB_ERROR")
		return
	}
	if status != "active" {
		fail(w, http.StatusForbidden, "REVOKED")
		return
	}
	if boundDevice == nil || *boundDevice != req.DeviceID {
		fail(w, http.StatusForbidden, "DEVICE_MISMATCH")
		return
	}
	_, _ = h.pool.Exec(ctx, `UPDATE licenses SET last_seen_at=now() WHERE key=$1`, canon)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "error": "OK"})
}

// PublicKey exposes the Ed25519 verify key (base64) — embed it in the apps.
func (h *Handler) PublicKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"public_key": base64.RawURLEncoding.EncodeToString(h.pub),
		"algorithm":  "Ed25519",
	})
}

// maskKey hides the middle groups of a serial, keeping the product prefix and
// the last group, e.g. CCPR-Q4MN-7P2K-9XW3-HJ5B -> CCPR-••••-••••-••••-HJ5B.
func maskKey(k string) string {
	parts := strings.Split(k, "-")
	if len(parts) != 5 {
		return "••••"
	}
	return parts[0] + "-••••-••••-••••-" + parts[4]
}

// Lookup returns the (masked) licenses bound to an email so a buyer can check
// what they own and each key's activation status. Public + rate-limited; keys
// are masked so the email alone never reveals a usable serial.
func (h *Handler) Lookup(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("email")))
	if email == "" || len(email) > 254 || !strings.Contains(email, "@") {
		fail(w, http.StatusBadRequest, "BAD_EMAIL")
		return
	}
	rows, err := h.pool.Query(r.Context(),
		`SELECT key, product, edition, status, COALESCE(device_id,''), created_at
		   FROM licenses WHERE lower(email)=$1 ORDER BY created_at DESC LIMIT 50`, email)
	if err != nil {
		fail(w, http.StatusInternalServerError, "DB_ERROR")
		return
	}
	defer rows.Close()
	out := make([]map[string]interface{}, 0, 8)
	for rows.Next() {
		var key, product, edition, status, device string
		var createdAt time.Time
		if err := rows.Scan(&key, &product, &edition, &status, &device, &createdAt); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"product":    product,
			"edition":    edition,
			"status":     status,
			"activated":  device != "",
			"key_masked": maskKey(key),
			"created_at": createdAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "count": len(out), "licenses": out})
}

// ---- admin (Basic Auth protected by the router) ----

type genReq struct {
	Product string `json:"product"` // BDST/BDPR/CCST/CCPR
	Count   int    `json:"count"`
	OrderID string `json:"order_id"`
	Email   string `json:"email"`
}

// Generate issues N new keys for a product and stores them.
func (h *Handler) GenerateAdmin(w http.ResponseWriter, r *http.Request) {
	var req genReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "BAD_REQUEST")
		return
	}
	if _, ok := ValidProducts[req.Product]; !ok {
		fail(w, http.StatusBadRequest, "WRONG_PRODUCT")
		return
	}
	if req.Count < 1 {
		req.Count = 1
	}
	if req.Count > 500 {
		req.Count = 500
	}
	ed := Edition(req.Product)
	keys := make([]string, 0, req.Count)
	ctx := r.Context()
	for i := 0; i < req.Count; i++ {
		var k string
		// retry on the (astronomically rare) unique collision
		for attempt := 0; attempt < 5; attempt++ {
			g, err := Generate(req.Product)
			if err != nil {
				fail(w, http.StatusInternalServerError, "GEN_ERROR")
				return
			}
			_, err = h.pool.Exec(ctx,
				`INSERT INTO licenses (key, product, edition, order_id, email)
				 VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''))`,
				g, req.Product, ed, req.OrderID, req.Email)
			if err == nil {
				k = g
				break
			}
		}
		if k == "" {
			fail(w, http.StatusInternalServerError, "DB_ERROR")
			return
		}
		keys = append(keys, k)
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"ok": true, "count": len(keys), "keys": keys})
}

// ListAdmin returns recent licenses for the dashboard.
func (h *Handler) ListAdmin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.pool.Query(ctx,
		`SELECT key, product, edition, COALESCE(order_id,''), COALESCE(email,''),
		        status, COALESCE(device_id,''), activated_at, created_at
		 FROM licenses ORDER BY created_at DESC LIMIT 500`)
	if err != nil {
		fail(w, http.StatusInternalServerError, "DB_ERROR")
		return
	}
	defer rows.Close()
	type row struct {
		Key, Product, Edition, OrderID, Email, Status, DeviceID string
		ActivatedAt                                             *time.Time
		CreatedAt                                               time.Time
	}
	var out []map[string]interface{}
	for rows.Next() {
		var x row
		rows.Scan(&x.Key, &x.Product, &x.Edition, &x.OrderID, &x.Email, &x.Status,
			&x.DeviceID, &x.ActivatedAt, &x.CreatedAt)
		out = append(out, map[string]interface{}{
			"key": x.Key, "product": x.Product, "edition": x.Edition,
			"order_id": x.OrderID, "email": x.Email, "status": x.Status,
			"device_id": x.DeviceID, "activated_at": x.ActivatedAt, "created_at": x.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// RevokeAdmin disables a key (e.g. after refund/chargeback).
func (h *Handler) RevokeAdmin(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	canon, _, code := Parse(key)
	if code != "" {
		fail(w, http.StatusBadRequest, code)
		return
	}
	ct, err := h.pool.Exec(r.Context(), `UPDATE licenses SET status='revoked' WHERE key=$1`, canon)
	if err != nil {
		fail(w, http.StatusInternalServerError, "DB_ERROR")
		return
	}
	if ct.RowsAffected() == 0 {
		fail(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// IssueForOrder issues exactly one key for a paid order, idempotently: if a key
// already exists for orderID, it returns that same key (so webhook retries never
// create duplicates). Returns the key and whether it was newly created.
func (h *Handler) IssueForOrder(product, orderID, email string) (key string, created bool, err error) {
	if _, ok := ValidProducts[product]; !ok {
		return "", false, errExt("unknown product")
	}
	ctx := context.Background()
	if orderID != "" {
		// already issued for this order?
		var existing string
		e := h.pool.QueryRow(ctx, `SELECT key FROM licenses WHERE order_id=$1 LIMIT 1`, orderID).Scan(&existing)
		if e == nil && existing != "" {
			return existing, false, nil
		}
	}
	ed := Edition(product)
	for attempt := 0; attempt < 6; attempt++ {
		g, gerr := Generate(product)
		if gerr != nil {
			return "", false, gerr
		}
		_, ierr := h.pool.Exec(ctx,
			`INSERT INTO licenses (key, product, edition, order_id, email)
			 VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''))`,
			g, product, ed, orderID, email)
		if ierr == nil {
			return g, true, nil
		}
	}
	return "", false, errExt("could not issue key after retries")
}

type extErr string

func (e extErr) Error() string { return string(e) }
func errExt(s string) error    { return extErr(s) }

// EditionOf returns the edition ("standard"/"pro") for a product prefix.
func (h *Handler) EditionOf(product string) string { return Edition(product) }

// RecordReceipt upserts a receipt row for a paid order (idempotent on order_id).
func (h *Handler) RecordReceipt(orderID, email, product, edition, amount, currency, key string) {
	if orderID == "" {
		return
	}
	_, _ = h.pool.Exec(context.Background(), `
		INSERT INTO receipts (order_id, email, product, edition, amount, currency, license_key)
		VALUES ($1,NULLIF($2,''),$3,$4,$5,$6,NULLIF($7,''))
		ON CONFLICT (order_id) DO NOTHING`,
		orderID, email, product, edition, amount, currency, key)
}
func (h *Handler) ResetDeviceAdmin(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	canon, _, code := Parse(key)
	if code != "" {
		fail(w, http.StatusBadRequest, code)
		return
	}
	if _, err := h.pool.Exec(r.Context(),
		`UPDATE licenses SET device_id=NULL, activated_at=NULL WHERE key=$1`, canon); err != nil {
		fail(w, http.StatusInternalServerError, "DB_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

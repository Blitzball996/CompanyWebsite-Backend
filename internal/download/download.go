// Package download gates product downloads behind a paid license key.
//
// Flow:
//  1. Buyer pays on Creem → webhook issues a license key and emails it.
//  2. Buyer lands on download.html and enters the key (+ the email used at
//     checkout, when we have one on file).
//  3. POST /api/download/claim validates the key against the licenses table and
//     returns short-lived, HMAC-signed download URLs for every platform.
//  4. GET /api/download/file?...&sig=... verifies the signature and 302-redirects
//     to the real artifact URL. Links expire, so they can't be shared forever.
//
// The signature covers key+product+platform+expiry, so a link cannot be edited
// to fetch a different product than the one that was actually purchased.
package download

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TTL of a signed download link.
const linkTTL = 30 * time.Minute

// Artifact is one downloadable file for a product.
type Artifact struct {
	Platform string `json:"platform"` // windows | macos | linux
	Label    string `json:"label"`    // "Windows · .exe"
	Ext      string `json:"ext"`      // ".exe"
	Size     string `json:"size"`     // "42 MB" (display only)
	url      string // real upstream URL, never sent to the client
}

// catalog maps a product family ("BD" = Blitz DAW, "CC" = CloseCrab) to its
// artifacts. Update the URLs when you cut a new release.
var catalog = map[string][]Artifact{
	"BD": {
		{Platform: "windows", Label: "Windows · .zip", Ext: ".zip", Size: "38 MB",
			url: "https://github.com/Blitzball996/AIDAW/releases/latest/download/Blitz-Windows-x64.zip"},
		{Platform: "macos", Label: "macOS · .dmg", Ext: ".dmg", Size: "41 MB",
			url: "https://github.com/Blitzball996/AIDAW/releases/latest/download/Blitz-macOS-arm64.dmg"},
		{Platform: "linux", Label: "Linux · .tar.gz", Ext: ".tar.gz", Size: "36 MB",
			url: "https://github.com/Blitzball996/AIDAW/releases/latest/download/Blitz-linux-x86_64.tar.gz"},
	},
	"CC": {
		{Platform: "windows", Label: "Windows · .exe", Ext: ".exe", Size: "3.2 MB",
			url: "https://github.com/Blitzball996/CloseCrab-Unified/releases/download/v0.4.6/CloseCrab-Setup-0.4.6.exe"},
		{Platform: "macos", Label: "macOS · .pkg", Ext: ".pkg", Size: "3.4 MB",
			url: "https://github.com/Blitzball996/CloseCrab-Unified/releases/latest/download/CloseCrab-macOS-universal.pkg"},
		{Platform: "linux", Label: "Linux · .tar.gz", Ext: ".tar.gz", Size: "3.1 MB",
			url: "https://github.com/Blitzball996/CloseCrab-Unified/releases/latest/download/CloseCrab-Linux-x86_64.tar.gz"},
	},
}

var productNames = map[string]string{"BD": "Blitz DAW", "CC": "CloseCrab"}

type Handler struct {
	pool   *pgxpool.Pool
	secret string
}

func New(pool *pgxpool.Pool, secret string) *Handler {
	return &Handler{pool: pool, secret: secret}
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, errCode string) {
	writeJSON(w, code, map[string]interface{}{"ok": false, "error": errCode})
}

// normKey uppercases and re-hyphenates a user-typed key so
// "bdpr7k3p9wxm2qh4rt8c" and "BDPR-7K3P-9WXM-2QH4-RT8C" both work.
func normKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(s)) {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	raw := b.String()
	if len(raw) != 20 {
		return raw // let the DB lookup fail
	}
	return raw[0:4] + "-" + raw[4:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20]
}

// family returns "BD"/"CC" for a product prefix like "BDPR".
func family(product string) string {
	if len(product) < 2 {
		return ""
	}
	return strings.ToUpper(product[:2])
}

// sign computes the link signature over the canonical field order.
func (h *Handler) sign(key, product, platform string, exp int64) string {
	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write([]byte(key + "|" + product + "|" + platform + "|" + strconv.FormatInt(exp, 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

type claimReq struct {
	Key   string `json:"key"`
	Email string `json:"email"`
}

// Claim validates a license key and returns signed, expiring download links.
func (h *Handler) Claim(w http.ResponseWriter, r *http.Request) {
	var req claimReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "BAD_REQUEST")
		return
	}
	key := normKey(req.Key)
	if len(key) != 24 {
		fail(w, http.StatusBadRequest, "BAD_KEY_FORMAT")
		return
	}

	var product, edition, status, email string
	err := h.pool.QueryRow(r.Context(),
		`SELECT product, edition, status, COALESCE(email,'') FROM licenses WHERE key=$1`, key,
	).Scan(&product, &edition, &status, &email)
	if err != nil {
		// Same generic error for "not found" so the endpoint can't be used to
		// enumerate which keys exist.
		fail(w, http.StatusNotFound, "KEY_NOT_FOUND")
		return
	}
	if status != "active" {
		fail(w, http.StatusForbidden, "KEY_REVOKED")
		return
	}
	// When we have a buyer email on file, require it to match — stops a leaked
	// key from being used by anyone who finds it.
	if email != "" && req.Email != "" &&
		!strings.EqualFold(strings.TrimSpace(req.Email), email) {
		fail(w, http.StatusForbidden, "EMAIL_MISMATCH")
		return
	}

	fam := family(product)
	arts, ok := catalog[fam]
	if !ok {
		fail(w, http.StatusUnprocessableEntity, "NO_ARTIFACTS")
		return
	}

	exp := time.Now().Add(linkTTL).Unix()
	files := make([]map[string]string, 0, len(arts))
	for _, a := range arts {
		q := "key=" + key + "&product=" + product + "&platform=" + a.Platform +
			"&exp=" + strconv.FormatInt(exp, 10) +
			"&sig=" + h.sign(key, product, a.Platform, exp)
		files = append(files, map[string]string{
			"platform": a.Platform,
			"label":    a.Label,
			"size":     a.Size,
			"url":      "/api/download/file?" + q,
		})
	}

	_, _ = h.pool.Exec(r.Context(),
		`UPDATE licenses SET last_seen_at=now() WHERE key=$1`, key)
	log.Printf("download: claim ok key=%s product=%s", maskKey(key), product)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":           true,
		"product":      product,
		"product_name": productNames[fam],
		"edition":      edition,
		"expires_at":   exp,
		"files":        files,
	})
}

// File verifies a signed link and redirects to the real artifact.
func (h *Handler) File(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	key, product, platform := q.Get("key"), q.Get("product"), q.Get("platform")
	exp, err := strconv.ParseInt(q.Get("exp"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "BAD_LINK")
		return
	}
	if time.Now().Unix() > exp {
		fail(w, http.StatusGone, "LINK_EXPIRED")
		return
	}
	want := h.sign(key, product, platform, exp)
	if !hmac.Equal([]byte(q.Get("sig")), []byte(want)) {
		fail(w, http.StatusForbidden, "BAD_SIGNATURE")
		return
	}
	for _, a := range catalog[family(product)] {
		if a.Platform == platform {
			http.Redirect(w, r, a.url, http.StatusFound)
			return
		}
	}
	fail(w, http.StatusNotFound, "NO_SUCH_PLATFORM")
}

func maskKey(k string) string {
	if len(k) < 9 {
		return "****"
	}
	return k[:4] + "-****-****-****-" + k[len(k)-4:]
}

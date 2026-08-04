// Package team implements CloseCrab Team Mode's cloud backend: a shared
// leaderboard and online-presence list scoped to a "team" (= all phone clients
// served by one activated CloseCrab license).
//
// Trust model: requests come server-to-server from CloseCrab-Web (which holds
// the license key, read from the host's registry after the user activated
// CloseCrab). Every call carries {key, device_id}; we re-validate that the key
// is active, bound to that device, and has remote_enabled, then derive
// team_id = SHA-256(key)[:16] so the raw key never travels to phones or appears
// in the leaderboard payload. Endpoints are CORS- and rate-limited by the router.
package team

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"blitzball-analytics/internal/license"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// onlineWindow is how recent a heartbeat must be to count as "online".
const onlineWindow = 90 * time.Second

// maxUsername bounds stored usernames (defensive; phones set these freely).
const maxUsername = 48

// Handler holds the DB pool for all team endpoints.
type Handler struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Handler { return &Handler{pool: pool} }

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, code int, errCode string) {
	writeJSON(w, code, map[string]interface{}{"ok": false, "error": errCode})
}

// teamID derives the public team identifier from a canonical license key.
func teamID(canonicalKey string) string {
	sum := sha256.Sum256([]byte("closecrab-team:" + canonicalKey))
	return hex.EncodeToString(sum[:])[:16]
}

func cleanUsername(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "Anonymous"
	}
	if len(s) > maxUsername {
		s = s[:maxUsername]
	}
	return s
}

// authResult is the validated context derived from a {key, device_id} pair.
type authResult struct {
	teamID  string
	product string
	edition string
}

// authorize validates the license key + device binding and returns the derived
// team context. Mirrors the checks in license.Verify but also requires the
// CloseCrab product family and remote_enabled. Returns ("", errCode) on failure.
func (h *Handler) authorize(r *http.Request, rawKey, deviceID string) (*authResult, string) {
	canon, product, code := license.Parse(rawKey)
	if code != "" {
		return nil, code
	}
	if !strings.HasPrefix(product, "CC") {
		return nil, "WRONG_PRODUCT"
	}
	if strings.TrimSpace(deviceID) == "" {
		return nil, "NO_DEVICE_ID"
	}
	var (
		edition, status string
		boundDevice     *string
		remoteEnabled   bool
	)
	err := h.pool.QueryRow(r.Context(),
		`SELECT edition, status, device_id, remote_enabled FROM licenses WHERE key=$1`, canon,
	).Scan(&edition, &status, &boundDevice, &remoteEnabled)
	if err == pgx.ErrNoRows {
		return nil, "NOT_FOUND"
	} else if err != nil {
		return nil, "DB_ERROR"
	}
	if status != "active" {
		return nil, "REVOKED"
	}
	if boundDevice == nil || *boundDevice != deviceID {
		return nil, "DEVICE_MISMATCH"
	}
	if !remoteEnabled {
		return nil, "REMOTE_DISABLED"
	}
	return &authResult{teamID: teamID(canon), product: product, edition: edition}, ""
}

// ---- request bodies ----

type scoreReq struct {
	Key      string   `json:"key"`
	DeviceID string   `json:"device_id"`
	Username string   `json:"username"`
	Score    int64    `json:"score"`
	Badges   []string `json:"badges"`
}

type presenceReq struct {
	Key      string `json:"key"`
	DeviceID string `json:"device_id"`
	Username string `json:"username"`
}

// Score upserts a player's score for their team (keeps the max so replays /
// reconnects never lower a standing score) and records presence.
func (h *Handler) Score(w http.ResponseWriter, r *http.Request) {
	var req scoreReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "BAD_REQUEST")
		return
	}
	auth, code := h.authorize(r, req.Key, req.DeviceID)
	if code != "" {
		fail(w, statusFor(code), code)
		return
	}
	username := cleanUsername(req.Username)
	if req.Score < 0 {
		req.Score = 0
	}
	badges := req.Badges
	if badges == nil {
		badges = []string{}
	}
	badgesJSON, _ := json.Marshal(badges)

	ctx := r.Context()
	if _, err := h.pool.Exec(ctx, `
		INSERT INTO team_scores (team_id, username, device_id, score, badges, updated_at)
		VALUES ($1,$2,$3,$4,$5,now())
		ON CONFLICT (team_id, username) DO UPDATE SET
			score      = GREATEST(team_scores.score, EXCLUDED.score),
			badges     = EXCLUDED.badges,
			device_id  = EXCLUDED.device_id,
			updated_at = now()`,
		auth.teamID, username, req.DeviceID, req.Score, string(badgesJSON)); err != nil {
		fail(w, http.StatusInternalServerError, "DB_ERROR")
		return
	}
	h.touchPresence(ctx, auth.teamID, username)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "team_id": auth.teamID})
}

// Leaderboard returns the top scores for the caller's team.
func (h *Handler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	auth, code := h.authorize(r, r.URL.Query().Get("key"), r.URL.Query().Get("device_id"))
	if code != "" {
		fail(w, statusFor(code), code)
		return
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT username, score, badges FROM team_scores
		WHERE team_id=$1 ORDER BY score DESC, updated_at ASC LIMIT 100`, auth.teamID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "DB_ERROR")
		return
	}
	defer rows.Close()
	out := make([]map[string]interface{}, 0, 16)
	for rows.Next() {
		var username string
		var score int64
		var badgesRaw []byte
		if err := rows.Scan(&username, &score, &badgesRaw); err != nil {
			continue
		}
		var badges []string
		if len(badgesRaw) > 0 {
			_ = json.Unmarshal(badgesRaw, &badges)
		}
		if badges == nil {
			badges = []string{}
		}
		out = append(out, map[string]interface{}{
			"username": username, "score": score, "badges": badges,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "team_id": auth.teamID, "scope": "cloud", "entries": out,
	})
}

// Online returns usernames seen within the freshness window for the team.
func (h *Handler) Online(w http.ResponseWriter, r *http.Request) {
	auth, code := h.authorize(r, r.URL.Query().Get("key"), r.URL.Query().Get("device_id"))
	if code != "" {
		fail(w, statusFor(code), code)
		return
	}
	cutoff := time.Now().Add(-onlineWindow)
	rows, err := h.pool.Query(r.Context(), `
		SELECT username, last_seen FROM team_presence
		WHERE team_id=$1 AND last_seen > $2 ORDER BY last_seen DESC LIMIT 200`,
		auth.teamID, cutoff)
	if err != nil {
		fail(w, http.StatusInternalServerError, "DB_ERROR")
		return
	}
	defer rows.Close()
	out := make([]map[string]interface{}, 0, 16)
	for rows.Next() {
		var username string
		var lastSeen time.Time
		if err := rows.Scan(&username, &lastSeen); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{"username": username, "last_seen": lastSeen})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "team_id": auth.teamID, "count": len(out), "members": out,
	})
}

// Heartbeat refreshes presence for a player without touching scores.
func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	var req presenceReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "BAD_REQUEST")
		return
	}
	auth, code := h.authorize(r, req.Key, req.DeviceID)
	if code != "" {
		fail(w, statusFor(code), code)
		return
	}
	h.touchPresence(r.Context(), auth.teamID, cleanUsername(req.Username))
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

func (h *Handler) touchPresence(ctx context.Context, teamID, username string) {
	_, _ = h.pool.Exec(ctx, `
		INSERT INTO team_presence (team_id, username, last_seen)
		VALUES ($1,$2,now())
		ON CONFLICT (team_id, username) DO UPDATE SET last_seen=now()`,
		teamID, username)
}

// statusFor maps an error code to an HTTP status.
func statusFor(code string) int {
	switch code {
	case "BAD_REQUEST", "BAD_FORMAT", "BAD_CHECKSUM", "WRONG_PRODUCT", "NO_DEVICE_ID":
		return http.StatusBadRequest
	case "NOT_FOUND":
		return http.StatusNotFound
	case "REVOKED", "DEVICE_MISMATCH", "REMOTE_DISABLED":
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

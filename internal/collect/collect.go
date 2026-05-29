package collect

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"blitzball-analytics/internal/middleware"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	pool *pgxpool.Pool
	salt string
}

func New(pool *pgxpool.Pool, salt string) *Handler {
	return &Handler{pool: pool, salt: salt}
}

// payload is what analytics.js POSTs.
type payload struct {
	Type        string                 `json:"type"`
	Name        string                 `json:"name"`
	SessionID   string                 `json:"session_id"`
	Page        string                 `json:"page"`
	Title       string                 `json:"title"`
	Referrer    string                 `json:"referrer"`
	UTMSource   string                 `json:"utm_source"`
	UTMMedium   string                 `json:"utm_medium"`
	UTMCampaign string                 `json:"utm_campaign"`
	Lang        string                 `json:"lang"`
	Screen      string                 `json:"screen"`
	DurationMs  int64                  `json:"duration_ms"`
	ScrollPct   int                    `json:"scroll_pct"`
	Meta        map[string]interface{} `json:"meta"`
}

func (h *Handler) Collect(w http.ResponseWriter, r *http.Request) {
	var p payload
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&p); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if p.Type == "" {
		p.Type = "pageview"
	}
	if p.Page == "" {
		p.Page = "/"
	}

	ua := r.Header.Get("User-Agent")
	ip := middleware.ClientIP(r)
	visitorID := h.hashVisitor(ip, ua)
	if p.SessionID == "" {
		p.SessionID = visitorID // fallback
	}

	device, browser, os := parseUA(ua)
	var metaJSON []byte
	if p.Meta != nil {
		metaJSON, _ = json.Marshal(p.Meta)
	}

	ctx := r.Context()
	_, err := h.pool.Exec(ctx, `
		INSERT INTO events
		  (type, name, visitor_id, session_id, page, title, referrer,
		   utm_source, utm_medium, utm_campaign, country, device, browser, os,
		   lang, screen, duration_ms, scroll_pct, meta, created_at)
		VALUES
		  ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
		p.Type, nullStr(p.Name), visitorID, p.SessionID, p.Page, nullStr(p.Title),
		nullStr(p.Referrer), nullStr(p.UTMSource), nullStr(p.UTMMedium), nullStr(p.UTMCampaign),
		"", device, browser, os, nullStr(p.Lang), nullStr(p.Screen),
		p.DurationMs, p.ScrollPct, metaJSON, time.Now(),
	)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) hashVisitor(ip, ua string) string {
	// Daily-rotating, salted hash → anonymous, cannot reverse to an IP.
	day := time.Now().UTC().Format("2006-01-02")
	sum := sha256.Sum256([]byte(h.salt + "|" + day + "|" + ip + "|" + ua))
	return hex.EncodeToString(sum[:])[:16]
}

func nullStr(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// parseUA is a lightweight User-Agent classifier (no external deps).
func parseUA(ua string) (device, browser, os string) {
	l := strings.ToLower(ua)
	switch {
	case strings.Contains(l, "ipad") || strings.Contains(l, "tablet"):
		device = "tablet"
	case strings.Contains(l, "mobi") || strings.Contains(l, "android") || strings.Contains(l, "iphone"):
		device = "mobile"
	default:
		device = "desktop"
	}
	switch {
	case strings.Contains(l, "edg/"):
		browser = "Edge"
	case strings.Contains(l, "chrome") && !strings.Contains(l, "chromium"):
		browser = "Chrome"
	case strings.Contains(l, "firefox"):
		browser = "Firefox"
	case strings.Contains(l, "safari") && !strings.Contains(l, "chrome"):
		browser = "Safari"
	default:
		browser = "Other"
	}
	switch {
	case strings.Contains(l, "windows"):
		os = "Windows"
	case strings.Contains(l, "mac os") || strings.Contains(l, "macintosh"):
		os = "macOS"
	case strings.Contains(l, "android"):
		os = "Android"
	case strings.Contains(l, "iphone") || strings.Contains(l, "ipad") || strings.Contains(l, "ios"):
		os = "iOS"
	case strings.Contains(l, "linux"):
		os = "Linux"
	default:
		os = "Other"
	}
	return
}

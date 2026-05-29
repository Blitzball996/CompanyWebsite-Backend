package stats

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Handler { return &Handler{pool: pool} }

// days returns the ?days= query param (default 7, capped 1..365).
func days(r *http.Request) int {
	d, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if d <= 0 {
		d = 7
	}
	if d > 365 {
		d = 365
	}
	return d
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

type kv struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// Summary: top cards (PV, UV, sessions, avg duration, bounce rate).
func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d := days(r)
	since := time.Now().AddDate(0, 0, -d)

	var pv, uv, sessions int64
	var avgDur float64
	h.pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE type='pageview' AND created_at>=$1`, since).Scan(&pv)
	h.pool.QueryRow(ctx, `SELECT count(DISTINCT visitor_id) FROM events WHERE created_at>=$1`, since).Scan(&uv)
	h.pool.QueryRow(ctx, `SELECT count(DISTINCT session_id) FROM events WHERE created_at>=$1`, since).Scan(&sessions)
	h.pool.QueryRow(ctx, `SELECT COALESCE(avg(duration_ms),0) FROM events WHERE type='pageview' AND duration_ms>0 AND created_at>=$1`, since).Scan(&avgDur)

	// bounce: sessions with exactly 1 pageview
	var bounced int64
	h.pool.QueryRow(ctx, `
		SELECT count(*) FROM (
		  SELECT session_id FROM events
		  WHERE type='pageview' AND created_at>=$1
		  GROUP BY session_id HAVING count(*)=1
		) t`, since).Scan(&bounced)
	bounce := 0.0
	if sessions > 0 {
		bounce = float64(bounced) / float64(sessions) * 100
	}

	// live: distinct visitors in last 5 min
	var live int64
	h.pool.QueryRow(ctx, `SELECT count(DISTINCT visitor_id) FROM events WHERE created_at>=$1`, time.Now().Add(-5*time.Minute)).Scan(&live)

	writeJSON(w, map[string]interface{}{
		"pv": pv, "uv": uv, "sessions": sessions,
		"avg_duration_ms": int64(avgDur),
		"bounce_rate":     bounce,
		"live":            live,
		"days":            d,
	})
}

// Trend: PV & UV grouped by day.
func (h *Handler) Trend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	since := time.Now().AddDate(0, 0, -days(r))
	rows, err := h.pool.Query(ctx, `
		SELECT to_char(date_trunc('day', created_at), 'YYYY-MM-DD') AS d,
		       count(*) FILTER (WHERE type='pageview') AS pv,
		       count(DISTINCT visitor_id) AS uv
		FROM events WHERE created_at>=$1
		GROUP BY 1 ORDER BY 1`, since)
	if err != nil {
		http.Error(w, "db", 500)
		return
	}
	defer rows.Close()
	type point struct {
		Date string `json:"date"`
		PV   int64  `json:"pv"`
		UV   int64  `json:"uv"`
	}
	var out []point
	for rows.Next() {
		var p point
		rows.Scan(&p.Date, &p.PV, &p.UV)
		out = append(out, p)
	}
	writeJSON(w, out)
}

func (h *Handler) groupBy(w http.ResponseWriter, r *http.Request, col string, pageviewOnly bool) {
	ctx := r.Context()
	since := time.Now().AddDate(0, 0, -days(r))
	filter := ""
	if pageviewOnly {
		filter = "AND type='pageview'"
	}
	q := `SELECT COALESCE(NULLIF(` + col + `,''),'(none)') AS label, count(*) AS c
	      FROM events WHERE created_at>=$1 ` + filter + `
	      GROUP BY 1 ORDER BY c DESC LIMIT 20`
	rows, err := h.pool.Query(ctx, q, since)
	if err != nil {
		http.Error(w, "db", 500)
		return
	}
	defer rows.Close()
	var out []kv
	for rows.Next() {
		var k kv
		rows.Scan(&k.Label, &k.Count)
		out = append(out, k)
	}
	writeJSON(w, out)
}

func (h *Handler) Pages(w http.ResponseWriter, r *http.Request)    { h.groupBy(w, r, "page", true) }
func (h *Handler) Referrers(w http.ResponseWriter, r *http.Request) { h.groupBy(w, r, "referrer", false) }
func (h *Handler) Devices(w http.ResponseWriter, r *http.Request)  { h.groupBy(w, r, "device", true) }
func (h *Handler) Browsers(w http.ResponseWriter, r *http.Request) { h.groupBy(w, r, "browser", true) }
func (h *Handler) OS(w http.ResponseWriter, r *http.Request)       { h.groupBy(w, r, "os", true) }
func (h *Handler) Langs(w http.ResponseWriter, r *http.Request)    { h.groupBy(w, r, "lang", true) }

// Events: top custom event names.
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	since := time.Now().AddDate(0, 0, -days(r))
	rows, err := h.pool.Query(ctx, `
		SELECT COALESCE(name,'(unnamed)') AS label, count(*) c
		FROM events WHERE type='event' AND created_at>=$1
		GROUP BY 1 ORDER BY c DESC LIMIT 20`, since)
	if err != nil {
		http.Error(w, "db", 500)
		return
	}
	defer rows.Close()
	var out []kv
	for rows.Next() {
		var k kv
		rows.Scan(&k.Label, &k.Count)
		out = append(out, k)
	}
	writeJSON(w, out)
}

var _ = context.Background

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

// Regions groups visits by ?level=country|province|city (default province).
func (h *Handler) Regions(w http.ResponseWriter, r *http.Request) {
	col := "province"
	switch r.URL.Query().Get("level") {
	case "country":
		col = "country"
	case "city":
		col = "city"
	}
	h.groupBy(w, r, col, false)
}

// Visitors lists recent unique visitors with their geo + aggregate behavior.
func (h *Handler) Visitors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	since := time.Now().AddDate(0, 0, -days(r))
	rows, err := h.pool.Query(ctx, `
		SELECT visitor_id,
		       COALESCE(max(ip),'')        AS ip,
		       COALESCE(max(country),'')   AS country,
		       COALESCE(max(province),'')  AS province,
		       COALESCE(max(city),'')      AS city,
		       COALESCE(max(isp),'')       AS isp,
		       COALESCE(max(device),'')    AS device,
		       COALESCE(max(browser),'')   AS browser,
		       COALESCE(max(os),'')        AS os,
		       count(*) FILTER (WHERE type='pageview') AS pv,
		       count(DISTINCT session_id)  AS sessions,
		       min(created_at)             AS first_seen,
		       max(created_at)             AS last_seen
		FROM events WHERE created_at>=$1
		GROUP BY visitor_id
		ORDER BY last_seen DESC
		LIMIT 100`, since)
	if err != nil {
		http.Error(w, "db", 500)
		return
	}
	defer rows.Close()
	type visitor struct {
		VisitorID string    `json:"visitor_id"`
		IP        string    `json:"ip"`
		Country   string    `json:"country"`
		Province  string    `json:"province"`
		City      string    `json:"city"`
		ISP       string    `json:"isp"`
		Device    string    `json:"device"`
		Browser   string    `json:"browser"`
		OS        string    `json:"os"`
		PV        int64     `json:"pv"`
		Sessions  int64     `json:"sessions"`
		FirstSeen time.Time `json:"first_seen"`
		LastSeen  time.Time `json:"last_seen"`
	}
	var out []visitor
	for rows.Next() {
		var v visitor
		rows.Scan(&v.VisitorID, &v.IP, &v.Country, &v.Province, &v.City, &v.ISP,
			&v.Device, &v.Browser, &v.OS, &v.PV, &v.Sessions, &v.FirstSeen, &v.LastSeen)
		out = append(out, v)
	}
	writeJSON(w, out)
}

// VisitorJourney returns the ordered page-by-page trail for one visitor (?id=).
func (h *Handler) VisitorJourney(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	rows, err := h.pool.Query(ctx, `
		SELECT created_at, type, COALESCE(name,''), page, COALESCE(title,''),
		       COALESCE(referrer,''), COALESCE(duration_ms,0), COALESCE(scroll_pct,0),
		       session_id
		FROM events WHERE visitor_id=$1
		ORDER BY created_at ASC
		LIMIT 500`, id)
	if err != nil {
		http.Error(w, "db", 500)
		return
	}
	defer rows.Close()
	type step struct {
		Time       time.Time `json:"time"`
		Type       string    `json:"type"`
		Name       string    `json:"name"`
		Page       string    `json:"page"`
		Title      string    `json:"title"`
		Referrer   string    `json:"referrer"`
		DurationMs int64     `json:"duration_ms"`
		ScrollPct  int       `json:"scroll_pct"`
		SessionID  string    `json:"session_id"`
	}
	var out []step
	for rows.Next() {
		var s step
		rows.Scan(&s.Time, &s.Type, &s.Name, &s.Page, &s.Title, &s.Referrer,
			&s.DurationMs, &s.ScrollPct, &s.SessionID)
		out = append(out, s)
	}
	writeJSON(w, out)
}

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

// Package blog implements a minimal bilingual blog: a public read API and
// Basic-Auth-protected write endpoints (create / update / delete).
package blog

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Handler { return &Handler{pool: pool} }

type Post struct {
	ID        int64     `json:"id"`
	Slug      string    `json:"slug"`
	TitleZH   string    `json:"title_zh"`
	TitleEN   string    `json:"title_en"`
	BodyZH    string    `json:"body_zh"`
	BodyEN    string    `json:"body_en"`
	CoverURL  string    `json:"cover_url"`
	Published bool      `json:"published"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// List returns published posts (public). Newest first.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.pool.Query(ctx, `
		SELECT id, COALESCE(slug,''), title_zh, title_en, body_zh, body_en,
		       COALESCE(cover_url,''), published, created_at, updated_at
		FROM posts WHERE published=true
		ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		http.Error(w, "db", 500)
		return
	}
	defer rows.Close()
	writeJSON(w, scan(rows))
}

// ListAll returns every post including drafts (admin only).
func (h *Handler) ListAll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.pool.Query(ctx, `
		SELECT id, COALESCE(slug,''), title_zh, title_en, body_zh, body_en,
		       COALESCE(cover_url,''), published, created_at, updated_at
		FROM posts ORDER BY created_at DESC LIMIT 500`)
	if err != nil {
		http.Error(w, "db", 500)
		return
	}
	defer rows.Close()
	writeJSON(w, scan(rows))
}

func scan(rows interface {
	Next() bool
	Scan(...any) error
}) []Post {
	var out []Post
	for rows.Next() {
		var p Post
		rows.Scan(&p.ID, &p.Slug, &p.TitleZH, &p.TitleEN, &p.BodyZH, &p.BodyEN,
			&p.CoverURL, &p.Published, &p.CreatedAt, &p.UpdatedAt)
		out = append(out, p)
	}
	if out == nil {
		out = []Post{}
	}
	return out
}

// Create inserts a new post (admin). Returns the created row.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var p Post
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&p); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(p.TitleZH) == "" && strings.TrimSpace(p.TitleEN) == "" {
		http.Error(w, "title required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	err := h.pool.QueryRow(ctx, `
		INSERT INTO posts (slug, title_zh, title_en, body_zh, body_en, cover_url, published)
		VALUES (NULLIF($1,''),$2,$3,$4,$5,NULLIF($6,''),$7)
		RETURNING id, created_at, updated_at`,
		p.Slug, p.TitleZH, p.TitleEN, p.BodyZH, p.BodyEN, p.CoverURL, p.Published,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		http.Error(w, "db: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, p)
}

// Update edits an existing post by id (admin).
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	var p Post
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&p); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	ct, err := h.pool.Exec(ctx, `
		UPDATE posts SET slug=NULLIF($2,''), title_zh=$3, title_en=$4,
		       body_zh=$5, body_en=$6, cover_url=NULLIF($7,''), published=$8,
		       updated_at=now()
		WHERE id=$1`,
		id, p.Slug, p.TitleZH, p.TitleEN, p.BodyZH, p.BodyEN, p.CoverURL, p.Published)
	if err != nil {
		http.Error(w, "db", http.StatusInternalServerError)
		return
	}
	if ct.RowsAffected() == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Delete removes a post by id (admin).
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if _, err := h.pool.Exec(r.Context(), `DELETE FROM posts WHERE id=$1`, id); err != nil {
		http.Error(w, "db", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var _ = context.Background

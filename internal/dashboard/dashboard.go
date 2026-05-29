package dashboard

import (
	"crypto/subtle"
	"html/template"
	"net/http"
	"path/filepath"
)

type Handler struct {
	user, pass string
	tmpl       *template.Template
}

func New(user, pass, templatesDir string) (*Handler, error) {
	t, err := template.ParseFiles(filepath.Join(templatesDir, "dashboard.html"))
	if err != nil {
		return nil, err
	}
	return &Handler{user: user, pass: pass, tmpl: t}, nil
}

// BasicAuth protects the dashboard page and stats API.
func (h *Handler) BasicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(u), []byte(h.user)) != 1 ||
			subtle.ConstantTimeCompare([]byte(p), []byte(h.pass)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Blitzball Analytics"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) Page(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.tmpl.Execute(w, nil)
}

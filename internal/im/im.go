package im

import (
	"blitzball-analytics/internal/auth"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strconv"
	"strings"
)

type Handler struct {
	pool  *pgxpool.Pool
	users *auth.Handler
}

func New(pool *pgxpool.Pool, users *auth.Handler) *Handler { return &Handler{pool: pool, users: users} }
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (h *Handler) user(w http.ResponseWriter, r *http.Request) *auth.User {
	u := h.users.UserFromRequest(r)
	if u == nil {
		write(w, 401, map[string]string{"error": "NOT_LOGGED_IN"})
	}
	return u
}
func (h *Handler) Conversations(w http.ResponseWriter, r *http.Request) {
	u := h.user(w, r)
	if u == nil {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT c.id,c.kind,c.title,c.updated_at FROM im_conversations c JOIN im_members m ON m.conversation_id=c.id WHERE m.user_id=$1 ORDER BY c.updated_at DESC`, u.ID)
	if err != nil {
		write(w, 500, map[string]string{"error": "DB_ERROR"})
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, kind, title string
		var updated any
		if err := rows.Scan(&id, &kind, &title, &updated); err == nil {
			out = append(out, map[string]any{"id": id, "kind": kind, "title": title, "updatedAt": updated})
		}
	}
	write(w, 200, out)
}
func (h *Handler) Messages(w http.ResponseWriter, r *http.Request) {
	u := h.user(w, r)
	if u == nil {
		return
	}
	id := r.PathValue("id")
	if len(id) < 20 {
		write(w, 400, map[string]string{"error": "BAD_CONVERSATION"})
		return
	}
	var member bool
	_ = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM im_members WHERE conversation_id=$1 AND user_id=$2)`, id, u.ID).Scan(&member)
	if !member {
		write(w, 403, map[string]string{"error": "FORBIDDEN"})
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	limit := 50
	if n, _ := strconv.Atoi(r.URL.Query().Get("limit")); n > 0 && n <= 100 {
		limit = n
	}
	rows, err := h.pool.Query(r.Context(), `SELECT cursor,id,sender_id,kind,body,created_at FROM im_messages WHERE conversation_id=$1 AND cursor>$2 AND deleted_at IS NULL ORDER BY cursor LIMIT $3`, id, after, limit)
	if err != nil {
		write(w, 500, map[string]string{"error": "DB_ERROR"})
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var cursor int64
		var mid string
		var sender int64
		var kind, body string
		var created any
		if rows.Scan(&cursor, &mid, &sender, &kind, &body, &created) == nil {
			out = append(out, map[string]any{"cursor": cursor, "id": mid, "senderId": sender, "kind": kind, "body": body, "createdAt": created})
		}
	}
	write(w, 200, out)
}

type sendRequest struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Body    string `json:"body"`
	ReplyTo string `json:"replyTo"`
}

func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	u := h.user(w, r)
	if u == nil {
		return
	}
	cid := r.PathValue("id")
	if len(cid) < 20 {
		write(w, 400, map[string]string{"error": "BAD_CONVERSATION"})
		return
	}
	var member bool
	_ = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM im_members WHERE conversation_id=$1 AND user_id=$2)`, cid, u.ID).Scan(&member)
	if !member {
		write(w, 403, map[string]string{"error": "FORBIDDEN"})
		return
	}
	var req sendRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil || strings.TrimSpace(req.Body) == "" {
		write(w, 400, map[string]string{"error": "BAD_MESSAGE"})
		return
	}
	if len(req.ID) < 20 {
		write(w, 400, map[string]string{"error": "BAD_MESSAGE_ID"})
		return
	}
	kind := req.Kind
	if kind == "" {
		kind = "text"
	}
	var cursor int64
	var created any
	err := h.pool.QueryRow(r.Context(), `INSERT INTO im_messages(conversation_id,id,sender_id,kind,body,idempotency_key) VALUES($1,$2,$3,$4,$5,$2) ON CONFLICT(sender_id,idempotency_key) DO UPDATE SET body=im_messages.body RETURNING cursor,created_at`, cid, req.ID, u.ID, kind, req.Body).Scan(&cursor, &created)
	if err != nil {
		if err == pgx.ErrNoRows {
			write(w, 409, map[string]string{"error": "MESSAGE_CONFLICT"})
		} else {
			write(w, 500, map[string]string{"error": "DB_ERROR"})
		}
		return
	}
	write(w, 201, map[string]any{"cursor": cursor, "id": req.ID, "senderId": u.ID, "kind": kind, "body": req.Body, "createdAt": created})
}
func (h *Handler) Friends(w http.ResponseWriter, r *http.Request) {
	u := h.user(w, r)
	if u == nil {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT CASE WHEN requester_id=$1 THEN addressee_id ELSE requester_id END,state,updated_at FROM im_friendships WHERE requester_id=$1 OR addressee_id=$1 ORDER BY updated_at DESC`, u.ID)
	if err != nil {
		write(w, 500, map[string]string{"error": "DB_ERROR"})
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var state string
		var updated any
		if rows.Scan(&id, &state, &updated) == nil {
			out = append(out, map[string]any{"userId": id, "state": state, "updatedAt": updated})
		}
	}
	write(w, 200, out)
}

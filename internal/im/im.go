package im

import (
	"blitzball-analytics/internal/auth"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Stream provides a dependency-free realtime transport for clients that cannot
// open WebSocket connections. It uses the same cursor contract as pagination:
// reconnecting with ?after=N never duplicates messages.
func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	u := h.user(w, r)
	if u == nil {
		return
	}
	cid := r.PathValue("id")
	var member bool
	_ = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM im_members WHERE conversation_id=$1 AND user_id=$2)`, cid, u.ID).Scan(&member)
	if !member {
		write(w, http.StatusForbidden, map[string]string{"error": "FORBIDDEN"})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		write(w, http.StatusNotImplemented, map[string]string{"error": "STREAM_UNSUPPORTED"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	for {
		rows, err := h.pool.Query(r.Context(), `SELECT cursor,id,sender_id,kind,body,created_at FROM im_messages WHERE conversation_id=$1 AND cursor>$2 AND deleted_at IS NULL ORDER BY cursor LIMIT 100`, cid, after)
		if err != nil {
			return
		}
		for rows.Next() {
			var cursor int64
			var id string
			var sender int64
			var kind, body string
			var created any
			if rows.Scan(&cursor, &id, &sender, &kind, &body, &created) == nil {
				payload, _ := json.Marshal(map[string]any{"cursor": cursor, "id": id, "senderId": sender, "kind": kind, "body": body, "createdAt": created})
				_, _ = w.Write([]byte("id: " + strconv.FormatInt(cursor, 10) + "\ndata: " + string(payload) + "\n\n"))
				after = cursor
			}
		}
		rows.Close()
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

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

type relationRequest struct {
	UserID int64 `json:"userId"`
}

func (h *Handler) FriendRequest(w http.ResponseWriter, r *http.Request) {
	u := h.user(w, r)
	if u == nil {
		return
	}
	var q relationRequest
	if json.NewDecoder(r.Body).Decode(&q) != nil || q.UserID <= 0 || q.UserID == u.ID {
		write(w, 400, map[string]string{"error": "BAD_USER"})
		return
	}
	_, err := h.pool.Exec(r.Context(), `INSERT INTO im_friendships(requester_id,addressee_id,state) VALUES($1,$2,'pending') ON CONFLICT(requester_id,addressee_id) DO UPDATE SET state='pending',updated_at=now()`, u.ID, q.UserID)
	if err != nil {
		write(w, 409, map[string]string{"error": "RELATION_CONFLICT"})
		return
	}
	write(w, 201, map[string]any{"ok": true, "state": "pending"})
}
func (h *Handler) FriendRespond(w http.ResponseWriter, r *http.Request) {
	u := h.user(w, r)
	if u == nil {
		return
	}
	var q struct {
		UserID int64  `json:"userId"`
		State  string `json:"state"`
	}
	if json.NewDecoder(r.Body).Decode(&q) != nil || q.UserID <= 0 || (q.State != "accepted" && q.State != "rejected" && q.State != "blocked") {
		write(w, 400, map[string]string{"error": "BAD_RELATION"})
		return
	}
	var command string
	if q.State == "blocked" {
		command = `UPDATE im_friendships SET state='blocked',updated_at=now() WHERE requester_id=$1 AND addressee_id=$2`
	} else {
		command = `UPDATE im_friendships SET state=$3,updated_at=now() WHERE requester_id=$1 AND addressee_id=$2 AND state='pending'`
	}
	var err error
	if q.State == "blocked" {
		_, err = h.pool.Exec(r.Context(), command, u.ID, q.UserID)
	} else {
		_, err = h.pool.Exec(r.Context(), command, q.UserID, u.ID, q.State)
	}
	if err != nil {
		write(w, 500, map[string]string{"error": "DB_ERROR"})
		return
	}
	write(w, 200, map[string]any{"ok": true, "state": q.State})
}
func (h *Handler) CreateConversation(w http.ResponseWriter, r *http.Request) {
	u := h.user(w, r)
	if u == nil {
		return
	}
	var q struct {
		Kind    string  `json:"kind"`
		Title   string  `json:"title"`
		Members []int64 `json:"members"`
	}
	if json.NewDecoder(r.Body).Decode(&q) != nil || (q.Kind != "direct" && q.Kind != "group") {
		write(w, 400, map[string]string{"error": "BAD_CONVERSATION"})
		return
	}
	var id string
	if err := h.pool.QueryRow(r.Context(), `INSERT INTO im_conversations(id,kind,title,created_by) VALUES(gen_random_uuid(),$1,$2,$3) RETURNING id`, q.Kind, q.Title, u.ID).Scan(&id); err != nil {
		write(w, 500, map[string]string{"error": "DB_ERROR"})
		return
	}
	if _, err := h.pool.Exec(r.Context(), `INSERT INTO im_members(conversation_id,user_id,role) VALUES($1,$2,'owner')`, id, u.ID); err != nil {
		write(w, 500, map[string]string{"error": "DB_ERROR"})
		return
	}
	for _, member := range q.Members {
		if member > 0 && member != u.ID {
			_, _ = h.pool.Exec(r.Context(), `INSERT INTO im_members(conversation_id,user_id) SELECT $1,$2 WHERE EXISTS(SELECT 1 FROM im_friendships WHERE ((requester_id=$3 AND addressee_id=$2) OR (requester_id=$2 AND addressee_id=$3)) AND state='accepted' ) ON CONFLICT DO NOTHING`, id, member, u.ID)
		}
	}
	write(w, 201, map[string]any{"id": id, "kind": q.Kind, "title": q.Title})
}

func (h *Handler) GrantAgent(w http.ResponseWriter, r *http.Request) {
	u := h.user(w, r)
	if u == nil {
		return
	}
	cid := r.PathValue("id")
	var member bool
	_ = h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM im_members WHERE conversation_id=$1 AND user_id=$2)`, cid, u.ID).Scan(&member)
	if !member {
		write(w, 403, map[string]string{"error": "FORBIDDEN"})
		return
	}
	var q struct {
		Scopes         []string `json:"scopes"`
		ExpiresMinutes int      `json:"expiresMinutes"`
	}
	if json.NewDecoder(r.Body).Decode(&q) != nil || len(q.Scopes) == 0 {
		write(w, 400, map[string]string{"error": "BAD_SCOPES"})
		return
	}
	if q.ExpiresMinutes <= 0 || q.ExpiresMinutes > 10080 {
		q.ExpiresMinutes = 60
	}
	var id string
	err := h.pool.QueryRow(r.Context(), `INSERT INTO im_agent_grants(id,conversation_id,granted_by,scopes,expires_at) VALUES(gen_random_uuid(),$1,$2,$3::jsonb,now()+($4||' minutes')::interval) RETURNING id`, cid, u.ID, mustJSON(q.Scopes), q.ExpiresMinutes).Scan(&id)
	if err != nil {
		write(w, 500, map[string]string{"error": "DB_ERROR"})
		return
	}
	write(w, 201, map[string]any{"id": id, "conversationId": cid, "scopes": q.Scopes, "expiresMinutes": q.ExpiresMinutes})
}
func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func (h *Handler) RevokeAgent(w http.ResponseWriter, r *http.Request) {
	u := h.user(w, r)
	if u == nil {
		return
	}
	id := r.PathValue("id")
	ct, err := h.pool.Exec(r.Context(), `UPDATE im_agent_grants SET revoked_at=now() WHERE id=$1 AND granted_by=$2 AND revoked_at IS NULL`, id, u.ID)
	if err != nil {
		write(w, 500, map[string]string{"error": "DB_ERROR"})
		return
	}
	if ct.RowsAffected() == 0 {
		write(w, 404, map[string]string{"error": "GRANT_NOT_FOUND"})
		return
	}
	write(w, 200, map[string]any{"ok": true})
}
func (h *Handler) Grants(w http.ResponseWriter, r *http.Request) {
	u := h.user(w, r)
	if u == nil {
		return
	}
	cid := r.PathValue("id")
	rows, err := h.pool.Query(r.Context(), `SELECT id,scopes,expires_at,revoked_at FROM im_agent_grants WHERE conversation_id=$1 AND granted_by=$2 ORDER BY created_at DESC`, cid, u.ID)
	if err != nil {
		write(w, 500, map[string]string{"error": "DB_ERROR"})
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id string
		var scopes any
		var expires, revoked any
		if rows.Scan(&id, &scopes, &expires, &revoked) == nil {
			out = append(out, map[string]any{"id": id, "scopes": scopes, "expiresAt": expires, "revokedAt": revoked})
		}
	}
	write(w, 200, out)
}

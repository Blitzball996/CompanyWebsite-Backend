// Creem webhook. Creem signs each request with:
//
//	creem-signature = hex( HMAC-SHA256(secret, rawBody) )
//
// Set the same secret in the Creem dashboard and in CREEM_WEBHOOK_SECRET.
//
// We map the paid Creem product id to one of our license prefixes, issue a key
// idempotently for the order, and email it to the buyer. Product ids are
// configured via env so adding a product doesn't need a rebuild:
//
//	CREEM_PRODUCT_MAP=prod_7MP9FYacUK2RwtfD1LI3oe:BDPR,prod_xxx:CCPR
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// creemEvent covers the fields we use from checkout.completed / subscription paid.
type creemEvent struct {
	ID     string `json:"id"`
	Eventy string `json:"eventType"`
	Object struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		OrderID  string `json:"order_id"`
		Customer struct {
			Email string `json:"email"`
		} `json:"customer"`
		Order struct {
			ID       string      `json:"id"`
			Amount   json.Number `json:"amount"`
			Currency string      `json:"currency"`
			Status   string      `json:"status"`
		} `json:"order"`
		Product struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"product"`
		Metadata map[string]interface{} `json:"metadata"`
	} `json:"object"`
}

// creemProductMap parses CREEM_PRODUCT_MAP into {creem product id -> prefix}.
func creemProductMap() map[string]string {
	m := map[string]string{
		// Blitz DAW — $150 lifetime (default, override via env).
		"prod_7MP9FYacUK2RwtfD1LI3oe": "BDPR",
	}
	for _, pair := range strings.Split(os.Getenv("CREEM_PRODUCT_MAP"), ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		if i := strings.LastIndex(pair, ":"); i > 0 {
			id := strings.TrimSpace(pair[:i])
			prefix := strings.ToUpper(strings.TrimSpace(pair[i+1:]))
			if id != "" && prefix != "" {
				m[id] = prefix
			}
		}
	}
	return m
}

// Creem handles POST /api/webhook/creem.
func (h *Handler) Creem(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read", http.StatusBadRequest)
		return
	}

	secret := os.Getenv("CREEM_WEBHOOK_SECRET")
	if secret == "" {
		secret = h.secret
	}
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		want := hex.EncodeToString(mac.Sum(nil))
		got := strings.ToLower(strings.TrimSpace(r.Header.Get("creem-signature")))
		if got == "" {
			got = strings.ToLower(strings.TrimSpace(r.Header.Get("x-creem-signature")))
		}
		if !hmac.Equal([]byte(got), []byte(want)) {
			log.Printf("webhook/creem: bad signature")
			http.Error(w, "bad signature", http.StatusUnauthorized)
			return
		}
	}

	var ev creemEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	if !creemPaid(ev) {
		w.WriteHeader(http.StatusOK)
		return
	}

	email := strings.TrimSpace(ev.Object.Customer.Email)
	if email == "" {
		email = metaStr(ev.Object.Metadata, "email")
	}

	product := creemProductMap()[ev.Object.Product.ID]
	if product == "" {
		// fall back to explicit metadata, e.g. metadata.product = "closecrab-pro"
		product = h.resolveProduct(metaStr(ev.Object.Metadata, "product"))
	}
	if product == "" {
		log.Printf("webhook/creem: paid event %s but product %q is not mapped — skipping issue",
			ev.ID, ev.Object.Product.ID)
		w.WriteHeader(http.StatusOK)
		return
	}

	orderID := firstNonEmpty(ev.Object.Order.ID, ev.Object.OrderID, ev.Object.ID, ev.ID)

	key, created, err := h.lic.IssueForOrder(product, orderID, email)
	if err != nil {
		log.Printf("webhook/creem: issue failed for order %s: %v", orderID, err)
		http.Error(w, "issue failed", http.StatusInternalServerError)
		return
	}

	cur := ev.Object.Order.Currency
	if cur == "" {
		cur = "USD"
	}
	h.lic.RecordReceipt(orderID, email, product, h.lic.EditionOf(product),
		ev.Object.Order.Amount.String(), cur, key)

	if created && email != "" && h.mail.Enabled() {
		if err := h.mail.Send(email, mailSubject(product), mailBody(product, key), true); err != nil {
			log.Printf("webhook/creem: key %s issued but email to %s failed: %v", key, email, err)
		}
	}
	log.Printf("webhook/creem: order %s → key %s (created=%v, emailed=%v)",
		orderID, key, created, email != "")
	w.WriteHeader(http.StatusOK)
}

func creemPaid(ev creemEvent) bool {
	t := strings.ToLower(ev.Eventy)
	if strings.Contains(t, "refund") || strings.Contains(t, "dispute") ||
		strings.Contains(t, "expired") || strings.Contains(t, "canceled") {
		return false
	}
	if strings.Contains(t, "checkout.completed") || strings.Contains(t, "payment") ||
		strings.Contains(t, "subscription.paid") || strings.Contains(t, "subscription.active") {
		// Trust the order/status field when present.
		for _, s := range []string{ev.Object.Order.Status, ev.Object.Status} {
			s = strings.ToLower(s)
			if s == "paid" || s == "completed" || s == "active" || s == "succeeded" {
				return true
			}
		}
		// checkout.completed with no explicit status is still a success signal.
		return strings.Contains(t, "checkout.completed")
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

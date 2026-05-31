// Package webhook handles payment-provider callbacks (Airwallex) that fire on a
// successful payment: it verifies the signature, issues a license key for the
// order (idempotently), and emails it to the buyer.
//
// Airwallex signs each request:  x-signature = hex( HMAC-SHA256(secret, x-timestamp + rawBody) )
// Set the same secret in the Airwallex webhook config and in WEBHOOK_SECRET.
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"blitzball-analytics/internal/license"
	"blitzball-analytics/internal/mailer"
)

type Handler struct {
	secret string
	lic    *license.Handler
	mail   *mailer.Mailer
	// productMap maps an Airwallex metadata "product" value to our prefix.
	// e.g. "blitz-pro" -> "BDPR". Falls back to the raw value if it is already a prefix.
	productMap map[string]string
}

func New(secret string, lic *license.Handler, mail *mailer.Mailer) *Handler {
	return &Handler{
		secret: secret, lic: lic, mail: mail,
		productMap: map[string]string{
			"blitz-pro": "BDPR", "blitz-standard": "BDST", "blitz": "BDPR",
			"closecrab-pro": "CCPR", "closecrab-standard": "CCST", "closecrab": "CCPR",
		},
	}
}

// airwallex event envelope (only the fields we use).
type event struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Data struct {
		Object struct {
			ID       string                 `json:"id"`
			Status   string                 `json:"status"`
			Amount   json.Number            `json:"amount"`
			Currency string                 `json:"currency"`
			Metadata map[string]interface{} `json:"metadata"`
			// some payloads carry the order id / customer email at top level too
			MerchantOrderID string `json:"merchant_order_id"`
		} `json:"object"`
	} `json:"data"`
}

func (h *Handler) resolveProduct(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if p, ok := h.productMap[v]; ok {
		return p
	}
	up := strings.ToUpper(v)
	if _, ok := license.ValidProducts[up]; ok {
		return up
	}
	return ""
}

func metaStr(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Airwallex POSTs here. We always return 200 for events we don't act on, so the
// provider doesn't keep retrying; real failures (signature) return 4xx.
func (h *Handler) Airwallex(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read", http.StatusBadRequest)
		return
	}

	// --- verify signature ---
	if h.secret != "" {
		ts := r.Header.Get("x-timestamp")
		sig := r.Header.Get("x-signature")
		mac := hmac.New(sha256.New, []byte(h.secret))
		mac.Write([]byte(ts))
		mac.Write(body)
		want := hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(strings.ToLower(sig)), []byte(want)) {
			http.Error(w, "bad signature", http.StatusUnauthorized)
			return
		}
	}

	var ev event
	if err := json.Unmarshal(body, &ev); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	// only act on successful payment events
	if !isSuccess(ev.Name) {
		w.WriteHeader(http.StatusOK)
		return
	}

	obj := ev.Data.Object
	email := metaStr(obj.Metadata, "email")
	productRaw := metaStr(obj.Metadata, "product")
	orderID := obj.MerchantOrderID
	if orderID == "" {
		orderID = obj.ID
	}
	product := h.resolveProduct(productRaw)
	if product == "" {
		log.Printf("webhook: paid event %s but no/unknown product metadata (%q) — skipping issue", ev.ID, productRaw)
		w.WriteHeader(http.StatusOK)
		return
	}

	key, created, err := h.lic.IssueForOrder(product, orderID, email)
	if err != nil {
		log.Printf("webhook: issue failed for order %s: %v", orderID, err)
		http.Error(w, "issue failed", http.StatusInternalServerError)
		return
	}
	// store a receipt (idempotent on order_id) for the customer portal
	cur := obj.Currency
	if cur == "" {
		cur = "CNY"
	}
	h.lic.RecordReceipt(orderID, email, product, h.lic.EditionOf(product), obj.Amount.String(), cur, key)
	if created && email != "" && h.mail.Enabled() {
		if err := h.mail.Send(email, mailSubject(product), mailBody(product, key), true); err != nil {
			log.Printf("webhook: key %s issued but email to %s failed: %v", key, email, err)
		}
	}
	log.Printf("webhook: order %s → key %s (created=%v, emailed=%v)", orderID, key, created, email != "")
	w.WriteHeader(http.StatusOK)
}

func isSuccess(name string) bool {
	name = strings.ToLower(name)
	return strings.Contains(name, "succeeded") ||
		strings.Contains(name, "captured") ||
		strings.Contains(name, "paid")
}

func mailSubject(product string) string {
	if strings.HasPrefix(product, "BD") {
		return "你的 Blitz DAW 序列号 / Your Blitz DAW license"
	}
	return "你的 CloseCrab 序列号 / Your CloseCrab license"
}

func mailBody(product, key string) string {
	name := "Blitz DAW"
	if strings.HasPrefix(product, "CC") {
		name = "CloseCrab"
	}
	return `<div style="font-family:system-ui,sans-serif;max-width:560px;margin:0 auto">
  <h2 style="color:#6d28d9">感谢购买 ` + name + `!</h2>
  <p>你的序列号 / Your license key:</p>
  <p style="font:18px monospace;background:#f4f0ff;padding:14px 18px;border-radius:10px;letter-spacing:1px">` + key + `</p>
  <p>在软件里输入此序列号即可联网激活。<b>一码一机</b>，请妥善保管。<br>
     Enter this key in the app to activate online. <b>One key = one device.</b></p>
  <hr style="border:none;border-top:1px solid #eee">
  <p style="color:#888;font-size:13px">Blitzball Labs · 如有问题请回复此邮件 / reply for support</p>
</div>`
}

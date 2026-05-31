package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// MyLicenses lists all license keys bought under the logged-in user's email.
func (h *Handler) MyLicenses(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if u == nil {
		fail(w, http.StatusUnauthorized, "NOT_LOGGED_IN")
		return
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT key, product, edition, status,
		       COALESCE(device_id,''), activated_at, COALESCE(order_id,''), created_at
		FROM licenses WHERE lower(email)=lower($1)
		ORDER BY created_at DESC`, u.Email)
	if err != nil {
		fail(w, http.StatusInternalServerError, "DB_ERROR")
		return
	}
	defer rows.Close()
	var out []map[string]interface{}
	for rows.Next() {
		var (
			key, product, edition, status, device, orderID string
			activatedAt                                     *time.Time
			createdAt                                       time.Time
		)
		rows.Scan(&key, &product, &edition, &status, &device, &activatedAt, &orderID, &createdAt)
		out = append(out, map[string]interface{}{
			"key": key, "product": product, "edition": edition, "status": status,
			"activated": device != "", "activated_at": activatedAt,
			"order_id": orderID, "created_at": createdAt,
		})
	}
	if out == nil {
		out = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "email": u.Email, "licenses": out})
}

// Receipt renders an HTML receipt for one of the user's orders (printable → PDF).
func (h *Handler) Receipt(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	if u == nil {
		fail(w, http.StatusUnauthorized, "NOT_LOGGED_IN")
		return
	}
	orderID := chi.URLParam(r, "order_id")
	var (
		email, product, edition, amount, currency, key string
		createdAt                                       time.Time
	)
	err := h.pool.QueryRow(r.Context(), `
		SELECT COALESCE(email,''), product, edition, amount, currency,
		       COALESCE(license_key,''), created_at
		FROM receipts WHERE order_id=$1`, orderID,
	).Scan(&email, &product, &edition, &amount, &currency, &key, &createdAt)
	if err == pgx.ErrNoRows {
		// fall back to license row if no explicit receipt was stored
		err = h.pool.QueryRow(r.Context(), `
			SELECT COALESCE(email,''), product, edition, key, created_at
			FROM licenses WHERE order_id=$1`, orderID,
		).Scan(&email, &product, &edition, &key, &createdAt)
		currency = "CNY"
	}
	if err != nil {
		fail(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	// authorization: the receipt's email must match the logged-in user
	if !strings.EqualFold(strings.TrimSpace(email), strings.TrimSpace(u.Email)) {
		fail(w, http.StatusForbidden, "NOT_YOURS")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(receiptHTML(orderID, email, product, edition, amount, currency, key, createdAt)))
}

func prodName(p string) string {
	switch {
	case strings.HasPrefix(p, "BD"):
		return "Blitz DAW"
	case strings.HasPrefix(p, "CC"):
		return "CloseCrab"
	}
	return p
}

func receiptHTML(orderID, email, product, edition, amount, currency, key string, t time.Time) string {
	amt := amount
	if amt == "" {
		amt = "—"
	}
	return `<!doctype html><html><head><meta charset="utf-8"><title>Receipt ` + orderID + `</title>
<style>
 body{font-family:system-ui,"Microsoft YaHei",sans-serif;color:#1a1a1a;max-width:680px;margin:30px auto;padding:0 20px}
 .head{display:flex;justify-content:space-between;align-items:center;border-bottom:2px solid #6d28d9;padding-bottom:16px}
 .brand{font-size:22px;font-weight:800;color:#6d28d9}
 h1{font-size:18px;margin:24px 0 4px}
 table{width:100%;border-collapse:collapse;margin:18px 0}
 td,th{text-align:left;padding:10px 8px;border-bottom:1px solid #eee;font-size:14px}
 th{color:#777;font-weight:600;width:40%}
 .key{font-family:monospace;background:#f4f0ff;padding:3px 8px;border-radius:6px}
 .tot{font-size:20px;font-weight:800}
 .btn{display:inline-block;margin-top:20px;background:#6d28d9;color:#fff;padding:10px 20px;border-radius:8px;text-decoration:none;border:none;cursor:pointer}
 @media print{.btn{display:none}}
</style></head><body>
 <div class="head"><div class="brand">Blitzball Labs</div><div>收据 / Receipt</div></div>
 <h1>订单 / Order #` + orderID + `</h1>
 <table>
  <tr><th>产品 / Product</th><td>` + prodName(product) + ` · ` + edition + `</td></tr>
  <tr><th>序列号 / License key</th><td><span class="key">` + key + `</span></td></tr>
  <tr><th>邮箱 / Email</th><td>` + email + `</td></tr>
  <tr><th>日期 / Date</th><td>` + t.Format("2006-01-02 15:04") + `</td></tr>
  <tr><th>金额 / Amount</th><td class="tot">` + amt + ` ` + currency + `</td></tr>
 </table>
 <p style="color:#888;font-size:12px">感谢购买！此为订单收据凭证，非增值税发票。如需发票请联系我们。<br>
 Thank you. This is an order receipt, not a VAT invoice. Contact us if you need a fapiao.</p>
 <button class="btn" onclick="window.print()">打印 / 存为 PDF · Print / Save as PDF</button>
</body></html>`
}

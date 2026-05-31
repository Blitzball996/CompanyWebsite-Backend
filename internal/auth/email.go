package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"log"
)

func jsonDecode(r *http.Request, v interface{}) error {
	return json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(v)
}

// sendVerify emails an account-verification link (or logs it if SMTP is off).
func (h *Handler) sendVerify(email, tok string) {
	link := h.baseURL + "/api/auth/verify?token=" + tok
	if !h.mail.Enabled() {
		log.Printf("auth: (no SMTP) verify link for %s: %s", email, link)
		return
	}
	body := `<div style="font-family:system-ui,sans-serif;max-width:520px;margin:0 auto">
  <h2 style="color:#6d28d9">验证你的邮箱 / Verify your email</h2>
  <p>点击下面的按钮完成 Blitzball Labs 账号注册。<br>Click to finish creating your Blitzball Labs account.</p>
  <p><a href="` + link + `" style="display:inline-block;background:#6d28d9;color:#fff;padding:12px 22px;border-radius:10px;text-decoration:none">验证邮箱 / Verify email</a></p>
  <p style="color:#888;font-size:12px">若按钮无效，复制此链接：<br>` + link + `</p></div>`
	if err := h.mail.Send(email, "验证你的邮箱 / Verify your email — Blitzball Labs", body, true); err != nil {
		log.Printf("auth: verify email to %s failed: %v", email, err)
	}
}

// sendReset emails a password-reset link.
func (h *Handler) sendReset(email, tok string) {
	link := h.baseURL + "/reset.html?token=" + tok
	if !h.mail.Enabled() {
		log.Printf("auth: (no SMTP) reset link for %s: %s", email, link)
		return
	}
	body := `<div style="font-family:system-ui,sans-serif;max-width:520px;margin:0 auto">
  <h2 style="color:#6d28d9">重置密码 / Reset your password</h2>
  <p>我们收到了重置密码的请求。链接 1 小时内有效。<br>We received a password-reset request. This link expires in 1 hour.</p>
  <p><a href="` + link + `" style="display:inline-block;background:#6d28d9;color:#fff;padding:12px 22px;border-radius:10px;text-decoration:none">重置密码 / Reset password</a></p>
  <p style="color:#888;font-size:12px">若不是你本人操作，忽略此邮件即可。If this wasn't you, ignore this email.<br>` + link + `</p></div>`
	if err := h.mail.Send(email, "重置密码 / Reset password — Blitzball Labs", body, true); err != nil {
		log.Printf("auth: reset email to %s failed: %v", email, err)
	}
}

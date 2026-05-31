// Package mailer sends transactional email (license delivery) over SMTP,
// using only the standard library. Supports implicit TLS (port 465) and
// STARTTLS (587/25). If not configured, Send is a no-op that returns an error
// so callers can log and still persist the issued key.
package mailer

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type Mailer struct {
	host, port, user, pass, from string
	enabled                      bool
}

// New builds a Mailer; it's "enabled" only when host/from are set.
func New(host, port, user, pass, from string) *Mailer {
	if port == "" {
		port = "587"
	}
	return &Mailer{
		host: host, port: port, user: user, pass: pass, from: from,
		enabled: host != "" && from != "",
	}
}

func (m *Mailer) Enabled() bool { return m.enabled }

// Send delivers a UTF-8 email. body may be plain text or HTML (set html=true).
func (m *Mailer) Send(to, subject, body string, html bool) error {
	if !m.enabled {
		return errors.New("mailer: not configured (set SMTP_HOST/SMTP_FROM)")
	}
	ctype := "text/plain; charset=UTF-8"
	if html {
		ctype = "text/html; charset=UTF-8"
	}
	msg := strings.Join([]string{
		"From: " + m.from,
		"To: " + to,
		"Subject: " + encodeHeader(subject),
		"MIME-Version: 1.0",
		"Content-Type: " + ctype,
		"Date: " + time.Now().Format(time.RFC1123Z),
		"",
		body,
	}, "\r\n")

	addr := net.JoinHostPort(m.host, m.port)
	var auth smtp.Auth
	if m.user != "" {
		auth = smtp.PlainAuth("", m.user, m.pass, m.host)
	}

	if m.port == "465" { // implicit TLS
		return m.sendImplicitTLS(addr, auth, to, []byte(msg))
	}
	// STARTTLS path (587/25)
	c, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer c.Close()
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: m.host}); err != nil {
			return err
		}
	}
	if auth != nil {
		if ok, _ := c.Extension("AUTH"); ok {
			if err := c.Auth(auth); err != nil {
				return err
			}
		}
	}
	return finish(c, m.from, to, []byte(msg))
}

func (m *Mailer) sendImplicitTLS(addr string, auth smtp.Auth, to string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: m.host})
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return err
	}
	defer c.Close()
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	return finish(c, m.from, to, msg)
}

func finish(c *smtp.Client, from, to string, msg []byte) error {
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// encodeHeader RFC2047-encodes a subject if it contains non-ASCII (Chinese).
func encodeHeader(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return mimeWord(s)
		}
	}
	return s
}

func mimeWord(s string) string {
	// =?UTF-8?B?...?= base64 word
	return fmt.Sprintf("=?UTF-8?B?%s?=", b64(s))
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

package auth

import (
	"blitzball-analytics/internal/mailer"
	"testing"
)

func TestVerificationRejectsUnconfiguredMailer(t *testing.T) {
	h := &Handler{mail: mailer.New("", "", "", "", ""), baseURL: "https://example.invalid"}
	if err := h.sendVerify("test@example.invalid", "test-token"); err == nil {
		t.Fatal("unconfigured delivery must not report success")
	}
}

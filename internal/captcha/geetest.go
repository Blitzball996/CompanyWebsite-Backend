// Package captcha integrates GeeTest v4 server-side ("secondary") validation.
// Flow: the browser solves the captcha and sends {lot_number, captcha_output,
// pass_token, gen_time}; we POST them plus sign_token = HMAC-SHA256(key, lot_number)
// to GeeTest's validate endpoint and require result=="success".
//
// Policy: if not configured (no id/key) verification is skipped (Enabled()=false).
// If GeeTest is unreachable we FAIL-OPEN (let the user through) per GeeTest's own
// recommendation, so an outage never locks out real users; an explicit "fail"
// result is rejected.
package captcha

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const validateURL = "https://gcaptcha4.geetest.com/validate"

type GeeTest struct {
	id, key string
	client  *http.Client
	enabled bool
}

func New(id, key string) *GeeTest {
	return &GeeTest{
		id: id, key: key,
		client:  &http.Client{Timeout: 5 * time.Second},
		enabled: id != "" && key != "",
	}
}

func (g *GeeTest) Enabled() bool { return g.enabled }

// Params is the client-side GeeTest v4 output the browser submits.
type Params struct {
	LotNumber     string `json:"lot_number"`
	CaptchaOutput string `json:"captcha_output"`
	PassToken     string `json:"pass_token"`
	GenTime       string `json:"gen_time"`
}

// Verify returns true if the captcha is valid (or if GeeTest is unreachable —
// fail-open). Returns false only on an explicit "fail" or clearly missing input.
func (g *GeeTest) Verify(p Params) bool {
	if !g.enabled {
		return true // not configured → don't block
	}
	if p.LotNumber == "" || p.CaptchaOutput == "" || p.PassToken == "" || p.GenTime == "" {
		return false // captcha enabled but client sent nothing → reject
	}
	mac := hmac.New(sha256.New, []byte(g.key))
	mac.Write([]byte(p.LotNumber))
	signToken := hex.EncodeToString(mac.Sum(nil))

	form := url.Values{
		"lot_number":     {p.LotNumber},
		"captcha_output": {p.CaptchaOutput},
		"pass_token":     {p.PassToken},
		"gen_time":       {p.GenTime},
		"sign_token":     {signToken},
	}
	resp, err := g.client.PostForm(validateURL+"?captcha_id="+url.QueryEscape(g.id), form)
	if err != nil {
		log.Printf("captcha: GeeTest unreachable (%v) — failing open", err)
		return true // fail-open
	}
	defer resp.Body.Close()
	var out struct {
		Result string `json:"result"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		log.Printf("captcha: bad GeeTest response (%v) — failing open", err)
		return true // fail-open
	}
	if strings.EqualFold(out.Result, "success") {
		return true
	}
	log.Printf("captcha: GeeTest rejected (result=%s reason=%s)", out.Result, out.Reason)
	return false
}

// ID exposes the public captcha_id for the frontend widget.
func (g *GeeTest) ID() string { return g.id }

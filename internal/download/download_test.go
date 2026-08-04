package download

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNormKey(t *testing.T) {
	want := "BDPR-7K3P-9WXM-2QH4-RT8C"
	for _, in := range []string{
		"BDPR-7K3P-9WXM-2QH4-RT8C",
		"bdpr7k3p9wxm2qh4rt8c",
		"  bdpr 7k3p-9wxm_2qh4 rt8c ",
	} {
		if got := normKey(in); got != want {
			t.Errorf("normKey(%q) = %q, want %q", in, got, want)
		}
	}
	if got := normKey("short"); got == want {
		t.Error("short key should not normalise to a valid key")
	}
}

func TestFamily(t *testing.T) {
	cases := map[string]string{"BDPR": "BD", "bdst": "BD", "CCPR": "CC", "": "", "C": ""}
	for in, want := range cases {
		if got := family(in); got != want {
			t.Errorf("family(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSignatureRejectsTamperedLink(t *testing.T) {
	h := New(nil, "test-secret")
	key, product, platform := "BDPR-7K3P-9WXM-2QH4-RT8C", "BDPR", "windows"
	exp := time.Now().Add(linkTTL).Unix()
	sig := h.sign(key, product, platform, exp)

	// A valid link resolves to a redirect.
	rec := httptest.NewRecorder()
	q := "key=" + key + "&product=" + product + "&platform=" + platform +
		"&exp=" + strconv.FormatInt(exp, 10) + "&sig=" + sig
	h.File(rec, httptest.NewRequest("GET", "/api/download/file?"+q, nil))
	if rec.Code != 302 {
		t.Fatalf("valid link: got %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "Blitz-Windows") {
		t.Errorf("unexpected redirect target: %q", loc)
	}

	// Swapping the product to grab a different app must fail.
	rec = httptest.NewRecorder()
	q2 := "key=" + key + "&product=CCPR&platform=" + platform +
		"&exp=" + strconv.FormatInt(exp, 10) + "&sig=" + sig
	h.File(rec, httptest.NewRequest("GET", "/api/download/file?"+q2, nil))
	if rec.Code != 403 {
		t.Errorf("tampered product: got %d, want 403", rec.Code)
	}

	// An expired link must fail even with a correct signature.
	past := time.Now().Add(-time.Minute).Unix()
	rec = httptest.NewRecorder()
	q3 := "key=" + key + "&product=" + product + "&platform=" + platform +
		"&exp=" + strconv.FormatInt(past, 10) + "&sig=" + h.sign(key, product, platform, past)
	h.File(rec, httptest.NewRequest("GET", "/api/download/file?"+q3, nil))
	if rec.Code != 410 {
		t.Errorf("expired link: got %d, want 410", rec.Code)
	}
}

func TestCatalogCoversBothProducts(t *testing.T) {
	for _, fam := range []string{"BD", "CC"} {
		arts, ok := catalog[fam]
		if !ok || len(arts) == 0 {
			t.Fatalf("catalog missing artifacts for %s", fam)
		}
		for _, a := range arts {
			if a.url == "" || a.Platform == "" {
				t.Errorf("%s: incomplete artifact %+v", fam, a)
			}
		}
	}
}

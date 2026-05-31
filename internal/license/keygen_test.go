package license

import "testing"

func TestGenerateParseRoundTrip(t *testing.T) {
	for p := range ValidProducts {
		for i := 0; i < 200; i++ {
			k, err := Generate(p)
			if err != nil {
				t.Fatalf("generate %s: %v", p, err)
			}
			canon, prod, code := Parse(k)
			if code != "" {
				t.Fatalf("parse %q failed: %s", k, code)
			}
			if canon != k {
				t.Fatalf("canonical mismatch: gen=%q parsed=%q", k, canon)
			}
			if prod != p {
				t.Fatalf("product mismatch: want %s got %s", p, prod)
			}
		}
	}
}

func TestParseRejects(t *testing.T) {
	// wrong product
	if _, _, c := Parse("XXXX-7K3P-9WXM-2QH4-RT8C"); c != "WRONG_PRODUCT" {
		t.Errorf("want WRONG_PRODUCT got %s", c)
	}
	// good key, then corrupt one body char → checksum fail
	k, _ := Generate("BDPR")
	bad := []byte(k)
	// flip the second body char (index 6, inside first group after prefix+dash)
	if bad[6] == '7' {
		bad[6] = '8'
	} else {
		bad[6] = '7'
	}
	if _, _, c := Parse(string(bad)); c != "BAD_CHECKSUM" && c != "BAD_FORMAT" {
		t.Errorf("corrupted key should fail, got %s", c)
	}
	// tolerant normalize: lowercase + spaces + I/O/L
	k2, _ := Generate("CCST")
	messy := "  " + k2 + "  "
	if _, _, c := Parse(messy); c != "" {
		t.Errorf("messy-but-valid should pass, got %s", c)
	}
}

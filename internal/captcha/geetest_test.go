package captcha

import "testing"

func TestVerifyGate(t *testing.T) {
	// not configured → always pass (don't block)
	off := New("", "")
	if off.Enabled() {
		t.Fatal("should be disabled with empty id/key")
	}
	if !off.Verify(Params{}) {
		t.Error("disabled captcha must pass through")
	}

	// configured → empty client params must be rejected (no network call)
	on := New("test-captcha-id", "test-key")
	if !on.Enabled() {
		t.Fatal("should be enabled with id+key")
	}
	if on.Verify(Params{}) {
		t.Error("enabled captcha with empty params must be rejected")
	}
	if on.Verify(Params{LotNumber: "x"}) { // partial → still rejected
		t.Error("enabled captcha with partial params must be rejected")
	}
}

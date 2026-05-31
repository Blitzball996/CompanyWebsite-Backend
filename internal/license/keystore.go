package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
)

// LoadOrCreateKey returns the Ed25519 signing key.
//
// Priority:
//  1. base64 env value (the 64-byte private key, raw-url or std base64)
//  2. a key file at path (created on first run if missing)
//
// The matching public key (embed it in the desktop apps) is returned base64.
func LoadOrCreateKey(envB64, path string) (ed25519.PrivateKey, string, error) {
	if envB64 != "" {
		raw, err := decodeB64(envB64)
		if err == nil && len(raw) == ed25519.PrivateKeySize {
			priv := ed25519.PrivateKey(raw)
			return priv, pub(priv), nil
		}
		if err == nil && len(raw) == ed25519.SeedSize {
			priv := ed25519.NewKeyFromSeed(raw)
			return priv, pub(priv), nil
		}
	}

	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			raw, derr := decodeB64(string(b))
			if derr == nil && len(raw) == ed25519.PrivateKeySize {
				priv := ed25519.PrivateKey(raw)
				return priv, pub(priv), nil
			}
		}
	}

	// generate fresh
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", err
	}
	if path != "" {
		_ = os.MkdirAll(filepath.Dir(path), 0o700)
		_ = os.WriteFile(path, []byte(base64.RawURLEncoding.EncodeToString(priv)), 0o600)
	}
	return priv, pub(priv), nil
}

func pub(priv ed25519.PrivateKey) string {
	return base64.RawURLEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
}

func decodeB64(s string) ([]byte, error) {
	s = trimSpace(s)
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}

func trimSpace(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			continue
		}
		out = append(out, c)
	}
	return string(out)
}

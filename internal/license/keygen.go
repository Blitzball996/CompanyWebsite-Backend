// Package license implements serial-number (license key) generation,
// validation, and online activation — see docs/序列号格式规范.md.
//
// Format:  PROD-GGGG-GGGG-GGGG-GGGG   (uppercase)
//   PROD  = BDST/BDPR (Blitz DAW) · CCST/CCPR (CloseCrab)
//   body  = 16 Crockford-Base32 chars (4 groups of 4); last char is a checksum.
package license

import (
	"crypto/rand"
	"errors"
	"strings"
)

// Crockford Base32 alphabet, excluding I L O U (see spec §2). 32 symbols.
const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// ValidProducts is the set of accepted product+edition prefixes.
var ValidProducts = map[string]string{
	"BDST": "standard", "BDPR": "pro", // Blitz DAW
	"CCST": "standard", "CCPR": "pro", // CloseCrab
}

var idxOf [256]int

func init() {
	for i := range idxOf {
		idxOf[i] = -1
	}
	for i := 0; i < len(alphabet); i++ {
		idxOf[alphabet[i]] = i
	}
}

// checksum computes the spec §4 mod-32 weighted-sum checksum char for the
// given 15 data symbols, returning the 16th (checksum) symbol.
func checksum(data15 string) byte {
	sum := 0
	for i := 0; i < len(data15); i++ {
		sum += idxOf[data15[i]] * (i + 1)
	}
	return alphabet[sum%32]
}

// Generate creates one new license key for the given product prefix.
func Generate(product string) (string, error) {
	if _, ok := ValidProducts[product]; !ok {
		return "", errors.New("license: unknown product prefix " + product)
	}
	// 15 random data chars from the alphabet (rejection-free: 32 divides 256? no,
	// so we map via modulo with rejection to avoid bias).
	data := make([]byte, 15)
	buf := make([]byte, 1)
	for i := 0; i < 15; {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		if buf[0] >= 248 { // 256 - (256 % 32) = 248; reject to keep uniform
			continue
		}
		data[i] = alphabet[int(buf[0])%32]
		i++
	}
	body := string(data) + string(checksum(string(data))) // 16 chars
	// group into 4×4 with dashes, prefixed by product
	return product + "-" + body[0:4] + "-" + body[4:8] + "-" + body[8:12] + "-" + body[12:16], nil
}

// Normalize strips spaces/dashes, uppercases, and applies tolerant
// substitutions for commonly mistyped characters (spec §6).
func Normalize(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	r := strings.NewReplacer(" ", "", "-", "", "I", "1", "L", "1", "O", "0", "U", "V")
	return r.Replace(s)
}

// Parse validates a (possibly messy) key string. It returns the canonical
// dashed key and the product prefix, or an error code string.
//
// Error codes: "BAD_FORMAT", "BAD_CHECKSUM", "WRONG_PRODUCT".
func Parse(raw string) (canonical, product string, errCode string) {
	n := Normalize(raw)
	if len(n) != 20 { // 4 prefix + 16 body
		return "", "", "BAD_FORMAT"
	}
	product = n[0:4]
	if _, ok := ValidProducts[product]; !ok {
		return "", "", "WRONG_PRODUCT"
	}
	body := n[4:20]
	for i := 0; i < len(body); i++ {
		if idxOf[body[i]] < 0 {
			return "", "", "BAD_FORMAT"
		}
	}
	if checksum(body[0:15]) != body[15] {
		return "", "", "BAD_CHECKSUM"
	}
	canonical = product + "-" + body[0:4] + "-" + body[4:8] + "-" + body[8:12] + "-" + body[12:16]
	return canonical, product, ""
}

// Edition returns "standard" or "pro" for a product prefix.
func Edition(product string) string { return ValidProducts[product] }

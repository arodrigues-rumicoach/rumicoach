package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// VerifySignature checks Meta's X-Hub-Signature-256 header ("sha256=<hex>")
// against the raw request body using the app secret, in constant time.
func VerifySignature(appSecret string, body []byte, header string) bool {
	if appSecret == "" || header == "" {
		return false
	}
	provided, ok := strings.CutPrefix(header, "sha256=")
	if !ok {
		return false
	}
	providedBytes, err := hex.DecodeString(provided)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write(body)
	return hmac.Equal(providedBytes, mac.Sum(nil))
}

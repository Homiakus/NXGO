package supervisor

import (
	"crypto/rand"
	"encoding/hex"
)

func newWorkerNonce() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "nonce-" + hex.EncodeToString(b), nil
}

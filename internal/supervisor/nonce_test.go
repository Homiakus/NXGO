package supervisor

import (
	"strings"
	"testing"
)

func TestNewWorkerNonceIsHighEntropy(t *testing.T) {
	a, err := newWorkerNonce()
	if err != nil {
		t.Fatal(err)
	}
	b, err := newWorkerNonce()
	if err != nil {
		t.Fatal(err)
	}
	if a == b || !strings.HasPrefix(a, "nonce-") || len(a) != len("nonce-")+64 {
		t.Fatalf("unexpected worker nonce values %q %q", a, b)
	}
}

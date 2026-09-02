package nxgo

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// requestIDGenerator combines a process-unique random prefix with a monotonic
// counter. This avoids relying on wall-clock resolution for request identity
// and gives the Agent/idempotency layer a stable key even under highly
// concurrent callers.
type requestIDGenerator struct {
	prefix   string
	sequence atomic.Uint64
}

func newRequestIDGenerator(prefix string) *requestIDGenerator {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = randomIDPrefix()
	}
	return &requestIDGenerator{prefix: prefix}
}

func (g *requestIDGenerator) next(kind string) string {
	kind = sanitizeRequestIDKind(kind)
	seq := g.sequence.Add(1)
	return fmt.Sprintf("req-%s-%s-%016x", kind, g.prefix, seq)
}

func sanitizeRequestIDKind(kind string) string {
	kind = strings.TrimSpace(strings.ToLower(kind))
	if kind == "" {
		return "call"
	}
	var b strings.Builder
	for _, r := range kind {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "call"
	}
	return b.String()
}

func randomIDPrefix() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}

	// crypto/rand failure is extraordinarily rare, but request creation must
	// still remain available. Mix process identity and high-resolution time;
	// the atomic sequence still prevents collisions inside this process.
	return fmt.Sprintf("fallback-%x-%x", uint64(os.Getpid()), uint64(time.Now().UnixNano()))
}

var defaultRequestIDs = newRequestIDGenerator("")

func newRequestID(kind string) string {
	return defaultRequestIDs.next(kind)
}

func newHandshakeNonce() string {
	return "nonce-" + randomIDPrefix()
}

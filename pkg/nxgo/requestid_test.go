package nxgo

import (
	"strings"
	"sync"
	"testing"
)

func TestRequestIDGeneratorIsUniqueUnderConcurrency(t *testing.T) {
	g := newRequestIDGenerator("fixedprefix")

	const workers = 32
	const perWorker = 500
	seen := make(map[string]struct{}, workers*perWorker)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				id := g.next("feature.create_block")
				mu.Lock()
				if _, exists := seen[id]; exists {
					t.Errorf("duplicate request id generated: %s", id)
				}
				seen[id] = struct{}{}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if got, want := len(seen), workers*perWorker; got != want {
		t.Fatalf("unique ID count mismatch: got %d want %d", got, want)
	}
}

func TestRequestIDGeneratorSanitizesKind(t *testing.T) {
	g := newRequestIDGenerator("fixed")
	id := g.next(" Part Save / Unsafe ")
	if !strings.HasPrefix(id, "req-part-save---unsafe-fixed-") {
		t.Fatalf("unexpected sanitized request id: %s", id)
	}
}

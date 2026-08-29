package fakeagent

import (
    "context"
    "errors"
    "fmt"
    "testing"
)

func TestChaosMutationResponseLossDoesNotDuplicateMutation(t *testing.T) {
    a := New()
    req := Request{ID: "req-1", Mutation: true, Fault: DropAfterCommit}
    if _, err := a.Execute(context.Background(), req); !errors.Is(err, ErrTransportLost) { t.Fatalf("expected transport loss, got %v", err) }
    resp, err := a.Execute(context.Background(), req)
    if err != nil { t.Fatal(err) }
    if a.Applied() != 1 || resp.Applied != 1 { t.Fatalf("mutation duplicated: applied=%d resp=%d", a.Applied(), resp.Applied) }
}

func TestChaosPoisonedSessionRejectsFurtherWork(t *testing.T) {
    a := New()
    _, _ = a.Execute(context.Background(), Request{ID: "p", Fault: PoisonSession})
    if _, err := a.Execute(context.Background(), Request{ID: "next"}); err == nil { t.Fatal("poisoned session accepted work") }
}

func TestSoakRepeatedIdempotentReplayStaysBounded(t *testing.T) {
    a := New()
    for i := 0; i < 10000; i++ {
        if _, err := a.Execute(context.Background(), Request{ID: "same", Mutation: true}); err != nil { t.Fatal(err) }
    }
    if a.Applied() != 1 { t.Fatalf("applied=%d", a.Applied()) }
    if a.RecordCount() != 1 { t.Fatalf("records=%d", a.RecordCount()) }
}

func BenchmarkExecuteUniqueRead(b *testing.B) {
    a := New()
    ctx := context.Background()
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        _, _ = a.Execute(ctx, Request{ID: fmt.Sprintf("r-%d", i)})
    }
}

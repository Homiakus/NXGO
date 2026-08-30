package fakeagent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/transport/pipe"
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

func TestFakeAgentTransportClientRoundtrip(t *testing.T) {
	agent := New()
	clientConn, serverConn := net.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = agent.ServeTransport(ctx, serverConn)
	}()

	client := pipe.NewClient(clientConn)
	defer client.Close()

	// 1. Handshake
	hsResp, err := client.Handshake(ctx, &protocol.HandshakeRequest{
		ProtocolVersion: protocol.Version{Major: protocol.CurrentProtocolMajor, Minor: protocol.CurrentProtocolMinor},
		SDKVersion:      "v0.1.0",
		ClientPID:       1000,
		Nonce:           "test-nonce-1",
	})
	if err != nil {
		t.Fatalf("handshake failed: %v", err)
	}
	if hsResp.SessionID != "fake-session-1" || hsResp.Epoch != 1 {
		t.Fatalf("unexpected handshake response: %+v", hsResp)
	}

	// 2. Query call
	resp, err := client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: "req-q1",
		Op:        "nx.ping",
	})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if resp.Status != protocol.StatusOK {
		t.Fatalf("expected status OK, got %s", resp.Status)
	}

	// 3. Mutating call with idempotent replay
	m1, err := client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: "req-mut-1",
		Op:        "part.save",
	})
	if err != nil {
		t.Fatalf("mutating call failed: %v", err)
	}
	if m1.Status != protocol.StatusOK {
		t.Fatalf("expected OK, got %s", m1.Status)
	}

	m2, err := client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: "req-mut-1", // same request ID
		Op:        "part.save",
	})
	if err != nil {
		t.Fatalf("idempotent replay failed: %v", err)
	}
	if m2.Status != protocol.StatusOK {
		t.Fatalf("expected OK on idempotent replay, got %s", m2.Status)
	}

	if agent.Applied() != 1 {
		t.Fatalf("mutation duplicated over transport: applied=%d", agent.Applied())
	}
}


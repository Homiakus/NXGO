package pipe

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
)

func TestThousandConcurrentRPCsRemainExactlyCorrelated(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	const total = 1000
	serverDone := make(chan error, 1)
	go func() {
		framed := NewFramedConn(serverConn, protocol.DefaultMaxPayloadBytes)
		requests := make([]*protocol.RequestEnvelope, 0, total)
		for i := 0; i < total; i++ {
			b, err := framed.Receive()
			if err != nil {
				serverDone <- fmt.Errorf("receive %d: %w", i, err)
				return
			}
			req, err := protocol.DecodePayload[protocol.RequestEnvelope](b)
			if err != nil {
				serverDone <- fmt.Errorf("decode %d: %w", i, err)
				return
			}
			requests = append(requests, req)
		}

		order := rand.New(rand.NewSource(20260902)).Perm(total)
		for _, index := range order {
			req := requests[index]
			payload, _ := json.Marshal(map[string]string{"echo": req.RequestID})
			resp := protocol.ResponseEnvelope{
				RequestID: req.RequestID,
				Status:    protocol.StatusOK,
				Payload:   payload,
			}
			b, err := protocol.EncodePayload(resp)
			if err != nil {
				serverDone <- err
				return
			}
			if err := framed.Send(b); err != nil {
				serverDone <- fmt.Errorf("send response %s: %w", req.RequestID, err)
				return
			}
		}
		serverDone <- nil
	}()

	client := NewClient(clientConn)
	client.maxPending = total + 16

	type callResult struct {
		requestID string
		response  *protocol.ResponseEnvelope
		err       error
	}
	results := make(chan callResult, total)
	start := make(chan struct{})
	for i := 0; i < total; i++ {
		requestID := fmt.Sprintf("stress-%04d", i)
		go func(id string) {
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			resp, err := client.Call(ctx, &protocol.RequestEnvelope{
				RequestID: id,
				Op:        "stress.echo",
			})
			results <- callResult{requestID: id, response: resp, err: err}
		}(requestID)
	}
	close(start)

	seen := make(map[string]struct{}, total)
	for i := 0; i < total; i++ {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatalf("call %s failed: %v", result.requestID, result.err)
			}
			if result.response == nil || result.response.RequestID != result.requestID {
				t.Fatalf("correlation mismatch for %s: %+v", result.requestID, result.response)
			}
			var body struct {
				Echo string `json:"echo"`
			}
			if err := json.Unmarshal(result.response.Payload, &body); err != nil {
				t.Fatalf("decode echo for %s: %v", result.requestID, err)
			}
			if body.Echo != result.requestID {
				t.Fatalf("payload from another request: want %s got %s", result.requestID, body.Echo)
			}
			if _, duplicate := seen[result.requestID]; duplicate {
				t.Fatalf("duplicate completion for %s", result.requestID)
			}
			seen[result.requestID] = struct{}{}
		case <-time.After(15 * time.Second):
			t.Fatalf("timed out after %d/%d correlated responses", i, total)
		}
	}

	if len(seen) != total {
		t.Fatalf("completed %d unique calls, want %d", len(seen), total)
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("server failed: %v", err)
	}
}

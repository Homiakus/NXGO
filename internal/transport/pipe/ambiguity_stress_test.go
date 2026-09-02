package pipe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
)

func TestDuplicateResponseIDQuarantinesConnection(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		server := NewFramedConn(serverConn, protocol.DefaultMaxPayloadBytes)
		b, err := server.Receive()
		if err != nil {
			return
		}
		req, err := protocol.DecodePayload[protocol.RequestEnvelope](b)
		if err != nil {
			return
		}
		resp := protocol.ResponseEnvelope{RequestID: req.RequestID, Status: protocol.StatusOK}
		payload, _ := protocol.EncodePayload(resp)
		_ = server.Send(payload)
		// The exact same response is a protocol violation after the first copy
		// has completed and removed the request from pending.
		_ = server.Send(payload)
	}()

	client := NewClient(clientConn)
	quarantined := make(chan error, 1)
	client.SetQuarantineHook(func(err error) { quarantined <- err })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resp, err := client.Call(ctx, &protocol.RequestEnvelope{RequestID: "dup-response", Op: "nx.ping"})
	if err != nil {
		t.Fatalf("first response should complete normally: %v", err)
	}
	if resp.RequestID != "dup-response" {
		t.Fatalf("unexpected first response: %+v", resp)
	}

	select {
	case err := <-quarantined:
		if !errors.Is(err, ErrProtocolViolation) {
			t.Fatalf("duplicate response must be protocol violation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("duplicate response did not quarantine connection")
	}

	_, err = client.Call(context.Background(), &protocol.RequestEnvelope{RequestID: "after-duplicate", Op: "nx.ping"})
	if err == nil || !errors.Is(err, ErrProtocolViolation) {
		t.Fatalf("quarantined connection accepted another call: %v", err)
	}
}

func TestThousandCancelledAfterSendRequestsNeverLeakIntoNextRPC(t *testing.T) {
	for i := 0; i < 1000; i++ {
		clientConn, serverConn := net.Pipe()
		client := NewClient(clientConn)
		server := NewFramedConn(serverConn, protocol.DefaultMaxPayloadBytes)

		receivedA := make(chan struct{})
		releaseLate := make(chan struct{})
		serverDone := make(chan string, 1)
		go func() {
			b, err := server.Receive()
			if err != nil {
				serverDone <- "receive-a-error"
				return
			}
			req, err := protocol.DecodePayload[protocol.RequestEnvelope](b)
			if err != nil {
				serverDone <- "decode-a-error"
				return
			}
			if req.RequestID == "" {
				serverDone <- "empty-a"
				return
			}
			close(receivedA)
			<-releaseLate

			late := protocol.ResponseEnvelope{RequestID: req.RequestID, Status: protocol.StatusOK}
			lateBytes, _ := protocol.EncodePayload(late)
			_ = server.Send(lateBytes)

			// Correct quarantine closes the stream, so request B can never arrive.
			b, err = server.Receive()
			if err != nil {
				serverDone <- "closed"
				return
			}
			next, err := protocol.DecodePayload[protocol.RequestEnvelope](b)
			if err != nil {
				serverDone <- "decode-b-error"
				return
			}
			serverDone <- next.RequestID
		}()

		ctx, cancel := context.WithCancel(context.Background())
		callA := make(chan error, 1)
		go func(iter int) {
			_, err := client.Call(ctx, &protocol.RequestEnvelope{
				RequestID: fmt.Sprintf("stress-A-%d", iter),
				Op:        "feature.create_block",
			})
			callA <- err
		}(i)

		select {
		case <-receivedA:
		case <-time.After(time.Second):
			t.Fatalf("iteration %d: server never received A", i)
		}
		cancel()

		select {
		case err := <-callA:
			if err == nil || !errors.Is(err, ErrOutcomeUnknown) {
				t.Fatalf("iteration %d: cancellation after send must be outcome-unknown, got %v", i, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("iteration %d: A did not complete after cancellation", i)
		}

		_, err := client.Call(context.Background(), &protocol.RequestEnvelope{
			RequestID: fmt.Sprintf("stress-B-%d", i),
			Op:        "part.save",
		})
		if err == nil || !errors.Is(err, ErrOutcomeUnknown) {
			t.Fatalf("iteration %d: B was not rejected by quarantined connection: %v", i, err)
		}

		close(releaseLate)
		select {
		case observed := <-serverDone:
			if observed != "closed" {
				t.Fatalf("iteration %d: request/garbage reached server after quarantine: %q", i, observed)
			}
		case <-time.After(time.Second):
			t.Fatalf("iteration %d: server did not observe closed quarantined connection", i)
		}

		_ = client.Close()
		_ = serverConn.Close()
		cancel()
	}
}

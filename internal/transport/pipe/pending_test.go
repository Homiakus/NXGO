package pipe

import (
	"errors"
	"net"
	"testing"
)

func TestRegisterPendingRejectsOverLimit(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	client := NewClient(clientConn)
	client.maxPending = 2

	if _, err := client.registerPending("req-1", make(chan callResult, 1)); err != nil {
		t.Fatalf("register req-1: %v", err)
	}
	if _, err := client.registerPending("req-2", make(chan callResult, 1)); err != nil {
		t.Fatalf("register req-2: %v", err)
	}
	if _, err := client.registerPending("req-3", make(chan callResult, 1)); err == nil || !errors.Is(err, ErrTooManyPending) {
		t.Fatalf("expected ErrTooManyPending, got %v", err)
	}
}

func TestRegisterPendingRejectsDuplicateRequestID(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	client := NewClient(clientConn)
	if _, err := client.registerPending("req-duplicate", make(chan callResult, 1)); err != nil {
		t.Fatalf("first registration failed: %v", err)
	}
	if _, err := client.registerPending("req-duplicate", make(chan callResult, 1)); err == nil || !errors.Is(err, ErrDuplicateRequest) {
		t.Fatalf("expected ErrDuplicateRequest, got %v", err)
	}
}

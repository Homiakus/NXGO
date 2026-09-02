package nxgo

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/transport/pipe"
)

func TestStalePartHandleFailsBeforeAnyTransportWrite(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	session := WrapClient(pipe.NewClient(clientConn), "session-current", 7, "2512")
	part := &Part{
		session: session,
		Ref: protocol.ObjectHandleWire{
			SessionID: "session-old",
			Epoch:     7,
			ObjectID:  "obj-part-1",
			Generation: 1,
			Kind:      "Part",
		},
	}

	_, err := part.Save(context.Background())
	if err == nil || !errors.Is(err, ErrStaleObjectRef) {
		t.Fatalf("expected ErrStaleObjectRef, got %v", err)
	}
	assertNoTransportWrite(t, serverConn)
}

func TestStaleEpochFailsBeforeAnyTransportWrite(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	session := WrapClient(pipe.NewClient(clientConn), "session-current", 8, "2512")
	part := &Part{
		session: session,
		Ref: protocol.ObjectHandleWire{
			SessionID: "session-current",
			Epoch:     7,
			ObjectID:  "obj-part-1",
			Generation: 1,
			Kind:      "Part",
		},
	}

	_, err := part.Summary(context.Background())
	if err == nil || !errors.Is(err, ErrStaleObjectRef) {
		t.Fatalf("expected ErrStaleObjectRef, got %v", err)
	}
	assertNoTransportWrite(t, serverConn)
}

func TestWrongObjectKindFailsBeforeAnyTransportWrite(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	session := WrapClient(pipe.NewClient(clientConn), "session-current", 1, "2512")
	part := &Part{
		session: session,
		Ref: protocol.ObjectHandleWire{
			SessionID: "session-current",
			Epoch:     1,
			ObjectID:  "obj-body-not-part",
			Generation: 1,
			Kind:      "Body",
		},
	}

	_, err := part.Bodies(context.Background())
	if err == nil || !errors.Is(err, ErrStaleObjectRef) {
		t.Fatalf("expected wrong-kind ErrStaleObjectRef, got %v", err)
	}
	assertNoTransportWrite(t, serverConn)
}

func TestUnsupportedBooleanFeatureOptionFailsBeforeAnyTransportWrite(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	session := WrapClient(pipe.NewClient(clientConn), "session-current", 1, "2512")
	part := &Part{
		session: session,
		Ref: protocol.ObjectHandleWire{
			SessionID: "session-current",
			Epoch:     1,
			ObjectID:  "obj-part-1",
			Generation: 1,
			Kind:      "Part",
		},
	}

	_, err := part.CreateBlock(context.Background(), BlockParams{
		Length:    10,
		Width:     10,
		Height:    10,
		BooleanOp: "subtract",
	})
	if err == nil || !errors.Is(err, ErrUnsupportedFeatureOption) {
		t.Fatalf("expected ErrUnsupportedFeatureOption, got %v", err)
	}
	assertNoTransportWrite(t, serverConn)
}

func TestTargetBodyIsRejectedUntilBackendActuallyHonorsIt(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	session := WrapClient(pipe.NewClient(clientConn), "session-current", 1, "2512")
	part := &Part{
		session: session,
		Ref: protocol.ObjectHandleWire{
			SessionID: "session-current",
			Epoch:     1,
			ObjectID:  "obj-part-1",
			Generation: 1,
			Kind:      "Part",
		},
	}
	target := protocol.ObjectHandleWire{
		SessionID: "session-current",
		Epoch:     1,
		ObjectID:  "obj-body-1",
		Generation: 1,
		Kind:      "Body",
	}

	_, err := part.CreateCylinder(context.Background(), CylinderParams{
		Diameter:      10,
		Height:        20,
		BooleanOp:     "create",
		TargetBodyRef: &target,
	})
	if err == nil || !errors.Is(err, ErrUnsupportedFeatureOption) {
		t.Fatalf("expected target body to be rejected until implemented, got %v", err)
	}
	assertNoTransportWrite(t, serverConn)
}

func TestReleaseObjectsRejectsForeignHandleBeforeTransport(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	session := WrapClient(pipe.NewClient(clientConn), "session-current", 1, "2512")
	err := session.ReleaseObjects(context.Background(), protocol.ObjectHandleWire{
		SessionID: "foreign-session",
		Epoch:     1,
		ObjectID:  "obj-1",
		Generation: 1,
		Kind:      "Body",
	})
	if err == nil || !errors.Is(err, ErrStaleObjectRef) {
		t.Fatalf("expected foreign release handle rejection, got %v", err)
	}
	assertNoTransportWrite(t, serverConn)
}

func TestNilSessionOperationsReturnSessionClosed(t *testing.T) {
	var session *Session
	if err := session.Ping(context.Background()); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("expected ErrSessionClosed from nil session, got %v", err)
	}
}

func assertNoTransportWrite(t *testing.T, conn net.Conn) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(40 * time.Millisecond)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	buf := make([]byte, 1)
	_, err := conn.Read(buf)
	if err == nil {
		t.Fatal("unexpected transport bytes were written")
	}
	if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
		t.Fatalf("expected read timeout proving no transport write, got %v", err)
	}
}

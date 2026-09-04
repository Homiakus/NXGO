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

func TestValidateCreateFeatureOptions(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	s := WrapClient(pipe.NewClient(clientConn), "sess-1", 1, "2512")
	validBody := &protocol.ObjectHandleWire{
		SessionID:  "sess-1",
		Epoch:      1,
		ObjectID:   "body-1",
		Generation: 1,
		Kind:       "Body",
	}
	invalidKind := &protocol.ObjectHandleWire{
		SessionID:  "sess-1",
		Epoch:      1,
		ObjectID:   "face-1",
		Generation: 1,
		Kind:       "Face",
	}

	// create op
	if err := validateCreateFeatureOptions(s, "create", nil); err != nil {
		t.Fatalf("expected nil for valid create, got %v", err)
	}
	if err := validateCreateFeatureOptions(s, "", nil); err != nil {
		t.Fatalf("expected nil for empty create, got %v", err)
	}
	if err := validateCreateFeatureOptions(s, "create", validBody); !errors.Is(err, ErrUnsupportedFeatureOption) {
		t.Fatalf("expected ErrUnsupportedFeatureOption for create with target body, got %v", err)
	}

	// boolean ops with target body
	for _, op := range []string{"unite", "subtract", "intersect"} {
		if err := validateCreateFeatureOptions(s, op, nil); !errors.Is(err, ErrUnsupportedFeatureOption) {
			t.Fatalf("expected ErrUnsupportedFeatureOption for %s without target body, got %v", op, err)
		}
		if err := validateCreateFeatureOptions(s, op, validBody); err != nil {
			t.Fatalf("expected nil for %s with valid target body, got %v", op, err)
		}
		if err := validateCreateFeatureOptions(s, op, invalidKind); err == nil {
			t.Fatalf("expected error for %s with invalid kind body, got nil", op)
		}
	}

	// invalid boolean op
	if err := validateCreateFeatureOptions(s, "invalid_op", validBody); !errors.Is(err, ErrUnsupportedFeatureOption) {
		t.Fatalf("expected ErrUnsupportedFeatureOption for invalid_op, got %v", err)
	}
}

func TestPartBooleanOperation(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	serverFramed := pipe.NewFramedConn(serverConn, protocol.DefaultMaxPayloadBytes)

	go func() {
		for {
			rawReq, err := serverFramed.Receive()
			if err != nil {
				return
			}
			req, err := protocol.DecodePayload[protocol.RequestEnvelope](rawReq)
			if err != nil {
				return
			}

			if req.Op == "feature.boolean" {
				boolReq, err := protocol.DecodePayload[protocol.FeatureBooleanRequest](req.Payload)
				if err != nil {
					return
				}

				resp := protocol.FeatureBooleanResponse{
					FeatureRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   "feat-bool-1",
						Generation: 1,
						Kind:       "Feature",
					},
					BodyRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   boolReq.TargetBodyRef.ObjectID,
						Generation: 1,
						Kind:       "Body",
					},
					FeatureName: boolReq.Op + "(1)",
					FeatureType: "Boolean",
				}
				respPayload, _ := protocol.EncodePayload(resp)

				respEnv := protocol.ResponseEnvelope{
					RequestID: req.RequestID,
					Status:    protocol.StatusOK,
					Payload:   respPayload,
				}
				rawResp, _ := protocol.EncodePayload(respEnv)
				_ = serverFramed.Send(rawResp)
			}
		}
	}()

	client := pipe.NewClient(clientConn)
	defer client.Close()

	session := WrapClient(client, "sess-1", 1, "v2512")

	part := &Part{
		session: session,
		Ref: protocol.ObjectHandleWire{
			SessionID:  "sess-1",
			Epoch:      1,
			ObjectID:   "part-1",
			Generation: 1,
			Kind:       "Part",
		},
		Name:  "TestPart",
		Units: "Millimeters",
	}

	targetBody := protocol.ObjectHandleWire{
		SessionID:  "sess-1",
		Epoch:      1,
		ObjectID:   "body-target",
		Generation: 1,
		Kind:       "Body",
	}

	toolBody := protocol.ObjectHandleWire{
		SessionID:  "sess-1",
		Epoch:      1,
		ObjectID:   "body-tool-1",
		Generation: 1,
		Kind:       "Body",
	}

	// Test unsupported op
	_, err := part.Boolean(ctx, BooleanParams{
		Op:            "slice",
		TargetBodyRef: targetBody,
		ToolBodyRefs:  []protocol.ObjectHandleWire{toolBody},
	})
	if !errors.Is(err, ErrUnsupportedFeatureOption) {
		t.Fatalf("expected ErrUnsupportedFeatureOption for unsupported op, got %v", err)
	}

	// Test empty tools
	_, err = part.Boolean(ctx, BooleanParams{
		Op:            "unite",
		TargetBodyRef: targetBody,
		ToolBodyRefs:  nil,
	})
	if err == nil {
		t.Fatalf("expected error for empty tool bodies, got nil")
	}

	// Test valid unite
	feat, err := part.Boolean(ctx, BooleanParams{
		Op:            "unite",
		TargetBodyRef: targetBody,
		ToolBodyRefs:  []protocol.ObjectHandleWire{toolBody},
	})
	if err != nil {
		t.Fatalf("unite failed: %v", err)
	}
	if feat.Ref.ObjectID != "feat-bool-1" {
		t.Fatalf("unexpected feature object id: %s", feat.Ref.ObjectID)
	}
	if feat.BodyRef.ObjectID != "body-target" {
		t.Fatalf("unexpected body object id: %s", feat.BodyRef.ObjectID)
	}
	if feat.Type != "Boolean" {
		t.Fatalf("unexpected feature type: %s", feat.Type)
	}

	// Test valid subtract
	featSub, err := part.Boolean(ctx, BooleanParams{
		Op:            "subtract",
		TargetBodyRef: targetBody,
		ToolBodyRefs:  []protocol.ObjectHandleWire{toolBody},
	})
	if err != nil {
		t.Fatalf("subtract failed: %v", err)
	}
	if featSub.Name != "subtract(1)" {
		t.Fatalf("unexpected feature name: %s", featSub.Name)
	}
}

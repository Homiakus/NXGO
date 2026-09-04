package nxgo

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/transport/pipe"
)

func TestExtrudeAndRevolveProtocol(t *testing.T) {
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

			var respPayload []byte
			switch req.Op {
			case "feature.create_extrude":
				resp := protocol.FeatureCreateExtrudeResponse{
					FeatureRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   "feat-extrude-1",
						Generation: 1,
						Kind:       "Feature",
					},
					BodyRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   "body-extrude-1",
						Generation: 1,
						Kind:       "Body",
					},
					FeatureName: "Extrude(1)",
					FeatureType: "Extrude",
				}
				respPayload, _ = protocol.EncodePayload(resp)

			case "feature.create_revolve":
				resp := protocol.FeatureCreateRevolveResponse{
					FeatureRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   "feat-revolve-1",
						Generation: 1,
						Kind:       "Feature",
					},
					BodyRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   "body-revolve-1",
						Generation: 1,
						Kind:       "Body",
					},
					FeatureName: "Revolve(1)",
					FeatureType: "Revolve",
				}
				respPayload, _ = protocol.EncodePayload(resp)
			}

			respEnv := protocol.ResponseEnvelope{
				RequestID: req.RequestID,
				Status:    protocol.StatusOK,
				Payload:   respPayload,
			}
			respBytes, _ := protocol.EncodePayload(respEnv)
			_ = serverFramed.Send(respBytes)
		}
	}()

	client := pipe.NewClient(clientConn)
	defer client.Close()

	session := WrapClient(client, "sess-1", 1, "2512")
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
		Units: "mm",
	}

	profileRef := protocol.ObjectHandleWire{
		SessionID:  "sess-1",
		Epoch:      1,
		ObjectID:   "prof-1",
		Generation: 1,
		Kind:       "Profile",
	}

	// 1. Extrude
	featExtrude, err := part.Extrude(ctx, ExtrudeParams{
		ProfileRef: profileRef,
		Direction:  Vector3D{0, 0, 1},
		StartLimit: 0,
		EndLimit:   50.0,
		BooleanOp:  "create",
	})
	if err != nil {
		t.Fatalf("Extrude failed: %v", err)
	}
	if featExtrude.Name != "Extrude(1)" || featExtrude.Type != "Extrude" {
		t.Errorf("unexpected extrude feature: %+v", featExtrude)
	}

	// 2. Revolve
	featRevolve, err := part.Revolve(ctx, RevolveParams{
		ProfileRef:    profileRef,
		AxisOrigin:    Point3D{0, 0, 0},
		AxisDirection: Vector3D{0, 1, 0},
		StartAngle:    0,
		EndAngle:      360.0,
		BooleanOp:     "create",
	})
	if err != nil {
		t.Fatalf("Revolve failed: %v", err)
	}
	if featRevolve.Name != "Revolve(1)" || featRevolve.Type != "Revolve" {
		t.Errorf("unexpected revolve feature: %+v", featRevolve)
	}
}

func TestExtrudeValidationFailClosed(t *testing.T) {
	ctx := context.Background()

	clientConn, _ := net.Pipe()
	client := pipe.NewClient(clientConn)
	defer client.Close()
	session := WrapClient(client, "sess-1", 1, "2512")

	part := &Part{
		session: session,
		Ref: protocol.ObjectHandleWire{
			SessionID:  "sess-1",
			Epoch:      1,
			ObjectID:   "part-1",
			Generation: 1,
			Kind:       "Part",
		},
	}

	// Bad profile ref kind
	badProfile := protocol.ObjectHandleWire{
		SessionID:  "sess-1",
		Epoch:      1,
		ObjectID:   "body-1",
		Generation: 1,
		Kind:       "Body", // not Profile or Section
	}
	_, err := part.Extrude(ctx, ExtrudeParams{
		ProfileRef: badProfile,
		EndLimit:   20,
	})
	if err == nil {
		t.Fatal("expected error with invalid profile kind")
	}

	// Extrude end_limit <= start_limit
	validProfile := protocol.ObjectHandleWire{
		SessionID:  "sess-1",
		Epoch:      1,
		ObjectID:   "prof-1",
		Generation: 1,
		Kind:       "Profile",
	}
	_, err = part.Extrude(ctx, ExtrudeParams{
		ProfileRef: validProfile,
		StartLimit: 10,
		EndLimit:   10,
	})
	if err == nil {
		t.Fatal("expected error with end_limit <= start_limit")
	}

	// Revolve end_angle <= start_angle
	_, err = part.Revolve(ctx, RevolveParams{
		ProfileRef: validProfile,
		StartAngle: 90,
		EndAngle:   90,
	})
	if err == nil {
		t.Fatal("expected error with end_angle <= start_angle")
	}
}

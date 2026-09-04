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

func TestCreateHoleValidation(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	session := WrapClient(pipe.NewClient(clientConn), "sess-1", 1, "2512")
	part := &Part{
		session: session,
		Ref: protocol.ObjectHandleWire{
			SessionID:  "sess-1",
			Epoch:      1,
			ObjectID:   "part-1",
			Generation: 1,
			Kind:       "Part",
		},
		Name:  "HolePart",
		Units: "Millimeters",
	}

	validBody := protocol.ObjectHandleWire{
		SessionID:  "sess-1",
		Epoch:      1,
		ObjectID:   "body-1",
		Generation: 1,
		Kind:       "Body",
	}
	invalidKindBody := protocol.ObjectHandleWire{
		SessionID:  "sess-1",
		Epoch:      1,
		ObjectID:   "face-1",
		Generation: 1,
		Kind:       "Face",
	}

	ctx := context.Background()

	// 1. Invalid target body kind
	_, err := part.CreateHole(ctx, HoleParams{
		TargetBodyRef: invalidKindBody,
		Diameter:      10,
		Depth:         20,
	})
	if err == nil {
		t.Fatalf("expected error for invalid body kind, got nil")
	}

	// 2. Non-positive diameter
	_, err = part.CreateHole(ctx, HoleParams{
		TargetBodyRef: validBody,
		Diameter:      0,
		Depth:         20,
	})
	if err == nil || err.Error() != "hole diameter must be positive" {
		t.Fatalf("expected 'hole diameter must be positive', got %v", err)
	}

	// 3. Non-positive depth for blind hole
	_, err = part.CreateHole(ctx, HoleParams{
		TargetBodyRef: validBody,
		Diameter:      10,
		Depth:         0,
		ThroughBody:   false,
	})
	if err == nil || err.Error() != "hole depth must be positive for blind hole" {
		t.Fatalf("expected 'hole depth must be positive for blind hole', got %v", err)
	}

	// 4. Invalid hole type
	_, err = part.CreateHole(ctx, HoleParams{
		TargetBodyRef: validBody,
		Type:          "threaded",
		Diameter:      10,
		Depth:         20,
	})
	if !errors.Is(err, ErrUnsupportedFeatureOption) {
		t.Fatalf("expected ErrUnsupportedFeatureOption for threaded, got %v", err)
	}

	// 5. Counterbore validations
	_, err = part.CreateHole(ctx, HoleParams{
		TargetBodyRef:       validBody,
		Type:                HoleTypeCounterbore,
		Diameter:            10,
		Depth:               20,
		CounterboreDiameter: 8, // less than diameter
		CounterboreDepth:    5,
	})
	if err == nil || err.Error() != "counterbore diameter must be greater than hole diameter" {
		t.Fatalf("expected 'counterbore diameter must be greater than hole diameter', got %v", err)
	}

	_, err = part.CreateHole(ctx, HoleParams{
		TargetBodyRef:       validBody,
		Type:                HoleTypeCounterbore,
		Diameter:            10,
		Depth:               20,
		CounterboreDiameter: 15,
		CounterboreDepth:    0, // zero depth
	})
	if err == nil || err.Error() != "counterbore depth must be positive" {
		t.Fatalf("expected 'counterbore depth must be positive', got %v", err)
	}

	// 6. Countersink validations
	_, err = part.CreateHole(ctx, HoleParams{
		TargetBodyRef:       validBody,
		Type:                HoleTypeCountersink,
		Diameter:            10,
		Depth:               20,
		CountersinkDiameter: 8, // less than diameter
		CountersinkAngle:    90,
	})
	if err == nil || err.Error() != "countersink diameter must be greater than hole diameter" {
		t.Fatalf("expected 'countersink diameter must be greater than hole diameter', got %v", err)
	}

	_, err = part.CreateHole(ctx, HoleParams{
		TargetBodyRef:       validBody,
		Type:                HoleTypeCountersink,
		Diameter:            10,
		Depth:               20,
		CountersinkDiameter: 15,
		CountersinkAngle:    180, // invalid angle
	})
	if err == nil || err.Error() != "countersink angle must be between 0 and 180 degrees" {
		t.Fatalf("expected 'countersink angle must be between 0 and 180 degrees', got %v", err)
	}
}

func TestCreateHoleProtocol(t *testing.T) {
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

			if req.Op == "feature.create_hole" {
				holeReq, err := protocol.DecodePayload[protocol.FeatureCreateHoleRequest](req.Payload)
				if err != nil {
					return
				}

				name := "Simple Hole(1)"
				if holeReq.HoleType == "counterbore" {
					name = "Counterbore Hole(2)"
				} else if holeReq.HoleType == "countersink" {
					name = "Countersink Hole(3)"
				}

				resp := protocol.FeatureCreateHoleResponse{
					FeatureRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   "feat-hole-1",
						Generation: 1,
						Kind:       "Feature",
					},
					BodyRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   holeReq.TargetBodyRef.ObjectID,
						Generation: 1,
						Kind:       "Body",
					},
					FeatureName: name,
					FeatureType: "Hole",
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
		Units: "Millimeters",
	}

	targetBody := protocol.ObjectHandleWire{
		SessionID:  "sess-1",
		Epoch:      1,
		ObjectID:   "body-1",
		Generation: 1,
		Kind:       "Body",
	}

	// 1. Simple Hole
	feat, err := part.CreateHole(ctx, HoleParams{
		Type:          HoleTypeSimple,
		TargetBodyRef: targetBody,
		Origin:        Point3D{10, 20, 0},
		Direction:     Vector3D{0, 0, -1},
		Diameter:      8.5,
		Depth:         25.0,
		TipAngle:      118.0,
	})
	if err != nil {
		t.Fatalf("CreateHole simple failed: %v", err)
	}
	if feat.Name != "Simple Hole(1)" || feat.Type != "Hole" {
		t.Fatalf("unexpected feature: %+v", feat)
	}
	if feat.BodyRef.ObjectID != "body-1" {
		t.Fatalf("unexpected body ref: %+v", feat.BodyRef)
	}

	// 2. Counterbore Hole
	featCB, err := part.CreateHole(ctx, HoleParams{
		Type:                HoleTypeCounterbore,
		TargetBodyRef:       targetBody,
		Origin:              Point3D{30, 40, 0},
		Diameter:            6.0,
		Depth:               30.0,
		CounterboreDiameter: 11.0,
		CounterboreDepth:    6.5,
	})
	if err != nil {
		t.Fatalf("CreateHole counterbore failed: %v", err)
	}
	if featCB.Name != "Counterbore Hole(2)" {
		t.Fatalf("unexpected counterbore name: %s", featCB.Name)
	}

	// 3. Countersink Hole
	featCS, err := part.CreateHole(ctx, HoleParams{
		Type:                HoleTypeCountersink,
		TargetBodyRef:       targetBody,
		Origin:              Point3D{50, 60, 0},
		Diameter:            5.0,
		Depth:               20.0,
		CountersinkDiameter: 10.0,
		CountersinkAngle:    90.0,
	})
	if err != nil {
		t.Fatalf("CreateHole countersink failed: %v", err)
	}
	if featCS.Name != "Countersink Hole(3)" {
		t.Fatalf("unexpected countersink name: %s", featCS.Name)
	}
}

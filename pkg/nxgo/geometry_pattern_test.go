package nxgo

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/transport/pipe"
)

func TestPatternProtocol(t *testing.T) {
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
			case "feature.create_pattern":
				patternReq, err := protocol.DecodePayload[protocol.FeatureCreatePatternRequest](req.Payload)
				if err != nil {
					return
				}
				name := "Pattern Feature(1)"
				if patternReq.PatternType == "circular" {
					name = "Circular Pattern(1)"
				}
				resp := protocol.FeatureCreatePatternResponse{
					FeatureRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   "feat-pat-1",
						Generation: 1,
						Kind:       "Feature",
					},
					BodyRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   "body-1",
						Generation: 1,
						Kind:       "Body",
					},
					FeatureName: name,
					FeatureType: "PatternFeature",
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

	featRef := protocol.ObjectHandleWire{
		SessionID:  "sess-1",
		Epoch:      1,
		ObjectID:   "hole-1",
		Generation: 1,
		Kind:       "Feature",
	}

	feat := &Feature{
		session: session,
		part:    part,
		Ref:     featRef,
		Name:    "Hole(1)",
		Type:    "Hole",
	}

	// 1. Part.CreateLinearPattern
	linPat, err := part.CreateLinearPattern(ctx, LinearPatternParams{
		FeatureRefs: []protocol.ObjectHandleWire{featRef},
		XDirection:  Vector3D{1, 0, 0},
		XCount:      3,
		XPitch:      15.0,
		YDirection:  Vector3D{0, 1, 0},
		YCount:      2,
		YPitch:      20.0,
	})
	if err != nil {
		t.Fatalf("CreateLinearPattern failed: %v", err)
	}
	if linPat.Name != "Pattern Feature(1)" || linPat.Type != "PatternFeature" {
		t.Errorf("unexpected linear pattern feature: %+v", linPat)
	}

	// 2. Part.CreateCircularPattern
	circPat, err := part.CreateCircularPattern(ctx, CircularPatternParams{
		FeatureRefs:   []protocol.ObjectHandleWire{featRef},
		AxisOrigin:    Point3D{0, 0, 0},
		AxisDirection: Vector3D{0, 0, 1},
		Count:         4,
		PitchAngle:    90.0,
	})
	if err != nil {
		t.Fatalf("CreateCircularPattern failed: %v", err)
	}
	if circPat.Name != "Circular Pattern(1)" || circPat.Type != "PatternFeature" {
		t.Errorf("unexpected circular pattern feature: %+v", circPat)
	}

	// 3. Feature.CreateLinearPattern convenience method
	featLinPat, err := feat.CreateLinearPattern(ctx, LinearPatternParams{
		XCount: 2,
		XPitch: 10.0,
	})
	if err != nil {
		t.Fatalf("Feature.CreateLinearPattern failed: %v", err)
	}
	if featLinPat.Ref.ObjectID != "feat-pat-1" {
		t.Errorf("unexpected feature ref: %+v", featLinPat)
	}

	// 4. Feature.CreateCircularPattern convenience method
	featCircPat, err := feat.CreateCircularPattern(ctx, CircularPatternParams{
		Count:      6,
		PitchAngle: 60.0,
	})
	if err != nil {
		t.Fatalf("Feature.CreateCircularPattern failed: %v", err)
	}
	if featCircPat.Ref.ObjectID != "feat-pat-1" {
		t.Errorf("unexpected feature ref: %+v", featCircPat)
	}
}

func TestPatternValidationFailClosed(t *testing.T) {
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

	featRef := protocol.ObjectHandleWire{
		SessionID:  "sess-1",
		Epoch:      1,
		ObjectID:   "hole-1",
		Generation: 1,
		Kind:       "Feature",
	}

	badKindRef := protocol.ObjectHandleWire{
		SessionID:  "sess-1",
		Epoch:      1,
		ObjectID:   "body-1",
		Generation: 1,
		Kind:       "Body", // expected Feature
	}

	// 1. Empty feature refs
	if _, err := part.CreateLinearPattern(ctx, LinearPatternParams{XCount: 2, XPitch: 10}); err == nil {
		t.Error("expected error on empty feature refs")
	}

	// 2. Bad feature kind
	if _, err := part.CreateLinearPattern(ctx, LinearPatternParams{
		FeatureRefs: []protocol.ObjectHandleWire{badKindRef},
		XCount:      2,
		XPitch:      10,
	}); err == nil {
		t.Error("expected error on bad feature kind")
	}

	// 3. Linear count < 2 in both directions
	if _, err := part.CreateLinearPattern(ctx, LinearPatternParams{
		FeatureRefs: []protocol.ObjectHandleWire{featRef},
		XCount:      1,
		YCount:      1,
	}); err == nil {
		t.Error("expected error when count < 2 in both directions")
	}

	// 4. Linear count >= 2 with pitch <= 0
	if _, err := part.CreateLinearPattern(ctx, LinearPatternParams{
		FeatureRefs: []protocol.ObjectHandleWire{featRef},
		XCount:      2,
		XPitch:      0,
	}); err == nil {
		t.Error("expected error when x_pitch <= 0")
	}

	// 5. Circular count < 2
	if _, err := part.CreateCircularPattern(ctx, CircularPatternParams{
		FeatureRefs: []protocol.ObjectHandleWire{featRef},
		Count:       1,
		PitchAngle:  90,
	}); err == nil {
		t.Error("expected error on circular count < 2")
	}

	// 6. Circular pitch angle <= 0
	if _, err := part.CreateCircularPattern(ctx, CircularPatternParams{
		FeatureRefs: []protocol.ObjectHandleWire{featRef},
		Count:       4,
		PitchAngle:  0,
	}); err == nil {
		t.Error("expected error on pitch angle <= 0")
	}

	// 7. Circular pitch angle > 360
	if _, err := part.CreateCircularPattern(ctx, CircularPatternParams{
		FeatureRefs: []protocol.ObjectHandleWire{featRef},
		Count:       4,
		PitchAngle:  361,
	}); err == nil {
		t.Error("expected error on pitch angle > 360")
	}
}

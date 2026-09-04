package nxgo

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/transport/pipe"
)

func TestSketchAndProfileProtocol(t *testing.T) {
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
			case "sketch.create":
				resp := protocol.SketchCreateResponse{
					SketchRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   "sketch-1",
						Generation: 1,
						Kind:       "Sketch",
					},
					FeatureRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   "feat-sketch-1",
						Generation: 1,
						Kind:       "Feature",
					},
					Name: "SKETCH_001",
				}
				respPayload, _ = protocol.EncodePayload(resp)

			case "sketch.add_geometry":
				gReq, _ := protocol.DecodePayload[protocol.SketchAddGeometryRequest](req.Payload)
				added := len(gReq.Lines) + len(gReq.Circles) + len(gReq.Arcs) + (len(gReq.Rectangles) * 4)
				resp := protocol.SketchAddGeometryResponse{
					AddedCount: added,
					CurveCount: added,
				}
				respPayload, _ = protocol.EncodePayload(resp)

			case "sketch.query_status":
				resp := protocol.SketchQueryStatusResponse{
					Status:     "under_constrained",
					DOFNeeded:  2,
					CurveCount: 4,
				}
				respPayload, _ = protocol.EncodePayload(resp)

			case "sketch.create_profile":
				resp := protocol.ProfileCreateResponse{
					ProfileRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   "prof-1",
						Generation: 1,
						Kind:       "Profile",
					},
					Name:      "Profile_1",
					LoopCount: 1,
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

	// 1. CreateSketch
	sketch, err := part.CreateSketch(ctx, SketchParams{
		Name: "SKETCH_001",
	})
	if err != nil {
		t.Fatalf("CreateSketch failed: %v", err)
	}
	if sketch.Name != "SKETCH_001" {
		t.Errorf("expected sketch name SKETCH_001, got %s", sketch.Name)
	}
	if sketch.Ref.Kind != "Sketch" {
		t.Errorf("expected sketch ref kind Sketch, got %s", sketch.Ref.Kind)
	}

	// 2. Add individual geometries
	if err := sketch.AddLine(ctx, [2]float64{0, 0}, [2]float64{100, 0}); err != nil {
		t.Fatalf("AddLine failed: %v", err)
	}
	if err := sketch.AddCircle(ctx, [2]float64{50, 50}, 25.0); err != nil {
		t.Fatalf("AddCircle failed: %v", err)
	}
	if err := sketch.AddArc(ctx, [2]float64{0, 0}, 10.0, 0, 1.57); err != nil {
		t.Fatalf("AddArc failed: %v", err)
	}
	if err := sketch.AddRectangle(ctx, [2]float64{10, 10}, 50, 30); err != nil {
		t.Fatalf("AddRectangle failed: %v", err)
	}

	// 3. Batch AddGeometry
	addResp, err := sketch.AddGeometry(ctx, SketchAddGeometryParams{
		Lines: []SketchLine2D{
			{Start: [2]float64{0, 0}, End: [2]float64{10, 0}},
			{Start: [2]float64{10, 0}, End: [2]float64{10, 10}},
		},
	})
	if err != nil {
		t.Fatalf("Batch AddGeometry failed: %v", err)
	}
	if addResp.AddedCount != 2 {
		t.Errorf("expected 2 added curves, got %d", addResp.AddedCount)
	}

	// 4. QueryStatus
	status, err := sketch.QueryStatus(ctx)
	if err != nil {
		t.Fatalf("QueryStatus failed: %v", err)
	}
	if status.Status != "under_constrained" {
		t.Errorf("expected under_constrained status, got %s", status.Status)
	}
	if status.DOFNeeded != 2 {
		t.Errorf("expected 2 DOF needed, got %d", status.DOFNeeded)
	}

	// 5. CreateProfile
	profile, err := sketch.CreateProfile(ctx, ProfileParams{
		ChainingTolerance: 0.02,
		DistanceTolerance: 0.02,
	})
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}
	if profile.Name != "Profile_1" {
		t.Errorf("expected profile name Profile_1, got %s", profile.Name)
	}
	if profile.LoopCount != 1 {
		t.Errorf("expected 1 loop count, got %d", profile.LoopCount)
	}
}

func TestSketchValidationFailClosed(t *testing.T) {
	ctx := context.Background()

	// Nil session
	partNil := &Part{
		session: nil,
	}
	_, err := partNil.CreateSketch(ctx, SketchParams{})
	if err == nil {
		t.Fatal("expected error on nil session")
	}

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

	// Invalid plane ref kind
	badPlane := &protocol.ObjectHandleWire{
		SessionID:  "sess-1",
		Epoch:      1,
		ObjectID:   "body-1",
		Generation: 1,
		Kind:       "Body", // not DatumPlane or Face
	}
	_, err = part.CreateSketch(ctx, SketchParams{PlaneRef: badPlane})
	if err == nil {
		t.Fatal("expected error with bad plane ref kind")
	}

	// Sketch geometry dimension validation
	sketch := &Sketch{
		part:    part,
		session: session,
		Ref: protocol.ObjectHandleWire{
			SessionID:  "sess-1",
			Epoch:      1,
			ObjectID:   "sketch-1",
			Generation: 1,
			Kind:       "Sketch",
		},
	}
	if err := sketch.AddCircle(ctx, [2]float64{0, 0}, -5.0); err == nil {
		t.Error("expected error for negative circle radius")
	}
	if err := sketch.AddArc(ctx, [2]float64{0, 0}, 0, 0, 1); err == nil {
		t.Error("expected error for zero arc radius")
	}
	if err := sketch.AddRectangle(ctx, [2]float64{0, 0}, -10, 20); err == nil {
		t.Error("expected error for negative rectangle dimensions")
	}
}

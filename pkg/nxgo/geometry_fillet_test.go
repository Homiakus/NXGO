package nxgo

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/transport/pipe"
)

func TestFilletAndChamferProtocol(t *testing.T) {
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
			case "feature.create_fillet":
				resp := protocol.FeatureCreateFilletResponse{
					FeatureRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   "feat-fillet-1",
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
					FeatureName: "Edge Blend(1)",
					FeatureType: "EdgeBlend",
				}
				respPayload, _ = protocol.EncodePayload(resp)

			case "feature.create_chamfer":
				resp := protocol.FeatureCreateChamferResponse{
					FeatureRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   "feat-chamfer-1",
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
					FeatureName: "Chamfer(1)",
					FeatureType: "Chamfer",
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

	bodyRef := protocol.ObjectHandleWire{
		SessionID:  "sess-1",
		Epoch:      1,
		ObjectID:   "body-1",
		Generation: 1,
		Kind:       "Body",
	}

	edgeRef := protocol.ObjectHandleWire{
		SessionID:  "sess-1",
		Epoch:      1,
		ObjectID:   "edge-1",
		Generation: 1,
		Kind:       "Edge",
	}

	// 1. CreateFillet with BodyRef
	featFillet, err := part.CreateFillet(ctx, FilletParams{
		BodyRef: &bodyRef,
		Radius:  5.0,
	})
	if err != nil {
		t.Fatalf("CreateFillet failed: %v", err)
	}
	if featFillet.Name != "Edge Blend(1)" || featFillet.Type != "EdgeBlend" {
		t.Errorf("unexpected fillet feature: %+v", featFillet)
	}

	// 2. CreateFillet with EdgeRefs
	featFilletEdges, err := part.CreateFillet(ctx, FilletParams{
		EdgeRefs: []protocol.ObjectHandleWire{edgeRef},
		Radius:   3.0,
	})
	if err != nil {
		t.Fatalf("CreateFillet with edge refs failed: %v", err)
	}
	if featFilletEdges.Ref.Kind != "Feature" {
		t.Errorf("unexpected fillet ref kind: %s", featFilletEdges.Ref.Kind)
	}

	// 3. CreateChamfer Symmetric
	featChamferSym, err := part.CreateChamfer(ctx, ChamferParams{
		BodyRef:  &bodyRef,
		Distance: 2.0,
		Option:   "symmetric",
	})
	if err != nil {
		t.Fatalf("CreateChamfer symmetric failed: %v", err)
	}
	if featChamferSym.Name != "Chamfer(1)" || featChamferSym.Type != "Chamfer" {
		t.Errorf("unexpected chamfer feature: %+v", featChamferSym)
	}

	// 4. CreateChamfer Two Offsets
	featChamferTwo, err := part.CreateChamfer(ctx, ChamferParams{
		BodyRef:        &bodyRef,
		Distance:       2.0,
		SecondDistance: 4.0,
		Option:         "two_offsets",
	})
	if err != nil {
		t.Fatalf("CreateChamfer two_offsets failed: %v", err)
	}
	if featChamferTwo.Ref.ObjectID != "feat-chamfer-1" {
		t.Errorf("unexpected chamfer ref: %+v", featChamferTwo)
	}

	// 5. CreateChamfer Offset and Angle
	featChamferAng, err := part.CreateChamfer(ctx, ChamferParams{
		BodyRef:  &bodyRef,
		Distance: 2.0,
		Angle:    30.0,
		Option:   "offset_and_angle",
	})
	if err != nil {
		t.Fatalf("CreateChamfer offset_and_angle failed: %v", err)
	}
	if featChamferAng.Ref.ObjectID != "feat-chamfer-1" {
		t.Errorf("unexpected chamfer ref: %+v", featChamferAng)
	}
}

func TestFilletChamferValidationFailClosed(t *testing.T) {
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

	bodyRef := protocol.ObjectHandleWire{
		SessionID:  "sess-1",
		Epoch:      1,
		ObjectID:   "body-1",
		Generation: 1,
		Kind:       "Body",
	}

	badKindRef := protocol.ObjectHandleWire{
		SessionID:  "sess-1",
		Epoch:      1,
		ObjectID:   "sketch-1",
		Generation: 1,
		Kind:       "Sketch",
	}

	// Fillet zero/negative radius
	if _, err := part.CreateFillet(ctx, FilletParams{BodyRef: &bodyRef, Radius: 0}); err == nil {
		t.Error("expected error on zero radius")
	}
	if _, err := part.CreateFillet(ctx, FilletParams{BodyRef: &bodyRef, Radius: -2}); err == nil {
		t.Error("expected error on negative radius")
	}

	// Fillet missing body and edge refs
	if _, err := part.CreateFillet(ctx, FilletParams{Radius: 5}); err == nil {
		t.Error("expected error on missing body and edge refs")
	}

	// Fillet bad edge kind
	if _, err := part.CreateFillet(ctx, FilletParams{EdgeRefs: []protocol.ObjectHandleWire{badKindRef}, Radius: 5}); err == nil {
		t.Error("expected error on bad edge kind")
	}

	// Chamfer zero/negative distance
	if _, err := part.CreateChamfer(ctx, ChamferParams{BodyRef: &bodyRef, Distance: 0}); err == nil {
		t.Error("expected error on zero chamfer distance")
	}
	if _, err := part.CreateChamfer(ctx, ChamferParams{BodyRef: &bodyRef, Distance: -3}); err == nil {
		t.Error("expected error on negative chamfer distance")
	}

	// Chamfer invalid option
	if _, err := part.CreateChamfer(ctx, ChamferParams{BodyRef: &bodyRef, Distance: 5, Option: "invalid_option"}); err == nil {
		t.Error("expected error on invalid chamfer option")
	}

	// Chamfer two_offsets with non-positive second distance
	if _, err := part.CreateChamfer(ctx, ChamferParams{BodyRef: &bodyRef, Distance: 5, SecondDistance: 0, Option: "two_offsets"}); err == nil {
		t.Error("expected error on zero second_distance for two_offsets")
	}

	// Chamfer offset_and_angle with out of range angle
	if _, err := part.CreateChamfer(ctx, ChamferParams{BodyRef: &bodyRef, Distance: 5, Angle: 0, Option: "offset_and_angle"}); err == nil {
		t.Error("expected error on 0 angle for offset_and_angle")
	}
	if _, err := part.CreateChamfer(ctx, ChamferParams{BodyRef: &bodyRef, Distance: 5, Angle: 95, Option: "offset_and_angle"}); err == nil {
		t.Error("expected error on 95 angle for offset_and_angle")
	}
}

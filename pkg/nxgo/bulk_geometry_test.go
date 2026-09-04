package nxgo

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"testing"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/transport/pipe"
)

func setupBulkTestSession(t *testing.T, responder func(op string, reqPayload []byte) ([]byte, error)) (*Session, func()) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
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

			var resp protocol.ResponseEnvelope
			resp.RequestID = req.RequestID

			payloadBytes, err := responder(req.Op, req.Payload)
			if err != nil {
				resp.Status = protocol.StatusError
				resp.Error = &protocol.ErrorEnvelope{
					Category: "OperationFailed",
					Message:  err.Error(),
					Op:       req.Op,
				}
			} else {
				resp.Status = protocol.StatusOK
				resp.Payload = payloadBytes
			}

			respBytes, err := protocol.EncodePayload(resp)
			if err != nil {
				return
			}
			if err := serverFramed.Send(respBytes); err != nil {
				return
			}
		}
	}()

	client := pipe.NewClient(clientConn)
	session := WrapClient(client, "test-session", 1, "v2512")

	cleanup := func() {
		_ = session.Close()
		_ = clientConn.Close()
		_ = serverConn.Close()
	}

	return session, cleanup
}

func TestPartBulkGeometryAnalysis(t *testing.T) {
	session, cleanup := setupBulkTestSession(t, func(op string, reqPayload []byte) ([]byte, error) {
		if op != "geometry.query_bulk_analysis" {
			return nil, fmt.Errorf("unexpected op: %s", op)
		}

		var requestPayload protocol.GeometryQueryBulkAnalysisRequest
		if err := json.Unmarshal(reqPayload, &requestPayload); err != nil {
			return nil, err
		}

		if requestPayload.PartRef == nil || requestPayload.PartRef.ObjectID != "part-1" {
			return nil, fmt.Errorf("expected part-1 in PartRef, got %+v", requestPayload.PartRef)
		}

		resp := protocol.GeometryQueryBulkAnalysisResponse{
			Units:      "mm",
			BodyCount:  2,
			SolidCount: 2,
			SheetCount: 0,
			AggregateMass: &protocol.GeometryQueryMassPropertiesResponse{
				Units:     "mm",
				Volume:    400.0,
				Area:      200.0,
				Mass:      8.0,
				Centroid:  [3]float64{15.0, 7.5, 0.0},
				SolidType: "aggregate",
			},
			AggregateBox: &protocol.GeometryQueryBoundingBoxResponse{
				Units:      "mm",
				MinCorner:  [3]float64{-1.0, -2.0, 0.0},
				MaxCorner:  [3]float64{20.0, 8.0, 10.0},
				Dimensions: [3]float64{21.0, 10.0, 10.0},
			},
			Bodies: []protocol.BodyGeometryAnalysisWire{
				{
					BodyRef: &protocol.ObjectHandleWire{
						SessionID:  "test-session",
						Epoch:      1,
						ObjectID:   "body-1",
						Generation: 1,
						Kind:       "Body",
					},
					Name:      "BlockBody",
					SolidType: "solid",
					FaceCount: 6,
					EdgeCount: 12,
					NativeTag: 101,
					MassProperties: &protocol.GeometryQueryMassPropertiesResponse{
						Units:     "mm",
						Volume:    100.0,
						Area:      60.0,
						Mass:      2.0,
						Centroid:  [3]float64{0.0, 0.0, 0.0},
						SolidType: "solid",
					},
					BoundingBox: &protocol.GeometryQueryBoundingBoxResponse{
						Units:      "mm",
						MinCorner:  [3]float64{-1.0, -1.0, 0.0},
						MaxCorner:  [3]float64{9.0, 9.0, 10.0},
						Dimensions: [3]float64{10.0, 10.0, 10.0},
					},
				},
				{
					BodyRef: &protocol.ObjectHandleWire{
						SessionID:  "test-session",
						Epoch:      1,
						ObjectID:   "body-2",
						Generation: 1,
						Kind:       "Body",
					},
					Name:      "CylinderBody",
					SolidType: "solid",
					FaceCount: 3,
					EdgeCount: 2,
					NativeTag: 102,
					MassProperties: &protocol.GeometryQueryMassPropertiesResponse{
						Units:     "mm",
						Volume:    300.0,
						Area:      140.0,
						Mass:      6.0,
						Centroid:  [3]float64{20.0, 10.0, 0.0},
						SolidType: "solid",
					},
					BoundingBox: &protocol.GeometryQueryBoundingBoxResponse{
						Units:      "mm",
						MinCorner:  [3]float64{10.0, -2.0, 0.0},
						MaxCorner:  [3]float64{20.0, 8.0, 10.0},
						Dimensions: [3]float64{10.0, 10.0, 10.0},
					},
				},
			},
		}

		return json.Marshal(resp)
	})
	defer cleanup()

	part := &Part{
		session: session,
		Ref: protocol.ObjectHandleWire{
			SessionID:  "test-session",
			Epoch:      1,
			ObjectID:   "part-1",
			Generation: 1,
			Kind:       "Part",
		},
		Name: "test_part.prt",
	}

	ctx := context.Background()
	analysis, err := part.AnalyzeGeometry(ctx)
	if err != nil {
		t.Fatalf("AnalyzeGeometry failed: %v", err)
	}

	if analysis.Units != "mm" {
		t.Errorf("expected mm, got %s", analysis.Units)
	}
	if analysis.BodyCount != 2 || analysis.SolidCount != 2 || analysis.SheetCount != 0 {
		t.Errorf("unexpected body counts: total=%d, solid=%d, sheet=%d", analysis.BodyCount, analysis.SolidCount, analysis.SheetCount)
	}
	if analysis.AggregateMass == nil || analysis.AggregateMass.Volume != 400.0 || analysis.AggregateMass.Mass != 8.0 {
		t.Errorf("unexpected aggregate mass: %+v", analysis.AggregateMass)
	}
	if math.Abs(analysis.AggregateMass.Centroid[0]-15.0) > 1e-9 || math.Abs(analysis.AggregateMass.Centroid[1]-7.5) > 1e-9 {
		t.Errorf("unexpected aggregate centroid: %+v", analysis.AggregateMass.Centroid)
	}
	if analysis.AggregateBox == nil || analysis.AggregateBox.Dimensions[0] != 21.0 {
		t.Errorf("unexpected aggregate box: %+v", analysis.AggregateBox)
	}
	if len(analysis.Bodies) != 2 {
		t.Fatalf("expected 2 bodies, got %d", len(analysis.Bodies))
	}
	if analysis.Bodies[0].Name != "BlockBody" || analysis.Bodies[0].FaceCount != 6 || analysis.Bodies[0].EdgeCount != 12 {
		t.Errorf("unexpected body 0 info: %+v", analysis.Bodies[0])
	}
	if analysis.Bodies[1].Name != "CylinderBody" || analysis.Bodies[1].NativeTag != 102 {
		t.Errorf("unexpected body 1 info: %+v", analysis.Bodies[1])
	}

	// Test BulkMassProperties shortcut
	mass, err := part.BulkMassProperties(ctx)
	if err != nil {
		t.Fatalf("BulkMassProperties failed: %v", err)
	}
	if mass.Volume != 400.0 || mass.Mass != 8.0 {
		t.Errorf("unexpected bulk mass: %+v", mass)
	}

	// Test BulkBoundingBox shortcut
	box, err := part.BulkBoundingBox(ctx)
	if err != nil {
		t.Fatalf("BulkBoundingBox failed: %v", err)
	}
	if box.Dimensions[0] != 21.0 || box.Dimensions[1] != 10.0 {
		t.Errorf("unexpected bulk box: %+v", box)
	}
}

func TestBulkGeometryAnalysisFailClosed(t *testing.T) {
	ctx := context.Background()

	var nilPart *Part
	_, err := nilPart.AnalyzeGeometry(ctx)
	if err != ErrSessionClosed {
		t.Errorf("expected ErrSessionClosed for nil part, got %v", err)
	}

	closedPart := &Part{
		session: &Session{},
		Ref: protocol.ObjectHandleWire{
			SessionID:  "test-session",
			Epoch:      1,
			ObjectID:   "part-1",
			Generation: 1,
			Kind:       "Part",
		},
	}
	_, err = closedPart.AnalyzeGeometry(ctx)
	if err != ErrSessionClosed {
		t.Errorf("expected ErrSessionClosed for closed session part, got %v", err)
	}

	// Stale handle validation
	liveSession, liveCleanup := setupBulkTestSession(t, func(op string, reqPayload []byte) ([]byte, error) {
		return nil, fmt.Errorf("should not be called")
	})
	defer liveCleanup()

	stalePart := &Part{
		session: liveSession,
		Ref: protocol.ObjectHandleWire{
			SessionID:  "other-session",
			Epoch:      1,
			ObjectID:   "part-1",
			Generation: 1,
			Kind:       "Part",
		},
	}
	_, err = stalePart.AnalyzeGeometry(ctx)
	if err == nil {
		t.Errorf("expected error for foreign session handle, got nil")
	}

	// Session.AnalyzeBodies with nil body
	_, err = liveSession.AnalyzeBodies(ctx, []*Body{nil})
	if err == nil {
		t.Errorf("expected error for nil body in slice, got nil")
	}

	// Session.AnalyzeBodies with empty slice returns empty analysis without transport call
	emptyAnalysis, err := liveSession.AnalyzeBodies(ctx, []*Body{})
	if err != nil {
		t.Fatalf("unexpected error for empty bodies: %v", err)
	}
	if emptyAnalysis.BodyCount != 0 {
		t.Errorf("expected 0 bodies in empty analysis, got %d", emptyAnalysis.BodyCount)
	}
}

func TestSessionAnalyzeBodiesProtocol(t *testing.T) {
	session, cleanup := setupBulkTestSession(t, func(op string, reqPayload []byte) ([]byte, error) {
		if op != "geometry.query_bulk_analysis" {
			return nil, fmt.Errorf("unexpected op: %s", op)
		}

		var requestPayload protocol.GeometryQueryBulkAnalysisRequest
		if err := json.Unmarshal(reqPayload, &requestPayload); err != nil {
			return nil, err
		}

		if len(requestPayload.BodyRefs) != 1 || requestPayload.BodyRefs[0].ObjectID != "body-1" {
			return nil, fmt.Errorf("expected body-1 in BodyRefs, got %+v", requestPayload.BodyRefs)
		}

		resp := protocol.GeometryQueryBulkAnalysisResponse{
			Units:      "mm",
			BodyCount:  1,
			SolidCount: 1,
			SheetCount: 0,
			AggregateMass: &protocol.GeometryQueryMassPropertiesResponse{
				Units:     "mm",
				Volume:    50.0,
				Area:      30.0,
				Mass:      1.0,
				Centroid:  [3]float64{1.0, 2.0, 3.0},
				SolidType: "solid",
			},
			AggregateBox: &protocol.GeometryQueryBoundingBoxResponse{
				Units:      "mm",
				MinCorner:  [3]float64{0.0, 0.0, 0.0},
				MaxCorner:  [3]float64{5.0, 5.0, 2.0},
				Dimensions: [3]float64{5.0, 5.0, 2.0},
			},
			Bodies: []protocol.BodyGeometryAnalysisWire{
				{
					Name:      "SingleBody",
					SolidType: "solid",
					FaceCount: 6,
					EdgeCount: 12,
					NativeTag: 201,
				},
			},
		}

		return json.Marshal(resp)
	})
	defer cleanup()

	body := &Body{
		session: session,
		Ref: protocol.ObjectHandleWire{
			SessionID:  "test-session",
			Epoch:      1,
			ObjectID:   "body-1",
			Generation: 1,
			Kind:       "Body",
		},
	}

	ctx := context.Background()
	analysis, err := session.AnalyzeBodies(ctx, []*Body{body})
	if err != nil {
		t.Fatalf("AnalyzeBodies failed: %v", err)
	}

	if analysis.BodyCount != 1 || analysis.SolidCount != 1 {
		t.Errorf("unexpected counts: %+v", analysis)
	}
	if analysis.AggregateMass == nil || analysis.AggregateMass.Volume != 50.0 {
		t.Errorf("unexpected mass: %+v", analysis.AggregateMass)
	}
}

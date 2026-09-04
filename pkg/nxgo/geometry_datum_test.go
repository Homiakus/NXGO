package nxgo

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/transport/pipe"
)

func TestCreateDatumPlaneAxisCsysProtocol(t *testing.T) {
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
			case "datum.create_plane":
				resp := protocol.DatumCreatePlaneResponse{
					PlaneRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   "dplane-1",
						Generation: 1,
						Kind:       "DatumPlane",
					},
					FeatureRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   "feat-plane-1",
						Generation: 1,
						Kind:       "Feature",
					},
					Name: "Datum Plane(1)",
				}
				respPayload, _ = protocol.EncodePayload(resp)

			case "datum.create_axis":
				resp := protocol.DatumCreateAxisResponse{
					AxisRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   "daxis-1",
						Generation: 1,
						Kind:       "DatumAxis",
					},
					FeatureRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   "feat-axis-1",
						Generation: 1,
						Kind:       "Feature",
					},
					Name: "Datum Axis(1)",
				}
				respPayload, _ = protocol.EncodePayload(resp)

			case "datum.create_csys":
				resp := protocol.DatumCreateCsysResponse{
					CsysRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   "dcsys-1",
						Generation: 1,
						Kind:       "CoordinateSystem",
					},
					FeatureRef: protocol.ObjectHandleWire{
						SessionID:  "sess-1",
						Epoch:      1,
						ObjectID:   "feat-csys-1",
						Generation: 1,
						Kind:       "Feature",
					},
					Name: "Csys(1)",
				}
				respPayload, _ = protocol.EncodePayload(resp)
			}

			respEnv := protocol.ResponseEnvelope{
				RequestID: req.RequestID,
				Status:    protocol.StatusOK,
				Payload:   respPayload,
			}
			rawResp, _ := protocol.EncodePayload(respEnv)
			_ = serverFramed.Send(rawResp)
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

	// 1. Datum Plane
	plane, err := part.CreateDatumPlane(ctx, DatumPlaneParams{
		Origin:    Point3D{0, 0, 50},
		Direction: Vector3D{0, 0, 1},
	})
	if err != nil {
		t.Fatalf("CreateDatumPlane failed: %v", err)
	}
	if plane.Name != "Datum Plane(1)" || plane.Ref.ObjectID != "dplane-1" || plane.FeatureRef.ObjectID != "feat-plane-1" {
		t.Fatalf("unexpected plane: %+v", plane)
	}

	// 2. Datum Axis
	axis, err := part.CreateDatumAxis(ctx, DatumAxisParams{
		Origin:    Point3D{10, 20, 30},
		Direction: Vector3D{1, 0, 0},
	})
	if err != nil {
		t.Fatalf("CreateDatumAxis failed: %v", err)
	}
	if axis.Name != "Datum Axis(1)" || axis.Ref.ObjectID != "daxis-1" || axis.FeatureRef.ObjectID != "feat-axis-1" {
		t.Fatalf("unexpected axis: %+v", axis)
	}

	// 3. Datum Csys
	csys, err := part.CreateDatumCsys(ctx, DatumCsysParams{
		Origin:     Point3D{0, 0, 0},
		XDirection: Vector3D{1, 0, 0},
		YDirection: Vector3D{0, 1, 0},
	})
	if err != nil {
		t.Fatalf("CreateDatumCsys failed: %v", err)
	}
	if csys.Name != "Csys(1)" || csys.Ref.ObjectID != "dcsys-1" || csys.FeatureRef.ObjectID != "feat-csys-1" {
		t.Fatalf("unexpected csys: %+v", csys)
	}
}

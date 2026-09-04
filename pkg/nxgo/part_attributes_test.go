package nxgo

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/transport/pipe"
)

func TestPartBatchAttributesAndBulkMetadata(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	serverFramed := pipe.NewFramedConn(serverConn, protocol.DefaultMaxPayloadBytes)

	// Simulated server responder
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
			case "part.get_attributes":
				resp := protocol.PartGetAttributesResponse{
					Attributes: []protocol.PartAttribute{
						{Title: "Material", Type: protocol.AttrTypeString, Value: "Steel-4140"},
						{Title: "Revision", Type: protocol.AttrTypeInteger, Value: 3},
						{Title: "WeightTarget", Type: protocol.AttrTypeReal, Value: 12.5},
						{Title: "IsApproved", Type: protocol.AttrTypeBoolean, Value: true},
					},
				}
				respPayload, _ = protocol.EncodePayload(resp)

			case "part.set_attributes":
				setReq, _ := protocol.DecodePayload[protocol.PartSetAttributesRequest](req.Payload)
				resp := protocol.PartSetAttributesResponse{
					UpdatedCount: len(setReq.Attributes),
				}
				respPayload, _ = protocol.EncodePayload(resp)

			case "part.bulk_metadata":
				resp := protocol.PartBulkMetadataResponse{
					Entries: []protocol.PartMetadataEntry{
						{
							PartRef: protocol.ObjectHandleWire{
								SessionID:  "sess-1",
								Epoch:      1,
								ObjectID:   "part-1",
								Generation: 1,
								Kind:       "Part",
							},
							Name:         "Bracket",
							FullPath:     "C:\\CAD\\bracket.prt",
							Units:        "Millimeters",
							IsModified:   false,
							BodyCount:    2,
							FeatureCount: 14,
							Attributes: []protocol.PartAttribute{
								{Title: "Material", Type: protocol.AttrTypeString, Value: "Steel-4140"},
							},
						},
					},
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
		Name:  "Bracket",
		Units: "Millimeters",
	}

	// 1. Test GetAttributes
	attrs, err := part.GetAttributes(ctx)
	if err != nil {
		t.Fatalf("GetAttributes failed: %v", err)
	}
	if len(attrs) != 4 {
		t.Fatalf("expected 4 attributes, got %d", len(attrs))
	}
	if attrs[0].Title != "Material" || attrs[0].Value != "Steel-4140" {
		t.Fatalf("unexpected attribute: %+v", attrs[0])
	}
	if attrs[1].Type != protocol.AttrTypeInteger {
		t.Fatalf("unexpected type for Revision: %s", attrs[1].Type)
	}

	// 2. Test SetAttributes
	toSet := []protocol.PartAttribute{
		{Title: "Color", Type: protocol.AttrTypeString, Value: "Blue"},
		{Title: "CostCode", Type: protocol.AttrTypeInteger, Value: 1042},
	}
	updatedCount, err := part.SetAttributes(ctx, toSet)
	if err != nil {
		t.Fatalf("SetAttributes failed: %v", err)
	}
	if updatedCount != 2 {
		t.Fatalf("expected 2 updated attributes, got %d", updatedCount)
	}

	// 3. Test Part.Metadata()
	meta, err := part.Metadata(ctx)
	if err != nil {
		t.Fatalf("Metadata failed: %v", err)
	}
	if meta == nil || meta.Name != "Bracket" || meta.BodyCount != 2 || meta.FeatureCount != 14 {
		t.Fatalf("unexpected metadata entry: %+v", meta)
	}

	// 4. Test Session.BulkMetadata()
	bulk, err := session.BulkMetadata(ctx, part)
	if err != nil {
		t.Fatalf("BulkMetadata failed: %v", err)
	}
	if len(bulk) != 1 || bulk[0].FullPath != "C:\\CAD\\bracket.prt" {
		t.Fatalf("unexpected bulk metadata response: %+v", bulk)
	}
}

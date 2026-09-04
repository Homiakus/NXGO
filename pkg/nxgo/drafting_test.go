package nxgo

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/transport/pipe"
)

func setupDraftingMockSession(t *testing.T, responder func(op string, reqPayload []byte) ([]byte, error)) (*Session, *Part, func()) {
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
	part := &Part{
		session: session,
		Ref: protocol.ObjectHandleWire{
			SessionID:  "test-session",
			Epoch:      1,
			ObjectID:   "part-drafting-1",
			Generation: 1,
			Kind:       "Part",
		},
		Name:  "bracket.prt",
		Units: "mm",
	}

	cleanup := func() {
		client.Close()
		clientConn.Close()
		serverConn.Close()
	}
	return session, part, cleanup
}

func TestDraftingCreateDrawingSheet(t *testing.T) {
	_, part, cleanup := setupDraftingMockSession(t, func(op string, payload []byte) ([]byte, error) {
		if op != "drafting.create_sheet" {
			return nil, fmt.Errorf("unexpected op: %s", op)
		}
		var cr protocol.DraftingCreateSheetRequest
		_ = json.Unmarshal(payload, &cr)
		return protocol.EncodePayload(protocol.DraftingCreateSheetResponse{
			SheetRef: protocol.ObjectHandleWire{
				SessionID:  "test-session",
				Epoch:      1,
				ObjectID:   "sheet-101",
				Generation: 1,
				Kind:       "DrawingSheet",
			},
			SheetName: cr.SheetName,
			Height:    cr.Height,
			Length:    cr.Length,
		})
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sheet, err := part.CreateDrawingSheet(ctx, CreateSheetParams{
		SheetName: "SHEET_1",
		Height:    297.0,
		Length:    420.0,
	})
	if err != nil {
		t.Fatalf("CreateDrawingSheet failed: %v", err)
	}
	if sheet.Name != "SHEET_1" || sheet.Height != 297.0 || sheet.Length != 420.0 {
		t.Fatalf("unexpected sheet properties: %+v", sheet)
	}
	if sheet.Ref.ObjectID != "sheet-101" {
		t.Fatalf("unexpected sheet ref: %+v", sheet.Ref)
	}
}

func TestDraftingExportPDF(t *testing.T) {
	_, part, cleanup := setupDraftingMockSession(t, func(op string, payload []byte) ([]byte, error) {
		if op != "drafting.export_pdf" {
			return nil, fmt.Errorf("unexpected op: %s", op)
		}
		var exp protocol.DraftingExportPDFRequest
		_ = json.Unmarshal(payload, &exp)
		return protocol.EncodePayload(protocol.DraftingExportPDFResponse{
			ExportedPath:  exp.OutputPDFPath,
			FileSizeBytes: 12450,
		})
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := part.ExportPDF(ctx, ExportPDFParams{
		OutputPDFPath: "C:\\out\\bracket.pdf",
		SheetNames:    []string{"SHEET_1"},
		ColorMode:     "black_and_white",
	})
	if err != nil {
		t.Fatalf("ExportPDF failed: %v", err)
	}
	if res.ExportedPath != "C:\\out\\bracket.pdf" || res.FileSizeBytes != 12450 {
		t.Fatalf("unexpected export result: %+v", res)
	}
}

func TestDraftingQuerySheets(t *testing.T) {
	_, part, cleanup := setupDraftingMockSession(t, func(op string, payload []byte) ([]byte, error) {
		if op != "drafting.query_sheets" {
			return nil, fmt.Errorf("unexpected op: %s", op)
		}
		return protocol.EncodePayload(protocol.DraftingQuerySheetsResponse{
			Sheets: []protocol.DraftingSheetInfoWire{
				{
					SheetRef: protocol.ObjectHandleWire{
						SessionID:  "test-session",
						Epoch:      1,
						ObjectID:   "sheet-1",
						Generation: 1,
						Kind:       "DrawingSheet",
					},
					Name:   "SH1",
					Height: 297.0,
					Length: 420.0,
				},
			},
		})
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sheets, err := part.DrawingSheets(ctx)
	if err != nil {
		t.Fatalf("DrawingSheets failed: %v", err)
	}
	if len(sheets) != 1 || sheets[0].Name != "SH1" {
		t.Fatalf("unexpected sheets list: %+v", sheets)
	}
}

func TestDraftingStandardViewsAndNotes(t *testing.T) {
	_, part, cleanup := setupDraftingMockSession(t, func(op string, payload []byte) ([]byte, error) {
		switch op {
		case "drafting.create_sheet":
			return protocol.EncodePayload(protocol.DraftingCreateSheetResponse{
				SheetRef: protocol.ObjectHandleWire{
					SessionID:  "test-session",
					Epoch:      1,
					ObjectID:   "sheet-views-1",
					Generation: 1,
					Kind:       "DrawingSheet",
				},
				SheetName: "SHEET_VIEWS",
				Height:    297.0,
				Length:    420.0,
			})
		case "drafting.create_standard_views":
			var sv protocol.DraftingCreateStandardViewsRequest
			_ = json.Unmarshal(payload, &sv)
			return protocol.EncodePayload(protocol.DraftingCreateStandardViewsResponse{
				Created:   true,
				Layout:    sv.Layout,
				ViewCount: 4,
				Views:     []string{"Front@SHEET_VIEWS", "Top@SHEET_VIEWS", "Right@SHEET_VIEWS", "Isometric@SHEET_VIEWS"},
			})
		case "drafting.add_note":
			var an protocol.DraftingAddNoteRequest
			_ = json.Unmarshal(payload, &an)
			return protocol.EncodePayload(protocol.DraftingAddNoteResponse{
				Added:     true,
				LineCount: len(an.TextLines),
				OriginX:   an.OriginX,
				OriginY:   an.OriginY,
			})
		default:
			return nil, fmt.Errorf("unexpected op: %s", op)
		}
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sheet, err := part.CreateDrawingSheet(ctx, CreateSheetParams{
		SheetName: "SHEET_VIEWS",
		Height:    297.0,
		Length:    420.0,
	})
	if err != nil {
		t.Fatalf("CreateDrawingSheet failed: %v", err)
	}

	viewRes, err := sheet.CreateStandardViews(ctx, StandardViewsParams{
		Layout: "front_top_right_iso",
	})
	if err != nil {
		t.Fatalf("CreateStandardViews failed: %v", err)
	}
	if !viewRes.Created || viewRes.ViewCount != 4 {
		t.Fatalf("unexpected standard views result: %+v", viewRes)
	}

	noteRes, err := sheet.AddNote(ctx, AddNoteParams{
		TextLines: []string{"Line 1", "Line 2"},
		OriginX:   20.0,
		OriginY:   20.0,
		Anchor:    "bottom_left",
	})
	if err != nil {
		t.Fatalf("AddNote failed: %v", err)
	}
	if !noteRes.Added || noteRes.LineCount != 2 {
		t.Fatalf("unexpected note result: %+v", noteRes)
	}

	// Test AddBoundingDimensions
	bbox := &BoundingBox{
		Units:      "mm",
		MinCorner:  Point3D{0, 0, 0},
		MaxCorner:  Point3D{100, 50, 25},
		Dimensions: Point3D{100, 50, 25},
	}
	dimRes, err := sheet.AddBoundingDimensions(ctx, bbox)
	if err != nil {
		t.Fatalf("AddBoundingDimensions failed: %v", err)
	}
	if !dimRes.Added || dimRes.LineCount != 5 {
		t.Fatalf("unexpected dim note result: %+v", dimRes)
	}
}

func TestDraftingGenerateAssemblyDrawing(t *testing.T) {
	_, part, cleanup := setupDraftingMockSession(t, func(op string, payload []byte) ([]byte, error) {
		switch op {
		case "drafting.create_sheet":
			return protocol.EncodePayload(protocol.DraftingCreateSheetResponse{
				SheetRef: protocol.ObjectHandleWire{
					SessionID:  "test-session",
					Epoch:      1,
					ObjectID:   "sheet-asm-1",
					Generation: 1,
					Kind:       "DrawingSheet",
				},
				SheetName: "ASM_SHEET_1",
				Height:    297.0,
				Length:    420.0,
			})
		case "drafting.create_standard_views":
			return protocol.EncodePayload(protocol.DraftingCreateStandardViewsResponse{
				Created:   true,
				Layout:    "front_top_right_iso",
				ViewCount: 4,
				Views:     []string{"Front", "Top", "Right", "Iso"},
			})
		case "assembly.query_bom":
			return protocol.EncodePayload(protocol.AssemblyQueryBOMResponse{
				Items: []protocol.AssemblyBOMItemWire{
					{
						PartName: "bracket.prt",
						Quantity: 2,
					},
					{
						PartName: "pin.prt",
						Quantity: 4,
					},
				},
			})
		case "drafting.add_note":
			var an protocol.DraftingAddNoteRequest
			_ = json.Unmarshal(payload, &an)
			return protocol.EncodePayload(protocol.DraftingAddNoteResponse{
				Added:     true,
				LineCount: len(an.TextLines),
				OriginX:   an.OriginX,
				OriginY:   an.OriginY,
			})
		default:
			return nil, fmt.Errorf("unexpected op: %s", op)
		}
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sheet, err := part.GenerateAssemblyDrawing(ctx, AssemblyDrawingParams{
		SheetSize:        "A3",
		IncludeBOMTable:  true,
		IncludeTechNotes: true,
	})
	if err != nil {
		t.Fatalf("GenerateAssemblyDrawing failed: %v", err)
	}
	if sheet == nil || sheet.Name != "ASM_SHEET_1" {
		t.Fatalf("unexpected assembly drawing sheet: %+v", sheet)
	}
}

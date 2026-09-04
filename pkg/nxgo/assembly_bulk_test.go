package nxgo

import (
	"context"
	"testing"

	"github.com/Homiakus/NXGO/internal/protocol"
)

func TestAssemblyBulkQuery(t *testing.T) {
	ctx := context.Background()

	session, cleanup := setupTestSessionWithResponder(t, func(op string, payload []byte) ([]byte, error) {
		if op != "assembly.query_bulk" {
			return nil, nil
		}
		req, err := protocol.DecodePayload[protocol.AssemblyQueryBulkRequest](payload)
		if err != nil {
			return nil, err
		}

		items := []protocol.AssemblyBulkComponentItem{
			{
				Name:          "Plate_1",
				DisplayName:   "Plate_1",
				PartPath:      "D:/CAD/plate.prt",
				PartName:      "plate",
				Depth:         1,
				IsSuppressed:  false,
				IsLoaded:      true,
				ReferenceSet:  "MODEL",
				NativeTag:     8001,
				Position:      [3]float64{10, 20, 30},
				ChildrenCount: 2,
			},
			{
				Name:          "Bolt_1",
				DisplayName:   "Bolt_1",
				PartPath:      "D:/CAD/bolt.prt",
				PartName:      "bolt",
				Depth:         2,
				IsSuppressed:  true,
				IsLoaded:      false,
				ReferenceSet:  "Entire Part",
				NativeTag:     8002,
				ChildrenCount: 0,
			},
		}

		return protocol.EncodePayload(protocol.AssemblyQueryBulkResponse{
			AssemblyPartRef:  *req.AssemblyPartRef,
			TotalComponents:  10,
			TotalLoaded:      8,
			TotalSuppressed:  2,
			UniquePartsCount: 5,
			Components:       items,
			HasMore:          true,
		})
	})
	defer cleanup()

	part := &Part{
		Ref:     protocol.ObjectHandleWire{SessionID: "test-session", Epoch: 1, ObjectID: "assy-part", Generation: 1, Kind: "Part"},
		session: session,
	}

	summary, err := part.QueryAssemblyBulk(ctx, AssemblyBulkFilter{
		MaxDepth:           3,
		IncludeSuppressed:  true,
		IncludeTransforms:  true,
		IncludeBoundingBox: true,
		Offset:             0,
		Limit:              2,
	})
	if err != nil {
		t.Fatalf("QueryAssemblyBulk failed: %v", err)
	}

	if summary.TotalComponents != 10 {
		t.Errorf("expected 10 total components, got %d", summary.TotalComponents)
	}
	if summary.TotalLoaded != 8 {
		t.Errorf("expected 8 loaded, got %d", summary.TotalLoaded)
	}
	if summary.TotalSuppressed != 2 {
		t.Errorf("expected 2 suppressed, got %d", summary.TotalSuppressed)
	}
	if summary.UniquePartsCount != 5 {
		t.Errorf("expected 5 unique parts, got %d", summary.UniquePartsCount)
	}
	if !summary.HasMore {
		t.Errorf("expected HasMore true")
	}
	if len(summary.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(summary.Components))
	}
	if summary.Components[0].Name != "Plate_1" || summary.Components[0].IsSuppressed {
		t.Errorf("unexpected component 0: %+v", summary.Components[0])
	}
	if summary.Components[1].Name != "Bolt_1" || !summary.Components[1].IsSuppressed {
		t.Errorf("unexpected component 1: %+v", summary.Components[1])
	}
}

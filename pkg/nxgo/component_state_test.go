package nxgo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
)

func TestComponentStateSuppressionAndLoad(t *testing.T) {
	compState := protocol.AssemblyQueryComponentStateResponse{
		ComponentRef: protocol.ObjectHandleWire{
			SessionID: "test-session", Epoch: 1, ObjectID: "comp-1", Generation: 1, Kind: "Component",
		},
		Name:         "Bolt_M6",
		IsSuppressed: false,
		IsLoaded:     true,
		ReferenceSet: "MODEL",
		NativeTag:    7001,
	}

	session, cleanup := setupTestSessionWithResponder(t, func(op string, payload []byte) ([]byte, error) {
		switch op {
		case "assembly.query_component_state":
			req, err := protocol.DecodePayload[protocol.AssemblyQueryComponentStateRequest](payload)
			if err != nil {
				return nil, err
			}
			if req.ComponentRef.ObjectID != compState.ComponentRef.ObjectID {
				return nil, errors.New("component not found")
			}
			return protocol.EncodePayload(compState)

		case "assembly.suppress_components":
			req, err := protocol.DecodePayload[protocol.AssemblySuppressComponentsRequest](payload)
			if err != nil {
				return nil, err
			}
			compState.IsSuppressed = true
			return protocol.EncodePayload(protocol.AssemblySuppressComponentsResponse{
				SuppressedCount: len(req.ComponentRefs),
			})

		case "assembly.unsuppress_components":
			req, err := protocol.DecodePayload[protocol.AssemblyUnsuppressComponentsRequest](payload)
			if err != nil {
				return nil, err
			}
			compState.IsSuppressed = false
			return protocol.EncodePayload(protocol.AssemblyUnsuppressComponentsResponse{
				UnsuppressedCount: len(req.ComponentRefs),
			})

		case "assembly.close_components":
			req, err := protocol.DecodePayload[protocol.AssemblyCloseComponentsRequest](payload)
			if err != nil {
				return nil, err
			}
			compState.IsLoaded = false
			return protocol.EncodePayload(protocol.AssemblyCloseComponentsResponse{
				ClosedCount: len(req.ComponentRefs),
			})

		case "assembly.open_components":
			req, err := protocol.DecodePayload[protocol.AssemblyOpenComponentsRequest](payload)
			if err != nil {
				return nil, err
			}
			compState.IsLoaded = true
			return protocol.EncodePayload(protocol.AssemblyOpenComponentsResponse{
				OpenedCount: len(req.ComponentRefs),
			})

		default:
			return nil, errors.New("unknown op: " + op)
		}
	})
	defer cleanup()

	part := &Part{
		session: session,
		Ref: protocol.ObjectHandleWire{
			SessionID: "test-session", Epoch: 1, ObjectID: "part-1", Generation: 1, Kind: "Part",
		},
		Name: "test_assembly.prt",
	}

	comp := &Component{
		session: session,
		part:    part,
		Ref: protocol.ObjectHandleWire{
			SessionID: "test-session", Epoch: 1, ObjectID: "comp-1", Generation: 1, Kind: "Component",
		},
		Name: "Bolt_M6",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. Query State
	st, err := comp.State(ctx)
	if err != nil {
		t.Fatalf("State query failed: %v", err)
	}
	if st.IsSuppressed || !st.IsLoaded || st.ReferenceSet != "MODEL" {
		t.Errorf("unexpected state: %+v", st)
	}

	// 2. Suppress
	if err := comp.Suppress(ctx); err != nil {
		t.Fatalf("Suppress failed: %v", err)
	}
	st, _ = comp.State(ctx)
	if !st.IsSuppressed {
		t.Errorf("expected component to be suppressed")
	}

	// 3. Unsuppress
	if err := comp.Unsuppress(ctx); err != nil {
		t.Fatalf("Unsuppress failed: %v", err)
	}
	st, _ = comp.State(ctx)
	if st.IsSuppressed {
		t.Errorf("expected component to not be suppressed")
	}

	// 4. Close component (unload)
	if err := comp.Close(ctx, false, false); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	st, _ = comp.State(ctx)
	if st.IsLoaded {
		t.Errorf("expected component to not be loaded")
	}

	// 5. Open component (reload)
	if err := comp.Open(ctx); err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	st, _ = comp.State(ctx)
	if !st.IsLoaded {
		t.Errorf("expected component to be loaded")
	}
}

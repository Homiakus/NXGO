package nxgo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
)

func TestAssemblyCreateConstraint(t *testing.T) {
	createdConstraints := make(map[string]protocol.AssemblyConstraintInfoWire)

	session, cleanup := setupTestSessionWithResponder(t, func(op string, payload []byte) ([]byte, error) {
		switch op {
		case "assembly.create_constraint":
			req, err := protocol.DecodePayload[protocol.AssemblyCreateConstraintRequest](payload)
			if err != nil {
				return nil, err
			}
			if req.Type == "" {
				return nil, errors.New("type required")
			}
			if req.TargetRef1.ObjectID == "" {
				return nil, errors.New("target 1 required")
			}
			if req.Type != protocol.ConstraintTypeFix && req.TargetRef2 == nil {
				return nil, errors.New("target 2 required for non-fix constraint")
			}

			cName := req.Name
			if cName == "" {
				cName = "Constraint_1"
			}
			ref := protocol.ObjectHandleWire{
				SessionID:  "test-session",
				Epoch:      1,
				ObjectID:   "constraint-" + cName,
				Generation: 1,
				Kind:       "ComponentConstraint",
			}

			info := protocol.AssemblyConstraintInfoWire{
				ConstraintRef: ref,
				Name:          cName,
				Type:          string(req.Type),
				Alignment:     string(req.Alignment),
				Status:        "Solved",
				Suppressed:    false,
				NativeTag:     8888,
			}
			createdConstraints[ref.ObjectID] = info

			return protocol.EncodePayload(protocol.AssemblyCreateConstraintResponse{
				ConstraintRef: ref,
				Name:          cName,
				Type:          string(req.Type),
				Alignment:     string(req.Alignment),
				Status:        "Solved",
				NativeTag:     8888,
			})
		default:
			return nil, errors.New("unknown op: " + op)
		}
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
		Name: "test_assembly.prt",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	target1 := protocol.ObjectHandleWire{
		SessionID:  "test-session",
		Epoch:      1,
		ObjectID:   "comp-1",
		Generation: 1,
		Kind:       "Component",
	}
	target2 := protocol.ObjectHandleWire{
		SessionID:  "test-session",
		Epoch:      1,
		ObjectID:   "comp-2",
		Generation: 1,
		Kind:       "Component",
	}

	// 1. Touch constraint
	c1, err := part.CreateConstraint(ctx, CreateConstraintParams{
		Type:      ConstraintTouch,
		Alignment: AlignInfer,
		Target1:   target1,
		Target2:   &target2,
		Name:      "Touch_1",
	})
	if err != nil {
		t.Fatalf("CreateConstraint touch failed: %v", err)
	}
	if c1.Name != "Touch_1" || c1.Type != "touch" || c1.Status != "Solved" {
		t.Errorf("unexpected constraint c1: %+v", c1)
	}

	// 2. Fix constraint (single target)
	c2, err := part.CreateConstraint(ctx, CreateConstraintParams{
		Type:    ConstraintFix,
		Target1: target1,
		Name:    "Fix_Comp1",
	})
	if err != nil {
		t.Fatalf("CreateConstraint fix failed: %v", err)
	}
	if c2.Type != "fix" || c2.Name != "Fix_Comp1" {
		t.Errorf("unexpected constraint c2: %+v", c2)
	}

	// 3. Distance constraint with offset
	c3, err := part.CreateConstraint(ctx, CreateConstraintParams{
		Type:      ConstraintDistance,
		Alignment: AlignCo,
		Target1:   target1,
		Target2:   &target2,
		Offset:    25.4,
		Name:      "Dist_25",
	})
	if err != nil {
		t.Fatalf("CreateConstraint distance failed: %v", err)
	}
	if c3.Type != "distance" || c3.Alignment != "co_align" {
		t.Errorf("unexpected constraint c3: %+v", c3)
	}

	// 4. Concentric constraint
	c4, err := part.CreateConstraint(ctx, CreateConstraintParams{
		Type:    ConstraintConcentric,
		Target1: target1,
		Target2: &target2,
	})
	if err != nil {
		t.Fatalf("CreateConstraint concentric failed: %v", err)
	}
	if c4.Type != "concentric" {
		t.Errorf("unexpected constraint c4: %+v", c4)
	}

	// 5. Validation error: missing target 2 for non-fix constraint
	_, err = part.CreateConstraint(ctx, CreateConstraintParams{
		Type:    ConstraintParallel,
		Target1: target1,
	})
	if err == nil {
		t.Errorf("expected error for non-fix constraint without target 2, got nil")
	}

	// 6. Validation error: empty target 1
	_, err = part.CreateConstraint(ctx, CreateConstraintParams{
		Type: ConstraintFix,
	})
	if err == nil {
		t.Errorf("expected error for empty target 1, got nil")
	}
}

func TestAssemblyQueryDeleteAndSuppressConstraints(t *testing.T) {
	constraints := map[string]protocol.AssemblyConstraintInfoWire{
		"c-1": {
			ConstraintRef: protocol.ObjectHandleWire{
				SessionID: "test-session", Epoch: 1, ObjectID: "c-1", Generation: 1, Kind: "ComponentConstraint",
			},
			Name:       "Touch_A",
			Type:       "touch",
			Alignment:  "infer_align",
			Status:     "Solved",
			Suppressed: false,
			NativeTag:  1001,
		},
		"c-2": {
			ConstraintRef: protocol.ObjectHandleWire{
				SessionID: "test-session", Epoch: 1, ObjectID: "c-2", Generation: 1, Kind: "ComponentConstraint",
			},
			Name:       "Align_B",
			Type:       "concentric",
			Alignment:  "co_align",
			Status:     "Solved",
			Suppressed: false,
			NativeTag:  1002,
		},
	}

	session, cleanup := setupTestSessionWithResponder(t, func(op string, payload []byte) ([]byte, error) {
		switch op {
		case "assembly.query_constraints":
			var list []protocol.AssemblyConstraintInfoWire
			for _, c := range constraints {
				list = append(list, c)
			}
			return protocol.EncodePayload(protocol.AssemblyQueryConstraintsResponse{
				Constraints: list,
			})

		case "assembly.delete_constraint":
			req, err := protocol.DecodePayload[protocol.AssemblyDeleteConstraintRequest](payload)
			if err != nil {
				return nil, err
			}
			if _, exists := constraints[req.ConstraintRef.ObjectID]; !exists {
				return nil, errors.New("constraint not found")
			}
			delete(constraints, req.ConstraintRef.ObjectID)
			return protocol.EncodePayload(protocol.AssemblyDeleteConstraintResponse{
				Deleted: true,
			})

		case "assembly.set_constraint_suppressed":
			req, err := protocol.DecodePayload[protocol.AssemblySetConstraintSuppressedRequest](payload)
			if err != nil {
				return nil, err
			}
			c, exists := constraints[req.ConstraintRef.ObjectID]
			if !exists {
				return nil, errors.New("constraint not found")
			}
			c.Suppressed = req.Suppressed
			constraints[req.ConstraintRef.ObjectID] = c
			return protocol.EncodePayload(protocol.AssemblySetConstraintSuppressedResponse{
				Suppressed: req.Suppressed,
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

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Query
	list, err := part.Constraints(ctx)
	if err != nil {
		t.Fatalf("Constraints query failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 constraints, got %d", len(list))
	}

	// Find c-1
	var target *Constraint
	for _, c := range list {
		if c.Ref.ObjectID == "c-1" {
			target = c
			break
		}
	}
	if target == nil {
		t.Fatalf("constraint c-1 not found in query results")
	}

	// Suppress
	if err := target.SetSuppressed(ctx, true); err != nil {
		t.Fatalf("SetSuppressed failed: %v", err)
	}
	if !target.Suppressed {
		t.Errorf("expected target.Suppressed to be true")
	}

	// Unsuppress
	if err := target.SetSuppressed(ctx, false); err != nil {
		t.Fatalf("SetSuppressed false failed: %v", err)
	}
	if target.Suppressed {
		t.Errorf("expected target.Suppressed to be false")
	}

	// Delete
	if err := target.Delete(ctx); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Re-query
	listAfter, err := part.Constraints(ctx)
	if err != nil {
		t.Fatalf("Constraints query after delete failed: %v", err)
	}
	if len(listAfter) != 1 {
		t.Errorf("expected 1 constraint after delete, got %d", len(listAfter))
	}
}

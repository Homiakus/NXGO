package nxgo

import (
	"context"
	"testing"

	"github.com/Homiakus/NXGO/internal/protocol"
)

func TestInterpartReferencesAndPolicy(t *testing.T) {
	ctx := context.Background()

	policyState := protocol.AssemblyGetInterpartPolicyResponse{
		InterpartDelay:      false,
		InterpartDataOption: "detect_out_of_date_and_load",
		ParentLoadOption:    "all",
	}

	session, cleanup := setupTestSessionWithResponder(t, func(op string, payload []byte) ([]byte, error) {
		switch op {
		case "assembly.query_interpart_references":
			req, err := protocol.DecodePayload[protocol.AssemblyQueryInterpartReferencesRequest](payload)
			if err != nil {
				return nil, err
			}
			return protocol.EncodePayload(protocol.AssemblyQueryInterpartReferencesResponse{
				PartRef: *req.PartRef,
				Parents: []protocol.InterpartReferenceWire{
					{
						PartRef:   protocol.ObjectHandleWire{SessionID: "test-session", Epoch: 1, ObjectID: "part-101", Generation: 1, Kind: "Part"},
						PartPath:  "D:/CAD/motor.prt",
						PartName:  "motor",
						NativeTag: 1001,
						Direction: "parent",
					},
				},
				Children: []protocol.InterpartReferenceWire{
					{
						PartRef:   protocol.ObjectHandleWire{SessionID: "test-session", Epoch: 1, ObjectID: "part-102", Generation: 1, Kind: "Part"},
						PartPath:  "D:/CAD/bracket.prt",
						PartName:  "bracket",
						NativeTag: 1002,
						Direction: "child",
					},
				},
				TotalCount: 2,
			})

		case "assembly.get_interpart_policy":
			return protocol.EncodePayload(policyState)

		case "assembly.set_interpart_policy":
			req, err := protocol.DecodePayload[protocol.AssemblySetInterpartPolicyRequest](payload)
			if err != nil {
				return nil, err
			}
			if req.InterpartDelay != nil {
				policyState.InterpartDelay = *req.InterpartDelay
			}
			if req.InterpartDataOption != nil {
				policyState.InterpartDataOption = *req.InterpartDataOption
			}
			if req.ParentLoadOption != nil {
				policyState.ParentLoadOption = *req.ParentLoadOption
			}
			return protocol.EncodePayload(protocol.AssemblySetInterpartPolicyResponse{
				InterpartDelay:      policyState.InterpartDelay,
				InterpartDataOption: policyState.InterpartDataOption,
				ParentLoadOption:    policyState.ParentLoadOption,
			})

		case "assembly.update_interpart_references":
			return protocol.EncodePayload(protocol.AssemblyUpdateInterpartReferencesResponse{
				Updated: true,
			})

		default:
			return nil, nil
		}
	})
	defer cleanup()

	part := &Part{
		Ref:     protocol.ObjectHandleWire{SessionID: "test-session", Epoch: 1, ObjectID: "part-200", Generation: 1, Kind: "Part"},
		session: session,
	}

	// 1. Query interpart references
	report, err := part.InterpartReferences(ctx)
	if err != nil {
		t.Fatalf("InterpartReferences failed: %v", err)
	}
	if report.TotalCount != 2 {
		t.Errorf("expected TotalCount 2, got %d", report.TotalCount)
	}
	if len(report.Parents) != 1 || report.Parents[0].PartName != "motor" {
		t.Errorf("unexpected parents: %+v", report.Parents)
	}
	if len(report.Children) != 1 || report.Children[0].PartName != "bracket" {
		t.Errorf("unexpected children: %+v", report.Children)
	}

	// 2. Query policy
	policy, err := session.InterpartPolicy(ctx)
	if err != nil {
		t.Fatalf("InterpartPolicy failed: %v", err)
	}
	if policy.InterpartDelay {
		t.Errorf("expected InterpartDelay false, got true")
	}
	if policy.InterpartDataOption != "detect_out_of_date_and_load" {
		t.Errorf("expected detect_out_of_date_and_load, got %s", policy.InterpartDataOption)
	}

	// 3. Set policy
	delay := true
	newOpt := "no_detect_and_no_load"
	updatedPolicy, err := session.SetInterpartPolicy(ctx, InterpartPolicyOptions{
		InterpartDelay:      &delay,
		InterpartDataOption: &newOpt,
	})
	if err != nil {
		t.Fatalf("SetInterpartPolicy failed: %v", err)
	}
	if !updatedPolicy.InterpartDelay {
		t.Errorf("expected InterpartDelay true, got false")
	}
	if updatedPolicy.InterpartDataOption != newOpt {
		t.Errorf("expected %s, got %s", newOpt, updatedPolicy.InterpartDataOption)
	}

	// 4. Update interpart references
	if err := part.UpdateInterpartReferences(ctx); err != nil {
		t.Fatalf("Part.UpdateInterpartReferences failed: %v", err)
	}
	if err := session.UpdateInterpartReferences(ctx, "session"); err != nil {
		t.Fatalf("Session.UpdateInterpartReferences failed: %v", err)
	}
}

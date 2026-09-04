package nxgo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
)

func TestArrangementsLifecycle(t *testing.T) {
	arrangements := map[string]protocol.AssemblyArrangementInfoWire{
		"arr-default": {
			ArrangementRef: protocol.ObjectHandleWire{
				SessionID: "test-session", Epoch: 1, ObjectID: "arr-default", Generation: 1, Kind: "Arrangement",
			},
			Name:                "Arrangement 1",
			IsActive:            true,
			IgnoringConstraints: false,
			NativeTag:           5001,
		},
	}
	activeName := "Arrangement 1"

	session, cleanup := setupTestSessionWithResponder(t, func(op string, payload []byte) ([]byte, error) {
		switch op {
		case "assembly.create_arrangement":
			req, err := protocol.DecodePayload[protocol.AssemblyCreateArrangementRequest](payload)
			if err != nil {
				return nil, err
			}
			if req.Name == "" {
				return nil, errors.New("name required")
			}
			ref := protocol.ObjectHandleWire{
				SessionID: "test-session", Epoch: 1, ObjectID: "arr-" + req.Name, Generation: 1, Kind: "Arrangement",
			}
			info := protocol.AssemblyArrangementInfoWire{
				ArrangementRef:      ref,
				Name:                req.Name,
				IsActive:            false,
				IgnoringConstraints: false,
				NativeTag:           5002,
			}
			arrangements[ref.ObjectID] = info
			return protocol.EncodePayload(protocol.AssemblyCreateArrangementResponse{
				ArrangementRef:      ref,
				Name:                req.Name,
				IsActive:            false,
				IgnoringConstraints: false,
				NativeTag:           5002,
			})

		case "assembly.query_arrangements":
			var list []protocol.AssemblyArrangementInfoWire
			for _, a := range arrangements {
				info := a
				info.IsActive = (info.Name == activeName)
				list = append(list, info)
			}
			return protocol.EncodePayload(protocol.AssemblyQueryArrangementsResponse{
				Arrangements:          list,
				ActiveArrangementName: activeName,
			})

		case "assembly.set_active_arrangement":
			req, err := protocol.DecodePayload[protocol.AssemblySetActiveArrangementRequest](payload)
			if err != nil {
				return nil, err
			}
			info, exists := arrangements[req.ArrangementRef.ObjectID]
			if !exists {
				return nil, errors.New("arrangement not found")
			}
			activeName = info.Name
			return protocol.EncodePayload(protocol.AssemblySetActiveArrangementResponse{
				ActiveArrangementName: activeName,
			})

		case "assembly.delete_arrangement":
			req, err := protocol.DecodePayload[protocol.AssemblyDeleteArrangementRequest](payload)
			if err != nil {
				return nil, err
			}
			if _, exists := arrangements[req.ArrangementRef.ObjectID]; !exists {
				return nil, errors.New("arrangement not found")
			}
			delete(arrangements, req.ArrangementRef.ObjectID)
			return protocol.EncodePayload(protocol.AssemblyDeleteArrangementResponse{
				Deleted: true,
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
		Name: "assembly_arr.prt",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. Query initial
	initial, err := part.Arrangements(ctx)
	if err != nil {
		t.Fatalf("Arrangements query failed: %v", err)
	}
	if len(initial) != 1 || initial[0].Name != "Arrangement 1" {
		t.Fatalf("unexpected initial arrangements: %+v", initial)
	}

	// 2. Create new arrangement
	created, err := part.CreateArrangement(ctx, CreateArrangementParams{
		Name: "Open_State",
	})
	if err != nil {
		t.Fatalf("CreateArrangement failed: %v", err)
	}
	if created.Name != "Open_State" {
		t.Errorf("expected name Open_State, got %s", created.Name)
	}

	// 3. Set active
	if err := created.SetActive(ctx); err != nil {
		t.Fatalf("SetActive failed: %v", err)
	}
	if !created.IsActive {
		t.Errorf("expected created.IsActive to be true")
	}

	// 4. Delete
	if err := created.Delete(ctx); err != nil {
		t.Fatalf("Delete arrangement failed: %v", err)
	}

	// 5. Query after delete
	after, err := part.Arrangements(ctx)
	if err != nil {
		t.Fatalf("Arrangements query after delete failed: %v", err)
	}
	if len(after) != 1 {
		t.Errorf("expected 1 arrangement after delete, got %d", len(after))
	}
}

func TestReferenceSetsLifecycle(t *testing.T) {
	refSets := map[string]protocol.PartReferenceSetInfoWire{
		"rs-model": {
			ReferenceSetRef: protocol.ObjectHandleWire{
				SessionID: "test-session", Epoch: 1, ObjectID: "rs-model", Generation: 1, Kind: "ReferenceSet",
			},
			Name:        "MODEL",
			MemberCount: 3,
			NativeTag:   6001,
		},
	}

	compRefSet := "MODEL"

	session, cleanup := setupTestSessionWithResponder(t, func(op string, payload []byte) ([]byte, error) {
		switch op {
		case "part.create_reference_set":
			req, err := protocol.DecodePayload[protocol.PartCreateReferenceSetRequest](payload)
			if err != nil {
				return nil, err
			}
			if req.Name == "" {
				return nil, errors.New("name required")
			}
			ref := protocol.ObjectHandleWire{
				SessionID: "test-session", Epoch: 1, ObjectID: "rs-" + req.Name, Generation: 1, Kind: "ReferenceSet",
			}
			info := protocol.PartReferenceSetInfoWire{
				ReferenceSetRef: ref,
				Name:            req.Name,
				MemberCount:     len(req.MemberRefs),
				NativeTag:       6002,
			}
			refSets[ref.ObjectID] = info
			return protocol.EncodePayload(protocol.PartCreateReferenceSetResponse{
				ReferenceSetRef: ref,
				Name:            req.Name,
				MemberCount:     len(req.MemberRefs),
				NativeTag:       6002,
			})

		case "part.query_reference_sets":
			var list []protocol.PartReferenceSetInfoWire
			for _, r := range refSets {
				list = append(list, r)
			}
			return protocol.EncodePayload(protocol.PartQueryReferenceSetsResponse{
				ReferenceSets: list,
			})

		case "part.modify_reference_set_members":
			req, err := protocol.DecodePayload[protocol.PartModifyReferenceSetMembersRequest](payload)
			if err != nil {
				return nil, err
			}
			info, exists := refSets[req.ReferenceSetRef.ObjectID]
			if !exists {
				return nil, errors.New("reference set not found")
			}
			info.MemberCount = info.MemberCount + len(req.AddMemberRefs) - len(req.RemoveMemberRefs)
			if info.MemberCount < 0 {
				info.MemberCount = 0
			}
			refSets[req.ReferenceSetRef.ObjectID] = info
			return protocol.EncodePayload(protocol.PartModifyReferenceSetMembersResponse{
				MemberCount: info.MemberCount,
			})

		case "part.delete_reference_set":
			req, err := protocol.DecodePayload[protocol.PartDeleteReferenceSetRequest](payload)
			if err != nil {
				return nil, err
			}
			if _, exists := refSets[req.ReferenceSetRef.ObjectID]; !exists {
				return nil, errors.New("reference set not found")
			}
			delete(refSets, req.ReferenceSetRef.ObjectID)
			return protocol.EncodePayload(protocol.PartDeleteReferenceSetResponse{
				Deleted: true,
			})

		case "assembly.set_component_reference_set":
			req, err := protocol.DecodePayload[protocol.AssemblySetComponentReferenceSetRequest](payload)
			if err != nil {
				return nil, err
			}
			compRefSet = req.ReferenceSetName
			return protocol.EncodePayload(protocol.AssemblySetComponentReferenceSetResponse{
				ReferenceSetName: compRefSet,
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
		Name: "part_refset.prt",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. Create reference set with members
	bodyRef := protocol.ObjectHandleWire{
		SessionID: "test-session", Epoch: 1, ObjectID: "body-1", Generation: 1, Kind: "Body",
	}
	rs, err := part.CreateReferenceSet(ctx, CreateReferenceSetParams{
		Name:    "LIGHTWEIGHT",
		Members: []protocol.ObjectHandleWire{bodyRef},
	})
	if err != nil {
		t.Fatalf("CreateReferenceSet failed: %v", err)
	}
	if rs.Name != "LIGHTWEIGHT" || rs.MemberCount != 1 {
		t.Errorf("unexpected rs: %+v", rs)
	}

	// 2. Add member
	body2Ref := protocol.ObjectHandleWire{
		SessionID: "test-session", Epoch: 1, ObjectID: "body-2", Generation: 1, Kind: "Body",
	}
	if err := rs.AddMembers(ctx, body2Ref); err != nil {
		t.Fatalf("AddMembers failed: %v", err)
	}
	if rs.MemberCount != 2 {
		t.Errorf("expected MemberCount 2, got %d", rs.MemberCount)
	}

	// 3. Remove member
	if err := rs.RemoveMembers(ctx, bodyRef); err != nil {
		t.Fatalf("RemoveMembers failed: %v", err)
	}
	if rs.MemberCount != 1 {
		t.Errorf("expected MemberCount 1, got %d", rs.MemberCount)
	}

	// 4. Query all
	all, err := part.ReferenceSets(ctx)
	if err != nil {
		t.Fatalf("ReferenceSets query failed: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 reference sets, got %d", len(all))
	}

	// 5. Component SetReferenceSet
	comp := &Component{
		session: session,
		part:    part,
		Ref: protocol.ObjectHandleWire{
			SessionID: "test-session", Epoch: 1, ObjectID: "comp-1", Generation: 1, Kind: "Component",
		},
		Name: "comp1",
	}
	if err := comp.SetReferenceSet(ctx, "LIGHTWEIGHT"); err != nil {
		t.Fatalf("SetReferenceSet failed: %v", err)
	}

	// 6. Delete reference set
	if err := rs.Delete(ctx); err != nil {
		t.Fatalf("Delete reference set failed: %v", err)
	}
}

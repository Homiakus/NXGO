package nxgo

import (
	"context"
	"errors"

	"github.com/Homiakus/NXGO/internal/protocol"
)

type CreateArrangementParams struct {
	Name            string
	BaseArrangement *protocol.ObjectHandleWire
	Isolated        bool
}

type Arrangement struct {
	session             *Session
	part                *Part
	Ref                 protocol.ObjectHandleWire
	Name                string
	IsActive            bool
	IgnoringConstraints bool
	NativeTag           uint32
}

func (a *Arrangement) validate() error {
	if a == nil || a.session == nil || a.part == nil {
		return errors.New("invalid or uninitialized arrangement")
	}
	return nil
}

func (p *Part) CreateArrangement(ctx context.Context, params CreateArrangementParams) (*Arrangement, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if params.Name == "" {
		return nil, errors.New("arrangement name is required")
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblyCreateArrangementRequest{
		AssemblyPartRef:    &p.Ref,
		Name:               params.Name,
		BaseArrangementRef: params.BaseArrangement,
		Isolated:           params.Isolated,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.create_arrangement"),
		Op:        "assembly.create_arrangement",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.AssemblyCreateArrangementResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &Arrangement{
		session:             p.session,
		part:                p,
		Ref:                 payload.ArrangementRef,
		Name:                payload.Name,
		IsActive:            payload.IsActive,
		IgnoringConstraints: payload.IgnoringConstraints,
		NativeTag:           payload.NativeTag,
	}, nil
}

func (p *Part) Arrangements(ctx context.Context) ([]*Arrangement, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblyQueryArrangementsRequest{
		AssemblyPartRef: &p.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.query_arrangements"),
		Op:        "assembly.query_arrangements",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.AssemblyQueryArrangementsResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	result := make([]*Arrangement, 0, len(payload.Arrangements))
	for _, wire := range payload.Arrangements {
		result = append(result, &Arrangement{
			session:             p.session,
			part:                p,
			Ref:                 wire.ArrangementRef,
			Name:                wire.Name,
			IsActive:            wire.IsActive,
			IgnoringConstraints: wire.IgnoringConstraints,
			NativeTag:           wire.NativeTag,
		})
	}
	return result, nil
}

func (a *Arrangement) SetActive(ctx context.Context) error {
	if err := a.validate(); err != nil {
		return err
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblySetActiveArrangementRequest{
		AssemblyPartRef: &a.part.Ref,
		ArrangementRef:  a.Ref,
	})
	if err != nil {
		return err
	}

	resp, err := a.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.set_active_arrangement"),
		Op:        "assembly.set_active_arrangement",
		Payload:   reqData,
	})
	if err != nil {
		return err
	}
	if resp.Status != protocol.StatusOK {
		return formatError(resp.Error)
	}
	a.IsActive = true
	return nil
}

func (a *Arrangement) Delete(ctx context.Context) error {
	if err := a.validate(); err != nil {
		return err
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblyDeleteArrangementRequest{
		AssemblyPartRef: &a.part.Ref,
		ArrangementRef:  a.Ref,
	})
	if err != nil {
		return err
	}

	resp, err := a.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.delete_arrangement"),
		Op:        "assembly.delete_arrangement",
		Payload:   reqData,
	})
	if err != nil {
		return err
	}
	if resp.Status != protocol.StatusOK {
		return formatError(resp.Error)
	}
	return nil
}

// Reference Sets API

type CreateReferenceSetParams struct {
	Name    string
	Members []protocol.ObjectHandleWire
}

type ReferenceSet struct {
	session     *Session
	part        *Part
	Ref         protocol.ObjectHandleWire
	Name        string
	MemberCount int
	NativeTag   uint32
}

func (r *ReferenceSet) validate() error {
	if r == nil || r.session == nil || r.part == nil {
		return errors.New("invalid or uninitialized reference set")
	}
	return nil
}

func (p *Part) CreateReferenceSet(ctx context.Context, params CreateReferenceSetParams) (*ReferenceSet, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if params.Name == "" {
		return nil, errors.New("reference set name is required")
	}

	reqData, err := protocol.EncodePayload(protocol.PartCreateReferenceSetRequest{
		PartRef:    &p.Ref,
		Name:       params.Name,
		MemberRefs: params.Members,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("part.create_reference_set"),
		Op:        "part.create_reference_set",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.PartCreateReferenceSetResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &ReferenceSet{
		session:     p.session,
		part:        p,
		Ref:         payload.ReferenceSetRef,
		Name:        payload.Name,
		MemberCount: payload.MemberCount,
		NativeTag:   payload.NativeTag,
	}, nil
}

func (p *Part) ReferenceSets(ctx context.Context) ([]*ReferenceSet, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}

	reqData, err := protocol.EncodePayload(protocol.PartQueryReferenceSetsRequest{
		PartRef: &p.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("part.query_reference_sets"),
		Op:        "part.query_reference_sets",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.PartQueryReferenceSetsResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	result := make([]*ReferenceSet, 0, len(payload.ReferenceSets))
	for _, wire := range payload.ReferenceSets {
		result = append(result, &ReferenceSet{
			session:     p.session,
			part:        p,
			Ref:         wire.ReferenceSetRef,
			Name:        wire.Name,
			MemberCount: wire.MemberCount,
			NativeTag:   wire.NativeTag,
		})
	}
	return result, nil
}

func (r *ReferenceSet) AddMembers(ctx context.Context, members ...protocol.ObjectHandleWire) error {
	if err := r.validate(); err != nil {
		return err
	}
	if len(members) == 0 {
		return nil
	}

	reqData, err := protocol.EncodePayload(protocol.PartModifyReferenceSetMembersRequest{
		PartRef:         &r.part.Ref,
		ReferenceSetRef: r.Ref,
		AddMemberRefs:   members,
	})
	if err != nil {
		return err
	}

	resp, err := r.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("part.modify_reference_set_members"),
		Op:        "part.modify_reference_set_members",
		Payload:   reqData,
	})
	if err != nil {
		return err
	}
	if resp.Status != protocol.StatusOK {
		return formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.PartModifyReferenceSetMembersResponse](resp.Payload)
	if err != nil {
		return err
	}
	r.MemberCount = payload.MemberCount
	return nil
}

func (r *ReferenceSet) RemoveMembers(ctx context.Context, members ...protocol.ObjectHandleWire) error {
	if err := r.validate(); err != nil {
		return err
	}
	if len(members) == 0 {
		return nil
	}

	reqData, err := protocol.EncodePayload(protocol.PartModifyReferenceSetMembersRequest{
		PartRef:          &r.part.Ref,
		ReferenceSetRef:  r.Ref,
		RemoveMemberRefs: members,
	})
	if err != nil {
		return err
	}

	resp, err := r.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("part.modify_reference_set_members"),
		Op:        "part.modify_reference_set_members",
		Payload:   reqData,
	})
	if err != nil {
		return err
	}
	if resp.Status != protocol.StatusOK {
		return formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.PartModifyReferenceSetMembersResponse](resp.Payload)
	if err != nil {
		return err
	}
	r.MemberCount = payload.MemberCount
	return nil
}

func (r *ReferenceSet) Delete(ctx context.Context) error {
	if err := r.validate(); err != nil {
		return err
	}

	reqData, err := protocol.EncodePayload(protocol.PartDeleteReferenceSetRequest{
		PartRef:         &r.part.Ref,
		ReferenceSetRef: r.Ref,
	})
	if err != nil {
		return err
	}

	resp, err := r.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("part.delete_reference_set"),
		Op:        "part.delete_reference_set",
		Payload:   reqData,
	})
	if err != nil {
		return err
	}
	if resp.Status != protocol.StatusOK {
		return formatError(resp.Error)
	}
	return nil
}

func (c *Component) SetReferenceSet(ctx context.Context, refSetName string) error {
	if err := c.validate(); err != nil {
		return err
	}
	if refSetName == "" {
		return errors.New("reference set name is required")
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblySetComponentReferenceSetRequest{
		AssemblyPartRef:  &c.part.Ref,
		ComponentRef:     c.Ref,
		ReferenceSetName: refSetName,
	})
	if err != nil {
		return err
	}

	resp, err := c.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.set_component_reference_set"),
		Op:        "assembly.set_component_reference_set",
		Payload:   reqData,
	})
	if err != nil {
		return err
	}
	if resp.Status != protocol.StatusOK {
		return formatError(resp.Error)
	}
	return nil
}

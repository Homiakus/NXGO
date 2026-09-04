package nxgo

import (
	"context"
	"strings"

	"github.com/Homiakus/NXGO/internal/protocol"
)

type InterpartReference struct {
	PartRef   protocol.ObjectHandleWire
	PartPath  string
	PartName  string
	NativeTag uint32
	Direction string
}

type InterpartReferencesReport struct {
	PartRef    protocol.ObjectHandleWire
	Parents    []InterpartReference
	Children   []InterpartReference
	TotalCount int
}

type InterpartPolicy struct {
	InterpartDelay      bool
	InterpartDataOption string
	ParentLoadOption    string
}

type InterpartPolicyOptions struct {
	InterpartDelay      *bool
	InterpartDataOption *string
	ParentLoadOption    *string
}

func (p *Part) InterpartReferences(ctx context.Context) (*InterpartReferencesReport, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblyQueryInterpartReferencesRequest{
		PartRef: &p.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.query_interpart_references"),
		Op:        "assembly.query_interpart_references",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	res, err := protocol.DecodePayload[protocol.AssemblyQueryInterpartReferencesResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	parents := make([]InterpartReference, len(res.Parents))
	for i, r := range res.Parents {
		parents[i] = InterpartReference{
			PartRef:   r.PartRef,
			PartPath:  r.PartPath,
			PartName:  r.PartName,
			NativeTag: r.NativeTag,
			Direction: r.Direction,
		}
	}

	children := make([]InterpartReference, len(res.Children))
	for i, r := range res.Children {
		children[i] = InterpartReference{
			PartRef:   r.PartRef,
			PartPath:  r.PartPath,
			PartName:  r.PartName,
			NativeTag: r.NativeTag,
			Direction: r.Direction,
		}
	}

	return &InterpartReferencesReport{
		PartRef:    res.PartRef,
		Parents:    parents,
		Children:   children,
		TotalCount: res.TotalCount,
	}, nil
}

func (p *Part) UpdateInterpartReferences(ctx context.Context) error {
	if err := p.validate(); err != nil {
		return err
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblyUpdateInterpartReferencesRequest{
		PartRef: &p.Ref,
		Scope:   "part",
	})
	if err != nil {
		return err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.update_interpart_references"),
		Op:        "assembly.update_interpart_references",
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

func (s *Session) InterpartPolicy(ctx context.Context) (*InterpartPolicy, error) {
	if err := s.validateOpen(); err != nil {
		return nil, err
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblyGetInterpartPolicyRequest{})
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.get_interpart_policy"),
		Op:        "assembly.get_interpart_policy",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	res, err := protocol.DecodePayload[protocol.AssemblyGetInterpartPolicyResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &InterpartPolicy{
		InterpartDelay:      res.InterpartDelay,
		InterpartDataOption: res.InterpartDataOption,
		ParentLoadOption:    res.ParentLoadOption,
	}, nil
}

func (s *Session) SetInterpartPolicy(ctx context.Context, opts InterpartPolicyOptions) (*InterpartPolicy, error) {
	if err := s.validateOpen(); err != nil {
		return nil, err
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblySetInterpartPolicyRequest{
		InterpartDelay:      opts.InterpartDelay,
		InterpartDataOption: opts.InterpartDataOption,
		ParentLoadOption:    opts.ParentLoadOption,
	})
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.set_interpart_policy"),
		Op:        "assembly.set_interpart_policy",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	res, err := protocol.DecodePayload[protocol.AssemblySetInterpartPolicyResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &InterpartPolicy{
		InterpartDelay:      res.InterpartDelay,
		InterpartDataOption: res.InterpartDataOption,
		ParentLoadOption:    res.ParentLoadOption,
	}, nil
}

func (s *Session) UpdateInterpartReferences(ctx context.Context, scope string) error {
	if err := s.validateOpen(); err != nil {
		return err
	}

	sc := strings.ToLower(strings.TrimSpace(scope))
	if sc == "" {
		sc = "session"
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblyUpdateInterpartReferencesRequest{
		Scope: sc,
	})
	if err != nil {
		return err
	}

	resp, err := s.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.update_interpart_references"),
		Op:        "assembly.update_interpart_references",
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

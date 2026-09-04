package nxgo

import (
	"context"

	"github.com/Homiakus/NXGO/internal/protocol"
)

type ComponentState struct {
	ComponentRef protocol.ObjectHandleWire
	Name         string
	IsSuppressed bool
	IsLoaded     bool
	ReferenceSet string
	NativeTag    uint32
}

func (c *Component) State(ctx context.Context) (*ComponentState, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblyQueryComponentStateRequest{
		AssemblyPartRef: &c.part.Ref,
		ComponentRef:    c.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := c.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.query_component_state"),
		Op:        "assembly.query_component_state",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.AssemblyQueryComponentStateResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &ComponentState{
		ComponentRef: payload.ComponentRef,
		Name:         payload.Name,
		IsSuppressed: payload.IsSuppressed,
		IsLoaded:     payload.IsLoaded,
		ReferenceSet: payload.ReferenceSet,
		NativeTag:    payload.NativeTag,
	}, nil
}

func (c *Component) Suppress(ctx context.Context) error {
	if err := c.validate(); err != nil {
		return err
	}
	return c.part.SuppressComponents(ctx, c)
}

func (c *Component) Unsuppress(ctx context.Context) error {
	if err := c.validate(); err != nil {
		return err
	}
	return c.part.UnsuppressComponents(ctx, c)
}

func (p *Part) SuppressComponents(ctx context.Context, components ...*Component) error {
	if err := p.validate(); err != nil {
		return err
	}
	if len(components) == 0 {
		return nil
	}

	refs := make([]protocol.ObjectHandleWire, 0, len(components))
	for _, c := range components {
		if c != nil {
			refs = append(refs, c.Ref)
		}
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblySuppressComponentsRequest{
		AssemblyPartRef: &p.Ref,
		ComponentRefs:   refs,
	})
	if err != nil {
		return err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.suppress_components"),
		Op:        "assembly.suppress_components",
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

func (p *Part) UnsuppressComponents(ctx context.Context, components ...*Component) error {
	if err := p.validate(); err != nil {
		return err
	}
	if len(components) == 0 {
		return nil
	}

	refs := make([]protocol.ObjectHandleWire, 0, len(components))
	for _, c := range components {
		if c != nil {
			refs = append(refs, c.Ref)
		}
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblyUnsuppressComponentsRequest{
		AssemblyPartRef: &p.Ref,
		ComponentRefs:   refs,
	})
	if err != nil {
		return err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.unsuppress_components"),
		Op:        "assembly.unsuppress_components",
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

func (p *Part) OpenComponents(ctx context.Context, option string, components ...*Component) error {
	if err := p.validate(); err != nil {
		return err
	}
	if len(components) == 0 {
		return nil
	}
	if option == "" {
		option = "whole_assembly"
	}

	refs := make([]protocol.ObjectHandleWire, 0, len(components))
	for _, c := range components {
		if c != nil {
			refs = append(refs, c.Ref)
		}
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblyOpenComponentsRequest{
		AssemblyPartRef: &p.Ref,
		ComponentRefs:   refs,
		Option:          option,
	})
	if err != nil {
		return err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.open_components"),
		Op:        "assembly.open_components",
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

func (p *Part) CloseComponents(ctx context.Context, wholeTree, closeModified bool, components ...*Component) error {
	if err := p.validate(); err != nil {
		return err
	}
	if len(components) == 0 {
		return nil
	}

	refs := make([]protocol.ObjectHandleWire, 0, len(components))
	for _, c := range components {
		if c != nil {
			refs = append(refs, c.Ref)
		}
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblyCloseComponentsRequest{
		AssemblyPartRef: &p.Ref,
		ComponentRefs:   refs,
		WholeTree:       wholeTree,
		CloseModified:   closeModified,
	})
	if err != nil {
		return err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.close_components"),
		Op:        "assembly.close_components",
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

func (c *Component) Open(ctx context.Context, option ...string) error {
	opt := "whole_assembly"
	if len(option) > 0 && option[0] != "" {
		opt = option[0]
	}
	return c.part.OpenComponents(ctx, opt, c)
}

func (c *Component) Close(ctx context.Context, wholeTree, closeModified bool) error {
	return c.part.CloseComponents(ctx, wholeTree, closeModified, c)
}

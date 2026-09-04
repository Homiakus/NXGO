package nxgo

import (
	"context"
	"errors"

	"github.com/Homiakus/NXGO/internal/protocol"
)

type ConstraintType = protocol.AssemblyConstraintType
type ConstraintAlignment = protocol.AssemblyConstraintAlignment

const (
	ConstraintTouch         = protocol.ConstraintTypeTouch
	ConstraintConcentric    = protocol.ConstraintTypeConcentric
	ConstraintFix           = protocol.ConstraintTypeFix
	ConstraintDistance      = protocol.ConstraintTypeDistance
	ConstraintParallel      = protocol.ConstraintTypeParallel
	ConstraintPerpendicular = protocol.ConstraintTypePerpendicular
	ConstraintCenter12      = protocol.ConstraintTypeCenter12
	ConstraintCenter22      = protocol.ConstraintTypeCenter22
	ConstraintAngle         = protocol.ConstraintTypeAngle
	ConstraintFit           = protocol.ConstraintTypeFit
	ConstraintBond          = protocol.ConstraintTypeBond
	ConstraintAlignLock     = protocol.ConstraintTypeAlignLock

	AlignInfer  = protocol.ConstraintAlignmentInfer
	AlignCo     = protocol.ConstraintAlignmentCo
	AlignContra = protocol.ConstraintAlignmentContra
)

type CreateConstraintParams struct {
	Type      ConstraintType
	Alignment ConstraintAlignment
	Target1   protocol.ObjectHandleWire
	Target2   *protocol.ObjectHandleWire
	Offset    float64
	Name      string
}

type Constraint struct {
	session    *Session
	part       *Part
	Ref        protocol.ObjectHandleWire
	Name       string
	Type       string
	Alignment  string
	Status     string
	Suppressed bool
	NativeTag  uint32
}

func (c *Constraint) validate() error {
	if c == nil || c.session == nil || c.part == nil {
		return errors.New("invalid or uninitialized constraint")
	}
	return nil
}

func (p *Part) CreateConstraint(ctx context.Context, params CreateConstraintParams) (*Constraint, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if params.Type == "" {
		return nil, errors.New("constraint type is required")
	}
	if params.Target1.ObjectID == "" {
		return nil, errors.New("target 1 object reference is required")
	}
	if params.Type != ConstraintFix && params.Target2 == nil {
		return nil, errors.New("target 2 object reference is required for non-fix constraints")
	}
	if params.Alignment == "" {
		params.Alignment = AlignInfer
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblyCreateConstraintRequest{
		AssemblyPartRef: &p.Ref,
		Type:            params.Type,
		Alignment:       params.Alignment,
		TargetRef1:      params.Target1,
		TargetRef2:      params.Target2,
		Offset:          params.Offset,
		Name:            params.Name,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.create_constraint"),
		Op:        "assembly.create_constraint",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.AssemblyCreateConstraintResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &Constraint{
		session:   p.session,
		part:      p,
		Ref:       payload.ConstraintRef,
		Name:      payload.Name,
		Type:      payload.Type,
		Alignment: payload.Alignment,
		Status:    payload.Status,
		NativeTag: payload.NativeTag,
	}, nil
}

func (p *Part) Constraints(ctx context.Context) ([]*Constraint, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblyQueryConstraintsRequest{
		AssemblyPartRef: &p.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.query_constraints"),
		Op:        "assembly.query_constraints",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.AssemblyQueryConstraintsResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	result := make([]*Constraint, 0, len(payload.Constraints))
	for _, wire := range payload.Constraints {
		result = append(result, &Constraint{
			session:    p.session,
			part:       p,
			Ref:        wire.ConstraintRef,
			Name:       wire.Name,
			Type:       wire.Type,
			Alignment:  wire.Alignment,
			Status:     wire.Status,
			Suppressed: wire.Suppressed,
			NativeTag:  wire.NativeTag,
		})
	}
	return result, nil
}

func (c *Constraint) Delete(ctx context.Context) error {
	if err := c.validate(); err != nil {
		return err
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblyDeleteConstraintRequest{
		AssemblyPartRef: &c.part.Ref,
		ConstraintRef:   c.Ref,
	})
	if err != nil {
		return err
	}

	resp, err := c.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.delete_constraint"),
		Op:        "assembly.delete_constraint",
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

func (c *Constraint) SetSuppressed(ctx context.Context, suppressed bool) error {
	if err := c.validate(); err != nil {
		return err
	}

	reqData, err := protocol.EncodePayload(protocol.AssemblySetConstraintSuppressedRequest{
		AssemblyPartRef: &c.part.Ref,
		ConstraintRef:   c.Ref,
		Suppressed:      suppressed,
	})
	if err != nil {
		return err
	}

	resp, err := c.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.set_constraint_suppressed"),
		Op:        "assembly.set_constraint_suppressed",
		Payload:   reqData,
	})
	if err != nil {
		return err
	}
	if resp.Status != protocol.StatusOK {
		return formatError(resp.Error)
	}
	c.Suppressed = suppressed
	return nil
}

package nxgo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Homiakus/NXGO/internal/protocol"
)

var (
	ErrExpressionNotFound     = errors.New("expression not found")
	ErrEmptyExpressionName    = errors.New("expression name cannot be empty")
	ErrEmptyExpressionFormula = errors.New("expression formula cannot be empty")
)

type ExpressionParams struct {
	Name        string
	Formula     string
	Type        string // default "Number"
	Units       string // e.g. "mm", "inch", "degrees"
	Description string
}

type Expression struct {
	session     *Session
	part        *Part
	Ref         protocol.ObjectHandleWire
	Name        string
	Formula     string
	Value       float64
	StringValue string
	Type        string
	Units       string
}

func (p *Part) CreateExpression(ctx context.Context, name, formula, units string) (*Expression, error) {
	return p.CreateExpressionWithParams(ctx, ExpressionParams{
		Name:    name,
		Formula: formula,
		Units:   units,
	})
}

func (p *Part) CreateExpressionWithParams(ctx context.Context, params ExpressionParams) (*Expression, error) {
	if strings.TrimSpace(params.Name) == "" {
		return nil, ErrEmptyExpressionName
	}
	if strings.TrimSpace(params.Formula) == "" {
		return nil, ErrEmptyExpressionFormula
	}
	if err := p.validate(); err != nil {
		return nil, err
	}

	reqData, err := protocol.EncodePayload(protocol.ExpressionCreateRequest{
		PartRef:     &p.Ref,
		Name:        params.Name,
		Formula:     params.Formula,
		Type:        params.Type,
		Units:       params.Units,
		Description: params.Description,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("expression.create"),
		Op:        "expression.create",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.ExpressionCreateResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &Expression{
		session:     p.session,
		part:        p,
		Ref:         payload.ExpressionRef,
		Name:        payload.Name,
		Formula:     payload.Formula,
		Value:       payload.Value,
		StringValue: payload.StringValue,
		Type:        payload.Type,
		Units:       payload.Units,
	}, nil
}

func (p *Part) GetExpression(ctx context.Context, name string) (*Expression, error) {
	if strings.TrimSpace(name) == "" {
		return nil, ErrEmptyExpressionName
	}
	if err := p.validate(); err != nil {
		return nil, err
	}

	reqData, err := protocol.EncodePayload(protocol.ExpressionQueryRequest{
		PartRef: &p.Ref,
		Name:    name,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("expression.query"),
		Op:        "expression.query",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.ExpressionQueryResponse](resp.Payload)
	if err != nil {
		return nil, err
	}
	if len(payload.Expressions) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrExpressionNotFound, name)
	}

	info := payload.Expressions[0]
	return &Expression{
		session:     p.session,
		part:        p,
		Ref:         info.ExpressionRef,
		Name:        info.Name,
		Formula:     info.Formula,
		Value:       info.Value,
		StringValue: info.StringValue,
		Type:        info.Type,
		Units:       info.Units,
	}, nil
}

func (p *Part) ListExpressions(ctx context.Context) ([]*Expression, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}

	reqData, err := protocol.EncodePayload(protocol.ExpressionQueryRequest{
		PartRef: &p.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("expression.query"),
		Op:        "expression.query",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.ExpressionQueryResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	result := make([]*Expression, len(payload.Expressions))
	for i, info := range payload.Expressions {
		result[i] = &Expression{
			session:     p.session,
			part:        p,
			Ref:         info.ExpressionRef,
			Name:        info.Name,
			Formula:     info.Formula,
			Value:       info.Value,
			StringValue: info.StringValue,
			Type:        info.Type,
			Units:       info.Units,
		}
	}
	return result, nil
}

func (p *Part) SetExpression(ctx context.Context, name, formula string) (*Expression, error) {
	if strings.TrimSpace(name) == "" {
		return nil, ErrEmptyExpressionName
	}
	if strings.TrimSpace(formula) == "" {
		return nil, ErrEmptyExpressionFormula
	}
	if err := p.validate(); err != nil {
		return nil, err
	}

	reqData, err := protocol.EncodePayload(protocol.ExpressionEditRequest{
		PartRef: &p.Ref,
		Name:    name,
		Formula: formula,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("expression.edit"),
		Op:        "expression.edit",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.ExpressionEditResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &Expression{
		session:     p.session,
		part:        p,
		Ref:         payload.ExpressionRef,
		Name:        payload.Name,
		Formula:     payload.Formula,
		Value:       payload.Value,
		StringValue: payload.StringValue,
	}, nil
}

func (p *Part) DeleteExpression(ctx context.Context, name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrEmptyExpressionName
	}
	if err := p.validate(); err != nil {
		return err
	}

	reqData, err := protocol.EncodePayload(protocol.ExpressionDeleteRequest{
		PartRef: &p.Ref,
		Name:    name,
	})
	if err != nil {
		return err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("expression.delete"),
		Op:        "expression.delete",
		Payload:   reqData,
	})
	if err != nil {
		return err
	}
	if resp.Status != protocol.StatusOK {
		return formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.ExpressionDeleteResponse](resp.Payload)
	if err != nil {
		return err
	}
	if !payload.Deleted {
		return fmt.Errorf("%w: failed to delete %s", ErrExpressionNotFound, name)
	}
	return nil
}

func (e *Expression) Edit(ctx context.Context, newFormula string) error {
	if e.part == nil || e.session == nil {
		return errors.New("expression is not bound to a valid part or session")
	}
	if strings.TrimSpace(newFormula) == "" {
		return ErrEmptyExpressionFormula
	}

	reqData, err := protocol.EncodePayload(protocol.ExpressionEditRequest{
		PartRef:       &e.part.Ref,
		ExpressionRef: &e.Ref,
		Name:          e.Name,
		Formula:       newFormula,
	})
	if err != nil {
		return err
	}

	resp, err := e.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("expression.edit"),
		Op:        "expression.edit",
		Payload:   reqData,
	})
	if err != nil {
		return err
	}
	if resp.Status != protocol.StatusOK {
		return formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.ExpressionEditResponse](resp.Payload)
	if err != nil {
		return err
	}

	e.Formula = payload.Formula
	e.Value = payload.Value
	e.StringValue = payload.StringValue
	return nil
}

func (e *Expression) Delete(ctx context.Context) error {
	if e.part == nil || e.session == nil {
		return errors.New("expression is not bound to a valid part or session")
	}

	reqData, err := protocol.EncodePayload(protocol.ExpressionDeleteRequest{
		PartRef:       &e.part.Ref,
		ExpressionRef: &e.Ref,
		Name:          e.Name,
	})
	if err != nil {
		return err
	}

	resp, err := e.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("expression.delete"),
		Op:        "expression.delete",
		Payload:   reqData,
	})
	if err != nil {
		return err
	}
	if resp.Status != protocol.StatusOK {
		return formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.ExpressionDeleteResponse](resp.Payload)
	if err != nil {
		return err
	}
	if !payload.Deleted {
		return fmt.Errorf("failed to delete expression %s", e.Name)
	}
	return nil
}

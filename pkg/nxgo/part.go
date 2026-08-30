package nxgo

import (
	"context"
	"fmt"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
)

type Part struct {
	session *Session
	Ref     protocol.ObjectHandleWire
	Name    string
	Units   string
}

func (s *Session) NewPart(ctx context.Context, name, units string) (*Part, error) {
	reqData, err := protocol.EncodePayload(protocol.PartNewRequest{
		Name:  name,
		Units: units,
	})
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-partnew-%d", time.Now().UnixNano()),
		Op:        "part.new",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.PartNewResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &Part{
		session: s,
		Ref:     payload.PartRef,
		Name:    payload.Name,
		Units:   payload.Units,
	}, nil
}

func (s *Session) OpenPart(ctx context.Context, filePath string) (*Part, error) {
	reqData, err := protocol.EncodePayload(protocol.PartOpenRequest{
		Path: filePath,
	})
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-partopen-%d", time.Now().UnixNano()),
		Op:        "part.open",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.PartOpenResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &Part{
		session: s,
		Ref:     payload.PartRef,
		Name:    payload.Name,
		Units:   payload.Units,
	}, nil
}

func (p *Part) Save(ctx context.Context) (*protocol.PartSaveResponse, error) {
	reqData, err := protocol.EncodePayload(protocol.PartSaveRequest{
		PartRef: &p.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-partsave-%d", time.Now().UnixNano()),
		Op:        "part.save",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	return protocol.DecodePayload[protocol.PartSaveResponse](resp.Payload)
}

func (p *Part) Close(ctx context.Context, save bool) error {
	reqData, err := protocol.EncodePayload(protocol.PartCloseRequest{
		PartRef: &p.Ref,
		Save:    save,
	})
	if err != nil {
		return err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-partclose-%d", time.Now().UnixNano()),
		Op:        "part.close",
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

func (p *Part) Summary(ctx context.Context) (*protocol.PartSummaryResponse, error) {
	reqData, err := protocol.EncodePayload(protocol.PartSummaryRequest{
		PartRef: &p.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-partsum-%d", time.Now().UnixNano()),
		Op:        "part.query_summary",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	return protocol.DecodePayload[protocol.PartSummaryResponse](resp.Payload)
}

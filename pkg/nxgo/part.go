package nxgo

import (
	"context"

	"github.com/Homiakus/NXGO/internal/protocol"
)

type Part struct {
	session *Session
	Ref     protocol.ObjectHandleWire
	Name    string
	Units   string
}

func (s *Session) NewPart(ctx context.Context, name, units string) (*Part, error) {
	if err := s.validateOpen(); err != nil {
		return nil, err
	}
	reqData, err := protocol.EncodePayload(protocol.PartNewRequest{
		Name:  name,
		Units: units,
	})
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("part.new"),
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
	if err := s.validateOpen(); err != nil {
		return nil, err
	}
	reqData, err := protocol.EncodePayload(protocol.PartOpenRequest{
		Path: filePath,
	})
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("part.open"),
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
	if err := p.validate(); err != nil {
		return nil, err
	}
	reqData, err := protocol.EncodePayload(protocol.PartSaveRequest{
		PartRef: &p.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("part.save"),
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

func (p *Part) SaveAs(ctx context.Context, filePath string) (*protocol.PartSaveResponse, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	reqData, err := protocol.EncodePayload(protocol.PartSaveRequest{
		PartRef: &p.Ref,
		Path:    filePath,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("part.save"),
		Op:        "part.save",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	res, err := protocol.DecodePayload[protocol.PartSaveResponse](resp.Payload)
	if err != nil {
		return nil, err
	}
	p.Name = res.Name
	return res, nil
}

func (p *Part) Close(ctx context.Context, save bool) error {
	if err := p.validate(); err != nil {
		return err
	}
	reqData, err := protocol.EncodePayload(protocol.PartCloseRequest{
		PartRef: &p.Ref,
		Save:    save,
	})
	if err != nil {
		return err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("part.close"),
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

func (p *Part) ForceCloseDiscard(ctx context.Context) error {
	return p.Close(ctx, false)
}

func (p *Part) Summary(ctx context.Context) (*protocol.PartSummaryResponse, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	reqData, err := protocol.EncodePayload(protocol.PartSummaryRequest{
		PartRef: &p.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("part.query_summary"),
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

func (p *Part) GetAttributes(ctx context.Context, titles ...string) ([]protocol.PartAttribute, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	reqData, err := protocol.EncodePayload(protocol.PartGetAttributesRequest{
		PartRef: &p.Ref,
		Titles:  titles,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("part.get_attributes"),
		Op:        "part.get_attributes",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	res, err := protocol.DecodePayload[protocol.PartGetAttributesResponse](resp.Payload)
	if err != nil {
		return nil, err
	}
	return res.Attributes, nil
}

func (p *Part) SetAttributes(ctx context.Context, attrs []protocol.PartAttribute) (int, error) {
	if err := p.validate(); err != nil {
		return 0, err
	}
	reqData, err := protocol.EncodePayload(protocol.PartSetAttributesRequest{
		PartRef:    &p.Ref,
		Attributes: attrs,
	})
	if err != nil {
		return 0, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("part.set_attributes"),
		Op:        "part.set_attributes",
		Payload:   reqData,
	})
	if err != nil {
		return 0, err
	}
	if resp.Status != protocol.StatusOK {
		return 0, formatError(resp.Error)
	}

	res, err := protocol.DecodePayload[protocol.PartSetAttributesResponse](resp.Payload)
	if err != nil {
		return 0, err
	}
	return res.UpdatedCount, nil
}

func (p *Part) Metadata(ctx context.Context) (*protocol.PartMetadataEntry, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	entries, err := p.session.BulkMetadata(ctx, p)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	return &entries[0], nil
}

func (s *Session) BulkMetadata(ctx context.Context, parts ...*Part) ([]protocol.PartMetadataEntry, error) {
	if err := s.validateOpen(); err != nil {
		return nil, err
	}
	var refs []protocol.ObjectHandleWire
	for _, p := range parts {
		if p != nil {
			refs = append(refs, p.Ref)
		}
	}
	reqData, err := protocol.EncodePayload(protocol.PartBulkMetadataRequest{
		PartRefs:          refs,
		IncludeAttributes: true,
	})
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("part.bulk_metadata"),
		Op:        "part.bulk_metadata",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	res, err := protocol.DecodePayload[protocol.PartBulkMetadataResponse](resp.Payload)
	if err != nil {
		return nil, err
	}
	return res.Entries, nil
}

func (p *Part) LoadStatus(ctx context.Context) (*protocol.PartLoadStatusResponse, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	reqData, err := protocol.EncodePayload(protocol.PartLoadStatusRequest{
		PartRef: &p.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("part.query_load_status"),
		Op:        "part.query_load_status",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	return protocol.DecodePayload[protocol.PartLoadStatusResponse](resp.Payload)
}

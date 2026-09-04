package nxgo

import (
	"context"

	"github.com/Homiakus/NXGO/internal/protocol"
)

type AssemblyBulkFilter struct {
	MaxDepth           int
	IncludeSuppressed  bool
	IncludeTransforms  bool
	IncludeBoundingBox bool
	NameFilter         string
	Offset             int
	Limit              int
}

type AssemblyBulkSummary struct {
	TotalComponents  int
	TotalLoaded      int
	TotalSuppressed  int
	UniquePartsCount int
	Components       []protocol.AssemblyBulkComponentItem
	HasMore          bool
}

func (p *Part) QueryAssemblyBulk(ctx context.Context, filter ...AssemblyBulkFilter) (*AssemblyBulkSummary, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}

	req := protocol.AssemblyQueryBulkRequest{
		AssemblyPartRef:   &p.Ref,
		IncludeSuppressed: true,
		IncludeTransforms: true,
	}

	if len(filter) > 0 {
		f := filter[0]
		req.MaxDepth = f.MaxDepth
		req.IncludeSuppressed = f.IncludeSuppressed
		req.IncludeTransforms = f.IncludeTransforms
		req.IncludeBoundingBox = f.IncludeBoundingBox
		req.NameFilter = f.NameFilter
		req.Offset = f.Offset
		req.Limit = f.Limit
	}

	reqData, err := protocol.EncodePayload(req)
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("assembly.query_bulk"),
		Op:        "assembly.query_bulk",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	res, err := protocol.DecodePayload[protocol.AssemblyQueryBulkResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &AssemblyBulkSummary{
		TotalComponents:  res.TotalComponents,
		TotalLoaded:      res.TotalLoaded,
		TotalSuppressed:  res.TotalSuppressed,
		UniquePartsCount: res.UniquePartsCount,
		Components:       res.Components,
		HasMore:          res.HasMore,
	}, nil
}

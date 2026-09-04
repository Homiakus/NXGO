package nxgo

import (
	"context"
	"errors"
	"fmt"

	"github.com/Homiakus/NXGO/internal/protocol"
)

// BulkAnalysisOptions configures what geometric information is retrieved
// in a single bulk analysis roundtrip.
type BulkAnalysisOptions struct {
	IncludeMassProperties bool
	IncludeBoundingBox    bool
	IncludeFacesAndEdges  bool
	ProduceBodyHandles    bool
}

// DefaultBulkAnalysisOptions returns standard analysis options with mass properties,
// bounding boxes, and topological face/edge counts enabled, but without producing
// ephemeral object handles (value-snapshot mode).
func DefaultBulkAnalysisOptions() BulkAnalysisOptions {
	return BulkAnalysisOptions{
		IncludeMassProperties: true,
		IncludeBoundingBox:    true,
		IncludeFacesAndEdges:  true,
		ProduceBodyHandles:    false,
	}
}

// BulkAnalysisOption configures a BulkAnalysisOptions struct.
type BulkAnalysisOption func(*BulkAnalysisOptions)

// WithMassProperties specifies whether to compute mass properties.
func WithMassProperties(include bool) BulkAnalysisOption {
	return func(o *BulkAnalysisOptions) {
		o.IncludeMassProperties = include
	}
}

// WithBoundingBox specifies whether to compute bounding boxes.
func WithBoundingBox(include bool) BulkAnalysisOption {
	return func(o *BulkAnalysisOptions) {
		o.IncludeBoundingBox = include
	}
}

// WithFacesAndEdges specifies whether to count faces and edges.
func WithFacesAndEdges(include bool) BulkAnalysisOption {
	return func(o *BulkAnalysisOptions) {
		o.IncludeFacesAndEdges = include
	}
}

// WithProduceBodyHandles specifies whether remote body handles should be registered in the session.
func WithProduceBodyHandles(produce bool) BulkAnalysisOption {
	return func(o *BulkAnalysisOptions) {
		o.ProduceBodyHandles = produce
	}
}

// BodyGeometryAnalysis contains comprehensive geometric and topological properties
// for a single body.
type BodyGeometryAnalysis struct {
	BodyRef        *protocol.ObjectHandleWire
	Name           string
	SolidType      string
	FaceCount      int
	EdgeCount      int
	NativeTag      uint32
	MassProperties *MassProperties
	BoundingBox    *BoundingBox
}

// PartGeometryAnalysis contains aggregated geometric data and per-body breakdowns
// for a part or a set of bodies.
type PartGeometryAnalysis struct {
	Units         string
	BodyCount     int
	SolidCount    int
	SheetCount    int
	AggregateMass *MassProperties
	AggregateBox  *BoundingBox
	Bodies        []BodyGeometryAnalysis
}

// AnalyzeGeometry executes a single-roundtrip bulk query across all bodies in the part.
func (p *Part) AnalyzeGeometry(ctx context.Context, opts ...BulkAnalysisOption) (*PartGeometryAnalysis, error) {
	if p == nil {
		return nil, ErrSessionClosed
	}
	if err := p.validate(); err != nil {
		return nil, err
	}

	options := DefaultBulkAnalysisOptions()
	for _, opt := range opts {
		opt(&options)
	}

	reqData, err := protocol.EncodePayload(protocol.GeometryQueryBulkAnalysisRequest{
		PartRef:               &p.Ref,
		IncludeMassProperties: options.IncludeMassProperties,
		IncludeBoundingBox:    options.IncludeBoundingBox,
		IncludeFacesAndEdges:  options.IncludeFacesAndEdges,
		ProduceBodyHandles:    options.ProduceBodyHandles,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("geometry.query_bulk_analysis.part"),
		Op:        "geometry.query_bulk_analysis",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.GeometryQueryBulkAnalysisResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return decodePartGeometryAnalysis(payload), nil
}

// BulkMassProperties performs a single-roundtrip mass properties calculation across
// all bodies in the part, avoiding N+1 roundtrips and temporary handle creation.
func (p *Part) BulkMassProperties(ctx context.Context) (*MassProperties, error) {
	analysis, err := p.AnalyzeGeometry(ctx, WithMassProperties(true), WithBoundingBox(false), WithFacesAndEdges(false), WithProduceBodyHandles(false))
	if err != nil {
		return nil, err
	}
	if analysis.AggregateMass != nil {
		return analysis.AggregateMass, nil
	}
	return &MassProperties{SolidType: "empty", Units: analysis.Units}, nil
}

// BulkBoundingBox performs a single-roundtrip bounding box calculation across all
// bodies in the part, avoiding N+1 roundtrips and temporary handle creation.
func (p *Part) BulkBoundingBox(ctx context.Context) (*BoundingBox, error) {
	analysis, err := p.AnalyzeGeometry(ctx, WithMassProperties(false), WithBoundingBox(true), WithFacesAndEdges(false), WithProduceBodyHandles(false))
	if err != nil {
		return nil, err
	}
	if analysis.AggregateBox != nil {
		return analysis.AggregateBox, nil
	}
	return &BoundingBox{Units: analysis.Units}, nil
}

// AnalyzeBodies executes a single-roundtrip bulk query across a specific slice of bodies.
func (s *Session) AnalyzeBodies(ctx context.Context, bodies []*Body, opts ...BulkAnalysisOption) (*PartGeometryAnalysis, error) {
	if s == nil || s.client == nil {
		return nil, ErrSessionClosed
	}
	if len(bodies) == 0 {
		return &PartGeometryAnalysis{
			Units:         "",
			BodyCount:     0,
			AggregateMass: &MassProperties{SolidType: "empty"},
			AggregateBox:  &BoundingBox{},
			Bodies:        nil,
		}, nil
	}

	options := DefaultBulkAnalysisOptions()
	for _, opt := range opts {
		opt(&options)
	}

	bodyRefs := make([]protocol.ObjectHandleWire, 0, len(bodies))
	for _, b := range bodies {
		if b == nil {
			return nil, errors.New("nil body in bodies slice")
		}
		if err := b.validate(); err != nil {
			return nil, fmt.Errorf("invalid body: %w", err)
		}
		bodyRefs = append(bodyRefs, b.Ref)
	}

	reqData, err := protocol.EncodePayload(protocol.GeometryQueryBulkAnalysisRequest{
		BodyRefs:              bodyRefs,
		IncludeMassProperties: options.IncludeMassProperties,
		IncludeBoundingBox:    options.IncludeBoundingBox,
		IncludeFacesAndEdges:  options.IncludeFacesAndEdges,
		ProduceBodyHandles:    options.ProduceBodyHandles,
	})
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("geometry.query_bulk_analysis.bodies"),
		Op:        "geometry.query_bulk_analysis",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.GeometryQueryBulkAnalysisResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return decodePartGeometryAnalysis(payload), nil
}

func decodePartGeometryAnalysis(payload *protocol.GeometryQueryBulkAnalysisResponse) *PartGeometryAnalysis {
	if payload == nil {
		return &PartGeometryAnalysis{}
	}
	result := &PartGeometryAnalysis{
		Units:      payload.Units,
		BodyCount:  payload.BodyCount,
		SolidCount: payload.SolidCount,
		SheetCount: payload.SheetCount,
	}

	if payload.AggregateMass != nil {
		result.AggregateMass = &MassProperties{
			Units:     payload.AggregateMass.Units,
			Volume:    payload.AggregateMass.Volume,
			Area:      payload.AggregateMass.Area,
			Mass:      payload.AggregateMass.Mass,
			Centroid:  payload.AggregateMass.Centroid,
			SolidType: payload.AggregateMass.SolidType,
		}
	}

	if payload.AggregateBox != nil {
		result.AggregateBox = &BoundingBox{
			Units:      payload.AggregateBox.Units,
			MinCorner:  payload.AggregateBox.MinCorner,
			MaxCorner:  payload.AggregateBox.MaxCorner,
			Dimensions: payload.AggregateBox.Dimensions,
		}
	}

	for _, b := range payload.Bodies {
		item := BodyGeometryAnalysis{
			BodyRef:   b.BodyRef,
			Name:      b.Name,
			SolidType: b.SolidType,
			FaceCount: b.FaceCount,
			EdgeCount: b.EdgeCount,
			NativeTag: b.NativeTag,
		}
		if b.MassProperties != nil {
			item.MassProperties = &MassProperties{
				Units:     b.MassProperties.Units,
				Volume:    b.MassProperties.Volume,
				Area:      b.MassProperties.Area,
				Mass:      b.MassProperties.Mass,
				Centroid:  b.MassProperties.Centroid,
				SolidType: b.MassProperties.SolidType,
			}
		}
		if b.BoundingBox != nil {
			item.BoundingBox = &BoundingBox{
				Units:      b.BoundingBox.Units,
				MinCorner:  b.BoundingBox.MinCorner,
				MaxCorner:  b.BoundingBox.MaxCorner,
				Dimensions: b.BoundingBox.Dimensions,
			}
		}
		result.Bodies = append(result.Bodies, item)
	}

	return result
}

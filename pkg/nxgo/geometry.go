package nxgo

import (
	"context"
	"fmt"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
)

type Point3D [3]float64
type Vector3D [3]float64

type BlockParams struct {
	Origin        Point3D
	Length        float64
	Width         float64
	Height        float64
	BooleanOp     string
	TargetBodyRef *protocol.ObjectHandleWire
}

type CylinderParams struct {
	Origin        Point3D
	Direction     Vector3D
	Diameter      float64
	Height        float64
	BooleanOp     string
	TargetBodyRef *protocol.ObjectHandleWire
}

type MassProperties struct {
	Volume    float64
	Area      float64
	Mass      float64
	Centroid  Point3D
	SolidType string
}

type BoundingBox struct {
	MinCorner  Point3D
	MaxCorner  Point3D
	Dimensions Point3D
}

type Feature struct {
	session *Session
	Ref     protocol.ObjectHandleWire
	BodyRef protocol.ObjectHandleWire
	Name    string
	Type    string
}

type Body struct {
	session   *Session
	Ref       protocol.ObjectHandleWire
	Name      string
	SolidType string
	FaceCount int
	EdgeCount int
	NativeTag uint32
}

func (p *Part) CreateBlock(ctx context.Context, params BlockParams) (*Feature, error) {
	reqData, err := protocol.EncodePayload(protocol.FeatureCreateBlockRequest{
		PartRef:       &p.Ref,
		Origin:        params.Origin,
		Length:        params.Length,
		Width:         params.Width,
		Height:        params.Height,
		BooleanOp:     params.BooleanOp,
		TargetBodyRef: params.TargetBodyRef,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-block-%d", time.Now().UnixNano()),
		Op:        "feature.create_block",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.FeatureCreateBlockResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &Feature{
		session: p.session,
		Ref:     payload.FeatureRef,
		BodyRef: payload.BodyRef,
		Name:    payload.FeatureName,
		Type:    payload.FeatureType,
	}, nil
}

func (p *Part) CreateCylinder(ctx context.Context, params CylinderParams) (*Feature, error) {
	reqData, err := protocol.EncodePayload(protocol.FeatureCreateCylinderRequest{
		PartRef:       &p.Ref,
		Origin:        params.Origin,
		Direction:     params.Direction,
		Diameter:      params.Diameter,
		Height:        params.Height,
		BooleanOp:     params.BooleanOp,
		TargetBodyRef: params.TargetBodyRef,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-cyl-%d", time.Now().UnixNano()),
		Op:        "feature.create_cylinder",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.FeatureCreateCylinderResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &Feature{
		session: p.session,
		Ref:     payload.FeatureRef,
		BodyRef: payload.BodyRef,
		Name:    payload.FeatureName,
		Type:    "Cylinder",
	}, nil
}

func (p *Part) Bodies(ctx context.Context) ([]*Body, error) {
	reqData, err := protocol.EncodePayload(protocol.PartQueryBodiesRequest{
		PartRef: &p.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-bodies-%d", time.Now().UnixNano()),
		Op:        "part.query_bodies",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.PartQueryBodiesResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	var bodies []*Body
	for _, b := range payload.Bodies {
		bodies = append(bodies, &Body{
			session:   p.session,
			Ref:       b.BodyRef,
			Name:      b.Name,
			SolidType: b.SolidType,
			FaceCount: b.FaceCount,
			EdgeCount: b.EdgeCount,
			NativeTag: b.NativeTag,
		})
	}
	return bodies, nil
}

func (p *Part) MassProperties(ctx context.Context) (*MassProperties, error) {
	reqData, err := protocol.EncodePayload(protocol.GeometryQueryMassPropertiesRequest{
		PartRef: &p.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-mass-%d", time.Now().UnixNano()),
		Op:        "geometry.query_mass_properties",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.GeometryQueryMassPropertiesResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &MassProperties{
		Volume:    payload.Volume,
		Area:      payload.Area,
		Mass:      payload.Mass,
		Centroid:  payload.Centroid,
		SolidType: payload.SolidType,
	}, nil
}

func (p *Part) BoundingBox(ctx context.Context) (*BoundingBox, error) {
	reqData, err := protocol.EncodePayload(protocol.GeometryQueryBoundingBoxRequest{
		PartRef: &p.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-bbox-%d", time.Now().UnixNano()),
		Op:        "geometry.query_bounding_box",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.GeometryQueryBoundingBoxResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &BoundingBox{
		MinCorner:  payload.MinCorner,
		MaxCorner:  payload.MaxCorner,
		Dimensions: payload.Dimensions,
	}, nil
}

func (b *Body) MassProperties(ctx context.Context) (*MassProperties, error) {
	reqData, err := protocol.EncodePayload(protocol.GeometryQueryMassPropertiesRequest{
		BodyRef: &b.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := b.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-bodymass-%d", time.Now().UnixNano()),
		Op:        "geometry.query_mass_properties",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.GeometryQueryMassPropertiesResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &MassProperties{
		Volume:    payload.Volume,
		Area:      payload.Area,
		Mass:      payload.Mass,
		Centroid:  payload.Centroid,
		SolidType: payload.SolidType,
	}, nil
}

func (b *Body) BoundingBox(ctx context.Context) (*BoundingBox, error) {
	reqData, err := protocol.EncodePayload(protocol.GeometryQueryBoundingBoxRequest{
		BodyRef: &b.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := b.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-bodybbox-%d", time.Now().UnixNano()),
		Op:        "geometry.query_bounding_box",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.GeometryQueryBoundingBoxResponse](resp.Payload)
	if err != nil {
		return nil, err
	}

	return &BoundingBox{
		MinCorner:  payload.MinCorner,
		MaxCorner:  payload.MaxCorner,
		Dimensions: payload.Dimensions,
	}, nil
}

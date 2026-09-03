package nxgo

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
)

var ErrUnsupportedFeatureOption = errors.New("feature option is not implemented by the active NXGO backend")

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
	Units     string
	Volume    float64
	Area      float64
	Mass      float64
	Centroid  Point3D
	SolidType string
}

type BoundingBox struct {
	Units      string
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

func validateCreateFeatureOptions(s *Session, booleanOp string, targetBodyRef *protocol.ObjectHandleWire) error {
	op := strings.TrimSpace(strings.ToLower(booleanOp))
	if op != "" && op != "create" {
		return fmt.Errorf("%w: boolean operation %q; only create is currently enforced", ErrUnsupportedFeatureOption, booleanOp)
	}
	if targetBodyRef != nil {
		if err := s.validateObjectHandle(targetBodyRef, "Body"); err != nil {
			return err
		}
		return fmt.Errorf("%w: target_body_ref is not yet honored by feature creation", ErrUnsupportedFeatureOption)
	}
	return nil
}

func (p *Part) CreateBlock(ctx context.Context, params BlockParams) (*Feature, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if err := validateCreateFeatureOptions(p.session, params.BooleanOp, params.TargetBodyRef); err != nil {
		return nil, err
	}
	reqData, err := protocol.EncodePayload(protocol.FeatureCreateBlockRequest{
		PartRef:       &p.Ref,
		Units:         p.Units,
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
		RequestID: newRequestID("feature.create_block"),
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
	if err := p.validate(); err != nil {
		return nil, err
	}
	if err := validateCreateFeatureOptions(p.session, params.BooleanOp, params.TargetBodyRef); err != nil {
		return nil, err
	}
	reqData, err := protocol.EncodePayload(protocol.FeatureCreateCylinderRequest{
		PartRef:       &p.Ref,
		Units:         p.Units,
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
		RequestID: newRequestID("feature.create_cylinder"),
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
	if err := p.validate(); err != nil {
		return nil, err
	}
	reqData, err := protocol.EncodePayload(protocol.PartQueryBodiesRequest{
		PartRef: &p.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("part.query_bodies"),
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

// MassProperties deliberately aggregates explicit Body results instead of
// sending a PartRef to the current production Agent. The audited Agent can
// otherwise fall back to the first body, which is semantically wrong for
// multi-body parts. This N+1 implementation is a safety bridge until the
// backend exposes a verified bulk part-level operation.
func (p *Part) MassProperties(ctx context.Context) (*MassProperties, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	bodies, err := p.Bodies(ctx)
	if err != nil {
		return nil, err
	}
	defer p.releaseTemporaryBodies(bodies)

	if len(bodies) == 0 {
		return &MassProperties{SolidType: "empty"}, nil
	}

	items := make([]*MassProperties, 0, len(bodies))
	for _, body := range bodies {
		props, err := body.MassProperties(ctx)
		if err != nil {
			return nil, fmt.Errorf("mass properties for body %s: %w", body.Ref.ObjectID, err)
		}
		items = append(items, props)
	}
	return aggregateMassProperties(items), nil
}

// BoundingBox uses explicit BodyRefs for the same fail-closed reason as
// MassProperties: a part-level resolver must never silently select one body.
func (p *Part) BoundingBox(ctx context.Context) (*BoundingBox, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	bodies, err := p.Bodies(ctx)
	if err != nil {
		return nil, err
	}
	defer p.releaseTemporaryBodies(bodies)

	if len(bodies) == 0 {
		return &BoundingBox{}, nil
	}

	boxes := make([]*BoundingBox, 0, len(bodies))
	for _, body := range bodies {
		box, err := body.BoundingBox(ctx)
		if err != nil {
			return nil, fmt.Errorf("bounding box for body %s: %w", body.Ref.ObjectID, err)
		}
		boxes = append(boxes, box)
	}
	return aggregateBoundingBoxes(boxes), nil
}

func (b *Body) MassProperties(ctx context.Context) (*MassProperties, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	reqData, err := protocol.EncodePayload(protocol.GeometryQueryMassPropertiesRequest{
		BodyRef: &b.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := b.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("geometry.query_mass_properties.body"),
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
		Units:     payload.Units,
		Volume:    payload.Volume,
		Area:      payload.Area,
		Mass:      payload.Mass,
		Centroid:  payload.Centroid,
		SolidType: payload.SolidType,
	}, nil
}

func (b *Body) BoundingBox(ctx context.Context) (*BoundingBox, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	reqData, err := protocol.EncodePayload(protocol.GeometryQueryBoundingBoxRequest{
		BodyRef: &b.Ref,
	})
	if err != nil {
		return nil, err
	}

	resp, err := b.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("geometry.query_bounding_box.body"),
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
		Units:      payload.Units,
		MinCorner:  payload.MinCorner,
		MaxCorner:  payload.MaxCorner,
		Dimensions: payload.Dimensions,
	}, nil
}

func aggregateMassProperties(items []*MassProperties) *MassProperties {
	result := &MassProperties{SolidType: "aggregate"}
	if len(items) == 0 {
		result.SolidType = "empty"
		return result
	}

	var massWeighted Point3D
	var volumeWeighted Point3D
	for _, item := range items {
		if item == nil {
			continue
		}
		if result.Units == "" && item.Units != "" {
			result.Units = item.Units
		}
		result.Volume += item.Volume
		result.Area += item.Area
		result.Mass += item.Mass
		for axis := 0; axis < 3; axis++ {
			massWeighted[axis] += item.Centroid[axis] * item.Mass
			volumeWeighted[axis] += item.Centroid[axis] * item.Volume
		}
	}

	if math.Abs(result.Mass) > 1e-15 {
		for axis := 0; axis < 3; axis++ {
			result.Centroid[axis] = massWeighted[axis] / result.Mass
		}
	} else if math.Abs(result.Volume) > 1e-15 {
		for axis := 0; axis < 3; axis++ {
			result.Centroid[axis] = volumeWeighted[axis] / result.Volume
		}
	}
	return result
}

func aggregateBoundingBoxes(boxes []*BoundingBox) *BoundingBox {
	if len(boxes) == 0 {
		return &BoundingBox{}
	}

	result := &BoundingBox{
		MinCorner: Point3D{math.Inf(1), math.Inf(1), math.Inf(1)},
		MaxCorner: Point3D{math.Inf(-1), math.Inf(-1), math.Inf(-1)},
	}
	seen := false
	for _, box := range boxes {
		if box == nil {
			continue
		}
		seen = true
		if result.Units == "" && box.Units != "" {
			result.Units = box.Units
		}
		for axis := 0; axis < 3; axis++ {
			result.MinCorner[axis] = math.Min(result.MinCorner[axis], box.MinCorner[axis])
			result.MaxCorner[axis] = math.Max(result.MaxCorner[axis], box.MaxCorner[axis])
		}
	}
	if !seen {
		return &BoundingBox{}
	}
	for axis := 0; axis < 3; axis++ {
		result.Dimensions[axis] = result.MaxCorner[axis] - result.MinCorner[axis]
	}
	return result
}

func (p *Part) releaseTemporaryBodies(bodies []*Body) {
	if p == nil || p.session == nil || len(bodies) == 0 {
		return
	}
	refs := make([]protocol.ObjectHandleWire, 0, len(bodies))
	for _, body := range bodies {
		if body != nil && body.Ref.ObjectID != "" {
			refs = append(refs, body.Ref)
		}
	}
	if len(refs) == 0 {
		return
	}

	// Cleanup has its own bounded context. If even handle release becomes
	// ambiguous, the transport's quarantine hook will recycle the worker rather
	// than allow an uncertain session to continue.
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.session.ReleaseObjects(cleanupCtx, refs...)
}

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

type HoleType string

const (
	HoleTypeSimple      HoleType = "simple"
	HoleTypeCounterbore HoleType = "counterbore"
	HoleTypeCountersink HoleType = "countersink"
)

type HoleParams struct {
	Type                HoleType
	TargetBodyRef       protocol.ObjectHandleWire
	FaceRef             *protocol.ObjectHandleWire
	Origin              Point3D
	Direction           Vector3D
	Diameter            float64
	Depth               float64
	TipAngle            float64
	ThroughBody         bool
	CounterboreDiameter float64
	CounterboreDepth    float64
	CountersinkDiameter float64
	CountersinkAngle    float64
}

type DatumPlaneParams struct {
	Origin    Point3D
	Direction Vector3D // normal vector, defaults to [0, 0, 1]
}

type DatumPlane struct {
	session    *Session
	Ref        protocol.ObjectHandleWire
	FeatureRef protocol.ObjectHandleWire
	Name       string
}

type DatumAxisParams struct {
	Origin    Point3D
	Direction Vector3D // axis direction, defaults to [0, 0, 1]
}

type DatumAxis struct {
	session    *Session
	Ref        protocol.ObjectHandleWire
	FeatureRef protocol.ObjectHandleWire
	Name       string
}

type DatumCsysParams struct {
	Origin     Point3D
	XDirection Vector3D // defaults to [1, 0, 0]
	YDirection Vector3D // defaults to [0, 1, 0]
}

type DatumCsys struct {
	session    *Session
	Ref        protocol.ObjectHandleWire
	FeatureRef protocol.ObjectHandleWire
	Name       string
}

type SketchParams struct {
	Name     string
	PlaneRef *protocol.ObjectHandleWire
}

type Sketch struct {
	part       *Part
	session    *Session
	Ref        protocol.ObjectHandleWire
	FeatureRef protocol.ObjectHandleWire
	Name       string
}

type SketchLine2D struct {
	Start [2]float64
	End   [2]float64
}

type SketchCircle2D struct {
	Center [2]float64
	Radius float64
}

type SketchArc2D struct {
	Center     [2]float64
	Radius     float64
	StartAngle float64
	EndAngle   float64
}

type SketchRect2D struct {
	Origin [2]float64
	Width  float64
	Height float64
}

type SketchAddGeometryParams struct {
	Lines      []SketchLine2D
	Circles    []SketchCircle2D
	Arcs       []SketchArc2D
	Rectangles []SketchRect2D
}

type SketchStatus struct {
	Status     string
	DOFNeeded  int
	CurveCount int
}

type ProfileParams struct {
	ChainingTolerance float64
	DistanceTolerance float64
}

type Profile struct {
	session   *Session
	Ref       protocol.ObjectHandleWire
	SketchRef protocol.ObjectHandleWire
	Name      string
	LoopCount int
}

type ExtrudeParams struct {
	ProfileRef    protocol.ObjectHandleWire
	Direction     Vector3D // defaults to [0, 0, 1]
	StartLimit    float64  // default 0.0
	EndLimit      float64  // extrude distance / height
	BooleanOp     string   // "create", "unite", "subtract", "intersect"
	TargetBodyRef *protocol.ObjectHandleWire
}

type RevolveParams struct {
	ProfileRef    protocol.ObjectHandleWire
	AxisOrigin    Point3D
	AxisDirection Vector3D // defaults to [0, 0, 1]
	StartAngle    float64  // degrees, default 0.0
	EndAngle      float64  // degrees, e.g. 360.0
	BooleanOp     string   // "create", "unite", "subtract", "intersect"
	TargetBodyRef *protocol.ObjectHandleWire
}

type FilletParams struct {
	BodyRef  *protocol.ObjectHandleWire
	EdgeRefs []protocol.ObjectHandleWire
	Radius   float64
}

type ChamferParams struct {
	BodyRef        *protocol.ObjectHandleWire
	EdgeRefs       []protocol.ObjectHandleWire
	Distance       float64
	SecondDistance float64 // used for "two_offsets"
	Angle          float64 // used for "offset_and_angle" in degrees
	Option         string  // "symmetric", "two_offsets", "offset_and_angle"
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
	part      *Part
	Ref       protocol.ObjectHandleWire
	Name      string
	SolidType string
	FaceCount int
	EdgeCount int
	NativeTag uint32
}

func validateCreateFeatureOptions(s *Session, booleanOp string, targetBodyRef *protocol.ObjectHandleWire) error {
	op := strings.TrimSpace(strings.ToLower(booleanOp))
	if op == "" || op == "create" {
		if targetBodyRef != nil {
			return fmt.Errorf("%w: target_body_ref cannot be specified with boolean create", ErrUnsupportedFeatureOption)
		}
		return nil
	}
	switch op {
	case "unite", "subtract", "intersect":
		if targetBodyRef == nil {
			return fmt.Errorf("%w: target_body_ref is required for boolean operation %q", ErrUnsupportedFeatureOption, booleanOp)
		}
		return s.validateObjectHandle(targetBodyRef, "Body")
	default:
		return fmt.Errorf("%w: boolean operation %q; only create, unite, subtract, intersect are supported", ErrUnsupportedFeatureOption, booleanOp)
	}
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

type BooleanParams struct {
	Op            string // "unite", "subtract", "intersect"
	TargetBodyRef protocol.ObjectHandleWire
	ToolBodyRefs  []protocol.ObjectHandleWire
}

func (p *Part) Boolean(ctx context.Context, params BooleanParams) (*Feature, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	op := strings.TrimSpace(strings.ToLower(params.Op))
	if op != "unite" && op != "subtract" && op != "intersect" {
		return nil, fmt.Errorf("%w: boolean op %q (supported: unite, subtract, intersect)", ErrUnsupportedFeatureOption, params.Op)
	}
	if err := p.session.validateObjectHandle(&params.TargetBodyRef, "Body"); err != nil {
		return nil, err
	}
	if len(params.ToolBodyRefs) == 0 {
		return nil, errors.New("at least one tool body is required for boolean operation")
	}
	for i := range params.ToolBodyRefs {
		if err := p.session.validateObjectHandle(&params.ToolBodyRefs[i], "Body"); err != nil {
			return nil, err
		}
	}

	reqData, err := protocol.EncodePayload(protocol.FeatureBooleanRequest{
		PartRef:       &p.Ref,
		Op:            op,
		TargetBodyRef: &params.TargetBodyRef,
		ToolBodyRefs:  params.ToolBodyRefs,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("feature.boolean"),
		Op:        "feature.boolean",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.FeatureBooleanResponse](resp.Payload)
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

func (p *Part) CreateHole(ctx context.Context, params HoleParams) (*Feature, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if err := p.session.validateObjectHandle(&params.TargetBodyRef, "Body"); err != nil {
		return nil, err
	}
	if params.FaceRef != nil {
		if err := p.session.validateObjectHandle(params.FaceRef, "Face"); err != nil {
			return nil, err
		}
	}
	if params.Diameter <= 0 {
		return nil, errors.New("hole diameter must be positive")
	}
	if !params.ThroughBody && params.Depth <= 0 {
		return nil, errors.New("hole depth must be positive for blind hole")
	}

	hType := strings.TrimSpace(strings.ToLower(string(params.Type)))
	if hType == "" {
		hType = string(HoleTypeSimple)
	}
	switch HoleType(hType) {
	case HoleTypeSimple:
		// simple hole
	case HoleTypeCounterbore:
		if params.CounterboreDiameter <= params.Diameter {
			return nil, errors.New("counterbore diameter must be greater than hole diameter")
		}
		if params.CounterboreDepth <= 0 {
			return nil, errors.New("counterbore depth must be positive")
		}
	case HoleTypeCountersink:
		if params.CountersinkDiameter <= params.Diameter {
			return nil, errors.New("countersink diameter must be greater than hole diameter")
		}
		if params.CountersinkAngle <= 0 || params.CountersinkAngle >= 180 {
			return nil, errors.New("countersink angle must be between 0 and 180 degrees")
		}
	default:
		return nil, fmt.Errorf("%w: unsupported hole type %q (supported: simple, counterbore, countersink)", ErrUnsupportedFeatureOption, params.Type)
	}

	dir := params.Direction
	if dir[0] == 0 && dir[1] == 0 && dir[2] == 0 {
		dir = Vector3D{0, 0, -1}
	}

	tipAngle := params.TipAngle
	if tipAngle <= 0 {
		tipAngle = 118.0
	}

	reqData, err := protocol.EncodePayload(protocol.FeatureCreateHoleRequest{
		PartRef:             &p.Ref,
		TargetBodyRef:       &params.TargetBodyRef,
		FaceRef:             params.FaceRef,
		Units:               p.Units,
		HoleType:            hType,
		Origin:              params.Origin,
		Direction:           dir,
		Diameter:            params.Diameter,
		Depth:               params.Depth,
		TipAngle:            tipAngle,
		ThroughBody:         params.ThroughBody,
		CounterboreDiameter: params.CounterboreDiameter,
		CounterboreDepth:    params.CounterboreDepth,
		CountersinkDiameter: params.CountersinkDiameter,
		CountersinkAngle:    params.CountersinkAngle,
	})
	if err != nil {
		return nil, err
	}

	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("feature.create_hole"),
		Op:        "feature.create_hole",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	payload, err := protocol.DecodePayload[protocol.FeatureCreateHoleResponse](resp.Payload)
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

func (p *Part) CreateDatumPlane(ctx context.Context, params DatumPlaneParams) (*DatumPlane, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	dir := params.Direction
	if dir[0] == 0 && dir[1] == 0 && dir[2] == 0 {
		dir = Vector3D{0, 0, 1}
	}
	reqData, err := protocol.EncodePayload(protocol.DatumCreatePlaneRequest{
		PartRef:   &p.Ref,
		Origin:    params.Origin,
		Direction: dir,
	})
	if err != nil {
		return nil, err
	}
	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("datum.create_plane"),
		Op:        "datum.create_plane",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}
	payload, err := protocol.DecodePayload[protocol.DatumCreatePlaneResponse](resp.Payload)
	if err != nil {
		return nil, err
	}
	return &DatumPlane{
		session:    p.session,
		Ref:        payload.PlaneRef,
		FeatureRef: payload.FeatureRef,
		Name:       payload.Name,
	}, nil
}

func (p *Part) CreateDatumAxis(ctx context.Context, params DatumAxisParams) (*DatumAxis, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	dir := params.Direction
	if dir[0] == 0 && dir[1] == 0 && dir[2] == 0 {
		dir = Vector3D{0, 0, 1}
	}
	reqData, err := protocol.EncodePayload(protocol.DatumCreateAxisRequest{
		PartRef:   &p.Ref,
		Origin:    params.Origin,
		Direction: dir,
	})
	if err != nil {
		return nil, err
	}
	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("datum.create_axis"),
		Op:        "datum.create_axis",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}
	payload, err := protocol.DecodePayload[protocol.DatumCreateAxisResponse](resp.Payload)
	if err != nil {
		return nil, err
	}
	return &DatumAxis{
		session:    p.session,
		Ref:        payload.AxisRef,
		FeatureRef: payload.FeatureRef,
		Name:       payload.Name,
	}, nil
}

func (p *Part) CreateDatumCsys(ctx context.Context, params DatumCsysParams) (*DatumCsys, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	xDir := params.XDirection
	if xDir[0] == 0 && xDir[1] == 0 && xDir[2] == 0 {
		xDir = Vector3D{1, 0, 0}
	}
	yDir := params.YDirection
	if yDir[0] == 0 && yDir[1] == 0 && yDir[2] == 0 {
		yDir = Vector3D{0, 1, 0}
	}
	reqData, err := protocol.EncodePayload(protocol.DatumCreateCsysRequest{
		PartRef:    &p.Ref,
		Origin:     params.Origin,
		XDirection: xDir,
		YDirection: yDir,
	})
	if err != nil {
		return nil, err
	}
	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("datum.create_csys"),
		Op:        "datum.create_csys",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}
	payload, err := protocol.DecodePayload[protocol.DatumCreateCsysResponse](resp.Payload)
	if err != nil {
		return nil, err
	}
	return &DatumCsys{
		session:    p.session,
		Ref:        payload.CsysRef,
		FeatureRef: payload.FeatureRef,
		Name:       payload.Name,
	}, nil
}

func (p *Part) CreateSketch(ctx context.Context, params SketchParams) (*Sketch, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if params.PlaneRef != nil {
		if err := p.session.validateObjectHandle(params.PlaneRef, "DatumPlane", "Face"); err != nil {
			return nil, err
		}
	}
	reqData, err := protocol.EncodePayload(protocol.SketchCreateRequest{
		PartRef:  &p.Ref,
		Name:     params.Name,
		PlaneRef: params.PlaneRef,
	})
	if err != nil {
		return nil, err
	}
	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("sketch.create"),
		Op:        "sketch.create",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}
	payload, err := protocol.DecodePayload[protocol.SketchCreateResponse](resp.Payload)
	if err != nil {
		return nil, err
	}
	return &Sketch{
		part:       p,
		session:    p.session,
		Ref:        payload.SketchRef,
		FeatureRef: payload.FeatureRef,
		Name:       payload.Name,
	}, nil
}

func (s *Sketch) validate() error {
	if s == nil || s.session == nil || s.part == nil {
		return ErrSessionClosed
	}
	return s.session.validateObjectHandle(&s.Ref, "Sketch")
}

func (s *Sketch) AddGeometry(ctx context.Context, params SketchAddGeometryParams) (*protocol.SketchAddGeometryResponse, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	linesWire := make([]protocol.SketchLine2DWire, len(params.Lines))
	for i, l := range params.Lines {
		linesWire[i] = protocol.SketchLine2DWire{Start: l.Start, End: l.End}
	}
	circlesWire := make([]protocol.SketchCircle2DWire, len(params.Circles))
	for i, c := range params.Circles {
		circlesWire[i] = protocol.SketchCircle2DWire{Center: c.Center, Radius: c.Radius}
	}
	arcsWire := make([]protocol.SketchArc2DWire, len(params.Arcs))
	for i, a := range params.Arcs {
		arcsWire[i] = protocol.SketchArc2DWire{Center: a.Center, Radius: a.Radius, StartAngle: a.StartAngle, EndAngle: a.EndAngle}
	}
	rectsWire := make([]protocol.SketchRect2DWire, len(params.Rectangles))
	for i, r := range params.Rectangles {
		rectsWire[i] = protocol.SketchRect2DWire{Origin: r.Origin, Width: r.Width, Height: r.Height}
	}

	reqData, err := protocol.EncodePayload(protocol.SketchAddGeometryRequest{
		PartRef:    &s.part.Ref,
		SketchRef:  &s.Ref,
		Lines:      linesWire,
		Circles:    circlesWire,
		Arcs:       arcsWire,
		Rectangles: rectsWire,
	})
	if err != nil {
		return nil, err
	}
	resp, err := s.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("sketch.add_geometry"),
		Op:        "sketch.add_geometry",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}
	payload, err := protocol.DecodePayload[protocol.SketchAddGeometryResponse](resp.Payload)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *Sketch) AddLine(ctx context.Context, start, end [2]float64) error {
	_, err := s.AddGeometry(ctx, SketchAddGeometryParams{
		Lines: []SketchLine2D{{Start: start, End: end}},
	})
	return err
}

func (s *Sketch) AddCircle(ctx context.Context, center [2]float64, radius float64) error {
	if radius <= 0 {
		return errors.New("circle radius must be positive")
	}
	_, err := s.AddGeometry(ctx, SketchAddGeometryParams{
		Circles: []SketchCircle2D{{Center: center, Radius: radius}},
	})
	return err
}

func (s *Sketch) AddArc(ctx context.Context, center [2]float64, radius, startAngle, endAngle float64) error {
	if radius <= 0 {
		return errors.New("arc radius must be positive")
	}
	_, err := s.AddGeometry(ctx, SketchAddGeometryParams{
		Arcs: []SketchArc2D{{Center: center, Radius: radius, StartAngle: startAngle, EndAngle: endAngle}},
	})
	return err
}

func (s *Sketch) AddRectangle(ctx context.Context, origin [2]float64, width, height float64) error {
	if width <= 0 || height <= 0 {
		return errors.New("rectangle dimensions must be positive")
	}
	_, err := s.AddGeometry(ctx, SketchAddGeometryParams{
		Rectangles: []SketchRect2D{{Origin: origin, Width: width, Height: height}},
	})
	return err
}

func (s *Sketch) QueryStatus(ctx context.Context) (*SketchStatus, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	reqData, err := protocol.EncodePayload(protocol.SketchQueryStatusRequest{
		SketchRef: &s.Ref,
	})
	if err != nil {
		return nil, err
	}
	resp, err := s.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("sketch.query_status"),
		Op:        "sketch.query_status",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}
	payload, err := protocol.DecodePayload[protocol.SketchQueryStatusResponse](resp.Payload)
	if err != nil {
		return nil, err
	}
	return &SketchStatus{
		Status:     payload.Status,
		DOFNeeded:  payload.DOFNeeded,
		CurveCount: payload.CurveCount,
	}, nil
}

func (s *Sketch) CreateProfile(ctx context.Context, params ProfileParams) (*Profile, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	reqData, err := protocol.EncodePayload(protocol.ProfileCreateRequest{
		PartRef:           &s.part.Ref,
		SketchRef:         &s.Ref,
		ChainingTolerance: params.ChainingTolerance,
		DistanceTolerance: params.DistanceTolerance,
	})
	if err != nil {
		return nil, err
	}
	resp, err := s.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("sketch.create_profile"),
		Op:        "sketch.create_profile",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}
	payload, err := protocol.DecodePayload[protocol.ProfileCreateResponse](resp.Payload)
	if err != nil {
		return nil, err
	}
	return &Profile{
		session:   s.session,
		Ref:       payload.ProfileRef,
		SketchRef: s.Ref,
		Name:      payload.Name,
		LoopCount: payload.LoopCount,
	}, nil
}

func (p *Part) Extrude(ctx context.Context, params ExtrudeParams) (*Feature, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if err := p.session.validateObjectHandle(&params.ProfileRef, "Profile", "Section"); err != nil {
		return nil, err
	}
	if err := validateCreateFeatureOptions(p.session, params.BooleanOp, params.TargetBodyRef); err != nil {
		return nil, err
	}
	if params.EndLimit <= params.StartLimit {
		return nil, errors.New("extrude end_limit must be greater than start_limit")
	}

	dir := params.Direction
	if dir[0] == 0 && dir[1] == 0 && dir[2] == 0 {
		dir = Vector3D{0, 0, 1}
	}

	reqData, err := protocol.EncodePayload(protocol.FeatureCreateExtrudeRequest{
		PartRef:       &p.Ref,
		ProfileRef:    &params.ProfileRef,
		Direction:     dir,
		StartLimit:    params.StartLimit,
		EndLimit:      params.EndLimit,
		BooleanOp:     params.BooleanOp,
		TargetBodyRef: params.TargetBodyRef,
	})
	if err != nil {
		return nil, err
	}
	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("feature.create_extrude"),
		Op:        "feature.create_extrude",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}
	payload, err := protocol.DecodePayload[protocol.FeatureCreateExtrudeResponse](resp.Payload)
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

func (p *Part) Revolve(ctx context.Context, params RevolveParams) (*Feature, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if err := p.session.validateObjectHandle(&params.ProfileRef, "Profile", "Section"); err != nil {
		return nil, err
	}
	if err := validateCreateFeatureOptions(p.session, params.BooleanOp, params.TargetBodyRef); err != nil {
		return nil, err
	}
	if params.EndAngle <= params.StartAngle {
		return nil, errors.New("revolve end_angle must be greater than start_angle")
	}

	axisDir := params.AxisDirection
	if axisDir[0] == 0 && axisDir[1] == 0 && axisDir[2] == 0 {
		axisDir = Vector3D{0, 0, 1}
	}

	reqData, err := protocol.EncodePayload(protocol.FeatureCreateRevolveRequest{
		PartRef:       &p.Ref,
		ProfileRef:    &params.ProfileRef,
		AxisOrigin:    params.AxisOrigin,
		AxisDirection: axisDir,
		StartAngle:    params.StartAngle,
		EndAngle:      params.EndAngle,
		BooleanOp:     params.BooleanOp,
		TargetBodyRef: params.TargetBodyRef,
	})
	if err != nil {
		return nil, err
	}
	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("feature.create_revolve"),
		Op:        "feature.create_revolve",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}
	payload, err := protocol.DecodePayload[protocol.FeatureCreateRevolveResponse](resp.Payload)
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

func (p *Part) CreateFillet(ctx context.Context, params FilletParams) (*Feature, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if params.Radius <= 0 {
		return nil, errors.New("fillet radius must be greater than zero")
	}
	if params.BodyRef == nil && len(params.EdgeRefs) == 0 {
		return nil, errors.New("fillet requires either body_ref or edge_refs")
	}
	if params.BodyRef != nil {
		if err := p.session.validateObjectHandle(params.BodyRef, "Body"); err != nil {
			return nil, err
		}
	}
	for i := range params.EdgeRefs {
		if err := p.session.validateObjectHandle(&params.EdgeRefs[i], "Edge"); err != nil {
			return nil, err
		}
	}

	reqData, err := protocol.EncodePayload(protocol.FeatureCreateFilletRequest{
		PartRef:  &p.Ref,
		BodyRef:  params.BodyRef,
		EdgeRefs: params.EdgeRefs,
		Radius:   params.Radius,
	})
	if err != nil {
		return nil, err
	}
	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("feature.create_fillet"),
		Op:        "feature.create_fillet",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}
	payload, err := protocol.DecodePayload[protocol.FeatureCreateFilletResponse](resp.Payload)
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

func (p *Part) CreateChamfer(ctx context.Context, params ChamferParams) (*Feature, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if params.Distance <= 0 {
		return nil, errors.New("chamfer distance must be greater than zero")
	}
	if params.BodyRef == nil && len(params.EdgeRefs) == 0 {
		return nil, errors.New("chamfer requires either body_ref or edge_refs")
	}
	opt := strings.TrimSpace(strings.ToLower(params.Option))
	if opt == "" {
		opt = "symmetric"
	}
	switch opt {
	case "symmetric":
	case "two_offsets":
		if params.SecondDistance <= 0 {
			return nil, errors.New("chamfer second_distance must be greater than zero for two_offsets")
		}
	case "offset_and_angle":
		if params.Angle <= 0 || params.Angle >= 90 {
			return nil, errors.New("chamfer angle must be between 0 and 90 degrees")
		}
	default:
		return nil, fmt.Errorf("unsupported chamfer option: %s", params.Option)
	}

	if params.BodyRef != nil {
		if err := p.session.validateObjectHandle(params.BodyRef, "Body"); err != nil {
			return nil, err
		}
	}
	for i := range params.EdgeRefs {
		if err := p.session.validateObjectHandle(&params.EdgeRefs[i], "Edge"); err != nil {
			return nil, err
		}
	}

	reqData, err := protocol.EncodePayload(protocol.FeatureCreateChamferRequest{
		PartRef:        &p.Ref,
		BodyRef:        params.BodyRef,
		EdgeRefs:       params.EdgeRefs,
		Distance:       params.Distance,
		SecondDistance: params.SecondDistance,
		Angle:          params.Angle,
		Option:         opt,
	})
	if err != nil {
		return nil, err
	}
	resp, err := p.session.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("feature.create_chamfer"),
		Op:        "feature.create_chamfer",
		Payload:   reqData,
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}
	payload, err := protocol.DecodePayload[protocol.FeatureCreateChamferResponse](resp.Payload)
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
			part:      p,
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

func (b *Body) CreateFillet(ctx context.Context, radius float64) (*Feature, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	if b.part == nil {
		return nil, errors.New("body is not attached to an active part")
	}
	return b.part.CreateFillet(ctx, FilletParams{
		BodyRef: &b.Ref,
		Radius:  radius,
	})
}

func (b *Body) CreateChamfer(ctx context.Context, distance float64) (*Feature, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	if b.part == nil {
		return nil, errors.New("body is not attached to an active part")
	}
	return b.part.CreateChamfer(ctx, ChamferParams{
		BodyRef:  &b.Ref,
		Distance: distance,
	})
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

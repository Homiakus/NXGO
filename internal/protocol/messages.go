package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	// v2 makes ObjectHandleWire.Generation mandatory. This is intentionally a
	// major-version boundary: v1 peers cannot safely reason about handle reuse.
	CurrentProtocolMajor = 2
	CurrentProtocolMinor = 0
)

var (
	ErrMajorVersionMismatch = errors.New("protocol major version mismatch")
	ErrEmptyRequestID       = errors.New("request_id cannot be empty")
	ErrEmptyOperation       = errors.New("operation name cannot be empty")
	ErrInvalidSessionHealth = errors.New("invalid session health state")
	ErrEmptyNonce           = errors.New("handshake nonce cannot be empty")
)

type Version struct {
	Major uint32 `json:"major"`
	Minor uint32 `json:"minor"`
}

func (v Version) String() string {
	return fmt.Sprintf("v%d.%d", v.Major, v.Minor)
}

func NegotiateVersion(client, server Version) error {
	if client.Major != server.Major {
		return fmt.Errorf("%w: client requested %s, server supports major %d", ErrMajorVersionMismatch, client, server.Major)
	}
	return nil
}

type HandshakeRequest struct {
	ProtocolVersion   Version  `json:"protocol_version"`
	SDKVersion        string   `json:"sdk_version"`
	RequestedMode     string   `json:"requested_mode,omitempty"`
	RequestedFeatures []string `json:"requested_features,omitempty"`
	ClientPID         int      `json:"client_pid"`
	ClientUser        string   `json:"client_user,omitempty"`
	Nonce             string   `json:"nonce"`
}

func (h *HandshakeRequest) Validate() error {
	if h.Nonce == "" {
		return ErrEmptyNonce
	}
	return nil
}

type HandshakeResponse struct {
	ProtocolVersion Version  `json:"protocol_version"`
	AgentVersion    string   `json:"agent_version"`
	NXRelease       string   `json:"nx_release,omitempty"`
	NXBuild         string   `json:"nx_build,omitempty"`
	NXProcessID     int      `json:"nx_pid"`
	SessionID       string   `json:"session_id"`
	Epoch           uint64   `json:"epoch"`
	Capabilities    []string `json:"capabilities,omitempty"`
	MaxPayloadBytes int      `json:"max_payload_bytes"`
	SecurityPolicy  string   `json:"security_policy,omitempty"`
}

type RequestEnvelope struct {
	RequestID     string            `json:"request_id"`
	CorrelationID string            `json:"correlation_id,omitempty"`
	RunID         string            `json:"run_id,omitempty"`
	TestID        string            `json:"test_id,omitempty"`
	Op            string            `json:"op"`
	TimeoutMs     int64             `json:"timeout_ms,omitempty"`
	TxID          string            `json:"tx_id,omitempty"`
	Payload       json.RawMessage   `json:"payload,omitempty"`
	TraceMeta     map[string]string `json:"trace_meta,omitempty"`
}

func (r *RequestEnvelope) Validate() error {
	if r.RequestID == "" {
		return ErrEmptyRequestID
	}
	if r.Op == "" {
		return ErrEmptyOperation
	}
	return nil
}

type ResponseStatus string

const (
	StatusOK        ResponseStatus = "OK"
	StatusError     ResponseStatus = "ERROR"
	StatusCancelled ResponseStatus = "CANCELLED"
)

type TimingData struct {
	QueueWaitMs   int64 `json:"queue_wait_ms,omitempty"`
	ExecutionMs   int64 `json:"execution_ms,omitempty"`
	SerializeMs   int64 `json:"serialize_ms,omitempty"`
	TotalDuration int64 `json:"total_duration_ms,omitempty"`
}

type ResponseEnvelope struct {
	RequestID       string             `json:"request_id"`
	Status          ResponseStatus     `json:"status"`
	Payload         json.RawMessage    `json:"payload,omitempty"`
	Error           *ErrorEnvelope     `json:"error,omitempty"`
	Warnings        []string           `json:"warnings,omitempty"`
	ProducedHandles []ObjectHandleWire `json:"produced_handles,omitempty"`
	Timing          TimingData         `json:"timing,omitempty"`
}

type ErrorCategory string

const (
	ErrCategoryInvalidArgument ErrorCategory = "INVALID_ARGUMENT"
	ErrCategoryNXException     ErrorCategory = "NX_EXCEPTION"
	ErrCategorySessionDirty    ErrorCategory = "SESSION_DIRTY"
	ErrCategorySessionLost     ErrorCategory = "SESSION_LOST"
	ErrCategoryTimeout         ErrorCategory = "TIMEOUT"
	ErrCategoryCancelled       ErrorCategory = "CANCELLED"
	ErrCategoryInternal        ErrorCategory = "INTERNAL"
)

type ErrorEnvelope struct {
	Category      ErrorCategory `json:"category"`
	NXErrorCode   int           `json:"nx_error_code,omitempty"`
	Message       string        `json:"message"`
	Op            string        `json:"op,omitempty"`
	Recoverable   bool          `json:"recoverable"`
	SessionHealth string        `json:"session_health"` // healthy | dirty | lost
	CorrelationID string        `json:"correlation_id,omitempty"`
	Diagnostic    string        `json:"diagnostic,omitempty"`
}

func (e *ErrorEnvelope) Validate() error {
	switch e.SessionHealth {
	case "healthy", "dirty", "lost":
		return nil
	default:
		return fmt.Errorf("%w: %q (expected healthy, dirty, or lost)", ErrInvalidSessionHealth, e.SessionHealth)
	}
}

type ObjectHandleWire struct {
	SessionID    string `json:"session_id"`
	Epoch        uint64 `json:"epoch"`
	ObjectID     string `json:"object_id"`
	Generation   uint32 `json:"generation"`
	Kind         string `json:"kind"`
	NativeTag    uint32 `json:"native_tag,omitempty"`
	LeaseScopeID string `json:"lease_scope_id,omitempty"`
}

type StreamKind string

const (
	StreamKindLog      StreamKind = "log"
	StreamKindEvent    StreamKind = "event"
	StreamKindProgress StreamKind = "progress"
)

type StreamEvent struct {
	Kind          StreamKind      `json:"kind"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Sequence      uint64          `json:"seq"`
	Timestamp     time.Time       `json:"timestamp"`
	Payload       json.RawMessage `json:"payload"`
	LossMarker    bool            `json:"loss_marker,omitempty"`
}

func EncodePayload(v any) ([]byte, error) {
	return json.Marshal(v)
}

func DecodePayload[T any](data []byte) (*T, error) {
	var target T
	if err := json.Unmarshal(data, &target); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	return &target, nil
}

// Transaction operation payloads (Phase 5)

type TransactionBeginRequest struct {
	Name string `json:"name,omitempty"`
}

type TransactionBeginResponse struct {
	TxID   string `json:"tx_id"`
	MarkID int    `json:"mark_id"`
}

type TransactionCommitRequest struct {
	TxID string `json:"tx_id"`
}

type TransactionCommitResponse struct {
	Committed bool   `json:"committed"`
	TxID      string `json:"tx_id"`
}

type TransactionRollbackRequest struct {
	TxID string `json:"tx_id"`
}

type TransactionRollbackResponse struct {
	RolledBack bool   `json:"rolled_back"`
	TxID       string `json:"tx_id"`
}

// Part operation payloads (Phase 7)

type PartNewRequest struct {
	Name  string `json:"name"`
	Units string `json:"units,omitempty"` // "mm" (default) or "in"
}

type PartNewResponse struct {
	PartRef ObjectHandleWire `json:"part_ref"`
	Name    string           `json:"name"`
	Units   string           `json:"units"`
}

type PartOpenRequest struct {
	Path string `json:"path"`
}

type PartOpenResponse struct {
	PartRef ObjectHandleWire `json:"part_ref"`
	Name    string           `json:"name"`
	Units   string           `json:"units"`
}

type PartSaveRequest struct {
	PartRef *ObjectHandleWire `json:"part_ref,omitempty"`
	Path    string            `json:"path,omitempty"`
}

type PartSaveResponse struct {
	Saved    bool   `json:"saved"`
	Name     string `json:"name"`
	FullPath string `json:"full_path,omitempty"`
}

type PartCloseRequest struct {
	PartRef *ObjectHandleWire `json:"part_ref,omitempty"`
	Save    bool              `json:"save,omitempty"`
}

type PartCloseResponse struct {
	Closed bool   `json:"closed"`
	Name   string `json:"name"`
}

type PartSummaryRequest struct {
	PartRef *ObjectHandleWire `json:"part_ref,omitempty"`
}

type PartSummaryResponse struct {
	Name           string `json:"name"`
	Units          string `json:"units"`
	BodyCount      int    `json:"body_count"`
	FeatureCount   int    `json:"feature_count"`
	ComponentCount int    `json:"component_count"`
	NativeTag      uint32 `json:"native_tag,omitempty"`
}

// Batch attributes & bulk metadata (Phase D1)

type AttributeType string

const (
	AttrTypeString  AttributeType = "string"
	AttrTypeInteger AttributeType = "integer"
	AttrTypeReal    AttributeType = "real"
	AttrTypeBoolean AttributeType = "boolean"
)

type PartAttribute struct {
	Title string        `json:"title"`
	Type  AttributeType `json:"type"`
	Value any           `json:"value"`
}

type PartGetAttributesRequest struct {
	PartRef *ObjectHandleWire `json:"part_ref,omitempty"`
	Titles  []string          `json:"titles,omitempty"`
}

type PartGetAttributesResponse struct {
	Attributes []PartAttribute `json:"attributes"`
}

type PartSetAttributesRequest struct {
	PartRef    *ObjectHandleWire `json:"part_ref,omitempty"`
	Attributes []PartAttribute   `json:"attributes"`
}

type PartSetAttributesResponse struct {
	UpdatedCount int `json:"updated_count"`
}

type PartMetadataEntry struct {
	PartRef        ObjectHandleWire `json:"part_ref"`
	Name           string           `json:"name"`
	FullPath       string           `json:"full_path,omitempty"`
	Units          string           `json:"units"`
	IsModified     bool             `json:"is_modified"`
	BodyCount      int              `json:"body_count"`
	FeatureCount   int              `json:"feature_count"`
	ComponentCount int              `json:"component_count"`
	Attributes     []PartAttribute  `json:"attributes,omitempty"`
}

type PartBulkMetadataRequest struct {
	PartRefs          []ObjectHandleWire `json:"part_refs,omitempty"`
	IncludeAttributes bool               `json:"include_attributes,omitempty"`
}

type PartBulkMetadataResponse struct {
	Entries []PartMetadataEntry `json:"entries"`
}

type PartUnloadedDependency struct {
	PartName          string `json:"part_name"`
	StatusCode        int    `json:"status_code"`
	StatusDescription string `json:"status_description,omitempty"`
}

type PartLoadStatusRequest struct {
	PartRef *ObjectHandleWire `json:"part_ref,omitempty"`
}

type PartLoadStatusResponse struct {
	IsFullyLoaded        bool                     `json:"is_fully_loaded"`
	IsModified           bool                     `json:"is_modified"`
	IsReadOnly           bool                     `json:"is_read_only"`
	HasWriteAccess       bool                     `json:"has_write_access"`
	LoadState            string                   `json:"load_state"`
	UnloadedDependencies []PartUnloadedDependency `json:"unloaded_dependencies,omitempty"`
}

// Object registry operation payloads (Phase 5)

type ObjectReleaseRequest struct {
	Handles      []ObjectHandleWire `json:"handles,omitempty"`
	LeaseScopeID string             `json:"lease_scope_id,omitempty"`
}

func (r ObjectReleaseRequest) Validate() error {
	if r.LeaseScopeID != "" && len(r.Handles) > 0 {
		return errors.New("object release cannot combine lease_scope_id and handles")
	}
	if r.LeaseScopeID == "" && len(r.Handles) == 0 {
		return errors.New("object release requires lease_scope_id or handles")
	}
	return nil
}

type ObjectReleaseResponse struct {
	ReleasedCount int `json:"released_count"`
}

// Geometry & Feature creation payloads (Phase 7)

type FeatureCreateBlockRequest struct {
	PartRef       *ObjectHandleWire `json:"part_ref,omitempty"`
	Units         string            `json:"units"`
	Origin        [3]float64        `json:"origin"`
	Length        float64           `json:"length"`
	Width         float64           `json:"width"`
	Height        float64           `json:"height"`
	BooleanOp     string            `json:"boolean_op,omitempty"` // "create", "unite", "subtract", "intersect"
	TargetBodyRef *ObjectHandleWire `json:"target_body_ref,omitempty"`
}

type FeatureCreateBlockResponse struct {
	FeatureRef  ObjectHandleWire `json:"feature_ref"`
	BodyRef     ObjectHandleWire `json:"body_ref"`
	FeatureName string           `json:"feature_name"`
	FeatureType string           `json:"feature_type"`
}

type FeatureCreateCylinderRequest struct {
	PartRef       *ObjectHandleWire `json:"part_ref,omitempty"`
	Units         string            `json:"units"`
	Origin        [3]float64        `json:"origin"`
	Direction     [3]float64        `json:"direction,omitempty"` // default [0, 0, 1]
	Diameter      float64           `json:"diameter"`
	Height        float64           `json:"height"`
	BooleanOp     string            `json:"boolean_op,omitempty"`
	TargetBodyRef *ObjectHandleWire `json:"target_body_ref,omitempty"`
}

type FeatureCreateCylinderResponse struct {
	FeatureRef  ObjectHandleWire `json:"feature_ref"`
	BodyRef     ObjectHandleWire `json:"body_ref"`
	FeatureName string           `json:"feature_name"`
}

type FeatureBooleanRequest struct {
	PartRef       *ObjectHandleWire  `json:"part_ref,omitempty"`
	Op            string             `json:"op"` // "unite", "subtract", "intersect"
	TargetBodyRef *ObjectHandleWire  `json:"target_body_ref"`
	ToolBodyRefs  []ObjectHandleWire `json:"tool_body_refs"`
}

type FeatureBooleanResponse struct {
	FeatureRef  ObjectHandleWire `json:"feature_ref"`
	BodyRef     ObjectHandleWire `json:"body_ref"`
	FeatureName string           `json:"feature_name"`
	FeatureType string           `json:"feature_type"`
}

type FeatureCreateHoleRequest struct {
	PartRef             *ObjectHandleWire `json:"part_ref,omitempty"`
	TargetBodyRef       *ObjectHandleWire `json:"target_body_ref"`
	FaceRef             *ObjectHandleWire `json:"face_ref,omitempty"`
	Units               string            `json:"units,omitempty"`
	HoleType            string            `json:"hole_type,omitempty"` // "simple", "counterbore", "countersink"
	Origin              [3]float64        `json:"origin"`
	Direction           [3]float64        `json:"direction"`
	Diameter            float64           `json:"diameter"`
	Depth               float64           `json:"depth"`
	TipAngle            float64           `json:"tip_angle,omitempty"`
	ThroughBody         bool              `json:"through_body,omitempty"`
	CounterboreDiameter float64           `json:"counterbore_diameter,omitempty"`
	CounterboreDepth    float64           `json:"counterbore_depth,omitempty"`
	CountersinkDiameter float64           `json:"countersink_diameter,omitempty"`
	CountersinkAngle    float64           `json:"countersink_angle,omitempty"`
}

type FeatureCreateHoleResponse struct {
	FeatureRef  ObjectHandleWire `json:"feature_ref"`
	BodyRef     ObjectHandleWire `json:"body_ref"`
	FeatureName string           `json:"feature_name"`
	FeatureType string           `json:"feature_type"`
}

type DatumCreatePlaneRequest struct {
	PartRef   *ObjectHandleWire `json:"part_ref,omitempty"`
	Origin    [3]float64        `json:"origin"`
	Direction [3]float64        `json:"direction"`
}

type DatumCreatePlaneResponse struct {
	PlaneRef   ObjectHandleWire `json:"plane_ref"`
	FeatureRef ObjectHandleWire `json:"feature_ref"`
	Name       string           `json:"name"`
}

type DatumCreateAxisRequest struct {
	PartRef   *ObjectHandleWire `json:"part_ref,omitempty"`
	Origin    [3]float64        `json:"origin"`
	Direction [3]float64        `json:"direction"`
}

type DatumCreateAxisResponse struct {
	AxisRef    ObjectHandleWire `json:"axis_ref"`
	FeatureRef ObjectHandleWire `json:"feature_ref"`
	Name       string           `json:"name"`
}

type DatumCreateCsysRequest struct {
	PartRef    *ObjectHandleWire `json:"part_ref,omitempty"`
	Origin     [3]float64        `json:"origin"`
	XDirection [3]float64        `json:"x_direction"`
	YDirection [3]float64        `json:"y_direction"`
}

type DatumCreateCsysResponse struct {
	CsysRef    ObjectHandleWire `json:"csys_ref"`
	FeatureRef ObjectHandleWire `json:"feature_ref"`
	Name       string           `json:"name"`
}

type SketchLine2DWire struct {
	Start [2]float64 `json:"start"`
	End   [2]float64 `json:"end"`
}

type SketchCircle2DWire struct {
	Center [2]float64 `json:"center"`
	Radius float64    `json:"radius"`
}

type SketchArc2DWire struct {
	Center     [2]float64 `json:"center"`
	Radius     float64    `json:"radius"`
	StartAngle float64    `json:"start_angle"`
	EndAngle   float64    `json:"end_angle"`
}

type SketchRect2DWire struct {
	Origin [2]float64 `json:"origin"`
	Width  float64    `json:"width"`
	Height float64    `json:"height"`
}

type SketchCreateRequest struct {
	PartRef  *ObjectHandleWire `json:"part_ref,omitempty"`
	Name     string            `json:"name,omitempty"`
	PlaneRef *ObjectHandleWire `json:"plane_ref,omitempty"`
}

type SketchCreateResponse struct {
	SketchRef  ObjectHandleWire `json:"sketch_ref"`
	FeatureRef ObjectHandleWire `json:"feature_ref"`
	Name       string           `json:"name"`
}

type SketchAddGeometryRequest struct {
	PartRef    *ObjectHandleWire    `json:"part_ref,omitempty"`
	SketchRef  *ObjectHandleWire    `json:"sketch_ref"`
	Lines      []SketchLine2DWire   `json:"lines,omitempty"`
	Circles    []SketchCircle2DWire `json:"circles,omitempty"`
	Arcs       []SketchArc2DWire    `json:"arcs,omitempty"`
	Rectangles []SketchRect2DWire   `json:"rectangles,omitempty"`
}

type SketchAddGeometryResponse struct {
	AddedCount int `json:"added_count"`
	CurveCount int `json:"curve_count"`
}

type SketchQueryStatusRequest struct {
	SketchRef *ObjectHandleWire `json:"sketch_ref"`
}

type SketchQueryStatusResponse struct {
	Status     string `json:"status"`
	DOFNeeded  int    `json:"dof_needed"`
	CurveCount int    `json:"curve_count"`
}

type ProfileCreateRequest struct {
	PartRef           *ObjectHandleWire `json:"part_ref,omitempty"`
	SketchRef         *ObjectHandleWire `json:"sketch_ref"`
	ChainingTolerance float64           `json:"chaining_tolerance,omitempty"`
	DistanceTolerance float64           `json:"distance_tolerance,omitempty"`
}

type ProfileCreateResponse struct {
	ProfileRef ObjectHandleWire `json:"profile_ref"`
	Name       string           `json:"name"`
	LoopCount  int              `json:"loop_count"`
}

type FeatureCreateExtrudeRequest struct {
	PartRef       *ObjectHandleWire `json:"part_ref,omitempty"`
	ProfileRef    *ObjectHandleWire `json:"profile_ref"`
	Direction     [3]float64        `json:"direction,omitempty"`
	StartLimit    float64           `json:"start_limit,omitempty"`
	EndLimit      float64           `json:"end_limit"`
	BooleanOp     string            `json:"boolean_op,omitempty"`
	TargetBodyRef *ObjectHandleWire `json:"target_body_ref,omitempty"`
}

type FeatureCreateExtrudeResponse struct {
	FeatureRef  ObjectHandleWire `json:"feature_ref"`
	BodyRef     ObjectHandleWire `json:"body_ref,omitempty"`
	FeatureName string           `json:"feature_name"`
	FeatureType string           `json:"feature_type"`
}

type FeatureCreateRevolveRequest struct {
	PartRef       *ObjectHandleWire `json:"part_ref,omitempty"`
	ProfileRef    *ObjectHandleWire `json:"profile_ref"`
	AxisOrigin    [3]float64        `json:"axis_origin,omitempty"`
	AxisDirection [3]float64        `json:"axis_direction,omitempty"`
	AxisRef       *ObjectHandleWire `json:"axis_ref,omitempty"`
	StartAngle    float64           `json:"start_angle,omitempty"`
	EndAngle      float64           `json:"end_angle"`
	BooleanOp     string            `json:"boolean_op,omitempty"`
	TargetBodyRef *ObjectHandleWire `json:"target_body_ref,omitempty"`
}

type FeatureCreateRevolveResponse struct {
	FeatureRef  ObjectHandleWire `json:"feature_ref"`
	BodyRef     ObjectHandleWire `json:"body_ref,omitempty"`
	FeatureName string           `json:"feature_name"`
	FeatureType string           `json:"feature_type"`
}

type FeatureCreateFilletRequest struct {
	PartRef    *ObjectHandleWire  `json:"part_ref,omitempty"`
	BodyRef    *ObjectHandleWire  `json:"body_ref,omitempty"`
	FeatureRef *ObjectHandleWire  `json:"feature_ref,omitempty"`
	EdgeRefs   []ObjectHandleWire `json:"edge_refs,omitempty"`
	Radius     float64            `json:"radius"`
}

type FeatureCreateFilletResponse struct {
	FeatureRef  ObjectHandleWire `json:"feature_ref"`
	BodyRef     ObjectHandleWire `json:"body_ref,omitempty"`
	FeatureName string           `json:"feature_name"`
	FeatureType string           `json:"feature_type"`
}

type FeatureCreateChamferRequest struct {
	PartRef        *ObjectHandleWire  `json:"part_ref,omitempty"`
	BodyRef        *ObjectHandleWire  `json:"body_ref,omitempty"`
	FeatureRef     *ObjectHandleWire  `json:"feature_ref,omitempty"`
	EdgeRefs       []ObjectHandleWire `json:"edge_refs,omitempty"`
	Distance       float64            `json:"distance"`
	SecondDistance float64            `json:"second_distance,omitempty"`
	Angle          float64            `json:"angle,omitempty"`
	Option         string             `json:"option,omitempty"`
}

type FeatureCreateChamferResponse struct {
	FeatureRef  ObjectHandleWire `json:"feature_ref"`
	BodyRef     ObjectHandleWire `json:"body_ref,omitempty"`
	FeatureName string           `json:"feature_name"`
	FeatureType string           `json:"feature_type"`
}

type GeometryQueryMassPropertiesRequest struct {
	BodyRef *ObjectHandleWire `json:"body_ref,omitempty"`
	PartRef *ObjectHandleWire `json:"part_ref,omitempty"`
}

type GeometryQueryMassPropertiesResponse struct {
	Units     string     `json:"units"`
	Volume    float64    `json:"volume"`
	Area      float64    `json:"area"`
	Mass      float64    `json:"mass"`
	Centroid  [3]float64 `json:"centroid"`
	SolidType string     `json:"solid_type"`
}

type GeometryQueryBoundingBoxRequest struct {
	BodyRef *ObjectHandleWire `json:"body_ref,omitempty"`
	PartRef *ObjectHandleWire `json:"part_ref,omitempty"`
}

type GeometryQueryBoundingBoxResponse struct {
	Units      string     `json:"units"`
	MinCorner  [3]float64 `json:"min_corner"`
	MaxCorner  [3]float64 `json:"max_corner"`
	Dimensions [3]float64 `json:"dimensions"`
}

type PartQueryBodiesRequest struct {
	PartRef *ObjectHandleWire `json:"part_ref,omitempty"`
}

type BodyInfoWire struct {
	BodyRef   ObjectHandleWire `json:"body_ref"`
	Name      string           `json:"name"`
	SolidType string           `json:"solid_type"`
	FaceCount int              `json:"face_count"`
	EdgeCount int              `json:"edge_count"`
	NativeTag uint32           `json:"native_tag,omitempty"`
}

type PartQueryBodiesResponse struct {
	Bodies []BodyInfoWire `json:"bodies"`
}

// Assembly operation payloads (Phase 7)

type AssemblyAddComponentRequest struct {
	AssemblyPartRef *ObjectHandleWire `json:"assembly_part_ref,omitempty"`
	PartPath        string            `json:"part_path"`
	ComponentName   string            `json:"component_name,omitempty"`
	Origin          [3]float64        `json:"origin"`
	Orientation     [9]float64        `json:"orientation,omitempty"` // 3x3 matrix row-major
	Layer           int               `json:"layer,omitempty"`
}

type AssemblyAddComponentResponse struct {
	ComponentRef  ObjectHandleWire `json:"component_ref"`
	ComponentName string           `json:"component_name"`
	PartPath      string           `json:"part_path"`
	NativeTag     uint32           `json:"native_tag,omitempty"`
}

type AssemblyQueryTreeRequest struct {
	AssemblyPartRef *ObjectHandleWire `json:"assembly_part_ref,omitempty"`
}

type AssemblyComponentNodeWire struct {
	ComponentRef  ObjectHandleWire            `json:"component_ref"`
	Name          string                      `json:"name"`
	DisplayName   string                      `json:"display_name"`
	PrototypePath string                      `json:"prototype_path"`
	Position      [3]float64                  `json:"position"`
	Children      []AssemblyComponentNodeWire `json:"children,omitempty"`
}

type AssemblyQueryTreeResponse struct {
	Root AssemblyComponentNodeWire `json:"root"`
}

type AssemblyQueryBOMRequest struct {
	AssemblyPartRef *ObjectHandleWire `json:"assembly_part_ref,omitempty"`
}

type AssemblyBOMItemWire struct {
	PartName       string   `json:"part_name"`
	PartPath       string   `json:"part_path"`
	Quantity       int      `json:"quantity"`
	ComponentNames []string `json:"component_names"`
}

type AssemblyQueryBOMResponse struct {
	Items []AssemblyBOMItemWire `json:"items"`
}

type AssemblyRemoveComponentRequest struct {
	AssemblyPartRef *ObjectHandleWire `json:"assembly_part_ref,omitempty"`
	ComponentRef    ObjectHandleWire  `json:"component_ref"`
}

type AssemblyRemoveComponentResponse struct {
	Removed bool `json:"removed"`
}

// Drafting & PDF Export operation payloads (Phase 8)

type DraftingCreateSheetRequest struct {
	PartRef          *ObjectHandleWire `json:"part_ref,omitempty"`
	SheetName        string            `json:"sheet_name"`
	Units            string            `json:"units,omitempty"` // "mm", "inch"
	Height           float64           `json:"height"`
	Length           float64           `json:"length"`
	ScaleNumerator   float64           `json:"scale_numerator,omitempty"`
	ScaleDenominator float64           `json:"scale_denominator,omitempty"`
}

type DraftingCreateSheetResponse struct {
	SheetRef  ObjectHandleWire `json:"sheet_ref"`
	SheetName string           `json:"sheet_name"`
	Height    float64          `json:"height"`
	Length    float64          `json:"length"`
	NativeTag uint32           `json:"native_tag,omitempty"`
}

type DraftingExportPDFRequest struct {
	PartRef       *ObjectHandleWire `json:"part_ref,omitempty"`
	OutputPDFPath string            `json:"output_pdf_path"`
	SheetNames    []string          `json:"sheet_names,omitempty"`
	ColorMode     string            `json:"color_mode,omitempty"`
}

type DraftingExportPDFResponse struct {
	ExportedPath  string `json:"exported_path"`
	FileSizeBytes int64  `json:"file_size_bytes"`
}

type DraftingQuerySheetsRequest struct {
	PartRef *ObjectHandleWire `json:"part_ref,omitempty"`
}

type DraftingSheetInfoWire struct {
	SheetRef    ObjectHandleWire `json:"sheet_ref"`
	Name        string           `json:"name"`
	Height      float64          `json:"height"`
	Length      float64          `json:"length"`
	Numerator   float64          `json:"numerator"`
	Denominator float64          `json:"denominator"`
	NativeTag   uint32           `json:"native_tag,omitempty"`
}

type DraftingQuerySheetsResponse struct {
	Sheets []DraftingSheetInfoWire `json:"sheets"`
}


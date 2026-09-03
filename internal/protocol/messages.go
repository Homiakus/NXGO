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

// Object registry operation payloads (Phase 5)

type ObjectReleaseRequest struct {
	Handles      []ObjectHandleWire `json:"handles,omitempty"`
	LeaseScopeID string             `json:"lease_scope_id,omitempty"`
}

type ObjectReleaseResponse struct {
	ReleasedCount int `json:"released_count"`
}

// Geometry & Feature creation payloads (Phase 7)

type FeatureCreateBlockRequest struct {
	PartRef       *ObjectHandleWire `json:"part_ref,omitempty"`
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

type GeometryQueryMassPropertiesRequest struct {
	BodyRef *ObjectHandleWire `json:"body_ref,omitempty"`
	PartRef *ObjectHandleWire `json:"part_ref,omitempty"`
}

type GeometryQueryMassPropertiesResponse struct {
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





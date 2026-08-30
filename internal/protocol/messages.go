package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	CurrentProtocolMajor = 1
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

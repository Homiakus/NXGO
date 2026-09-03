package nxgo

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/transport/pipe"
)

var (
	ErrSessionClosed  = errors.New("nxgo session is closed")
	ErrNullObjectRef  = errors.New("object reference is nil")
	ErrStaleObjectRef = errors.New("object reference is stale or belongs to a different session/epoch")
)

type SessionInfo struct {
	Release    string `json:"release"`
	BaseDir    string `json:"base_dir"`
	ThreadID   int    `json:"thread_id"`
	WorkPart   string `json:"work_part,omitempty"`
	Epoch      uint64 `json:"epoch"`
	SessionID  string `json:"session_id"`
	SyslogPath string `json:"syslog_path,omitempty"`
}

type Session struct {
	client       *pipe.Client
	sessionID    string
	epoch        uint64
	capabilities []string
	nxRelease    string
}

type ConnectOption func(*connectOptions)

type connectOptions struct {
	nonce string
}

func WithNonce(nonce string) ConnectOption {
	return func(o *connectOptions) {
		o.nonce = nonce
	}
}

func Connect(ctx context.Context, pipePath string, opts ...ConnectOption) (*Session, error) {
	conn, err := pipe.DialPipe(ctx, pipePath)
	if err != nil {
		return nil, fmt.Errorf("dial NX pipe: %w", err)
	}

	var options connectOptions
	for _, opt := range opts {
		opt(&options)
	}
	nonce := options.nonce
	if nonce == "" {
		nonce = newHandshakeNonce()
	}

	client := pipe.NewClient(conn)
	hsResp, err := client.Handshake(ctx, &protocol.HandshakeRequest{
		ProtocolVersion: protocol.Version{Major: protocol.CurrentProtocolMajor, Minor: protocol.CurrentProtocolMinor},
		SDKVersion:      "v0.1.0",
		ClientPID:       os.Getpid(),
		Nonce:           nonce,
	})
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("handshake: %w", err)
	}

	return &Session{
		client:       client,
		sessionID:    hsResp.SessionID,
		epoch:        hsResp.Epoch,
		capabilities: hsResp.Capabilities,
		nxRelease:    hsResp.NXRelease,
	}, nil
}

func WrapClient(client *pipe.Client, sessionID string, epoch uint64, nxRelease string) *Session {
	return &Session{
		client:    client,
		sessionID: sessionID,
		epoch:     epoch,
		nxRelease: nxRelease,
	}
}

func (s *Session) SessionID() string {
	if s == nil {
		return ""
	}
	return s.sessionID
}

func (s *Session) Epoch() uint64 {
	if s == nil {
		return 0
	}
	return s.epoch
}

func (s *Session) NXRelease() string {
	if s == nil {
		return ""
	}
	return s.nxRelease
}

func (s *Session) Ping(ctx context.Context) error {
	if err := s.validateOpen(); err != nil {
		return err
	}
	resp, err := s.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("nx.ping"),
		Op:        "nx.ping",
	})
	if err != nil {
		return err
	}
	if resp.Status != protocol.StatusOK {
		return formatError(resp.Error)
	}
	return nil
}

func (s *Session) Info(ctx context.Context) (*SessionInfo, error) {
	if err := s.validateOpen(); err != nil {
		return nil, err
	}
	resp, err := s.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("session.info"),
		Op:        "session.info",
	})
	if err != nil {
		return nil, err
	}
	if resp.Status != protocol.StatusOK {
		return nil, formatError(resp.Error)
	}

	return protocol.DecodePayload[SessionInfo](resp.Payload)
}

func (s *Session) ReleaseObjects(ctx context.Context, refs ...protocol.ObjectHandleWire) error {
	if err := s.validateOpen(); err != nil {
		return err
	}
	if len(refs) == 0 {
		return nil
	}
	for i := range refs {
		if err := s.validateObjectHandle(&refs[i]); err != nil {
			return fmt.Errorf("release object %d: %w", i, err)
		}
	}
	releaseRequest := protocol.ObjectReleaseRequest{Handles: refs}
	if err := releaseRequest.Validate(); err != nil {
		return err
	}
	reqData, err := protocol.EncodePayload(releaseRequest)
	if err != nil {
		return err
	}
	resp, err := s.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: newRequestID("object.release"),
		Op:        "object.release",
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

// ReleaseScope releases all ephemeral handles issued for a request scope.
func (s *Session) ReleaseScope(ctx context.Context, scopeID string) error {
	if err := s.validateOpen(); err != nil {
		return err
	}
	if scopeID == "" {
		return errors.New("lease scope id is required")
	}
	releaseRequest := protocol.ObjectReleaseRequest{LeaseScopeID: scopeID}
	if err := releaseRequest.Validate(); err != nil {
		return err
	}
	reqData, err := protocol.EncodePayload(releaseRequest)
	if err != nil {
		return err
	}
	resp, err := s.client.Call(ctx, &protocol.RequestEnvelope{RequestID: newRequestID("object.release"), Op: "object.release", Payload: reqData})
	if err != nil {
		return err
	}
	if resp.Status != protocol.StatusOK {
		return formatError(resp.Error)
	}
	return nil
}

func (s *Session) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

func formatError(errEnv *protocol.ErrorEnvelope) error {
	if errEnv == nil {
		return errors.New("unknown error from NX agent")
	}
	return fmt.Errorf("[%s] %s (nx_code=%d, health=%s, recoverable=%t)",
		errEnv.Category, errEnv.Message, errEnv.NXErrorCode, errEnv.SessionHealth, errEnv.Recoverable)
}

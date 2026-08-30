package nxgo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/transport/pipe"
)

var (
	ErrSessionClosed   = errors.New("nxgo session is closed")
	ErrNullObjectRef   = errors.New("object reference is nil")
	ErrStaleObjectRef  = errors.New("object reference is stale or belongs to a different session/epoch")
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

func Connect(ctx context.Context, pipePath string) (*Session, error) {
	conn, err := pipe.DialPipe(ctx, pipePath)
	if err != nil {
		return nil, fmt.Errorf("dial NX pipe: %w", err)
	}

	client := pipe.NewClient(conn)
	hsResp, err := client.Handshake(ctx, &protocol.HandshakeRequest{
		ProtocolVersion: protocol.Version{Major: protocol.CurrentProtocolMajor, Minor: protocol.CurrentProtocolMinor},
		SDKVersion:      "v0.1.0",
		ClientPID:       os.Getpid(),
		Nonce:           fmt.Sprintf("nonce-%d", time.Now().UnixNano()),
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
	return s.sessionID
}

func (s *Session) Epoch() uint64 {
	return s.epoch
}

func (s *Session) NXRelease() string {
	return s.nxRelease
}

func (s *Session) Ping(ctx context.Context) error {
	resp, err := s.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-ping-%d", time.Now().UnixNano()),
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
	resp, err := s.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-info-%d", time.Now().UnixNano()),
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
	if len(refs) == 0 {
		return nil
	}
	reqData, err := protocol.EncodePayload(protocol.ObjectReleaseRequest{Handles: refs})
	if err != nil {
		return err
	}
	resp, err := s.client.Call(ctx, &protocol.RequestEnvelope{
		RequestID: fmt.Sprintf("req-rel-%d", time.Now().UnixNano()),
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

func (s *Session) Close() error {
	if s.client == nil {
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

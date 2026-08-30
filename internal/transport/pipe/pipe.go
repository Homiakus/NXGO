package pipe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/Homiakus/NXGO/internal/protocol"
)

var (
	ErrNotConnected     = errors.New("transport is not connected")
	ErrClosed           = errors.New("transport closed")
	ErrHandshakeFailed  = errors.New("handshake failed")
	ErrSessionUnhealthy = errors.New("session is not healthy")
)

type FramedConn struct {
	rwc        io.ReadWriteCloser
	mu         sync.Mutex
	maxPayload int
}

func NewFramedConn(rwc io.ReadWriteCloser, maxPayload int) *FramedConn {
	if maxPayload <= 0 {
		maxPayload = protocol.DefaultMaxPayloadBytes
	}
	return &FramedConn{
		rwc:        rwc,
		maxPayload: maxPayload,
	}
}

func (fc *FramedConn) Send(payload []byte) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.rwc == nil {
		return ErrNotConnected
	}
	frame, err := protocol.EncodeFrame(payload)
	if err != nil {
		return err
	}
	_, err = fc.rwc.Write(frame)
	return err
}

func (fc *FramedConn) Receive() ([]byte, error) {
	header := make([]byte, protocol.FrameHeaderSize)
	if _, err := io.ReadFull(fc.rwc, header); err != nil {
		return nil, err
	}

	length := uint32(header[0]) | uint32(header[1])<<8 | uint32(header[2])<<16 | uint32(header[3])<<24
	if length > uint32(fc.maxPayload) {
		return nil, protocol.ErrInvalidLength
	}

	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(fc.rwc, payload); err != nil {
			return nil, err
		}
	}
	return payload, nil
}

func (fc *FramedConn) Close() error {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.rwc == nil {
		return nil
	}
	err := fc.rwc.Close()
	fc.rwc = nil
	return err
}

func DialPipe(ctx context.Context, pipePath string) (io.ReadWriteCloser, error) {
	if runtime.GOOS != "windows" && !strings.HasPrefix(pipePath, `\\.\pipe\`) {
		return nil, fmt.Errorf("named pipes require windows: %s", pipePath)
	}

	type dialResult struct {
		file *os.File
		err  error
	}
	ch := make(chan dialResult, 1)
	go func() {
		f, err := os.OpenFile(pipePath, os.O_RDWR, 0)
		ch <- dialResult{file: f, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.err != nil {
			return nil, fmt.Errorf("dial pipe %q: %w", pipePath, res.err)
		}
		return res.file, nil
	}
}

type Client struct {
	conn       *FramedConn
	mu         sync.Mutex
	sessionID  string
	epoch      uint64
	serverInfo *protocol.HandshakeResponse
}

func NewClient(rwc io.ReadWriteCloser) *Client {
	return &Client{
		conn: NewFramedConn(rwc, protocol.DefaultMaxPayloadBytes),
	}
}

func (c *Client) Handshake(ctx context.Context, req *protocol.HandshakeRequest) (*protocol.HandshakeResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid handshake request: %w", err)
	}

	payload, err := protocol.EncodePayload(req)
	if err != nil {
		return nil, err
	}

	if err := c.conn.Send(payload); err != nil {
		return nil, fmt.Errorf("send handshake: %w", err)
	}

	respBytes, err := c.conn.Receive()
	if err != nil {
		return nil, fmt.Errorf("receive handshake: %w", err)
	}

	resp, err := protocol.DecodePayload[protocol.HandshakeResponse](respBytes)
	if err != nil {
		return nil, fmt.Errorf("decode handshake response: %w", err)
	}

	if err := protocol.NegotiateVersion(req.ProtocolVersion, resp.ProtocolVersion); err != nil {
		_ = c.conn.Close()
		return nil, fmt.Errorf("%w: %v", ErrHandshakeFailed, err)
	}

	c.sessionID = resp.SessionID
	c.epoch = resp.Epoch
	c.serverInfo = resp
	return resp, nil
}

func (c *Client) Call(ctx context.Context, req *protocol.RequestEnvelope) (*protocol.ResponseEnvelope, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	payload, err := protocol.EncodePayload(req)
	if err != nil {
		return nil, err
	}

	if err := c.conn.Send(payload); err != nil {
		return nil, fmt.Errorf("send request %s: %w", req.RequestID, err)
	}

	type callResult struct {
		respBytes []byte
		err       error
	}
	resChan := make(chan callResult, 1)

	go func() {
		b, err := c.conn.Receive()
		resChan <- callResult{respBytes: b, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-resChan:
		if res.err != nil {
			return nil, fmt.Errorf("receive response %s: %w", req.RequestID, res.err)
		}
		resp, err := protocol.DecodePayload[protocol.ResponseEnvelope](res.respBytes)
		if err != nil {
			return nil, fmt.Errorf("decode response %s: %w", req.RequestID, err)
		}
		return resp, nil
	}
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) SessionInfo() (sessionID string, epoch uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionID, c.epoch
}

package pipe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/Homiakus/NXGO/internal/protocol"
)

const DefaultMaxPendingRequests = 128

var (
	ErrNotConnected      = errors.New("transport is not connected")
	ErrClosed            = errors.New("transport closed")
	ErrHandshakeFailed   = errors.New("handshake failed")
	ErrSessionUnhealthy  = errors.New("session is not healthy")
	ErrOutcomeUnknown    = errors.New("request outcome is unknown; connection quarantined")
	ErrProtocolViolation = errors.New("transport protocol violation")
	ErrDuplicateRequest  = errors.New("duplicate in-flight request id")
	ErrTooManyPending    = errors.New("too many in-flight requests")
)

type FramedConn struct {
	rwc        io.ReadWriteCloser
	writeMu    sync.Mutex
	stateMu    sync.Mutex
	closed     bool
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
	fc.writeMu.Lock()
	defer fc.writeMu.Unlock()

	fc.stateMu.Lock()
	closed := fc.closed
	maxPayload := fc.maxPayload
	fc.stateMu.Unlock()
	if closed || fc.rwc == nil {
		return ErrNotConnected
	}
	if len(payload) > maxPayload {
		return fmt.Errorf("%w: %d > %d", protocol.ErrInvalidLength, len(payload), maxPayload)
	}

	frame, err := protocol.EncodeFrame(payload)
	if err != nil {
		return err
	}
	for written := 0; written < len(frame); {
		n, err := fc.rwc.Write(frame[written:])
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		written += n
	}
	return nil
}

func (fc *FramedConn) Receive() ([]byte, error) {
	if fc.rwc == nil {
		return nil, ErrNotConnected
	}

	header := make([]byte, protocol.FrameHeaderSize)
	if _, err := io.ReadFull(fc.rwc, header); err != nil {
		return nil, err
	}

	length := uint32(header[0]) | uint32(header[1])<<8 | uint32(header[2])<<16 | uint32(header[3])<<24
	fc.stateMu.Lock()
	maxPayload := fc.maxPayload
	fc.stateMu.Unlock()
	if length > uint32(maxPayload) {
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
	fc.writeMu.Lock()
	defer fc.writeMu.Unlock()

	fc.stateMu.Lock()
	if fc.closed {
		fc.stateMu.Unlock()
		return nil
	}
	fc.closed = true
	fc.stateMu.Unlock()

	if fc.rwc == nil {
		return nil
	}
	return fc.rwc.Close()
}

func DialPipe(ctx context.Context, pipePath string) (io.ReadWriteCloser, error) {
	return openPipe(ctx, pipePath)
}

type callResult struct {
	resp *protocol.ResponseEnvelope
	err  error
}

type Client struct {
	conn *FramedConn

	handshakeMu sync.Mutex
	writeMu     sync.Mutex
	stateMu     sync.Mutex

	sessionID      string
	epoch          uint64
	serverInfo     *protocol.HandshakeResponse
	pending        map[string]chan callResult
	maxPending     int
	readerStarted  bool
	closed         bool
	quarantined    bool
	terminalErr    error
	quarantineHook func(error)
}

func NewClient(rwc io.ReadWriteCloser) *Client {
	return &Client{
		conn:       NewFramedConn(rwc, protocol.DefaultMaxPayloadBytes),
		pending:    make(map[string]chan callResult),
		maxPending: DefaultMaxPendingRequests,
	}
}

// SetQuarantineHook installs a lifecycle callback that is invoked exactly once
// when this connection becomes unsafe to reuse. The hook is intentionally
// asynchronous: supervisors commonly terminate an owning worker process from
// the callback, and doing that inline could deadlock a caller already holding
// worker lifecycle locks.
//
// If the client is already quarantined when the hook is installed, the hook is
// invoked immediately (asynchronously) with the terminal cause.
func (c *Client) SetQuarantineHook(hook func(error)) {
	c.stateMu.Lock()
	c.quarantineHook = hook
	quarantined := c.quarantined
	cause := c.terminalErrorLocked()
	c.stateMu.Unlock()

	if hook != nil && quarantined {
		go hook(cause)
	}
}

func (c *Client) Handshake(ctx context.Context, req *protocol.HandshakeRequest) (*protocol.HandshakeResponse, error) {
	c.handshakeMu.Lock()
	defer c.handshakeMu.Unlock()

	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid handshake request: %w", err)
	}

	c.stateMu.Lock()
	if c.closed {
		c.stateMu.Unlock()
		return nil, ErrClosed
	}
	if c.quarantined {
		err := c.terminalErrorLocked()
		c.stateMu.Unlock()
		return nil, err
	}
	if c.readerStarted {
		c.stateMu.Unlock()
		return nil, fmt.Errorf("%w: handshake must complete before request reader starts", ErrHandshakeFailed)
	}
	c.stateMu.Unlock()

	payload, err := protocol.EncodePayload(req)
	if err != nil {
		return nil, err
	}

	c.writeMu.Lock()
	err = c.conn.Send(payload)
	c.writeMu.Unlock()
	if err != nil {
		c.quarantine(fmt.Errorf("%w: handshake send failed: %v", ErrHandshakeFailed, err))
		return nil, fmt.Errorf("send handshake: %w", err)
	}

	type handshakeResult struct {
		payload []byte
		err     error
	}
	resultCh := make(chan handshakeResult, 1)
	go func() {
		b, recvErr := c.conn.Receive()
		resultCh <- handshakeResult{payload: b, err: recvErr}
	}()

	var respBytes []byte
	select {
	case <-ctx.Done():
		c.quarantine(fmt.Errorf("%w: handshake interrupted: %v", ErrHandshakeFailed, ctx.Err()))
		return nil, ctx.Err()
	case res := <-resultCh:
		if res.err != nil {
			c.quarantine(fmt.Errorf("%w: handshake receive failed: %v", ErrHandshakeFailed, res.err))
			return nil, fmt.Errorf("receive handshake: %w", res.err)
		}
		respBytes = res.payload
	}

	resp, err := protocol.DecodePayload[protocol.HandshakeResponse](respBytes)
	if err != nil {
		c.quarantine(fmt.Errorf("%w: invalid handshake response: %v", ErrProtocolViolation, err))
		return nil, fmt.Errorf("decode handshake response: %w", err)
	}

	if err := protocol.NegotiateVersion(req.ProtocolVersion, resp.ProtocolVersion); err != nil {
		c.quarantine(fmt.Errorf("%w: %v", ErrHandshakeFailed, err))
		return nil, fmt.Errorf("%w: %v", ErrHandshakeFailed, err)
	}

	c.stateMu.Lock()
	c.sessionID = resp.SessionID
	c.epoch = resp.Epoch
	c.serverInfo = resp
	c.readerStarted = true
	c.stateMu.Unlock()
	go c.readLoop()

	return resp, nil
}

func (c *Client) Call(ctx context.Context, req *protocol.RequestEnvelope) (*protocol.ResponseEnvelope, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	payload, err := protocol.EncodePayload(req)
	if err != nil {
		return nil, err
	}

	resultCh := make(chan callResult, 1)
	startReader, err := c.registerPending(req.RequestID, resultCh)
	if err != nil {
		return nil, err
	}
	if startReader {
		go c.readLoop()
	}

	c.writeMu.Lock()
	err = c.conn.Send(payload)
	c.writeMu.Unlock()
	if err != nil {
		c.removePending(req.RequestID)
		terminal := fmt.Errorf("%w: send request %s failed: %v", ErrOutcomeUnknown, req.RequestID, err)
		c.quarantine(terminal)
		return nil, terminal
	}

	select {
	case <-ctx.Done():
		terminal := fmt.Errorf("%w: request %s interrupted after send: %w", ErrOutcomeUnknown, req.RequestID, ctx.Err())
		c.quarantine(terminal)
		return nil, terminal
	case result := <-resultCh:
		if result.err != nil {
			return nil, result.err
		}
		if result.resp == nil {
			return nil, fmt.Errorf("%w: nil response for request %s", ErrProtocolViolation, req.RequestID)
		}
		if result.resp.RequestID != req.RequestID {
			terminal := fmt.Errorf("%w: response request_id %q does not match %q", ErrProtocolViolation, result.resp.RequestID, req.RequestID)
			c.quarantine(terminal)
			return nil, terminal
		}
		return result.resp, nil
	}
}

func (c *Client) registerPending(requestID string, ch chan callResult) (bool, error) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	if c.closed {
		return false, ErrClosed
	}
	if c.quarantined {
		return false, c.terminalErrorLocked()
	}
	if _, exists := c.pending[requestID]; exists {
		return false, fmt.Errorf("%w: %s", ErrDuplicateRequest, requestID)
	}
	if c.maxPending <= 0 {
		c.maxPending = DefaultMaxPendingRequests
	}
	if len(c.pending) >= c.maxPending {
		return false, fmt.Errorf("%w: %d >= %d", ErrTooManyPending, len(c.pending), c.maxPending)
	}

	startReader := !c.readerStarted
	if startReader {
		c.readerStarted = true
	}
	c.pending[requestID] = ch
	return startReader, nil
}

func (c *Client) removePending(requestID string) {
	c.stateMu.Lock()
	delete(c.pending, requestID)
	c.stateMu.Unlock()
}

func (c *Client) readLoop() {
	for {
		respBytes, err := c.conn.Receive()
		if err != nil {
			c.stateMu.Lock()
			closed := c.closed
			quarantined := c.quarantined
			c.stateMu.Unlock()
			if closed || quarantined {
				return
			}
			c.quarantine(classifyReceiveError(err))
			return
		}

		resp, err := protocol.DecodePayload[protocol.ResponseEnvelope](respBytes)
		if err != nil {
			c.quarantine(fmt.Errorf("%w: decode response: %v", ErrProtocolViolation, err))
			return
		}
		if resp.RequestID == "" {
			c.quarantine(fmt.Errorf("%w: response has empty request_id", ErrProtocolViolation))
			return
		}

		c.stateMu.Lock()
		ch, ok := c.pending[resp.RequestID]
		if ok {
			delete(c.pending, resp.RequestID)
		}
		c.stateMu.Unlock()
		if !ok {
			c.quarantine(fmt.Errorf("%w: unexpected response request_id %q", ErrProtocolViolation, resp.RequestID))
			return
		}

		ch <- callResult{resp: resp}
	}
}

func classifyReceiveError(err error) error {
	if errors.Is(err, protocol.ErrInvalidLength) ||
		errors.Is(err, protocol.ErrLengthMismatch) ||
		errors.Is(err, protocol.ErrTruncatedFrame) {
		return fmt.Errorf("%w: malformed response frame: %v", ErrProtocolViolation, err)
	}
	return fmt.Errorf("%w: receive failed: %v", ErrOutcomeUnknown, err)
}

func (c *Client) quarantine(cause error) {
	if cause == nil {
		cause = ErrOutcomeUnknown
	}

	c.stateMu.Lock()
	if c.closed || c.quarantined {
		c.stateMu.Unlock()
		return
	}
	c.quarantined = true
	c.terminalErr = cause
	pending := c.pending
	c.pending = make(map[string]chan callResult)
	hook := c.quarantineHook
	c.stateMu.Unlock()

	_ = c.conn.Close()
	for _, ch := range pending {
		select {
		case ch <- callResult{err: cause}:
		default:
		}
	}
	if hook != nil {
		go hook(cause)
	}
}

func (c *Client) terminalErrorLocked() error {
	if c.terminalErr != nil {
		return c.terminalErr
	}
	if c.quarantined {
		return ErrOutcomeUnknown
	}
	return ErrClosed
}

func (c *Client) Close() error {
	c.stateMu.Lock()
	if c.closed {
		c.stateMu.Unlock()
		return nil
	}
	c.closed = true
	pending := c.pending
	c.pending = make(map[string]chan callResult)
	c.stateMu.Unlock()

	err := c.conn.Close()
	for _, ch := range pending {
		select {
		case ch <- callResult{err: ErrClosed}:
		default:
		}
	}
	return err
}

func (c *Client) SessionInfo() (sessionID string, epoch uint64) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.sessionID, c.epoch
}

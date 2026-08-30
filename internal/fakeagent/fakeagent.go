package fakeagent

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/sessionhealth"
	"github.com/Homiakus/NXGO/internal/transport/pipe"
)

var ErrTransportLost = errors.New("simulated transport loss")

type Fault uint8
const (
    NoFault Fault = iota
    DropAfterCommit
    PoisonSession
)

type Request struct {
    ID string
    Mutation bool
    Fault Fault
}

type Response struct { Applied int }

type Agent struct {
    mu sync.Mutex
    records map[string]Response
    dropped map[string]bool
    applied int
    health sessionhealth.State
}

func New() *Agent { return &Agent{records: map[string]Response{}, dropped: map[string]bool{}, health: sessionhealth.Healthy} }

func (a *Agent) Execute(ctx context.Context, r Request) (Response, error) {
    if err := ctx.Err(); err != nil { return Response{}, err }
    a.mu.Lock()
    defer a.mu.Unlock()

    if old, ok := a.records[r.ID]; ok { return old, nil }
    if a.health == sessionhealth.Poisoned || a.health == sessionhealth.Lost { return Response{}, errors.New("session unavailable") }

    if r.Mutation { a.applied++ }
    resp := Response{Applied: a.applied}
    a.records[r.ID] = resp

    switch r.Fault {
    case DropAfterCommit:
        if !a.dropped[r.ID] {
            a.dropped[r.ID] = true
            return Response{}, ErrTransportLost
        }
    case PoisonSession:
        next, _ := sessionhealth.Transition(a.health, sessionhealth.PoisonFailure)
        a.health = next
        return Response{}, errors.New("simulated poisoned NX session")
    }
    return resp, nil
}

func (a *Agent) Applied() int { a.mu.Lock(); defer a.mu.Unlock(); return a.applied }
func (a *Agent) RecordCount() int { a.mu.Lock(); defer a.mu.Unlock(); return len(a.records) }
func (a *Agent) Health() sessionhealth.State { a.mu.Lock(); defer a.mu.Unlock(); return a.health }

func (a *Agent) ServeTransport(ctx context.Context, rwc io.ReadWriteCloser) error {
	defer rwc.Close()
	framed := pipe.NewFramedConn(rwc, protocol.DefaultMaxPayloadBytes)

	// 1. Handshake
	hsBytes, err := framed.Receive()
	if err != nil {
		return err
	}
	hsReq, err := protocol.DecodePayload[protocol.HandshakeRequest](hsBytes)
	if err != nil {
		return err
	}
	if err := hsReq.Validate(); err != nil {
		return err
	}

	hsResp := protocol.HandshakeResponse{
		ProtocolVersion: protocol.Version{Major: protocol.CurrentProtocolMajor, Minor: protocol.CurrentProtocolMinor},
		AgentVersion:    "v0.1.0-fake",
		NXRelease:       "FakeNX-2512",
		NXBuild:         "2512.1000",
		NXProcessID:     9999,
		SessionID:       "fake-session-1",
		Epoch:           1,
		Capabilities:    []string{"nx.ping", "part.open", "part.save", "part.close"},
		MaxPayloadBytes: protocol.DefaultMaxPayloadBytes,
		SecurityPolicy:  "local_pipe_only",
	}
	hsRespBytes, err := protocol.EncodePayload(hsResp)
	if err != nil {
		return err
	}
	if err := framed.Send(hsRespBytes); err != nil {
		return err
	}

	// 2. Request / Response loop
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		reqBytes, err := framed.Receive()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}

		req, err := protocol.DecodePayload[protocol.RequestEnvelope](reqBytes)
		if err != nil {
			return err
		}

		var resp protocol.ResponseEnvelope
		resp.RequestID = req.RequestID

		// Execute through idempotency / health state
		res, execErr := a.Execute(ctx, Request{
			ID:       req.RequestID,
			Mutation: req.Op == "part.save" || req.Op == "part.modify",
			Fault:    NoFault,
		})

		if execErr != nil {
			resp.Status = protocol.StatusError
			resp.Error = &protocol.ErrorEnvelope{
				Category:      protocol.ErrCategorySessionDirty,
				Message:       execErr.Error(),
				Op:            req.Op,
				Recoverable:   false,
				SessionHealth: a.Health().String(),
				CorrelationID: req.CorrelationID,
			}
		} else {
			resp.Status = protocol.StatusOK
			respPayload, _ := protocol.EncodePayload(res)
			resp.Payload = respPayload
			resp.Timing = protocol.TimingData{ExecutionMs: 1}
		}

		respBytes, err := protocol.EncodePayload(resp)
		if err != nil {
			return err
		}
		if err := framed.Send(respBytes); err != nil {
			return err
		}
	}
}


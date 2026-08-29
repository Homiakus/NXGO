package fakeagent

import (
    "context"
    "errors"
    "sync"

    "github.com/Homiakus/NXGO/internal/sessionhealth"
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

package nxgo

import (
	"fmt"
	"strings"

	"github.com/Homiakus/NXGO/internal/protocol"
)

// validateObjectHandle rejects references that cannot belong to this live SDK
// session before any IPC is attempted. Server-side validation remains
// mandatory, but client-side fail-closed checks prevent accidental cross-
// session and stale-epoch mutations from reaching NX at all.
func (s *Session) validateObjectHandle(ref *protocol.ObjectHandleWire, expectedKinds ...string) error {
	if s == nil || s.client == nil {
		return ErrSessionClosed
	}
	if ref == nil || strings.TrimSpace(ref.ObjectID) == "" {
		return ErrNullObjectRef
	}
	if ref.SessionID == "" || ref.SessionID != s.sessionID || ref.Epoch != s.epoch {
		return fmt.Errorf(
			"%w: handle(session=%q epoch=%d object=%q) current(session=%q epoch=%d)",
			ErrStaleObjectRef,
			ref.SessionID,
			ref.Epoch,
			ref.ObjectID,
			s.sessionID,
			s.epoch,
		)
	}

	if len(expectedKinds) == 0 {
		return nil
	}
	for _, expected := range expectedKinds {
		if strings.EqualFold(strings.TrimSpace(ref.Kind), strings.TrimSpace(expected)) {
			return nil
		}
	}
	return fmt.Errorf(
		"%w: object %q has kind %q, expected one of %v",
		ErrStaleObjectRef,
		ref.ObjectID,
		ref.Kind,
		expectedKinds,
	)
}

func (p *Part) validate() error {
	if p == nil || p.session == nil {
		return ErrSessionClosed
	}
	return p.session.validateObjectHandle(&p.Ref, "Part")
}

func (b *Body) validate() error {
	if b == nil || b.session == nil {
		return ErrSessionClosed
	}
	return b.session.validateObjectHandle(&b.Ref, "Body")
}

func (c *Component) validate() error {
	if c == nil || c.session == nil || c.part == nil {
		return ErrSessionClosed
	}
	if err := c.part.validate(); err != nil {
		return err
	}
	return c.session.validateObjectHandle(&c.Ref, "Component")
}

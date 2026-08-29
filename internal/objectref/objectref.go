package objectref

import "errors"

var (
    ErrStaleSession = errors.New("stale NX session")
    ErrStaleEpoch = errors.New("stale NX session epoch")
    ErrInvalidObject = errors.New("invalid NX object reference")
)

type Ref struct {
    SessionID string
    Epoch uint64
    ObjectID uint64
    Generation uint32
    Type string
}

func (r Ref) Validate(sessionID string, epoch uint64) error {
    if r.SessionID == "" || r.ObjectID == 0 || r.Generation == 0 { return ErrInvalidObject }
    if r.SessionID != sessionID { return ErrStaleSession }
    if r.Epoch != epoch { return ErrStaleEpoch }
    return nil
}

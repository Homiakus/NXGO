package objectref

import (
    "errors"
    "testing"
)

func FuzzReferenceNeverValidAcrossDifferentEpoch(f *testing.F) {
    f.Add("session-a", uint64(1), uint64(2), uint64(42), uint32(1))
    f.Add("s", uint64(10), uint64(11), uint64(1), uint32(7))

    f.Fuzz(func(t *testing.T, session string, createdEpoch, currentEpoch, objectID uint64, generation uint32) {
        if session == "" || objectID == 0 || generation == 0 || createdEpoch == currentEpoch {
            t.Skip()
        }
        r := Ref{
            SessionID: session,
            Epoch: createdEpoch,
            ObjectID: objectID,
            Generation: generation,
            Type: "FuzzObject",
        }
        if err := r.Validate(session, currentEpoch); !errors.Is(err, ErrStaleEpoch) {
            t.Fatalf("reference from epoch %d accepted in epoch %d: %v", createdEpoch, currentEpoch, err)
        }
    })
}

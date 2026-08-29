package objectref

import (
    "errors"
    "testing"
)

func TestReferenceInvalidAfterEpochChange(t *testing.T) {
    r := Ref{SessionID: "s1", Epoch: 7, ObjectID: 42, Generation: 1, Type: "Face"}
    if err := r.Validate("s1", 7); err != nil { t.Fatal(err) }
    if err := r.Validate("s1", 8); !errors.Is(err, ErrStaleEpoch) { t.Fatalf("expected stale epoch, got %v", err) }
}

func TestReferenceCannotCrossSessions(t *testing.T) {
    r := Ref{SessionID: "s1", Epoch: 1, ObjectID: 1, Generation: 1}
    if err := r.Validate("s2", 1); !errors.Is(err, ErrStaleSession) { t.Fatalf("expected stale session, got %v", err) }
}

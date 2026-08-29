package sessionhealth

import "testing"

func TestPoisonedSessionCannotReturnHealthy(t *testing.T) {
    s, err := Transition(Healthy, PoisonFailure)
    if err != nil { t.Fatal(err) }
    if s != Poisoned { t.Fatalf("got %v", s) }
    if _, err := Transition(s, VerifiedClean); err == nil { t.Fatal("poisoned session was allowed to recover in-place") }
}

func TestSuspectRequiresExplicitVerification(t *testing.T) {
    s, err := Transition(Healthy, SuspectFailure)
    if err != nil { t.Fatal(err) }
    if s != Suspect { t.Fatalf("got %v", s) }
    s, err = Transition(s, VerifiedClean)
    if err != nil { t.Fatal(err) }
    if s != Healthy { t.Fatalf("got %v", s) }
}

func TestTerminalStatesAreAbsorbing(t *testing.T) {
    for _, s := range []State{Poisoned, Lost} {
        for _, e := range []Event{RecoverableFailure, SuspectFailure, PoisonFailure, ProcessLost, VerifiedClean} {
            got, err := Transition(s, e)
            if err == nil { t.Fatalf("state %v event %v unexpectedly accepted", s, e) }
            if got != s { t.Fatalf("terminal state changed: %v -> %v", s, got) }
        }
    }
}

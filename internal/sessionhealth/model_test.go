package sessionhealth

import "testing"

// TestStateMachineExhaustive explores every event sequence up to depth 4.
// It is a small deterministic model-based test for the session-health automaton.
func TestStateMachineExhaustive(t *testing.T) {
    events := []Event{RecoverableFailure, SuspectFailure, PoisonFailure, ProcessLost, VerifiedClean}
    var walk func(State, int)
    walk = func(state State, depth int) {
        if depth == 0 { return }
        for _, event := range events {
            next, err := Transition(state, event)
            if state == Poisoned || state == Lost {
                if err == nil { t.Fatalf("terminal state %s accepted event %d", state, event) }
                if next != state { t.Fatalf("terminal state %s changed to %s", state, next) }
                continue
            }
            if err != nil { t.Fatalf("non-terminal state %s rejected event %d: %v", state, event, err) }
            if event == PoisonFailure && next != Poisoned { t.Fatalf("poison event did not poison: %s", next) }
            if event == ProcessLost && next != Lost { t.Fatalf("process loss did not mark lost: %s", next) }
            walk(next, depth-1)
        }
    }
    walk(Healthy, 4)
}

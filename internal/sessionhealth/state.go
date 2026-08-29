package sessionhealth

import "fmt"

type State uint8

const (
    Healthy State = iota
    Suspect
    Poisoned
    Lost
)

type Event uint8

const (
    RecoverableFailure Event = iota
    SuspectFailure
    PoisonFailure
    ProcessLost
    VerifiedClean
)

func (s State) String() string {
    switch s {
    case Healthy: return "healthy"
    case Suspect: return "suspect"
    case Poisoned: return "poisoned"
    case Lost: return "lost"
    default: return "unknown"
    }
}

func Transition(s State, e Event) (State, error) {
    if s == Poisoned || s == Lost {
        return s, fmt.Errorf("terminal session state %s cannot be reused; start a new session epoch", s)
    }
    switch e {
    case RecoverableFailure:
        return s, nil
    case SuspectFailure:
        return Suspect, nil
    case PoisonFailure:
        return Poisoned, nil
    case ProcessLost:
        return Lost, nil
    case VerifiedClean:
        if s == Suspect { return Healthy, nil }
        return s, nil
    default:
        return s, fmt.Errorf("unknown event %d", e)
    }
}

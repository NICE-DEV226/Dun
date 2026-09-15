package extension

import (
	"context"
	"fmt"
	"sync"
)

// LifecycleState is the state of a plugin's lifecycle.
//
// DISCOVER -> VALIDATE -> LOAD -> COMPILE -> INSTANTIATE -> INITIALIZE
// -> READY -> SERVING -> DRAINING -> CLOSED
type LifecycleState int

const (
	StateDiscover LifecycleState = iota
	StateValidate
	StateLoad
	StateCompile
	StateInstantiate
	StateInitialize
	StateReady
	StateServing
	StateDraining
	StateClosed
)

var stateNames = map[LifecycleState]string{
	StateDiscover:    "DISCOVER",
	StateValidate:    "VALIDATE",
	StateLoad:        "LOAD",
	StateCompile:     "COMPILE",
	StateInstantiate: "INSTANTIATE",
	StateInitialize:  "INITIALIZE",
	StateReady:       "READY",
	StateServing:     "SERVING",
	StateDraining:    "DRAINING",
	StateClosed:      "CLOSED",
}

func (s LifecycleState) String() string {
	if name, ok := stateNames[s]; ok {
		return name
	}
	return fmt.Sprintf("STATE_%d", int(s))
}

// TransitionError is returned when an invalid lifecycle transition is attempted.
type TransitionError struct {
	From LifecycleState
	To   LifecycleState
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("invalid lifecycle transition: %s -> %s", e.From, e.To)
}

// Liveness tracks the lifecycle state of an extension.
type Liveness struct {
	mu    sync.RWMutex
	state LifecycleState
}

// NewLiveness starts an extension in the DISCOVER state.
func NewLiveness() *Liveness {
	return &Liveness{state: StateDiscover}
}

// State returns the current lifecycle state.
func (l *Liveness) State() LifecycleState {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.state
}

// Transition moves the extension to the next state.
func (l *Liveness) Transition(next LifecycleState) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !validTransition(l.state, next) {
		return &TransitionError{From: l.state, To: next}
	}
	l.state = next
	return nil
}

func validTransition(from, to LifecycleState) bool {
	switch from {
	case StateDiscover:
		return to == StateValidate
	case StateValidate:
		return to == StateLoad || to == StateClosed
	case StateLoad:
		return to == StateCompile || to == StateClosed
	case StateCompile:
		return to == StateInstantiate || to == StateClosed
	case StateInstantiate:
		return to == StateInitialize || to == StateClosed
	case StateInitialize:
		return to == StateReady || to == StateClosed
	case StateReady:
		return to == StateServing
	case StateServing:
		return to == StateDraining || to == StateClosed
	case StateDraining:
		return to == StateClosed
	case StateClosed:
		return to == StateDiscover
	default:
		return false
	}
}

// MustTransition is a convenience wrapper that panics on invalid transitions.
func (l *Liveness) MustTransition(next LifecycleState) {
	if err := l.Transition(next); err != nil {
		panic(err)
	}
}

// Validate transitions the extension into VALIDATE and validates ctx.
// Returns an error if validation fails, leaving the state at VALIDATE.
func (l *Liveness) Validate() error {
	if err := l.Transition(StateValidate); err != nil {
		return err
	}
	return nil
}

// Close closes the extension, transitioning it to CLOSED.
func (l *Liveness) Close(ctx context.Context) error {
	if l.State() == StateDraining {
		return l.Transition(StateClosed)
	}
	if l.State() == StateServing || l.State() == StateReady {
		return l.Transition(StateClosed)
	}
	return &TransitionError{From: l.State(), To: StateClosed}
}

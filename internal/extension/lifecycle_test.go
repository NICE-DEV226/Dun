package extension

import "testing"

func TestLifecycleFullPath(t *testing.T) {
	l := NewLiveness()

	steps := []LifecycleState{
		StateValidate,
		StateLoad,
		StateCompile,
		StateInstantiate,
		StateInitialize,
		StateReady,
		StateServing,
		StateDraining,
		StateClosed,
	}

	for _, want := range steps {
		if err := l.Transition(want); err != nil {
			t.Errorf("transition to %s: %v", want, err)
		}
		if l.State() != want {
			t.Errorf("expected %s, got %s", want, l.State())
		}
	}
}

func TestLifecycleInvalidTransition(t *testing.T) {
	l := NewLiveness()

	// Can't go straight to SERVING.
	if err := l.Transition(StateServing); err == nil {
		t.Fatal("expected error for invalid transition")
	}

	// Validate, then try to go back to DISCOVER.
	if err := l.Transition(StateValidate); err != nil {
		t.Fatal(err)
	}
	if err := l.Transition(StateDiscover); err == nil {
		t.Fatal("expected error going backward")
	}
}

func TestLifecycleRejectedAtValidate(t *testing.T) {
	l := NewLiveness()
	if err := l.Transition(StateValidate); err != nil {
		t.Fatal(err)
	}
	// A failed validation goes to CLOSED.
	if err := l.Transition(StateClosed); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleStateString(t *testing.T) {
	l := NewLiveness()
	if l.State().String() != "DISCOVER" {
		t.Errorf("expected DISCOVER, got %s", l.State())
	}
}

func TestLifecycleClose(t *testing.T) {
	l := NewLiveness()
	for _, s := range []LifecycleState{
		StateValidate, StateLoad, StateCompile, StateInstantiate,
		StateInitialize, StateReady, StateServing,
	} {
		if err := l.Transition(s); err != nil {
			t.Fatalf("transition to %s: %v", s, err)
		}
	}
	if err := l.Close(nil); err != nil {
		t.Fatalf("unexpected error closing: %v", err)
	}
	if l.State() != StateClosed {
		t.Errorf("expected CLOSED, got %s", l.State())
	}
}

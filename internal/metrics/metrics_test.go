package metrics

import (
	"strings"
	"testing"
)

func TestCounterIncrement(t *testing.T) {
	r := NewRegistry()
	r.Incr("requests_total")
	r.Incr("requests_total")
	r.Incr("requests_total")

	if got := r.GetCounter("requests_total"); got != 3 {
		t.Errorf("expected 3, got %d", got)
	}
}

func TestCounterMultipleNames(t *testing.T) {
	r := NewRegistry()
	r.Incr("a")
	r.Incr("a")
	r.Incr("b")

	if got := r.GetCounter("a"); got != 2 {
		t.Errorf("expected 2, got %d", got)
	}
	if got := r.GetCounter("b"); got != 1 {
		t.Errorf("expected 1, got %d", got)
	}
}

func TestCounterMissing(t *testing.T) {
	r := NewRegistry()
	if got := r.GetCounter("nonexistent"); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				r.Incr("concurrent")
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	if got := r.GetCounter("concurrent"); got != 1000 {
		t.Errorf("expected 1000, got %d", got)
	}
}

func TestHistogramObservation(t *testing.T) {
	r := NewRegistry()
	r.Observe("request_duration_seconds", 0.25)
	r.Observe("request_duration_seconds", 0.75)

	count, sum := r.GetHistogram("request_duration_seconds")
	if count != 2 {
		t.Fatalf("expected 2 observations, got %d", count)
	}
	if sum != 1 {
		t.Fatalf("expected sum 1, got %f", sum)
	}
}

func TestPrometheusOutput(t *testing.T) {
	r := NewRegistry()
	r.Incr("requests_total")
	r.Observe("request_duration_seconds", 0.25)

	got := r.Prometheus()
	for _, want := range []string{
		"requests_total 1\n",
		"request_duration_seconds_count 1\n",
		"request_duration_seconds_sum 0.25\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Prometheus output missing %q in %q", want, got)
		}
	}
}

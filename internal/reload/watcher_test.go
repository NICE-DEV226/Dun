package reload

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatcherDetectsChange(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "plugin.wasm")
	if err := os.WriteFile(f, []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}

	var hits atomic.Int32
	w := NewWatcher([]string{f}, 10*time.Millisecond, func(path string) {
		hits.Add(1)
	}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Start(ctx)

	// Rewrite with a newer mtime.
	time.Sleep(30 * time.Millisecond) // ensure a distinct mtime
	if err := os.WriteFile(f, []byte("v2"), 0644); err != nil {
		t.Fatal(err)
	}

	waitFor(t, 2*time.Second, func() bool { return hits.Load() >= 1 })
	if got := hits.Load(); got < 1 {
		t.Fatalf("expected at least one change notification, got %d", got)
	}

	cancel()
	w.Stop()
}

func TestWatcherStartsStopped(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "plugin.wasm")
	os.WriteFile(f, []byte("v1"), 0644)

	var hits atomic.Int32
	w := NewWatcher([]string{f}, time.Millisecond, func(path string) { hits.Add(1) }, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	go w.Start(ctx)
	time.Sleep(20 * time.Millisecond)
	cancel()
	w.Stop()

	// After Stop, modifications must not fire.
	start := hits.Load()
	os.WriteFile(f, []byte("v2"), 0644)
	time.Sleep(30 * time.Millisecond)
	if got := hits.Load(); got != start {
		t.Errorf("callback fired after stop: %d → %d", start, got)
	}
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

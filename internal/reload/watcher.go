// Package reload provides poll-based file change detection for Dun plugin
// wasm artifacts. It has no external dependencies (no fsnotify) and
// implements a simple stat-based polling loop portable across all Go-
// supported platforms.
package reload

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"
)

// Watcher monitors a set of file paths for modification-time changes and
// invokes a callback when a change is detected.
type Watcher struct {
	paths    map[string]time.Time
	interval time.Duration
	onChange func(path string)
	logger   *slog.Logger

	mu   sync.Mutex
	stop chan struct{}
	done chan struct{}
}

// NewWatcher creates a file watcher that polls paths at the given
// interval. onChange is called (in its own goroutine, one at a time)
// whenever a file's ModTime changes.
func NewWatcher(paths []string, interval time.Duration, onChange func(path string), logger *slog.Logger) *Watcher {
	if logger == nil {
		logger = slog.Default()
	}
	m := make(map[string]time.Time, len(paths))
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil {
			m[p] = info.ModTime()
		}
	}
	return &Watcher{
		paths:    m,
		interval: interval,
		onChange: onChange,
		logger:   logger,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Paths returns the set of paths currently being watched.
func (w *Watcher) Paths() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, 0, len(w.paths))
	for p := range w.paths {
		out = append(out, p)
	}
	return out
}

// Start begins the polling loop. It blocks until ctx is cancelled or Stop
// is called.
func (w *Watcher) Start(ctx context.Context) {
	defer close(w.done)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Snapshot initial state so the first poll detects changes made
	// between NewWatcher and Start.
	w.refresh()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stop:
			return
		case <-ticker.C:
			w.poll()
		}
	}
}

// Stop terminates the polling loop.
func (w *Watcher) Stop() {
	select {
	case <-w.stop:
		// already stopped
	default:
		close(w.stop)
	}
	<-w.done
}

// refresh re-reads modtimes for all watched paths.
func (w *Watcher) refresh() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for p := range w.paths {
		if info, err := os.Stat(p); err == nil {
			w.paths[p] = info.ModTime()
		}
	}
}

// poll checks for changes and fires the callback.
func (w *Watcher) poll() {
	w.mu.Lock()
	changed := make([]string, 0)
	for p, prev := range w.paths {
		info, err := os.Stat(p)
		if err != nil {
			continue // file disappeared or unreadable — skip
		}
		if info.ModTime() != prev {
			w.paths[p] = info.ModTime()
			changed = append(changed, p)
		}
	}
	w.mu.Unlock()

	for _, p := range changed {
		w.logger.Info("plugin file changed", "path", p)
		w.onChange(p)
	}
}

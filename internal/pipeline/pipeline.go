// Package pipeline provides the extension pipeline abstraction for Dun.
//
// The pipeline is the ordered sequence of extensions that process an HTTP
// request before and after it is proxied to the upstream service. Each
// extension receives a constrained view of the request (based on its
// declared capabilities) and returns a decision that determines the
// pipeline's next action.
package pipeline

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
)

// Decision represents the action an extension takes after processing a request.
type Decision int

const (
	// DecisionContinue means the request should proceed to the next extension.
	DecisionContinue Decision = iota
	// DecisionRespond means the extension will handle the response directly.
	DecisionRespond
	// DecisionReject means the request should be rejected (e.g. 403).
	DecisionReject
	// DecisionError means an internal error occurred.
	DecisionError
)

func (d Decision) String() string {
	switch d {
	case DecisionContinue:
		return "CONTINUE"
	case DecisionRespond:
		return "RESPOND"
	case DecisionReject:
		return "REJECT"
	case DecisionError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Extension is the interface all pipeline extensions must implement.
type Extension interface {
	// Name returns the extension's identifier.
	Name() string
	// ProcessRequest evaluates the request and returns a decision.
	// Extensions may modify the request in-place for CONTINUE decisions.
	ProcessRequest(ctx context.Context, w http.ResponseWriter, req *http.Request) Decision
	// ProcessResponse evaluates the response before it is sent to the client.
	// Extensions may modify response headers and status.
	ProcessResponse(ctx context.Context, w http.ResponseWriter, req *http.Request, status int)
}

// RequestState holds per-request data shared across extensions in a pipeline.
type RequestState struct {
	metadata map[string]string
	mu       sync.RWMutex
}

// NewRequestState creates a new per-request state container.
func NewRequestState() *RequestState {
	return &RequestState{
		metadata: make(map[string]string),
	}
}

// Get retrieves a metadata value by key.
func (s *RequestState) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.metadata[key]
	return v, ok
}

// Set stores a metadata value.
func (s *RequestState) Set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metadata[key] = value
}

// Pipeline is an ordered collection of extensions.
type Pipeline struct {
	extensions []Extension
	logger     *slog.Logger
}

// New creates a Pipeline with the given extensions.
func New(extensions []Extension, logger *slog.Logger) *Pipeline {
	if logger == nil {
		logger = slog.Default()
	}
	return &Pipeline{
		extensions: extensions,
		logger:     logger,
	}
}

// Execute runs all extensions in order for the incoming request.
// It returns the final decision and any extension that short-circuited the pipeline.
func (p *Pipeline) Execute(ctx context.Context, w http.ResponseWriter, req *http.Request) (Decision, Extension) {
	state := NewRequestState()
	ctx = context.WithValue(ctx, stateKey, state)

	for _, ext := range p.extensions {
		decision := ext.ProcessRequest(ctx, w, req)
		p.logger.Debug("extension processed",
			"extension", ext.Name(),
			"decision", decision.String(),
		)

		switch decision {
		case DecisionContinue:
			continue
		case DecisionRespond, DecisionReject, DecisionError:
			return decision, ext
		}
	}

	return DecisionContinue, nil
}

// ExecuteResponse runs all extensions in reverse order for the response.
func (p *Pipeline) ExecuteResponse(ctx context.Context, w http.ResponseWriter, req *http.Request, status int) {
	for i := len(p.extensions) - 1; i >= 0; i-- {
		p.extensions[i].ProcessResponse(ctx, w, req, status)
	}
}

// Len returns the number of extensions in the pipeline.
func (p *Pipeline) Len() int {
	return len(p.extensions)
}

type contextKey string

const stateKey contextKey = "dun.request.state"

// GetRequestState retrieves the per-request state from context.
func GetRequestState(ctx context.Context) *RequestState {
	state, _ := ctx.Value(stateKey).(*RequestState)
	return state
}

// NoopExtension is a base type for extensions that do not modify requests.
type NoopExtension struct {
	Name_ string
}

func (e *NoopExtension) Name() string { return e.Name_ }

func (e *NoopExtension) ProcessRequest(_ context.Context, _ http.ResponseWriter, _ *http.Request) Decision {
	return DecisionContinue
}

func (e *NoopExtension) ProcessResponse(_ context.Context, _ http.ResponseWriter, _ *http.Request, _ int) {
}

// ErrExtension wraps a named extension that always returns an error decision.
type ErrExtension struct {
	NoopExtension
	Err error
}

func (e *ErrExtension) ProcessRequest(_ context.Context, _ http.ResponseWriter, _ *http.Request) Decision {
	return DecisionError
}

// String returns a human-readable representation.
func (p *Pipeline) String() string {
	names := make([]string, len(p.extensions))
	for i, ext := range p.extensions {
		names[i] = ext.Name()
	}
	return "Pipeline[" + strings.Join(names, ",") + "]"
}

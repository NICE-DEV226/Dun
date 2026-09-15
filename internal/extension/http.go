package extension

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/NICE-DEV226/Dun/internal/pipeline"
)

// httpBridge implements HTTPBridge, giving an extension a constrained,
// capability-scoped view of the current HTTP transaction.
//
// It is per-request: created at invocation time and never shared. The plugin
// never sees Go objects directly — only these accessors behind host funcs.
type httpBridge struct {
	req     *http.Request
	headers http.Header
	logger  *slog.Logger
}

// NewHTTPBridge builds an HTTPBridge for the current request.
// Header mutations are applied to w.Header(), which the proxy layer honors
// when it writes the final response.
func NewHTTPBridge(w http.ResponseWriter, req *http.Request, logger *slog.Logger) HTTPBridge {
	if logger == nil {
		logger = slog.Default()
	}
	return &httpBridge{
		req:     req,
		headers: w.Header(),
		logger:  logger,
	}
}

func (b *httpBridge) RequestHeader(name string) string {
	return b.req.Header.Get(name)
}

func (b *httpBridge) SetResponseHeader(name, value string) {
	b.headers.Set(name, value)
}

func (b *httpBridge) Log(level int, msg string) {
	attrs := []any{"extension_msg", msg}
	switch level {
	case 0:
		b.logger.Debug("extension log", attrs...)
	case 1:
		b.logger.Info("extension log", attrs...)
	case 2:
		b.logger.Warn("extension log", attrs...)
	case 3:
		b.logger.Error("extension log", attrs...)
	default:
		b.logger.Info("extension log", attrs...)
	}
}

// HTTPExtension adapts a loaded, serving extension into a pipeline.Extension.
// It is the bridge between the core request path and the Wasm sandbox.
type HTTPExtension struct {
	Name_   string
	Manager *Manager
	Logger  *slog.Logger
}

// Name implements pipeline.Extension.
func (e *HTTPExtension) Name() string { return e.Name_ }

// ProcessRequest invokes the extension and maps its ABI decision onto the
// pipeline decision model.
func (e *HTTPExtension) ProcessRequest(ctx context.Context, w http.ResponseWriter, req *http.Request) pipeline.Decision {
	bridge := NewHTTPBridge(w, req, e.Logger)
	res, err := e.Manager.Invoke(ctx, e.Name_, "process_request", nil, bridge)
	if err != nil {
		e.Logger.Error("extension invocation failed",
			"extension", e.Name_,
			"error", err,
		)
		return pipeline.DecisionError
	}

	switch res.Decision {
	case "reject":
		return pipeline.DecisionReject
	case "respond":
		return pipeline.DecisionRespond
	case "error":
		return pipeline.DecisionError
	default:
		return pipeline.DecisionContinue
	}
}

// ProcessResponse is a no-op in v0: response handling happens on the request
// path through header mutation on w.Header().
func (e *HTTPExtension) ProcessResponse(context.Context, http.ResponseWriter, *http.Request, int) {}

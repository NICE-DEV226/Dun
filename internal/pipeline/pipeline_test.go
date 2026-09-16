package pipeline

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type testExtension struct {
	NoopExtension
	decision  Decision
	responses int
}

func (e *testExtension) ProcessRequest(_ context.Context, _ http.ResponseWriter, _ *http.Request) Decision {
	return e.decision
}

func (e *testExtension) ProcessResponse(_ context.Context, _ http.ResponseWriter, _ *http.Request, _ int) {
	e.responses++
}

func TestPipelineContinue(t *testing.T) {
	exts := []Extension{
		&testExtension{NoopExtension: NoopExtension{Name_: "ext1"}, decision: DecisionContinue},
		&testExtension{NoopExtension: NoopExtension{Name_: "ext2"}, decision: DecisionContinue},
	}
	p := New(exts, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)

	decision, caller := p.Execute(context.Background(), w, req)
	if decision != DecisionContinue {
		t.Errorf("expected CONTINUE, got %s", decision)
	}
	if caller != nil {
		t.Error("expected nil caller for CONTINUE")
	}
}

func TestPipelineReject(t *testing.T) {
	exts := []Extension{
		&testExtension{NoopExtension: NoopExtension{Name_: "auth"}, decision: DecisionReject},
		&testExtension{NoopExtension: NoopExtension{Name_: "next"}, decision: DecisionContinue},
	}
	p := New(exts, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)

	decision, caller := p.Execute(context.Background(), w, req)
	if decision != DecisionReject {
		t.Errorf("expected REJECT, got %s", decision)
	}
	if caller == nil || caller.Name() != "auth" {
		t.Error("expected 'auth' as caller")
	}
}

func TestPipelineRespond(t *testing.T) {
	exts := []Extension{
		&testExtension{NoopExtension: NoopExtension{Name_: "cache"}, decision: DecisionRespond},
	}
	p := New(exts, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)

	decision, _ := p.Execute(context.Background(), w, req)
	if decision != DecisionRespond {
		t.Errorf("expected RESPOND, got %s", decision)
	}
}

func TestPipelineLen(t *testing.T) {
	exts := []Extension{
		&testExtension{NoopExtension: NoopExtension{Name_: "a"}},
		&testExtension{NoopExtension: NoopExtension{Name_: "b"}},
	}
	p := New(exts, nil)
	if p.Len() != 2 {
		t.Errorf("expected 2, got %d", p.Len())
	}
}

func TestPipelineExecuteResponseWithoutRequestState(t *testing.T) {
	ext := &testExtension{NoopExtension: NoopExtension{Name_: "response"}}
	p := New([]Extension{ext}, nil)
	p.ExecuteResponse(context.Background(), httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil), http.StatusOK)
	if ext.responses != 1 {
		t.Fatalf("expected response hook once, got %d", ext.responses)
	}
}

func TestPipelineString(t *testing.T) {
	exts := []Extension{
		&testExtension{NoopExtension: NoopExtension{Name_: "auth"}},
		&testExtension{NoopExtension: NoopExtension{Name_: "ratelimit"}},
	}
	p := New(exts, nil)
	s := p.String()
	if s != "Pipeline[auth,ratelimit]" {
		t.Errorf("unexpected string: %s", s)
	}
}

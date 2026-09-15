package extension

import "testing"

func TestSetBasic(t *testing.T) {
	s := NewSet("request.headers.read", "response.headers.write")
	if !s.Has(CapRequestHeadersRead) {
		t.Error("expected header read capability")
	}
	if !s.Has(CapResponseHeadersWrite) {
		t.Error("expected header write capability")
	}
	if s.Has(CapMetadataWrite) {
		t.Error("did not expect metadata write capability")
	}
}

func TestSetSorted(t *testing.T) {
	s := NewSet("b", "a", "c")
	got := s.Sorted()
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("unexpected sorted order: %v", got)
	}
}

func TestSetHasAll(t *testing.T) {
	s := NewSet("request.headers.read", "log.info", "log.warn")
	if !s.HasAll(CapRequestHeadersRead, CapLogInfo) {
		t.Error("expected all caps present")
	}
	if s.HasAll(CapRequestHeadersRead, CapMetadataWrite) {
		t.Error("did not expect metadata write present")
	}
}

func TestSetAdd(t *testing.T) {
	s := NewSet()
	s.Add(CapLogDebug)
	if !s.Has(CapLogDebug) {
		t.Error("Add + Has mismatch")
	}
	if err := ValidateManifest(s); err != nil {
		t.Error("expected valid manifest")
	}
}

func TestValidateManifestBad(t *testing.T) {
	s := NewSet("request.headers.read", "totally.bogus")
	if err := ValidateManifest(s); err == nil {
		t.Fatal("expected error for bogus capability")
	}
}

func TestSetIntersection(t *testing.T) {
	a := NewSet("log.info", "response.headers.write")
	b := NewSet("log.info", "request.headers.read")
	c := a.Intersection(b)
	if !c.Has(CapLogInfo) {
		t.Error("expected log.info in intersection")
	}
	if c.Has(CapResponseHeadersWrite) {
		t.Error("did not expect response.headers.write in intersection")
	}
}

func TestSetUnion(t *testing.T) {
	a := NewSet("log.info")
	b := NewSet("log.warn")
	c := a.Union(b)
	if !c.Has(CapLogInfo) || !c.Has(CapLogWarn) {
		t.Error("union missing capabilities")
	}
}

func TestSetString(t *testing.T) {
	s := NewSet("z", "a")
	if s.String() != "a,z" {
		t.Errorf("unexpected string: %s", s.String())
	}
}

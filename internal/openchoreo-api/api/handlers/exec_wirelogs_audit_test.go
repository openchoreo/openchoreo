// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openchoreo/openchoreo/internal/auditconfig"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

func newTestAuditEmitter(t *testing.T, logger *slog.Logger) *audit.Emitter {
	t.Helper()
	auditCfg := config.AuditDefaults()
	policies, err := auditCfg.BuildPolicySet(auditconfig.Vocabulary{}, nil)
	if err != nil {
		t.Fatalf("unexpected error building policy set: %v", err)
	}
	emitter, err := audit.NewEmitter("openchoreo-api", policies, audit.NewLogger(logger))
	if err != nil {
		t.Fatalf("unexpected error building emitter: %v", err)
	}
	return emitter
}

// TestExecWirelogsAuth401IsAudited proves the composition main.go uses for
// both routes — audit.NewUnauthenticatedMiddleware wrapping jwtMiddleware
// wrapping the pattern-map middleware — records a token rejection.
//
// The inner middleware cannot: auth answers a rejected request itself and
// never calls next, so nothing pattern-map-driven ever runs. Both routes
// reach the data plane (a live shell, a live traffic stream), so a rejected
// attempt on them is exactly the event worth having.
//
// fakeAuth stands in for jwtMiddleware and is deliberately shaped like it —
// writing its own 401 and returning without calling next — because that
// short-circuit is the entire reason the outer instance exists.
func TestExecWirelogsAuth401IsAudited(t *testing.T) {
	// next is deliberately dropped: that short-circuit is what the outer
	// instance exists to cover.
	fakeAuth := func(_ http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
	}

	for _, tc := range []struct {
		name    string
		pattern string
		target  string
	}{
		{"exec", ExecRoutePattern, "/exec/namespaces/ns-a/components/c-a"},
		{"wirelogs", WirelogsRoutePattern, "/api/v1/namespaces/ns-a/environments/dev-a/wirelogs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, nil))
			emitter := newTestAuditEmitter(t, logger)
			mw, err := NewExecWirelogsAuditMiddleware(logger, emitter, true)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			unauthedMw := audit.NewUnauthenticatedMiddleware(emitter, audit.OriginAPI, true)

			inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Error("handler must not run on a rejected request")
			})

			mux := http.NewServeMux()
			mux.Handle(tc.pattern, unauthedMw(fakeAuth(mw.Handler(inner))))

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.target, nil))

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			records := auditRecords(t, &buf)
			if len(records) != 1 {
				t.Fatalf("expected exactly one AUDIT-LOG record, got %d:\n%s", len(records), buf.String())
			}
			if records[0]["result"] != "unauthenticated" {
				t.Errorf("result = %v, want unauthenticated", records[0]["result"])
			}
			if records[0]["origin"] != string(audit.OriginAPI) {
				t.Errorf("origin = %v, want %q", records[0]["origin"], audit.OriginAPI)
			}
			actor, ok := records[0]["actor"].(map[string]any)
			if !ok || actor["id"] != "anonymous" {
				t.Errorf("actor = %v, want an anonymous actor", records[0]["actor"])
			}
		})
	}
}

// TestExecWirelogsAuthenticatedRequestEmitsExactlyOnce is the other half of
// the mutual-exclusion contract: when auth passes, the inner pattern-map
// middleware owns the event and the outer instance must stay silent. Without
// this, a change that made the outer one fire unconditionally would still
// pass the 401 test above while double-auditing every real exec session.
func TestExecWirelogsAuthenticatedRequestEmitsExactlyOnce(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	emitter := newTestAuditEmitter(t, logger)
	mw, err := NewExecWirelogsAuditMiddleware(logger, emitter, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	unauthedMw := audit.NewUnauthenticatedMiddleware(emitter, audit.OriginAPI, true)

	passthroughAuth := func(next http.Handler) http.Handler { return next }
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux := http.NewServeMux()
	mux.Handle(ExecRoutePattern, unauthedMw(passthroughAuth(mw.Handler(inner))))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/exec/namespaces/ns-a/components/c-a", nil))

	records := auditRecords(t, &buf)
	if len(records) != 1 {
		t.Fatalf("expected exactly one AUDIT-LOG record, got %d:\n%s", len(records), buf.String())
	}
	if records[0]["action"] != "exec_component" {
		t.Errorf("action = %v, want exec_component (the inner instance's event, not the outer's)",
			records[0]["action"])
	}
}

// TestFindFlusher_SeesThroughAuditWrapper guards the fix this deliverable
// depends on: wirelogs.go's flush check must find a Flusher through the
// audit middleware's responseWriter wrapper (which implements Unwrap() but
// not Flusher itself), not just on a bare, unwrapped ResponseWriter.
func TestFindFlusher_SeesThroughAuditWrapper(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	emitter := newTestAuditEmitter(t, logger)
	mw, err := NewExecWirelogsAuditMiddleware(logger, emitter, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var sawFlusher bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, sawFlusher = findFlusher(w)
		w.WriteHeader(http.StatusOK)
	})

	mux := http.NewServeMux()
	mux.Handle(WirelogsRoutePattern, mw.Handler(inner))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/ns/environments/dev/wirelogs", nil)
	rec := httptest.NewRecorder() // implements http.Flusher
	mux.ServeHTTP(rec, req)

	if !sawFlusher {
		t.Error("findFlusher did not find a Flusher through the audit middleware's wrapper, " +
			"want it to follow Unwrap() the same way http.ResponseController does")
	}
}

// TestExecWirelogsAuditMiddleware_ResolvesBothRoutes proves the hand-declared
// pattern map resolves both routes to their own Operation and emits exactly
// one event per request, routed through a real http.ServeMux (the same
// routing exec/wirelogs run behind in production — see
// cmd/openchoreo-api/main.go) so this exercises the actual r.Pattern value,
// not an assumed one.
func TestExecWirelogsAuditMiddleware_ResolvesBothRoutes(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	emitter := newTestAuditEmitter(t, logger)
	mw, err := NewExecWirelogsAuditMiddleware(logger, emitter, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux := http.NewServeMux()
	mux.Handle(ExecRoutePattern, mw.Handler(inner))
	mux.Handle(WirelogsRoutePattern, mw.Handler(inner))

	tests := []struct {
		name       string
		path       string
		wantAction string
	}{
		{"exec", "/exec/namespaces/ns/components/comp", "exec_component"},
		{"wirelogs", "/api/v1/namespaces/ns/environments/dev/wirelogs", "view_wirelogs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			records := auditRecords(t, &buf)
			if len(records) != 1 {
				t.Fatalf("expected exactly one AUDIT-LOG record, got %d:\n%s", len(records), buf.String())
			}
			if got := records[0]["action"]; got != tt.wantAction {
				t.Errorf("action = %v, want %q", got, tt.wantAction)
			}
			if got := records[0]["result"]; got != "success" {
				t.Errorf("result = %v, want success", got)
			}
		})
	}
}

// TestExecHandler_SetsResourceNameFromParsedPath guards resource.name for
// exec specifically: its route has no named wildcard the audit middleware
// could seed from, so ExecHandler.ServeHTTP must call audit.SetResource
// itself once it parses the component name out of the raw path — including
// on a request that never reaches the gateway (this one fails with a nil
// AuthzChecker before any gateway call), proving the call happens before any
// error return, not only on the eventual success path.
func TestExecHandler_SetsResourceNameFromParsedPath(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	emitter := newTestAuditEmitter(t, logger)
	mw, err := NewExecWirelogsAuditMiddleware(logger, emitter, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	execHandler := NewExecHandler(nil, nil, "", nil, nil, logger) // nil authzChecker -> 500 before any gateway call

	mux := http.NewServeMux()
	mux.Handle(ExecRoutePattern, mw.Handler(execHandler))

	req := httptest.NewRequest(http.MethodGet, "/exec/namespaces/ns-a/components/comp-a", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	records := auditRecords(t, &buf)
	if len(records) != 1 {
		t.Fatalf("expected exactly one AUDIT-LOG record, got %d:\n%s", len(records), buf.String())
	}
	resource, ok := records[0]["resource"].(map[string]any)
	if !ok {
		t.Fatalf("resource not populated: %+v", records[0])
	}
	if resource["name"] != "comp-a" {
		t.Errorf("resource.name = %v, want %q", resource["name"], "comp-a")
	}
}

// TestWirelogsHandler_SetsResourceNamespaceFromParsedPath guards
// resource.namespace for wirelogs specifically: the audit middleware's
// pre-handler seed hardcodes r.PathValue("namespaceName"), but wirelogs'
// route uses "{namespace}", so that seed's namespace is always empty here.
// WirelogsHandler.ServeHTTP must call audit.SetResource itself right after
// parsing the path — including on a request that never reaches
// authorization (this one fails with a nil AuthzChecker before any k8s or
// gateway call), proving the call happens before every pre-authz exit, not
// only on the eventual success path.
func TestWirelogsHandler_SetsResourceNamespaceFromParsedPath(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	emitter := newTestAuditEmitter(t, logger)
	mw, err := NewExecWirelogsAuditMiddleware(logger, emitter, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wirelogsHandler := NewWirelogsHandler(nil, nil, "", nil, nil, logger) // nil authzChecker -> 500 before any gateway call

	mux := http.NewServeMux()
	mux.Handle(WirelogsRoutePattern, mw.Handler(wirelogsHandler))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/ns-a/environments/dev-a/wirelogs", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	records := auditRecords(t, &buf)
	if len(records) != 1 {
		t.Fatalf("expected exactly one AUDIT-LOG record, got %d:\n%s", len(records), buf.String())
	}
	resource, ok := records[0]["resource"].(map[string]any)
	if !ok {
		t.Fatalf("resource not populated: %+v", records[0])
	}
	if resource["namespace"] != "ns-a" {
		t.Errorf("resource.namespace = %v, want %q", resource["namespace"], "ns-a")
	}
	if resource["name"] != "dev-a" {
		t.Errorf("resource.name = %v, want %q", resource["name"], "dev-a")
	}
}

// TestWirelogsHandler_MalformedNamespaceNotAudited guards the other half of
// the fix above: audit.SetResource must run after the length/RFC1123
// validation, not before it, so a malformed namespace — client-controlled
// and otherwise bounded only by MaxHeaderBytes — never lands verbatim in the
// audit record on the 400 it triggers. This is parity with every
// spec-derived REST route, where "a malformed path parameter never reaches
// audit" (see docs/audit/coverage-matrix.md's Known non-events), not a
// regression: the namespace is simply absent, the same as those routes.
func TestWirelogsHandler_MalformedNamespaceNotAudited(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	emitter := newTestAuditEmitter(t, logger)
	mw, err := NewExecWirelogsAuditMiddleware(logger, emitter, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wirelogsHandler := NewWirelogsHandler(nil, nil, "", nil, nil, logger)

	mux := http.NewServeMux()
	mux.Handle(WirelogsRoutePattern, mw.Handler(wirelogsHandler))

	// Uppercase fails the RFC1123-style name regex, triggering the 400 before
	// audit.SetResource's (now-later) call site.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/namespaces/NS_A/environments/dev-a/wirelogs", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	records := auditRecords(t, &buf)
	if len(records) != 1 {
		t.Fatalf("expected exactly one AUDIT-LOG record, got %d:\n%s", len(records), buf.String())
	}
	// The middleware's own pre-handler seed still populates resource.name
	// from the (valid) environment path segment — that seed is unaffected by
	// this fix and isn't what N4 was about. What must be absent is the
	// malformed namespace itself.
	if resource, ok := records[0]["resource"].(map[string]any); ok {
		if ns, present := resource["namespace"]; present {
			t.Errorf("resource.namespace = %v, want absent — the malformed namespace must not reach "+
				"the audit record", ns)
		}
	}
}

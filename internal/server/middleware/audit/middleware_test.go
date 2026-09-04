// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// Shared fixture values for this package's tests — a stand-in operation and
// its route pattern, reused across middleware/pattern-map/operation-def
// tests rather than repeated as raw literals everywhere.
const (
	testProjectOpID    = "CreateProject"
	testProjectPattern = "POST /projects"
)

func TestExtractActor(t *testing.T) {
	tests := []struct {
		name             string
		subjectCtx       *auth.SubjectContext
		wantType         string
		wantID           string
		wantEntitlements map[string][]string
	}{
		{
			name:       "no subject context",
			subjectCtx: nil,
			wantType:   "anonymous",
			wantID:     "anonymous",
		},
		{
			name: "subject with ID set, EntitlementValues empty",
			subjectCtx: &auth.SubjectContext{
				ID:                "user-123",
				Type:              "user",
				EntitlementClaim:  "groups",
				EntitlementValues: []string{},
			},
			wantType:         "user",
			wantID:           "user-123",
			wantEntitlements: map[string][]string{"groups": {}},
		},
		{
			name: "subject with empty ID",
			subjectCtx: &auth.SubjectContext{
				ID:   "",
				Type: "user",
			},
			wantType: "user",
			wantID:   "unknown",
		},
		{
			// Defense-in-depth: not reachable via jwt/resolver.go today (it
			// reads sub explicitly rather than through fmt.Sprintf), but a
			// fabricated actor identity is undetectable downstream if some
			// other SubjectContext constructor ever reintroduces it.
			name: "subject with ID literally \"<nil>\"",
			subjectCtx: &auth.SubjectContext{
				ID:   "<nil>",
				Type: "user",
			},
			wantType: "user",
			wantID:   "unknown",
		},
		{
			name: "subject with no entitlement claim configured",
			subjectCtx: &auth.SubjectContext{
				ID:   "user-456",
				Type: "user",
			},
			wantType:         "user",
			wantID:           "user-456",
			wantEntitlements: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.subjectCtx != nil {
				ctx = auth.SetSubjectContext(ctx, tt.subjectCtx)
			}

			actor := ExtractActor(ctx)

			if actor.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", actor.Type, tt.wantType)
			}
			if actor.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", actor.ID, tt.wantID)
			}
			if !reflect.DeepEqual(actor.Entitlements, tt.wantEntitlements) {
				t.Errorf("Entitlements = %#v, want %#v", actor.Entitlements, tt.wantEntitlements)
			}
		})
	}
}

// TestMiddleware_Handler_EmitsOnPanic guards against a panicking handler
// producing zero audit events. The panic must still propagate afterward —
// this only closes the audit gap, it doesn't change failure behavior.
func TestMiddleware_Handler_EmitsOnPanic(t *testing.T) {
	sink := &recordingSink{}
	policies, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: true}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	emitter, err := NewEmitter("test-service", policies, sink)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	op := &Operation{ID: testProjectOpID, Action: "create_project", ResourceType: "project", Category: CategoryManagement}
	patternMap := map[string]*Operation{testProjectPattern: op}
	mw := newMiddleware(slog.Default(), patternMap, emitter, true)

	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("handler blew up after mutating state")
	})

	req := httptest.NewRequest(http.MethodPost, "/projects", nil)
	req.Pattern = testProjectPattern
	rw := httptest.NewRecorder()

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected the panic to propagate out of Handler, got none")
			}
		}()
		mw.Handler(panicking).ServeHTTP(rw, req)
	}()

	if len(sink.events) != 1 {
		t.Fatalf("expected exactly one audit event despite the panic, got %d", len(sink.events))
	}
	if sink.events[0].Result != ResultFailure {
		t.Errorf("Result = %v, want failure", sink.events[0].Result)
	}
}

func TestMiddleware_Handler_DisabledSkipsAllAuditLogic(t *testing.T) {
	sink := &recordingSink{}
	policies, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: true}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	emitter, err := NewEmitter("test-service", policies, sink)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	patternMap := map[string]*Operation{testProjectPattern: {ID: testProjectOpID, Category: CategoryManagement}}
	mw := newMiddleware(slog.Default(), patternMap, emitter, false)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/projects", nil)
	req.Pattern = testProjectPattern
	rw := httptest.NewRecorder()

	mw.Handler(next).ServeHTTP(rw, req)

	if !called {
		t.Fatal("expected next to be called even when audit is disabled")
	}
	if len(sink.events) != 0 {
		t.Errorf("expected no audit events when disabled, got %d", len(sink.events))
	}
}

func TestMiddleware_Handler_UnmatchedPatternPassesThrough(t *testing.T) {
	sink := &recordingSink{}
	policies, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: true}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	emitter, err := NewEmitter("test-service", policies, sink)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty pattern map: no route is audited, so every request must pass
	// straight through with no event.
	mw := newMiddleware(slog.Default(), map[string]*Operation{}, emitter, true)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Pattern = "GET /healthz"
	rw := httptest.NewRecorder()

	mw.Handler(next).ServeHTTP(rw, req)

	if !called {
		t.Fatal("expected next to be called for an unaudited route")
	}
	if len(sink.events) != 0 {
		t.Errorf("expected no audit events for an unaudited route, got %d", len(sink.events))
	}
}

// TestMiddleware_Handler_EmptyPatternLogsAndPassesThrough guards against an
// empty r.Pattern (e.g. a non-Go-1.22-ServeMux BaseRouter that never sets it)
// taking down the whole API: it must log loudly for operational visibility
// but still call next and emit no audit event, rather than panicking and
// turning every request — not just audited ones — into a 500.
func TestMiddleware_Handler_EmptyPatternLogsAndPassesThrough(t *testing.T) {
	policies, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: true}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	sink := &recordingSink{}
	emitter, err := NewEmitter("test-service", policies, sink)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	mw := newMiddleware(logger, map[string]*Operation{}, emitter, true)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/whatever", nil) // req.Pattern left empty
	rw := httptest.NewRecorder()

	mw.Handler(next).ServeHTTP(rw, req)

	if !called {
		t.Fatal("expected next to be called despite the empty r.Pattern")
	}
	if len(sink.events) != 0 {
		t.Errorf("expected no audit events for an empty r.Pattern, got %d", len(sink.events))
	}
	if !strings.Contains(buf.String(), "r.Pattern is empty") {
		t.Errorf("expected an error to be logged for the empty r.Pattern, got:\n%s", buf.String())
	}
}

func TestMiddleware_Handler_ResultClassification(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       Result
	}{
		{name: "2xx is success", statusCode: http.StatusOK, want: ResultSuccess},
		{name: "401 is unauthenticated", statusCode: http.StatusUnauthorized, want: ResultUnauthenticated},
		{name: "403 is denied", statusCode: http.StatusForbidden, want: ResultDenied},
		{name: "500 is failure", statusCode: http.StatusInternalServerError, want: ResultFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := &recordingSink{}
			policies, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: true}, nil)
			if len(errs) != 0 {
				t.Fatalf("unexpected validation errors: %v", errs)
			}
			emitter, err := NewEmitter("test-service", policies, sink)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			op := &Operation{ID: testProjectOpID, Action: "create_project", ResourceType: "project", Category: CategoryManagement}
			patternMap := map[string]*Operation{testProjectPattern: op}
			mw := newMiddleware(slog.Default(), patternMap, emitter, true)

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte("body"))
			})

			req := httptest.NewRequest(http.MethodPost, "/projects", nil)
			req.Pattern = testProjectPattern
			rw := httptest.NewRecorder()

			mw.Handler(next).ServeHTTP(rw, req)

			if len(sink.events) != 1 {
				t.Fatalf("expected exactly one audit event, got %d", len(sink.events))
			}
			if sink.events[0].Result != tt.want {
				t.Errorf("Result = %v, want %v", sink.events[0].Result, tt.want)
			}
			if rw.Code != tt.statusCode {
				t.Errorf("recorded status = %d, want %d (responseWriter must not alter the real response)", rw.Code, tt.statusCode)
			}
		})
	}
}

// TestMiddleware_Handler_WriteWithoutExplicitWriteHeader guards
// responseWriter.Write's implicit-200 path: a handler that calls Write
// without ever calling WriteHeader must still be recorded as a 200/success,
// matching real net/http.ResponseWriter semantics.
func TestMiddleware_Handler_WriteWithoutExplicitWriteHeader(t *testing.T) {
	sink := &recordingSink{}
	policies, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: true}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	emitter, err := NewEmitter("test-service", policies, sink)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	op := &Operation{ID: testProjectOpID, Action: "create_project", ResourceType: "project", Category: CategoryManagement}
	patternMap := map[string]*Operation{testProjectPattern: op}
	mw := newMiddleware(slog.Default(), patternMap, emitter, true)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("body")) // no explicit WriteHeader call
	})

	req := httptest.NewRequest(http.MethodPost, "/projects", nil)
	req.Pattern = testProjectPattern
	rw := httptest.NewRecorder()

	mw.Handler(next).ServeHTTP(rw, req)

	if len(sink.events) != 1 {
		t.Fatalf("expected exactly one audit event, got %d", len(sink.events))
	}
	if sink.events[0].Result != ResultSuccess {
		t.Errorf("Result = %v, want success (implicit 200)", sink.events[0].Result)
	}
}

// TestMiddleware_Handler_SetResultOverridesStatusCode guards the fix for a
// hijacking handler (exec's WebSocket upgrade): once hijacked, nothing
// written to the raw connection touches the wrapped ResponseWriter, so a
// post-hijack failure would otherwise be misclassified as success from the
// unchanged 200 default. A handler that calls SetResult before returning
// must have that result win over rw.statusCode's classification.
func TestMiddleware_Handler_SetResultOverridesStatusCode(t *testing.T) {
	sink := &recordingSink{}
	policies, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: true}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	emitter, err := NewEmitter("test-service", policies, sink)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	op := &Operation{ID: testProjectOpID, Action: "create_project", ResourceType: "project", Category: CategoryManagement}
	patternMap := map[string]*Operation{testProjectPattern: op}
	mw := newMiddleware(slog.Default(), patternMap, emitter, true)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulates a hijack: the wrapped ResponseWriter never sees a
		// non-200 status, but the handler knows the real outcome failed.
		SetResult(r.Context(), ResultFailure)
	})

	req := httptest.NewRequest(http.MethodPost, "/projects", nil)
	req.Pattern = testProjectPattern
	rw := httptest.NewRecorder()

	mw.Handler(next).ServeHTTP(rw, req)

	if len(sink.events) != 1 {
		t.Fatalf("expected exactly one audit event, got %d", len(sink.events))
	}
	if sink.events[0].Result != ResultFailure {
		t.Errorf("Result = %v, want failure (SetResult must override the unchanged 200 status code)",
			sink.events[0].Result)
	}
	if rw.Code != http.StatusOK {
		t.Errorf("recorded status = %d, want 200 (responseWriter must not alter the real response)", rw.Code)
	}
}

// flushRecorder wraps httptest.ResponseRecorder to additionally implement
// http.Flusher, so this test can prove http.ResponseController reaches
// through responseWriter to it — httptest.ResponseRecorder already has a
// Flush method, but it isn't itself asserted against http.Flusher anywhere
// this test relies on, so wrapping it here keeps the intent explicit.
type flushRecorder struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (f *flushRecorder) Flush() {
	f.flushed = true
	f.ResponseRecorder.Flush()
}

// TestResponseWriter_UnwrapReachesFlusher guards against responseWriter
// hiding optional interfaces (Flusher, Hijacker, the deadline setters) that
// the real ResponseWriter implements — without Unwrap, http.ResponseController
// can't see past this wrapper to reach them.
func TestResponseWriter_UnwrapReachesFlusher(t *testing.T) {
	rec := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	rw := &responseWriter{ResponseWriter: rec}

	if err := (http.NewResponseController(rw)).Flush(); err != nil {
		t.Fatalf("ResponseController.Flush() through responseWriter failed: %v", err)
	}
	if !rec.flushed {
		t.Error("expected the underlying ResponseWriter's Flush to be called via Unwrap")
	}
}

func TestNewMiddleware_Success(t *testing.T) {
	ops := []Operation{{ID: testProjectOpID, Action: "create_project", ResourceType: "project", Category: CategoryManagement}}
	policies, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: true}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	emitter, err := NewEmitter("test-service", policies, &recordingSink{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mw, err := NewMiddleware(slog.Default(), ops, func() (*openapi3.T, error) { return loadTestSpec(t), nil }, emitter, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mw == nil {
		t.Fatal("NewMiddleware returned a nil Middleware with no error")
	}
}

func TestNewMiddleware_GetSwaggerError(t *testing.T) {
	policies, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: true}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	emitter, err := NewEmitter("test-service", policies, &recordingSink{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantErr := errors.New("spec load failed")

	_, err = NewMiddleware(slog.Default(), nil, func() (*openapi3.T, error) { return nil, wantErr }, emitter, true)
	if err == nil {
		t.Fatal("expected an error when getSwagger fails, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want it to wrap %v", err, wantErr)
	}
}

func TestNewMiddleware_BuildPatternMapError(t *testing.T) {
	// An operationId with no matching route makes BuildPatternMap fail;
	// NewMiddleware must propagate that error rather than constructing a
	// Middleware with an incomplete pattern map.
	ops := []Operation{{ID: "CreateWidget", Action: "create_widget", ResourceType: "widgets", Category: CategoryManagement}}
	policies, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: true}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	emitter, err := NewEmitter("test-service", policies, &recordingSink{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = NewMiddleware(slog.Default(), ops, func() (*openapi3.T, error) { return loadTestSpec(t), nil }, emitter, true)
	if err == nil {
		t.Fatal("expected a BuildPatternMap error to propagate, got nil")
	}
}

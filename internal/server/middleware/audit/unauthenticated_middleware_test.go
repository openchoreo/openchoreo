// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

func newTestUnauthenticatedEmitter(t *testing.T, sink *recordingSink) *Emitter {
	t.Helper()
	policies, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: true}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	emitter, err := NewEmitter("test-service", policies, sink)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return emitter
}

// fakeAuth mimics the real JWT middleware's shape (jwt/middleware.go): on
// success it attaches the subject to a *derived* request via r.WithContext
// and calls next with that; on rejection it writes 401 and never calls
// next. Precisely this shape broke an earlier version of
// NewUnauthenticatedMiddleware, which re-read auth.GetSubjectContextFromContext
// off the request it originally received — a derived request built by a
// downstream WithContext call never propagates back to a variable an
// enclosing closure already captured, so that read always saw no subject
// regardless of whether auth succeeded. Tests must go through something
// shaped like this, not pre-seed the subject directly on the request the
// outer middleware receives, or they don't exercise the bug at all.
func fakeAuth(authenticated bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !authenticated {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			ctx := auth.SetSubjectContext(r.Context(), &auth.SubjectContext{ID: "u1", Type: "user"})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// chain composes outer(fakeAuth(inner(handler))) — exactly the production
// ordering (see APIMiddlewares: logger -> unauthenticatedAudit -> auth ->
// audit -> handler, with logger irrelevant to this package's tests).
func chain(outer, auth func(http.Handler) http.Handler, inner *Middleware, handler http.Handler) http.Handler {
	return outer(auth(inner.Handler(handler)))
}

func TestUnauthenticatedMiddleware_401NoSubjectEmits(t *testing.T) {
	sink := &recordingSink{}
	emitter := newTestUnauthenticatedEmitter(t, sink)
	outer := NewUnauthenticatedMiddleware(emitter, true)
	inner := newMiddleware(slog.Default(), map[string]*Operation{}, emitter, true)

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK) // unreachable: fakeAuth(false) rejects first
	})

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	rec := httptest.NewRecorder()
	chain(outer, fakeAuth(false), inner, handler).ServeHTTP(rec, req)

	if len(sink.events) != 1 {
		t.Fatalf("len(sink.events) = %d, want 1", len(sink.events))
	}
	event := sink.events[0]
	if event.Result != ResultUnauthenticated {
		t.Errorf("Result = %q, want %q", event.Result, ResultUnauthenticated)
	}
	if event.OperationID != "" || event.Action != "" || event.Category != "" {
		t.Errorf("event carries operation fields, want a nil-operation event: %+v", event)
	}
}

// TestUnauthenticatedMiddleware_AuthenticatedRequestEmitsExactlyOnce is the
// direct regression test for the double-emission bug: a real
// derived-request auth success reaching a resolved operation must produce
// exactly one event, from the inner Middleware, with the outer staying
// silent. Covers every status the inner might record, including 401 for a
// reason unrelated to authentication (the exact scenario that used to
// double-count, since the outer's old subject check couldn't see the
// derived request and fired again).
func TestUnauthenticatedMiddleware_AuthenticatedRequestEmitsExactlyOnce(t *testing.T) {
	for _, code := range []int{http.StatusOK, http.StatusForbidden, http.StatusUnauthorized, http.StatusInternalServerError} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			sink := &recordingSink{}
			emitter := newTestUnauthenticatedEmitter(t, sink)
			outer := NewUnauthenticatedMiddleware(emitter, true)
			op := &Operation{ID: testProjectOpID, Action: "create_project", ResourceType: "project", Category: CategoryManagement}
			inner := newMiddleware(slog.Default(), map[string]*Operation{testProjectPattern: op}, emitter, true)

			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
			})

			req := httptest.NewRequest(http.MethodPost, "/projects", nil)
			req.Pattern = testProjectPattern
			rec := httptest.NewRecorder()
			chain(outer, fakeAuth(true), inner, handler).ServeHTTP(rec, req)

			if len(sink.events) != 1 {
				t.Fatalf("len(sink.events) = %d, want exactly 1 (double-emission regression)", len(sink.events))
			}
			if sink.events[0].OperationID != testProjectOpID {
				t.Errorf("the inner Middleware's event should have recorded, got OperationID = %q", sink.events[0].OperationID)
			}
		})
	}
}

func TestUnauthenticatedMiddleware_NonAuthFailuresDoNotEmit(t *testing.T) {
	tests := []int{http.StatusOK, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError}
	for _, code := range tests {
		sink := &recordingSink{}
		emitter := newTestUnauthenticatedEmitter(t, sink)
		mw := NewUnauthenticatedMiddleware(emitter, true)

		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
		})

		req := httptest.NewRequest(http.MethodGet, "/anything", nil)
		rec := httptest.NewRecorder()
		mw(next).ServeHTTP(rec, req)

		if len(sink.events) != 0 {
			t.Errorf("status %d with no inner Middleware ever running: len(sink.events) = %d, want 0", code, len(sink.events))
		}
	}
}

func TestUnauthenticatedMiddleware_Disabled(t *testing.T) {
	sink := &recordingSink{}
	emitter := newTestUnauthenticatedEmitter(t, sink)
	mw := NewUnauthenticatedMiddleware(emitter, false)

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)

	if len(sink.events) != 0 {
		t.Fatalf("disabled: len(sink.events) = %d, want 0", len(sink.events))
	}
}

// TestUnauthenticatedMiddleware_PanicNoAuthEmitsFailureAndRepanics covers a
// panic before auth ever runs next (e.g. a bug in auth itself), which the
// inner Middleware never gets a chance to observe.
func TestUnauthenticatedMiddleware_PanicNoAuthEmitsFailureAndRepanics(t *testing.T) {
	sink := &recordingSink{}
	emitter := newTestUnauthenticatedEmitter(t, sink)
	mw := NewUnauthenticatedMiddleware(emitter, true)

	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	rec := httptest.NewRecorder()

	defer func() {
		p := recover()
		if p == nil {
			t.Fatal("expected the panic to propagate (re-panic), but it did not")
		}
		if len(sink.events) != 1 {
			t.Fatalf("len(sink.events) = %d, want 1", len(sink.events))
		}
		if sink.events[0].Result != ResultFailure {
			t.Errorf("Result = %q, want %q", sink.events[0].Result, ResultFailure)
		}
	}()
	mw(next).ServeHTTP(rec, req)
}

// TestUnauthenticatedMiddleware_PanicAfterAuthEmitsExactlyOnce is the panic
// counterpart to the double-emission regression test above: a panic in a
// handler reached via real derived-request auth success must be recorded
// once, by the inner Middleware, and re-propagate — not recorded again by
// the outer.
func TestUnauthenticatedMiddleware_PanicAfterAuthEmitsExactlyOnce(t *testing.T) {
	sink := &recordingSink{}
	emitter := newTestUnauthenticatedEmitter(t, sink)
	outer := NewUnauthenticatedMiddleware(emitter, true)
	op := &Operation{ID: testProjectOpID, Action: "create_project", ResourceType: "project", Category: CategoryManagement}
	inner := newMiddleware(slog.Default(), map[string]*Operation{testProjectPattern: op}, emitter, true)

	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodPost, "/projects", nil)
	req.Pattern = testProjectPattern
	rec := httptest.NewRecorder()

	defer func() {
		p := recover()
		if p == nil {
			t.Fatal("expected the panic to propagate (re-panic), but it did not")
		}
		if len(sink.events) != 1 {
			t.Fatalf("len(sink.events) = %d, want exactly 1 (double-emission regression)", len(sink.events))
		}
		if sink.events[0].Result != ResultFailure || sink.events[0].OperationID != testProjectOpID {
			t.Errorf("expected the inner Middleware's failure event, got %+v", sink.events[0])
		}
	}()
	chain(outer, fakeAuth(true), inner, handler).ServeHTTP(rec, req)
}

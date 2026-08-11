// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
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
			if len(actor.Entitlements) != len(tt.wantEntitlements) {
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
	emitter := NewEmitter("test-service", policies, sink)
	op := &Operation{ID: "CreateProject", Action: "create_project", ResourceType: "projects", Category: CategoryManagement}
	patternMap := map[string]*Operation{"POST /projects": op}
	mw := newMiddleware(slog.Default(), patternMap, emitter, true)

	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("handler blew up after mutating state")
	})

	req := httptest.NewRequest(http.MethodPost, "/projects", nil)
	req.Pattern = "POST /projects"
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

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	authzmocks "github.com/openchoreo/openchoreo/internal/authz/core/mocks"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// wirelogsRequest builds an HTTP request that mimics what the JWT middleware
// would hand the handler: a subject is attached to the request context so the
// AuthzChecker can resolve it.
func wirelogsRequest(t *testing.T, path string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	ctx := auth.SetSubjectContext(req.Context(), &auth.SubjectContext{
		ID:                "user-1",
		Type:              "user",
		EntitlementClaim:  "groups",
		EntitlementValues: []string{"viewers"},
	})
	return req.WithContext(ctx)
}

func TestWirelogsHandler_RejectsMalformedPath(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"missing namespaces segment", "/wirelogs/checkout"},
		{"missing projects segment", "/wirelogs/namespaces/ns-a/components/checkout"},
		{"missing components segment", "/wirelogs/namespaces/ns-a/projects/demo"},
		{"empty namespace", "/wirelogs/namespaces//projects/demo/components/checkout"},
		{"empty project", "/wirelogs/namespaces/ns-a/projects//components/checkout"},
		{"empty component", "/wirelogs/namespaces/ns-a/projects/demo/components/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// authzChecker is set so the handler can't short-circuit there;
			// URL parsing runs before authz so this path must fail with 400.
			pdp := authzmocks.NewMockPDP(t) // no expectations: panics if called
			h := &WirelogsHandler{
				authzChecker: svcpkg.NewAuthzChecker(pdp, slog.Default()),
				logger:       slog.Default(),
			}

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, wirelogsRequest(t, tt.path))

			assert.Equal(t, http.StatusBadRequest, rec.Code, "malformed URL must produce 400")
		})
	}
}

func TestWirelogsHandler_AuthzNotConfigured(t *testing.T) {
	h := &WirelogsHandler{logger: slog.Default()} // authzChecker == nil

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t, "/wirelogs/namespaces/ns-a/projects/demo/components/checkout?environment=development"))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "authorization not configured")
}

func TestWirelogsHandler_Forbidden(t *testing.T) {
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Return(&authz.Decision{Decision: false, Context: &authz.DecisionContext{Reason: "no logs:view"}}, nil)

	h := &WirelogsHandler{
		authzChecker: svcpkg.NewAuthzChecker(pdp, slog.Default()),
		logger:       slog.Default(),
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t, "/wirelogs/namespaces/ns-a/projects/demo/components/checkout?environment=development"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestWirelogsHandler_PassesActionViewLogs(t *testing.T) {
	// Verify the handler asks the PDP about logs:view at the component scope.
	// Capture the request via RunAndReturn so we can assert its shape.
	var captured *authz.EvaluateRequest

	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, req *authz.EvaluateRequest) (*authz.Decision, error) {
			require.NotNil(t, req)
			captured = req
			// Deny so the handler returns 403 before attempting to dial the gateway.
			return &authz.Decision{Decision: false, Context: &authz.DecisionContext{}}, nil
		})

	h := &WirelogsHandler{
		authzChecker: svcpkg.NewAuthzChecker(pdp, slog.Default()),
		logger:       slog.Default(),
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t, "/wirelogs/namespaces/ns-a/projects/demo/components/checkout?environment=development"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	require.NotNil(t, captured)
	assert.Equal(t, authz.ActionViewLogs, captured.Action, "wirelogs must reuse the logs:view permission")
	assert.Equal(t, "component", captured.Resource.Type)
	assert.Equal(t, "checkout", captured.Resource.ID)
	assert.Equal(t, "ns-a", captured.Resource.Hierarchy.Namespace)
	assert.Equal(t, "demo", captured.Resource.Hierarchy.Project, "project must come from the URL path")
}

func TestBuildGatewayWirelogsURL(t *testing.T) {
	h := &WirelogsHandler{gatewayURL: "https://gw.example.com:8443"}

	got, err := h.buildGatewayWirelogsURL(execPlaneInfo{
		planeType:   "dataplane",
		planeID:     "prod-cluster",
		crNamespace: "team-a",
		crName:      "prod-dp",
	}, "checkout", "shopfront", "development", "ns-a")
	require.NoError(t, err)

	assert.Contains(t, got, "https://gw.example.com:8443/api/wirelogs/dataplane/prod-cluster/team-a/prod-dp")
	assert.Contains(t, got, "component=checkout")
	assert.Contains(t, got, "project=shopfront")
	assert.Contains(t, got, "environment=development")
	assert.Contains(t, got, "namespace=ns-a")
}

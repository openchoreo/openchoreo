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

// Mimics the JWT middleware by attaching a subject the AuthzChecker can resolve.
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
		{"missing namespaces segment", "/environments/development/wirelogs"},
		{"missing environments segment", "/namespaces/ns-a/wirelogs"},
		{"wrong final segment", "/namespaces/ns-a/environments/development/foo"},
		{"empty namespace", "/namespaces//environments/development/wirelogs"},
		{"empty environment", "/namespaces/ns-a/environments//wirelogs"},
		{"extra trailing path", "/namespaces/ns-a/environments/development/wirelogs/extra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Path parsing runs before authz, so a configured PDP with no
			// expectations doubles as a guard that authz isn't reached.
			pdp := authzmocks.NewMockPDP(t)
			h := &WirelogsHandler{
				authzChecker: svcpkg.NewAuthzChecker(pdp, slog.Default()),
				logger:       slog.Default(),
			}

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, wirelogsRequest(t, tt.path))

			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestWirelogsHandler_AcceptsEnvironmentWideRequest(t *testing.T) {
	// No project/component query params -> entire environment scope.
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Return(&authz.Decision{Decision: false, Context: &authz.DecisionContext{}}, nil)

	h := &WirelogsHandler{
		authzChecker: svcpkg.NewAuthzChecker(pdp, slog.Default()),
		logger:       slog.Default(),
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t, "/namespaces/ns-a/environments/development/wirelogs"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestWirelogsHandler_AuthzNotConfigured(t *testing.T) {
	h := &WirelogsHandler{logger: slog.Default()}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t, "/namespaces/ns-a/environments/development/wirelogs"))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "authorization not configured")
}

func TestWirelogsHandler_Forbidden(t *testing.T) {
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Return(&authz.Decision{Decision: false, Context: &authz.DecisionContext{Reason: "no wirelogs:view"}}, nil)

	h := &WirelogsHandler{
		authzChecker: svcpkg.NewAuthzChecker(pdp, slog.Default()),
		logger:       slog.Default(),
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t, "/namespaces/ns-a/environments/development/wirelogs?project=demo&component=checkout"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// captureAuthzRequest runs ServeHTTP with a PDP that records the EvaluateRequest
// and denies, so the handler returns 403 before doing any data-plane lookups.
func captureAuthzRequest(t *testing.T, path string) *authz.EvaluateRequest {
	t.Helper()

	var captured *authz.EvaluateRequest
	pdp := authzmocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, req *authz.EvaluateRequest) (*authz.Decision, error) {
			require.NotNil(t, req)
			captured = req
			return &authz.Decision{Decision: false, Context: &authz.DecisionContext{}}, nil
		})

	h := &WirelogsHandler{
		authzChecker: svcpkg.NewAuthzChecker(pdp, slog.Default()),
		logger:       slog.Default(),
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t, path))
	assert.Equal(t, http.StatusForbidden, rec.Code)

	require.NotNil(t, captured)
	return captured
}

func TestWirelogsHandler_AuthzScope_Component(t *testing.T) {
	req := captureAuthzRequest(t, "/namespaces/ns-a/environments/development/wirelogs?project=demo&component=checkout")

	assert.Equal(t, authz.ActionViewWirelogs, req.Action, "wirelogs must check its own action, not logs:view")
	assert.Equal(t, "component", req.Resource.Type)
	assert.Equal(t, "checkout", req.Resource.ID)
	assert.Equal(t, "ns-a", req.Resource.Hierarchy.Namespace)
	assert.Equal(t, "demo", req.Resource.Hierarchy.Project)
	assert.Equal(t, "checkout", req.Resource.Hierarchy.Component)
	assert.Equal(t, "ns-a/development", req.Context.Resource.Environment,
		"resource.environment must always be set so CEL conditions can scope per env")
}

func TestWirelogsHandler_AuthzScope_ProjectOnly(t *testing.T) {
	req := captureAuthzRequest(t, "/namespaces/ns-a/environments/development/wirelogs?project=demo")

	assert.Equal(t, "project", req.Resource.Type)
	assert.Equal(t, "demo", req.Resource.ID)
	assert.Equal(t, "ns-a", req.Resource.Hierarchy.Namespace)
	assert.Equal(t, "demo", req.Resource.Hierarchy.Project)
	assert.Empty(t, req.Resource.Hierarchy.Component)
}

func TestWirelogsHandler_ComponentWithoutProjectRejected(t *testing.T) {
	pdp := authzmocks.NewMockPDP(t)
	h := &WirelogsHandler{
		authzChecker: svcpkg.NewAuthzChecker(pdp, slog.Default()),
		logger:       slog.Default(),
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, wirelogsRequest(t, "/namespaces/ns-a/environments/development/wirelogs?component=checkout"))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "component filter requires project filter")
}

func TestWirelogsHandler_AuthzScope_EnvironmentWide(t *testing.T) {
	req := captureAuthzRequest(t, "/namespaces/ns-a/environments/development/wirelogs")

	assert.Equal(t, "environment", req.Resource.Type)
	assert.Equal(t, "development", req.Resource.ID)
	assert.Equal(t, "ns-a", req.Resource.Hierarchy.Namespace)
	assert.Empty(t, req.Resource.Hierarchy.Project)
	assert.Empty(t, req.Resource.Hierarchy.Component)
	assert.Equal(t, "ns-a/development", req.Context.Resource.Environment)
}

func TestBuildGatewayWirelogsURL_AllFilters(t *testing.T) {
	h := &WirelogsHandler{gatewayURL: "https://gw.example.com:8443"}

	got, err := h.buildGatewayWirelogsURL(execPlaneInfo{
		planeType:   "dataplane",
		planeID:     "prod-cluster",
		crNamespace: "team-a",
		crName:      "prod-dp",
	}, "ns-a", "development", "shopfront", "checkout")
	require.NoError(t, err)

	assert.Contains(t, got, "https://gw.example.com:8443/api/wirelogs/dataplane/prod-cluster/team-a/prod-dp")
	assert.Contains(t, got, "namespace=ns-a")
	assert.Contains(t, got, "environment=development")
	assert.Contains(t, got, "project=shopfront")
	assert.Contains(t, got, "component=checkout")
}

func TestBuildGatewayWirelogsURL_OmitsBlankFilters(t *testing.T) {
	h := &WirelogsHandler{gatewayURL: "https://gw.example.com:8443"}

	got, err := h.buildGatewayWirelogsURL(execPlaneInfo{
		planeType:   "dataplane",
		planeID:     "prod-cluster",
		crNamespace: "team-a",
		crName:      "prod-dp",
	}, "ns-a", "development", "", "")
	require.NoError(t, err)

	assert.Contains(t, got, "namespace=ns-a")
	assert.Contains(t, got, "environment=development")
	assert.NotContains(t, got, "project=")
	assert.NotContains(t, got, "component=")
}

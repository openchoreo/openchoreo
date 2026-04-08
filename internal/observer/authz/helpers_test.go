// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	coremocks "github.com/openchoreo/openchoreo/internal/authz/core/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// noopLogger returns a logger that discards all output, suitable for tests.
func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// ─────────────────────── ComponentScopeAuthz ───────────────────────

func TestComponentScopeAuthz(t *testing.T) {
	tests := []struct {
		name             string
		namespace        string
		project          string
		component        string
		wantResourceType ResourceType
		wantResourceName string
		wantHierarchy    authzcore.ResourceHierarchy
	}{
		{
			name:             "all empty returns unknown",
			wantResourceType: ResourceTypeUnknown,
			wantResourceName: "",
			wantHierarchy:    authzcore.ResourceHierarchy{},
		},
		{
			name:             "namespace only returns namespace type",
			namespace:        "acme",
			wantResourceType: ResourceTypeNamespace,
			wantResourceName: "acme",
			wantHierarchy:    authzcore.ResourceHierarchy{Namespace: "acme"},
		},
		{
			name:             "namespace and project returns project type",
			namespace:        "acme",
			project:          "payments",
			wantResourceType: ResourceTypeProject,
			wantResourceName: "payments",
			wantHierarchy:    authzcore.ResourceHierarchy{Namespace: "acme", Project: "payments"},
		},
		{
			name:             "namespace project and component returns component type",
			namespace:        "acme",
			project:          "payments",
			component:        "api",
			wantResourceType: ResourceTypeComponent,
			wantResourceName: "api",
			wantHierarchy:    authzcore.ResourceHierarchy{Namespace: "acme", Project: "payments", Component: "api"},
		},
		{
			name:             "component without namespace or project returns component type",
			component:        "orphan",
			wantResourceType: ResourceTypeComponent,
			wantResourceName: "orphan",
			wantHierarchy:    authzcore.ResourceHierarchy{Component: "orphan"},
		},
		{
			name:             "component with namespace but no project returns component type",
			namespace:        "acme",
			component:        "api",
			wantResourceType: ResourceTypeComponent,
			wantResourceName: "api",
			wantHierarchy:    authzcore.ResourceHierarchy{Namespace: "acme", Component: "api"},
		},
		{
			name:             "project without namespace returns project type",
			project:          "payments",
			wantResourceType: ResourceTypeProject,
			wantResourceName: "payments",
			wantHierarchy:    authzcore.ResourceHierarchy{Project: "payments"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotName, gotHierarchy := ComponentScopeAuthz(tt.namespace, tt.project, tt.component)
			assert.Equal(t, tt.wantResourceType, gotType)
			assert.Equal(t, tt.wantResourceName, gotName)
			assert.Equal(t, tt.wantHierarchy, gotHierarchy)
		})
	}
}

// ─────────────────────── LogsScopeAuthz ───────────────────────

func TestLogsScopeAuthz(t *testing.T) {
	tests := []struct {
		name             string
		req              *types.LogsQueryRequest
		wantResourceType ResourceType
		wantResourceName string
		wantHierarchy    authzcore.ResourceHierarchy
		wantErr          bool
		wantErrMsg       string
	}{
		{
			name:       "nil request returns error",
			req:        nil,
			wantErr:    true,
			wantErrMsg: "request is required",
		},
		{
			name:       "nil search scope returns error",
			req:        &types.LogsQueryRequest{SearchScope: nil},
			wantErr:    true,
			wantErrMsg: "search scope is required",
		},
		{
			name: "neither component nor workflow scope set returns error",
			req: &types.LogsQueryRequest{
				SearchScope: &types.SearchScope{},
			},
			wantErr:    true,
			wantErrMsg: "invalid search scope",
		},
		{
			name: "component scope with namespace only returns namespace type",
			req: &types.LogsQueryRequest{
				SearchScope: &types.SearchScope{
					Component: &types.ComponentSearchScope{Namespace: "acme"},
				},
			},
			wantResourceType: ResourceTypeNamespace,
			wantResourceName: "acme",
			wantHierarchy:    authzcore.ResourceHierarchy{Namespace: "acme"},
		},
		{
			name: "component scope with namespace and project returns project type",
			req: &types.LogsQueryRequest{
				SearchScope: &types.SearchScope{
					Component: &types.ComponentSearchScope{Namespace: "acme", Project: "payments"},
				},
			},
			wantResourceType: ResourceTypeProject,
			wantResourceName: "payments",
			wantHierarchy:    authzcore.ResourceHierarchy{Namespace: "acme", Project: "payments"},
		},
		{
			name: "component scope with all fields returns component type",
			req: &types.LogsQueryRequest{
				SearchScope: &types.SearchScope{
					Component: &types.ComponentSearchScope{
						Namespace: "acme", Project: "payments", Component: "api",
					},
				},
			},
			wantResourceType: ResourceTypeComponent,
			wantResourceName: "api",
			wantHierarchy:    authzcore.ResourceHierarchy{Namespace: "acme", Project: "payments", Component: "api"},
		},
		{
			name: "workflow scope with workflowRunName returns workflowRun type",
			req: &types.LogsQueryRequest{
				SearchScope: &types.SearchScope{
					Workflow: &types.WorkflowSearchScope{
						Namespace: "acme", WorkflowRunName: "run-abc",
					},
				},
			},
			wantResourceType: ResourceTypeWorkflowRun,
			wantResourceName: "run-abc",
			wantHierarchy:    authzcore.ResourceHierarchy{Namespace: "acme"},
		},
		{
			name: "workflow scope without workflowRunName returns namespace type",
			req: &types.LogsQueryRequest{
				SearchScope: &types.SearchScope{
					Workflow: &types.WorkflowSearchScope{Namespace: "acme"},
				},
			},
			wantResourceType: ResourceTypeNamespace,
			wantResourceName: "acme",
			wantHierarchy:    authzcore.ResourceHierarchy{Namespace: "acme"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotName, gotHierarchy, err := LogsScopeAuthz(tt.req)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantResourceType, gotType)
			assert.Equal(t, tt.wantResourceName, gotName)
			assert.Equal(t, tt.wantHierarchy, gotHierarchy)
		})
	}
}

// ─────────────────────── CheckAuthorization ───────────────────────

func newSubjectContext() *auth.SubjectContext {
	return &auth.SubjectContext{
		ID:                "user-123",
		Type:              "user",
		EntitlementClaim:  "groups",
		EntitlementValues: []string{"dev-team"},
	}
}

func ctxWithSubject() context.Context {
	return auth.SetSubjectContext(context.Background(), newSubjectContext())
}

func TestCheckAuthorization_PDPNil(t *testing.T) {
	err := CheckAuthorization(
		context.Background(),
		noopLogger(),
		nil,
		ActionViewLogs,
		ResourceTypeComponent,
		"api",
		authzcore.ResourceHierarchy{Namespace: "acme"},
	)
	assert.NoError(t, err, "nil PDP should skip authorization")
}

func TestCheckAuthorization_NoSubjectContext(t *testing.T) {
	mockPDP := coremocks.NewMockPDP(t)

	err := CheckAuthorization(
		context.Background(), // no subject context set
		noopLogger(),
		mockPDP,
		ActionViewLogs,
		ResourceTypeComponent,
		"api",
		authzcore.ResourceHierarchy{},
	)
	assert.ErrorIs(t, err, ErrAuthzUnauthorized)
}

func TestCheckAuthorization_PDPEvaluateError(t *testing.T) {
	upstreamErr := fmt.Errorf("upstream failure")
	mockPDP := coremocks.NewMockPDP(t)
	mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(nil, upstreamErr)

	err := CheckAuthorization(
		ctxWithSubject(),
		noopLogger(),
		mockPDP,
		ActionViewLogs,
		ResourceTypeComponent,
		"api",
		authzcore.ResourceHierarchy{Namespace: "acme"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upstream failure")
}

func TestCheckAuthorization_DecisionAllowed(t *testing.T) {
	mockPDP := coremocks.NewMockPDP(t)
	mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Return(&authzcore.Decision{Decision: true}, nil)

	err := CheckAuthorization(
		ctxWithSubject(),
		noopLogger(),
		mockPDP,
		ActionViewLogs,
		ResourceTypeComponent,
		"api",
		authzcore.ResourceHierarchy{Namespace: "acme"},
	)
	assert.NoError(t, err)
}

func TestCheckAuthorization_DecisionDenied(t *testing.T) {
	mockPDP := coremocks.NewMockPDP(t)
	mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Return(&authzcore.Decision{Decision: false}, nil)

	err := CheckAuthorization(
		ctxWithSubject(),
		noopLogger(),
		mockPDP,
		ActionViewLogs,
		ResourceTypeComponent,
		"api",
		authzcore.ResourceHierarchy{Namespace: "acme"},
	)
	assert.ErrorIs(t, err, ErrAuthzForbidden)
}

func TestCheckAuthorization_BuildsCorrectRequest(t *testing.T) {
	var capturedReq *authzcore.EvaluateRequest
	mockPDP := coremocks.NewMockPDP(t)
	mockPDP.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Run(func(_ context.Context, req *authzcore.EvaluateRequest) { capturedReq = req }).
		Return(&authzcore.Decision{Decision: true}, nil)

	hierarchy := authzcore.ResourceHierarchy{Namespace: "acme", Project: "payments", Component: "api"}
	err := CheckAuthorization(
		ctxWithSubject(),
		noopLogger(),
		mockPDP,
		ActionViewLogs,
		ResourceTypeComponent,
		"api",
		hierarchy,
	)
	require.NoError(t, err)
	require.NotNil(t, capturedReq)
	assert.Equal(t, string(ActionViewLogs), capturedReq.Action)
	assert.Equal(t, string(ResourceTypeComponent), capturedReq.Resource.Type)
	assert.Equal(t, "api", capturedReq.Resource.ID)
	assert.Equal(t, hierarchy, capturedReq.Resource.Hierarchy)
	require.NotNil(t, capturedReq.SubjectContext)
	assert.Equal(t, "user", capturedReq.SubjectContext.Type)
	assert.Equal(t, "groups", capturedReq.SubjectContext.EntitlementClaim)
	assert.Equal(t, []string{"dev-team"}, capturedReq.SubjectContext.EntitlementValues)
}

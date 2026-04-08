// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	authzsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/authz"
	authzmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/authz/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

func newHandlerWithAuthzService(t *testing.T, svc authzsvc.Service, cfg *config.Config) *Handler {
	t.Helper()
	return &Handler{
		services: &handlerservices.Services{AuthzService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Config:   cfg,
	}
}

// --- Evaluation & Profile ---

func TestListActionsHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().ListActions(mock.Anything).Return([]authzcore.Action{
			{Name: "view", LowestScope: "namespace"},
			{Name: "create", LowestScope: "cluster"},
		}, nil)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.ListActions(ctx, gen.ListActionsRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListActions200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed, 2)
		assert.Equal(t, "view", typed[0].Name)
		assert.Equal(t, gen.ActionInfoLowestScope("namespace"), typed[0].LowestScope)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().ListActions(mock.Anything).Return(nil, svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.ListActions(ctx, gen.ListActionsRequestObject{})
		require.NoError(t, err)
		assert.IsType(t, gen.ListActions403JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().ListActions(mock.Anything).Return(nil, errors.New("internal server error"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.ListActions(ctx, gen.ListActionsRequestObject{})
		require.NoError(t, err)
		assert.IsType(t, gen.ListActions500JSONResponse{}, resp)
	})
}

func TestEvaluatesHandler(t *testing.T) {
	ctx := testContext()

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.Evaluates(ctx, gen.EvaluatesRequestObject{Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.Evaluates400JSONResponse{}, resp)
	})

	t.Run("invalid request error returns 400", func(t *testing.T) {
		body := []gen.EvaluateRequest{{
			Action: "view",
			Resource: gen.Resource{
				Type:      "project",
				Id:        nil,
				Hierarchy: gen.ResourceHierarchy{},
			},
			SubjectContext: gen.SubjectContext{
				Type:              gen.SubjectContextType("user"),
				EntitlementClaim:  "",
				EntitlementValues: nil,
			},
		}}
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().Evaluate(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, reqs []authzcore.EvaluateRequest) ([]authzcore.Decision, error) {
			require.Len(t, reqs, 1)
			assert.Equal(t, "view", reqs[0].Action)
			assert.Equal(t, "project", reqs[0].Resource.Type)
			assert.Equal(t, "", reqs[0].Resource.ID, "nil pointer must convert to empty string")
			return nil, authzcore.ErrInvalidRequest
		})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.Evaluates(ctx, gen.EvaluatesRequestObject{Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.Evaluates400JSONResponse{}, resp)
	})

	t.Run("success converts decisions and reason context", func(t *testing.T) {
		ns := "acme"
		id := "proj-1"
		reason := "policy allows"
		body := []gen.EvaluateRequest{
			{
				Action: "view",
				Resource: gen.Resource{
					Type: "project",
					Id:   &id,
					Hierarchy: gen.ResourceHierarchy{
						Namespace: &ns,
					},
				},
				SubjectContext: gen.SubjectContext{
					Type:              gen.SubjectContextType("user"),
					EntitlementClaim:  "groups",
					EntitlementValues: []string{"admin"},
				},
			},
			{
				Action: "delete",
				Resource: gen.Resource{
					Type: "component",
					Hierarchy: gen.ResourceHierarchy{
						Namespace: &ns,
						Project:   &id,
					},
				},
				SubjectContext: gen.SubjectContext{
					Type:              gen.SubjectContextType("user"),
					EntitlementClaim:  "",
					EntitlementValues: nil,
				},
			},
		}

		svc := authzmocks.NewMockService(t)
		svc.EXPECT().Evaluate(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, reqs []authzcore.EvaluateRequest) ([]authzcore.Decision, error) {
			require.Len(t, reqs, 2)
			assert.Equal(t, id, reqs[0].Resource.ID)
			assert.Equal(t, ns, reqs[0].Resource.Hierarchy.Namespace)
			require.NotNil(t, reqs[0].SubjectContext)
			assert.Equal(t, "user", reqs[0].SubjectContext.Type)
			assert.Equal(t, "groups", reqs[0].SubjectContext.EntitlementClaim)
			assert.Equal(t, []string{"admin"}, reqs[0].SubjectContext.EntitlementValues)

			return []authzcore.Decision{
				{Decision: true, Context: &authzcore.DecisionContext{Reason: reason}},
				{Decision: false, Context: &authzcore.DecisionContext{}},
			}, nil
		})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.Evaluates(ctx, gen.EvaluatesRequestObject{Body: &body})
		require.NoError(t, err)
		typed, ok := resp.(gen.Evaluates200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed, 2)
		assert.True(t, typed[0].Decision)
		require.NotNil(t, typed[0].Context)
		require.NotNil(t, typed[0].Context.Reason)
		assert.Equal(t, reason, *typed[0].Context.Reason)

		assert.False(t, typed[1].Decision)
		assert.Nil(t, typed[1].Context, "empty reason must omit context")
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		body := []gen.EvaluateRequest{{
			Action:         "view",
			Resource:       gen.Resource{Type: "project"},
			SubjectContext: gen.SubjectContext{Type: gen.SubjectContextType("user")},
		}}
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(nil, errors.New("unexpected failure"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.Evaluates(ctx, gen.EvaluatesRequestObject{Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.Evaluates500JSONResponse{}, resp)
	})
}

func TestGetSubjectProfileHandler(t *testing.T) {
	cfg := &config.Config{}

	t.Run("missing subject context returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		h := newHandlerWithAuthzService(t, svc, cfg)

		resp, err := h.GetSubjectProfile(context.Background(), gen.GetSubjectProfileRequestObject{})
		require.NoError(t, err)
		assert.IsType(t, gen.GetSubjectProfile403JSONResponse{}, resp)
	})

	t.Run("invalid request returns 400", func(t *testing.T) {
		ctx := testContext()
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetSubjectProfile(mock.Anything, mock.Anything).Return(nil, authzcore.ErrInvalidRequest)
		h := newHandlerWithAuthzService(t, svc, cfg)

		resp, err := h.GetSubjectProfile(ctx, gen.GetSubjectProfileRequestObject{})
		require.NoError(t, err)
		assert.IsType(t, gen.GetSubjectProfile400JSONResponse{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		ctx := testContext()
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetSubjectProfile(mock.Anything, mock.Anything).Return(nil, svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, cfg)

		resp, err := h.GetSubjectProfile(ctx, gen.GetSubjectProfileRequestObject{})
		require.NoError(t, err)
		assert.IsType(t, gen.GetSubjectProfile403JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		ctx := testContext()
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetSubjectProfile(mock.Anything, mock.Anything).Return(nil, errors.New("internal failure"))
		h := newHandlerWithAuthzService(t, svc, cfg)

		resp, err := h.GetSubjectProfile(ctx, gen.GetSubjectProfileRequestObject{})
		require.NoError(t, err)
		assert.IsType(t, gen.GetSubjectProfile500JSONResponse{}, resp)
	})

	t.Run("success converts profile", func(t *testing.T) {
		subCtx := &auth.SubjectContext{ID: "u-1", Type: "user"}
		ctx := auth.SetSubjectContext(context.Background(), subCtx)

		now := time.Now().UTC().Truncate(time.Second)
		constraintsVal := interface{}(map[string]interface{}{"env": "prod"})
		profile := &authzcore.UserCapabilitiesResponse{
			User: &authzcore.SubjectContext{
				Type: "user",
			},
			GeneratedAt: now,
			Capabilities: map[string]*authzcore.ActionCapability{
				"view": {
					Allowed: []*authzcore.CapabilityResource{
						{Path: "namespace/acme", Constraints: &constraintsVal},
					},
					Denied: []*authzcore.CapabilityResource{
						{Path: "namespace/acme/project/secret", Constraints: nil},
					},
				},
			},
		}

		var captured *authzcore.ProfileRequest
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetSubjectProfile(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, req *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
			captured = req
			return profile, nil
		})
		h := newHandlerWithAuthzService(t, svc, cfg)

		ns := "acme"
		resp, err := h.GetSubjectProfile(ctx, gen.GetSubjectProfileRequestObject{
			Params: gen.GetSubjectProfileParams{Namespace: &ns},
		})
		require.NoError(t, err)

		require.NotNil(t, captured)
		assert.Equal(t, "acme", captured.Scope.Namespace)
		require.NotNil(t, captured.SubjectContext)
		assert.Equal(t, "user", captured.SubjectContext.Type)

		typed, ok := resp.(gen.GetSubjectProfile200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.NotNil(t, typed.EvaluatedAt)
		assert.Equal(t, now, *typed.EvaluatedAt)
		require.NotNil(t, typed.Capabilities)

		viewCaps, ok := (*typed.Capabilities)["view"]
		require.True(t, ok)
		require.NotNil(t, viewCaps.Allowed)
		require.Len(t, *viewCaps.Allowed, 1)
		require.NotNil(t, (*viewCaps.Allowed)[0].Constraints)
		assert.Equal(t, "prod", (*(*viewCaps.Allowed)[0].Constraints)["env"])

		require.NotNil(t, viewCaps.Denied)
		require.Len(t, *viewCaps.Denied, 1)
		assert.Nil(t, (*viewCaps.Denied)[0].Constraints)
	})
}

// --- Subject Types ---

func TestListSubjectTypesHandler(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			Subjects: map[string]config.SubjectConfig{
				"user": {
					DisplayName: "User",
					Priority:    2,
					Mechanisms: map[string]config.MechanismConfig{
						"jwt": {Entitlement: config.EntitlementConfig{Claim: "groups", DisplayName: "Groups"}},
					},
				},
				"service": {
					DisplayName: "Service",
					Priority:    1,
					Mechanisms: map[string]config.MechanismConfig{
						"jwt": {Entitlement: config.EntitlementConfig{Claim: "sub", DisplayName: "Subject"}},
					},
				},
			},
		},
	}

	svc := authzmocks.NewMockService(t)
	h := newHandlerWithAuthzService(t, svc, cfg)

	resp, err := h.ListSubjectTypes(testContext(), gen.ListSubjectTypesRequestObject{})
	require.NoError(t, err)

	typed, ok := resp.(gen.ListSubjectTypes200JSONResponse)
	require.True(t, ok, "expected 200 response, got %T", resp)
	require.Len(t, typed, 2)

	// Sorted by priority: service (1) then user (2)
	assert.Equal(t, "service", typed[0].Type)
	assert.Equal(t, "Service", typed[0].DisplayName)
	require.Len(t, typed[0].AuthMechanisms, 1)
	assert.Equal(t, "jwt", typed[0].AuthMechanisms[0].Type)
	assert.Equal(t, "sub", typed[0].AuthMechanisms[0].Entitlement.Claim)

	assert.Equal(t, "user", typed[1].Type)
	assert.Equal(t, "User", typed[1].DisplayName)
	require.Len(t, typed[1].AuthMechanisms, 1)
	assert.Equal(t, "jwt", typed[1].AuthMechanisms[0].Type)
	assert.Equal(t, "groups", typed[1].AuthMechanisms[0].Entitlement.Claim)
}

// --- Cluster Roles ---

func TestListClusterRolesHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success forwards params and converts response", func(t *testing.T) {
		limit := 10
		cursor := "prev-cursor"
		selector := "env=prod"

		var capturedOpts svcpkg.ListOptions
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().ListClusterRoles(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, opts svcpkg.ListOptions) (*svcpkg.ListResult[openchoreov1alpha1.ClusterAuthzRole], error) {
			capturedOpts = opts
			return &svcpkg.ListResult[openchoreov1alpha1.ClusterAuthzRole]{
				Items: []openchoreov1alpha1.ClusterAuthzRole{
					{ObjectMeta: metav1.ObjectMeta{Name: "admin"}, Spec: openchoreov1alpha1.ClusterAuthzRoleSpec{Actions: []string{"*"}, Description: "full access"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "viewer"}, Spec: openchoreov1alpha1.ClusterAuthzRoleSpec{Actions: []string{"view"}}},
				},
				NextCursor: "next-cursor",
			}, nil
		})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.ListClusterRoles(ctx, gen.ListClusterRolesRequestObject{
			Params: gen.ListClusterRolesParams{
				Limit:         &limit,
				Cursor:        &cursor,
				LabelSelector: &selector,
			},
		})
		require.NoError(t, err)

		assert.Equal(t, 10, capturedOpts.Limit)
		assert.Equal(t, "prev-cursor", capturedOpts.Cursor)
		assert.Equal(t, "env=prod", capturedOpts.LabelSelector)

		typed, ok := resp.(gen.ListClusterRoles200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 2)
		assert.Equal(t, "admin", typed.Items[0].Metadata.Name)
		require.NotNil(t, typed.Items[0].Spec)
		assert.Equal(t, []string{"*"}, typed.Items[0].Spec.Actions)
		require.NotNil(t, typed.Items[0].Spec.Description)
		assert.Equal(t, "full access", *typed.Items[0].Spec.Description)
		assert.Equal(t, "viewer", typed.Items[1].Metadata.Name)
		require.NotNil(t, typed.Pagination.NextCursor)
		assert.Equal(t, "next-cursor", *typed.Pagination.NextCursor)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().ListClusterRoles(mock.Anything, mock.Anything).Return(nil, svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.ListClusterRoles(ctx, gen.ListClusterRolesRequestObject{})
		require.NoError(t, err)
		assert.IsType(t, gen.ListClusterRoles403JSONResponse{}, resp)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().ListClusterRoles(mock.Anything, mock.Anything).Return(nil, &svcpkg.ValidationError{Msg: "bad selector"})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.ListClusterRoles(ctx, gen.ListClusterRolesRequestObject{})
		require.NoError(t, err)
		assert.IsType(t, gen.ListClusterRoles400JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().ListClusterRoles(mock.Anything, mock.Anything).Return(nil, errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.ListClusterRoles(ctx, gen.ListClusterRolesRequestObject{})
		require.NoError(t, err)
		assert.IsType(t, gen.ListClusterRoles500JSONResponse{}, resp)
	})
}

func TestCreateClusterRoleHandler(t *testing.T) {
	ctx := testContext()

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.CreateClusterRole(ctx, gen.CreateClusterRoleRequestObject{Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterRole400JSONResponse{}, resp)
	})

	t.Run("success converts body and response", func(t *testing.T) {
		desc := "read-only access"
		body := gen.ClusterAuthzRole{
			Metadata: gen.ObjectMeta{Name: "viewer"},
			Spec:     &gen.ClusterAuthzRoleSpec{Actions: []string{"view", "list"}, Description: &desc},
		}

		var capturedRole *openchoreov1alpha1.ClusterAuthzRole
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateClusterRole(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, role *openchoreov1alpha1.ClusterAuthzRole) (*openchoreov1alpha1.ClusterAuthzRole, error) {
			capturedRole = role
			role.ObjectMeta.Name = "viewer"
			return role, nil
		})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.CreateClusterRole(ctx, gen.CreateClusterRoleRequestObject{Body: &body})
		require.NoError(t, err)

		require.NotNil(t, capturedRole)
		assert.Equal(t, []string{"view", "list"}, capturedRole.Spec.Actions)
		assert.Equal(t, "read-only access", capturedRole.Spec.Description)

		typed, ok := resp.(gen.CreateClusterRole201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "viewer", typed.Metadata.Name)
		require.NotNil(t, typed.Spec)
		assert.Equal(t, []string{"view", "list"}, typed.Spec.Actions)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateClusterRole(mock.Anything, mock.Anything).Return(nil, svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.ClusterAuthzRole{}
		resp, err := h.CreateClusterRole(ctx, gen.CreateClusterRoleRequestObject{Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterRole403JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateClusterRole(mock.Anything, mock.Anything).Return(nil, authzsvc.ErrRoleAlreadyExists)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.ClusterAuthzRole{}
		resp, err := h.CreateClusterRole(ctx, gen.CreateClusterRoleRequestObject{Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterRole409JSONResponse{}, resp)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateClusterRole(mock.Anything, mock.Anything).Return(nil, &svcpkg.ValidationError{Msg: "name required"})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.ClusterAuthzRole{}
		resp, err := h.CreateClusterRole(ctx, gen.CreateClusterRoleRequestObject{Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterRole400JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateClusterRole(mock.Anything, mock.Anything).Return(nil, errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.ClusterAuthzRole{}
		resp, err := h.CreateClusterRole(ctx, gen.CreateClusterRoleRequestObject{Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterRole500JSONResponse{}, resp)
	})
}

func TestGetClusterRoleHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success converts and returns role", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetClusterRole(mock.Anything, "admin").Return(&openchoreov1alpha1.ClusterAuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: "admin"},
			Spec:       openchoreov1alpha1.ClusterAuthzRoleSpec{Actions: []string{"*"}, Description: "full access"},
		}, nil)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.GetClusterRole(ctx, gen.GetClusterRoleRequestObject{Name: "admin"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetClusterRole200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "admin", typed.Metadata.Name)
		require.NotNil(t, typed.Spec)
		assert.Equal(t, []string{"*"}, typed.Spec.Actions)
		require.NotNil(t, typed.Spec.Description)
		assert.Equal(t, "full access", *typed.Spec.Description)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetClusterRole(mock.Anything, "admin").Return(nil, svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.GetClusterRole(ctx, gen.GetClusterRoleRequestObject{Name: "admin"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterRole403JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetClusterRole(mock.Anything, "missing").Return(nil, authzsvc.ErrRoleNotFound)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.GetClusterRole(ctx, gen.GetClusterRoleRequestObject{Name: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterRole404JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetClusterRole(mock.Anything, "admin").Return(nil, errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.GetClusterRole(ctx, gen.GetClusterRoleRequestObject{Name: "admin"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterRole500JSONResponse{}, resp)
	})
}

func TestUpdateClusterRoleHandler(t *testing.T) {
	ctx := testContext()

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.UpdateClusterRole(ctx, gen.UpdateClusterRoleRequestObject{Name: "admin", Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterRole400JSONResponse{}, resp)
	})

	t.Run("success injects URL name into CRD and converts response", func(t *testing.T) {
		body := gen.ClusterAuthzRole{
			Metadata: gen.ObjectMeta{Name: "body-name-ignored"},
			Spec:     &gen.ClusterAuthzRoleSpec{Actions: []string{"view", "create"}},
		}

		var capturedRole *openchoreov1alpha1.ClusterAuthzRole
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateClusterRole(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, role *openchoreov1alpha1.ClusterAuthzRole) (*openchoreov1alpha1.ClusterAuthzRole, error) {
			capturedRole = role
			return &openchoreov1alpha1.ClusterAuthzRole{
				ObjectMeta: metav1.ObjectMeta{Name: "admin"},
				Spec:       openchoreov1alpha1.ClusterAuthzRoleSpec{Actions: []string{"view", "create"}},
			}, nil
		})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.UpdateClusterRole(ctx, gen.UpdateClusterRoleRequestObject{Name: "admin", Body: &body})
		require.NoError(t, err)

		require.NotNil(t, capturedRole)
		assert.Equal(t, "admin", capturedRole.Name, "handler must override body name with URL path name")
		assert.Equal(t, []string{"view", "create"}, capturedRole.Spec.Actions)

		typed, ok := resp.(gen.UpdateClusterRole200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "admin", typed.Metadata.Name)
		require.NotNil(t, typed.Spec)
		assert.Equal(t, []string{"view", "create"}, typed.Spec.Actions)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateClusterRole(mock.Anything, mock.Anything).Return(nil, svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.ClusterAuthzRole{}
		resp, err := h.UpdateClusterRole(ctx, gen.UpdateClusterRoleRequestObject{Name: "admin", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterRole403JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateClusterRole(mock.Anything, mock.Anything).Return(nil, authzsvc.ErrRoleNotFound)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.ClusterAuthzRole{}
		resp, err := h.UpdateClusterRole(ctx, gen.UpdateClusterRoleRequestObject{Name: "missing", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterRole404JSONResponse{}, resp)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateClusterRole(mock.Anything, mock.Anything).Return(nil, &svcpkg.ValidationError{Msg: "actions required"})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.ClusterAuthzRole{}
		resp, err := h.UpdateClusterRole(ctx, gen.UpdateClusterRoleRequestObject{Name: "admin", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterRole400JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateClusterRole(mock.Anything, mock.Anything).Return(nil, errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.ClusterAuthzRole{}
		resp, err := h.UpdateClusterRole(ctx, gen.UpdateClusterRoleRequestObject{Name: "admin", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterRole500JSONResponse{}, resp)
	})
}

func TestDeleteClusterRoleHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success returns 204", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().DeleteClusterRole(mock.Anything, "admin").Return(nil)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.DeleteClusterRole(ctx, gen.DeleteClusterRoleRequestObject{Name: "admin"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterRole204Response{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().DeleteClusterRole(mock.Anything, "admin").Return(svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.DeleteClusterRole(ctx, gen.DeleteClusterRoleRequestObject{Name: "admin"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterRole403JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().DeleteClusterRole(mock.Anything, "missing").Return(authzsvc.ErrRoleNotFound)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.DeleteClusterRole(ctx, gen.DeleteClusterRoleRequestObject{Name: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterRole404JSONResponse{}, resp)
	})

	t.Run("role in use returns 409", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().DeleteClusterRole(mock.Anything, "admin").Return(authzsvc.ErrRoleInUse)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.DeleteClusterRole(ctx, gen.DeleteClusterRoleRequestObject{Name: "admin"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterRole409JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().DeleteClusterRole(mock.Anything, "admin").Return(errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.DeleteClusterRole(ctx, gen.DeleteClusterRoleRequestObject{Name: "admin"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterRole500JSONResponse{}, resp)
	})
}

// --- Cluster Role Bindings ---

func TestListClusterRoleBindingsHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success converts items and pagination", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().ListClusterRoleBindings(mock.Anything, mock.Anything).Return(&svcpkg.ListResult[openchoreov1alpha1.ClusterAuthzRoleBinding]{
			Items: []openchoreov1alpha1.ClusterAuthzRoleBinding{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "admin-binding"},
					Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
						Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
						RoleMappings: []openchoreov1alpha1.ClusterRoleMapping{
							{RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "admin"}},
						},
					},
				},
			},
			NextCursor: "next",
		}, nil)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.ListClusterRoleBindings(ctx, gen.ListClusterRoleBindingsRequestObject{})
		require.NoError(t, err)
		typed, ok := resp.(gen.ListClusterRoleBindings200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "admin-binding", typed.Items[0].Metadata.Name)
		require.NotNil(t, typed.Items[0].Spec)
		assert.Equal(t, "groups", typed.Items[0].Spec.Entitlement.Claim)
		assert.Equal(t, "admins", typed.Items[0].Spec.Entitlement.Value)
		require.NotNil(t, typed.Pagination.NextCursor)
		assert.Equal(t, "next", *typed.Pagination.NextCursor)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().ListClusterRoleBindings(mock.Anything, mock.Anything).Return(nil, svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.ListClusterRoleBindings(ctx, gen.ListClusterRoleBindingsRequestObject{})
		require.NoError(t, err)
		assert.IsType(t, gen.ListClusterRoleBindings403JSONResponse{}, resp)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().ListClusterRoleBindings(mock.Anything, mock.Anything).Return(nil, &svcpkg.ValidationError{Msg: "bad selector"})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.ListClusterRoleBindings(ctx, gen.ListClusterRoleBindingsRequestObject{})
		require.NoError(t, err)
		assert.IsType(t, gen.ListClusterRoleBindings400JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().ListClusterRoleBindings(mock.Anything, mock.Anything).Return(nil, errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.ListClusterRoleBindings(ctx, gen.ListClusterRoleBindingsRequestObject{})
		require.NoError(t, err)
		assert.IsType(t, gen.ListClusterRoleBindings500JSONResponse{}, resp)
	})
}

func TestCreateClusterRoleBindingHandler(t *testing.T) {
	ctx := testContext()

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.CreateClusterRoleBinding(ctx, gen.CreateClusterRoleBindingRequestObject{Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterRoleBinding400JSONResponse{}, resp)
	})

	t.Run("success converts body and response", func(t *testing.T) {
		var capturedBinding *openchoreov1alpha1.ClusterAuthzRoleBinding
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateClusterRoleBinding(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, binding *openchoreov1alpha1.ClusterAuthzRoleBinding) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error) {
			capturedBinding = binding
			binding.Name = "admin-binding"
			return binding, nil
		})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.ClusterAuthzRoleBinding{
			Metadata: gen.ObjectMeta{Name: "admin-binding"},
			Spec: &gen.ClusterAuthzRoleBindingSpec{
				Entitlement: gen.AuthzEntitlementClaim{Claim: "groups", Value: "admins"},
				RoleMappings: []gen.ClusterAuthzRoleMapping{{RoleRef: struct {
					Kind gen.ClusterAuthzRoleMappingRoleRefKind `json:"kind"`
					Name string                                 `json:"name"`
				}{Kind: "ClusterAuthzRole", Name: "admin"}}},
			},
		}
		resp, err := h.CreateClusterRoleBinding(ctx, gen.CreateClusterRoleBindingRequestObject{Body: &body})
		require.NoError(t, err)

		require.NotNil(t, capturedBinding)
		assert.Equal(t, "groups", capturedBinding.Spec.Entitlement.Claim)
		assert.Equal(t, "admins", capturedBinding.Spec.Entitlement.Value)
		require.Len(t, capturedBinding.Spec.RoleMappings, 1)
		assert.Equal(t, "admin", capturedBinding.Spec.RoleMappings[0].RoleRef.Name)

		typed, ok := resp.(gen.CreateClusterRoleBinding201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "admin-binding", typed.Metadata.Name)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateClusterRoleBinding(mock.Anything, mock.Anything).Return(nil, svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.ClusterAuthzRoleBinding{}
		resp, err := h.CreateClusterRoleBinding(ctx, gen.CreateClusterRoleBindingRequestObject{Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterRoleBinding403JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateClusterRoleBinding(mock.Anything, mock.Anything).Return(nil, authzsvc.ErrRoleBindingAlreadyExists)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.ClusterAuthzRoleBinding{}
		resp, err := h.CreateClusterRoleBinding(ctx, gen.CreateClusterRoleBindingRequestObject{Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterRoleBinding409JSONResponse{}, resp)
	})

	t.Run("role not found returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateClusterRoleBinding(mock.Anything, mock.Anything).Return(nil, authzsvc.ErrRoleNotFound)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.ClusterAuthzRoleBinding{}
		resp, err := h.CreateClusterRoleBinding(ctx, gen.CreateClusterRoleBindingRequestObject{Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterRoleBinding400JSONResponse{}, resp)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateClusterRoleBinding(mock.Anything, mock.Anything).Return(nil, &svcpkg.ValidationError{Msg: "entitlement required"})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.ClusterAuthzRoleBinding{}
		resp, err := h.CreateClusterRoleBinding(ctx, gen.CreateClusterRoleBindingRequestObject{Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterRoleBinding400JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateClusterRoleBinding(mock.Anything, mock.Anything).Return(nil, errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.ClusterAuthzRoleBinding{}
		resp, err := h.CreateClusterRoleBinding(ctx, gen.CreateClusterRoleBindingRequestObject{Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateClusterRoleBinding500JSONResponse{}, resp)
	})
}

func TestGetClusterRoleBindingHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success converts and returns binding", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetClusterRoleBinding(mock.Anything, "admin-binding").Return(&openchoreov1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "admin-binding"},
			Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
				RoleMappings: []openchoreov1alpha1.ClusterRoleMapping{
					{RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindClusterAuthzRole, Name: "admin"}},
				},
			},
		}, nil)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.GetClusterRoleBinding(ctx, gen.GetClusterRoleBindingRequestObject{Name: "admin-binding"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetClusterRoleBinding200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "admin-binding", typed.Metadata.Name)
		require.NotNil(t, typed.Spec)
		assert.Equal(t, "groups", typed.Spec.Entitlement.Claim)
		require.Len(t, typed.Spec.RoleMappings, 1)
		assert.Equal(t, "admin", typed.Spec.RoleMappings[0].RoleRef.Name)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetClusterRoleBinding(mock.Anything, "admin-binding").Return(nil, svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.GetClusterRoleBinding(ctx, gen.GetClusterRoleBindingRequestObject{Name: "admin-binding"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterRoleBinding403JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetClusterRoleBinding(mock.Anything, "missing").Return(nil, authzsvc.ErrRoleBindingNotFound)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.GetClusterRoleBinding(ctx, gen.GetClusterRoleBindingRequestObject{Name: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterRoleBinding404JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetClusterRoleBinding(mock.Anything, "admin-binding").Return(nil, errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.GetClusterRoleBinding(ctx, gen.GetClusterRoleBindingRequestObject{Name: "admin-binding"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetClusterRoleBinding500JSONResponse{}, resp)
	})
}

func TestUpdateClusterRoleBindingHandler(t *testing.T) {
	ctx := testContext()

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.UpdateClusterRoleBinding(ctx, gen.UpdateClusterRoleBindingRequestObject{Name: "admin-binding", Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterRoleBinding400JSONResponse{}, resp)
	})

	t.Run("success injects URL name into CRD and converts response", func(t *testing.T) {
		var capturedBinding *openchoreov1alpha1.ClusterAuthzRoleBinding
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateClusterRoleBinding(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, binding *openchoreov1alpha1.ClusterAuthzRoleBinding) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error) {
			capturedBinding = binding
			return &openchoreov1alpha1.ClusterAuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "admin-binding"},
				Spec: openchoreov1alpha1.ClusterAuthzRoleBindingSpec{
					Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: "groups", Value: "admins"},
				},
			}, nil
		})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.ClusterAuthzRoleBinding{
			Metadata: gen.ObjectMeta{Name: "body-name-ignored"},
		}
		resp, err := h.UpdateClusterRoleBinding(ctx, gen.UpdateClusterRoleBindingRequestObject{Name: "admin-binding", Body: &body})
		require.NoError(t, err)

		require.NotNil(t, capturedBinding)
		assert.Equal(t, "admin-binding", capturedBinding.Name, "handler must override body name with URL path name")

		typed, ok := resp.(gen.UpdateClusterRoleBinding200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "admin-binding", typed.Metadata.Name)
		require.NotNil(t, typed.Spec)
		assert.Equal(t, "groups", typed.Spec.Entitlement.Claim)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateClusterRoleBinding(mock.Anything, mock.Anything).Return(nil, svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.ClusterAuthzRoleBinding{}
		resp, err := h.UpdateClusterRoleBinding(ctx, gen.UpdateClusterRoleBindingRequestObject{Name: "admin-binding", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterRoleBinding403JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateClusterRoleBinding(mock.Anything, mock.Anything).Return(nil, authzsvc.ErrRoleBindingNotFound)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.ClusterAuthzRoleBinding{}
		resp, err := h.UpdateClusterRoleBinding(ctx, gen.UpdateClusterRoleBindingRequestObject{Name: "missing", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterRoleBinding404JSONResponse{}, resp)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateClusterRoleBinding(mock.Anything, mock.Anything).Return(nil, &svcpkg.ValidationError{Msg: "entitlement required"})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.ClusterAuthzRoleBinding{}
		resp, err := h.UpdateClusterRoleBinding(ctx, gen.UpdateClusterRoleBindingRequestObject{Name: "admin-binding", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterRoleBinding400JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateClusterRoleBinding(mock.Anything, mock.Anything).Return(nil, errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.ClusterAuthzRoleBinding{}
		resp, err := h.UpdateClusterRoleBinding(ctx, gen.UpdateClusterRoleBindingRequestObject{Name: "admin-binding", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateClusterRoleBinding500JSONResponse{}, resp)
	})
}

func TestDeleteClusterRoleBindingHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success returns 204", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().DeleteClusterRoleBinding(mock.Anything, "admin-binding").Return(nil)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.DeleteClusterRoleBinding(ctx, gen.DeleteClusterRoleBindingRequestObject{Name: "admin-binding"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterRoleBinding204Response{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().DeleteClusterRoleBinding(mock.Anything, "admin-binding").Return(svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.DeleteClusterRoleBinding(ctx, gen.DeleteClusterRoleBindingRequestObject{Name: "admin-binding"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterRoleBinding403JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().DeleteClusterRoleBinding(mock.Anything, "missing").Return(authzsvc.ErrRoleBindingNotFound)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.DeleteClusterRoleBinding(ctx, gen.DeleteClusterRoleBindingRequestObject{Name: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterRoleBinding404JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().DeleteClusterRoleBinding(mock.Anything, "admin-binding").Return(errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.DeleteClusterRoleBinding(ctx, gen.DeleteClusterRoleBindingRequestObject{Name: "admin-binding"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteClusterRoleBinding500JSONResponse{}, resp)
	})
}

// --- Namespace Roles ---

func TestListNamespaceRolesHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success forwards namespace and converts response", func(t *testing.T) {
		var capturedNS string
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().ListNamespaceRoles(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, ns string, _ svcpkg.ListOptions) (*svcpkg.ListResult[openchoreov1alpha1.AuthzRole], error) {
			capturedNS = ns
			return &svcpkg.ListResult[openchoreov1alpha1.AuthzRole]{
				Items: []openchoreov1alpha1.AuthzRole{
					{ObjectMeta: metav1.ObjectMeta{Name: "viewer", Namespace: "acme"}, Spec: openchoreov1alpha1.AuthzRoleSpec{Actions: []string{"view"}}},
				},
			}, nil
		})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.ListNamespaceRoles(ctx, gen.ListNamespaceRolesRequestObject{NamespaceName: "acme"})
		require.NoError(t, err)

		assert.Equal(t, "acme", capturedNS)

		typed, ok := resp.(gen.ListNamespaceRoles200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "viewer", typed.Items[0].Metadata.Name)
		require.NotNil(t, typed.Items[0].Spec)
		assert.Equal(t, []string{"view"}, typed.Items[0].Spec.Actions)
		assert.Nil(t, typed.Pagination.NextCursor, "no more pages")
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().ListNamespaceRoles(mock.Anything, "acme", mock.Anything).Return(nil, svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.ListNamespaceRoles(ctx, gen.ListNamespaceRolesRequestObject{NamespaceName: "acme"})
		require.NoError(t, err)
		assert.IsType(t, gen.ListNamespaceRoles403JSONResponse{}, resp)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().ListNamespaceRoles(mock.Anything, "acme", mock.Anything).Return(nil, &svcpkg.ValidationError{Msg: "bad selector"})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.ListNamespaceRoles(ctx, gen.ListNamespaceRolesRequestObject{NamespaceName: "acme"})
		require.NoError(t, err)
		assert.IsType(t, gen.ListNamespaceRoles400JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().ListNamespaceRoles(mock.Anything, "acme", mock.Anything).Return(nil, errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.ListNamespaceRoles(ctx, gen.ListNamespaceRolesRequestObject{NamespaceName: "acme"})
		require.NoError(t, err)
		assert.IsType(t, gen.ListNamespaceRoles500JSONResponse{}, resp)
	})
}

func TestCreateNamespaceRoleHandler(t *testing.T) {
	ctx := testContext()

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.CreateNamespaceRole(ctx, gen.CreateNamespaceRoleRequestObject{NamespaceName: "acme", Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateNamespaceRole400JSONResponse{}, resp)
	})

	t.Run("success forwards namespace, converts body and response", func(t *testing.T) {
		var capturedNS string
		var capturedRole *openchoreov1alpha1.AuthzRole
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateNamespaceRole(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, ns string, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
			capturedNS = ns
			capturedRole = role
			role.Name = "viewer"
			role.Namespace = ns
			return role, nil
		})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRole{
			Metadata: gen.ObjectMeta{Name: "viewer"},
			Spec:     &gen.AuthzRoleSpec{Actions: []string{"view", "list"}},
		}
		resp, err := h.CreateNamespaceRole(ctx, gen.CreateNamespaceRoleRequestObject{NamespaceName: "acme", Body: &body})
		require.NoError(t, err)

		assert.Equal(t, "acme", capturedNS)
		require.NotNil(t, capturedRole)
		assert.Equal(t, []string{"view", "list"}, capturedRole.Spec.Actions)

		typed, ok := resp.(gen.CreateNamespaceRole201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "viewer", typed.Metadata.Name)
		require.NotNil(t, typed.Spec)
		assert.Equal(t, []string{"view", "list"}, typed.Spec.Actions)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateNamespaceRole(mock.Anything, "acme", mock.Anything).Return(nil, svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRole{}
		resp, err := h.CreateNamespaceRole(ctx, gen.CreateNamespaceRoleRequestObject{NamespaceName: "acme", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateNamespaceRole403JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateNamespaceRole(mock.Anything, "acme", mock.Anything).Return(nil, authzsvc.ErrRoleAlreadyExists)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRole{}
		resp, err := h.CreateNamespaceRole(ctx, gen.CreateNamespaceRoleRequestObject{NamespaceName: "acme", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateNamespaceRole409JSONResponse{}, resp)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateNamespaceRole(mock.Anything, "acme", mock.Anything).Return(nil, &svcpkg.ValidationError{Msg: "name required"})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRole{}
		resp, err := h.CreateNamespaceRole(ctx, gen.CreateNamespaceRoleRequestObject{NamespaceName: "acme", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateNamespaceRole400JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateNamespaceRole(mock.Anything, "acme", mock.Anything).Return(nil, errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRole{}
		resp, err := h.CreateNamespaceRole(ctx, gen.CreateNamespaceRoleRequestObject{NamespaceName: "acme", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateNamespaceRole500JSONResponse{}, resp)
	})
}

func TestGetNamespaceRoleHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success converts and returns role", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetNamespaceRole(mock.Anything, "acme", "viewer").Return(&openchoreov1alpha1.AuthzRole{
			ObjectMeta: metav1.ObjectMeta{Name: "viewer", Namespace: "acme"},
			Spec:       openchoreov1alpha1.AuthzRoleSpec{Actions: []string{"view"}, Description: "read-only"},
		}, nil)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.GetNamespaceRole(ctx, gen.GetNamespaceRoleRequestObject{NamespaceName: "acme", Name: "viewer"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetNamespaceRole200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "viewer", typed.Metadata.Name)
		require.NotNil(t, typed.Metadata.Namespace)
		assert.Equal(t, "acme", *typed.Metadata.Namespace)
		require.NotNil(t, typed.Spec)
		assert.Equal(t, []string{"view"}, typed.Spec.Actions)
		require.NotNil(t, typed.Spec.Description)
		assert.Equal(t, "read-only", *typed.Spec.Description)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetNamespaceRole(mock.Anything, "acme", "viewer").Return(nil, svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.GetNamespaceRole(ctx, gen.GetNamespaceRoleRequestObject{NamespaceName: "acme", Name: "viewer"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetNamespaceRole403JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetNamespaceRole(mock.Anything, "acme", "missing").Return(nil, authzsvc.ErrRoleNotFound)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.GetNamespaceRole(ctx, gen.GetNamespaceRoleRequestObject{NamespaceName: "acme", Name: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetNamespaceRole404JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetNamespaceRole(mock.Anything, "acme", "viewer").Return(nil, errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.GetNamespaceRole(ctx, gen.GetNamespaceRoleRequestObject{NamespaceName: "acme", Name: "viewer"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetNamespaceRole500JSONResponse{}, resp)
	})
}

func TestUpdateNamespaceRoleHandler(t *testing.T) {
	ctx := testContext()

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.UpdateNamespaceRole(ctx, gen.UpdateNamespaceRoleRequestObject{NamespaceName: "acme", Name: "viewer", Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateNamespaceRole400JSONResponse{}, resp)
	})

	t.Run("success injects URL name, forwards namespace and converts response", func(t *testing.T) {
		var capturedNS string
		var capturedRole *openchoreov1alpha1.AuthzRole
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateNamespaceRole(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, ns string, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
			capturedNS = ns
			capturedRole = role
			return &openchoreov1alpha1.AuthzRole{
				ObjectMeta: metav1.ObjectMeta{Name: "viewer", Namespace: ns},
				Spec:       openchoreov1alpha1.AuthzRoleSpec{Actions: []string{"view", "list"}},
			}, nil
		})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRole{
			Metadata: gen.ObjectMeta{Name: "body-name-ignored"},
			Spec:     &gen.AuthzRoleSpec{Actions: []string{"view", "list"}},
		}
		resp, err := h.UpdateNamespaceRole(ctx, gen.UpdateNamespaceRoleRequestObject{NamespaceName: "acme", Name: "viewer", Body: &body})
		require.NoError(t, err)

		assert.Equal(t, "acme", capturedNS)
		require.NotNil(t, capturedRole)
		assert.Equal(t, "viewer", capturedRole.Name, "handler must override body name with URL path name")
		assert.Equal(t, []string{"view", "list"}, capturedRole.Spec.Actions)

		typed, ok := resp.(gen.UpdateNamespaceRole200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "viewer", typed.Metadata.Name)
		require.NotNil(t, typed.Spec)
		assert.Equal(t, []string{"view", "list"}, typed.Spec.Actions)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateNamespaceRole(mock.Anything, "acme", mock.Anything).Return(nil, svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRole{}
		resp, err := h.UpdateNamespaceRole(ctx, gen.UpdateNamespaceRoleRequestObject{NamespaceName: "acme", Name: "viewer", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateNamespaceRole403JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateNamespaceRole(mock.Anything, "acme", mock.Anything).Return(nil, authzsvc.ErrRoleNotFound)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRole{}
		resp, err := h.UpdateNamespaceRole(ctx, gen.UpdateNamespaceRoleRequestObject{NamespaceName: "acme", Name: "missing", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateNamespaceRole404JSONResponse{}, resp)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateNamespaceRole(mock.Anything, "acme", mock.Anything).Return(nil, &svcpkg.ValidationError{Msg: "actions required"})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRole{}
		resp, err := h.UpdateNamespaceRole(ctx, gen.UpdateNamespaceRoleRequestObject{NamespaceName: "acme", Name: "viewer", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateNamespaceRole400JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateNamespaceRole(mock.Anything, "acme", mock.Anything).Return(nil, errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRole{}
		resp, err := h.UpdateNamespaceRole(ctx, gen.UpdateNamespaceRoleRequestObject{NamespaceName: "acme", Name: "viewer", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateNamespaceRole500JSONResponse{}, resp)
	})
}

func TestDeleteNamespaceRoleHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success returns 204", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().DeleteNamespaceRole(mock.Anything, "acme", "viewer").Return(nil)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.DeleteNamespaceRole(ctx, gen.DeleteNamespaceRoleRequestObject{NamespaceName: "acme", Name: "viewer"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteNamespaceRole204Response{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().DeleteNamespaceRole(mock.Anything, "acme", "viewer").Return(svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.DeleteNamespaceRole(ctx, gen.DeleteNamespaceRoleRequestObject{NamespaceName: "acme", Name: "viewer"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteNamespaceRole403JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().DeleteNamespaceRole(mock.Anything, "acme", "missing").Return(authzsvc.ErrRoleNotFound)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.DeleteNamespaceRole(ctx, gen.DeleteNamespaceRoleRequestObject{NamespaceName: "acme", Name: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteNamespaceRole404JSONResponse{}, resp)
	})

	t.Run("role in use returns 409", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().DeleteNamespaceRole(mock.Anything, "acme", "viewer").Return(authzsvc.ErrRoleInUse)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.DeleteNamespaceRole(ctx, gen.DeleteNamespaceRoleRequestObject{NamespaceName: "acme", Name: "viewer"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteNamespaceRole409JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().DeleteNamespaceRole(mock.Anything, "acme", "viewer").Return(errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.DeleteNamespaceRole(ctx, gen.DeleteNamespaceRoleRequestObject{NamespaceName: "acme", Name: "viewer"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteNamespaceRole500JSONResponse{}, resp)
	})
}

// --- Namespace Role Bindings ---

func TestListNamespaceRoleBindingsHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success forwards namespace and converts items", func(t *testing.T) {
		var capturedNS string
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().ListNamespaceRoleBindings(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, ns string, _ svcpkg.ListOptions) (*svcpkg.ListResult[openchoreov1alpha1.AuthzRoleBinding], error) {
			capturedNS = ns
			return &svcpkg.ListResult[openchoreov1alpha1.AuthzRoleBinding]{
				Items: []openchoreov1alpha1.AuthzRoleBinding{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "dev-binding", Namespace: "acme"},
						Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
							Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
							RoleMappings: []openchoreov1alpha1.RoleMapping{
								{RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindAuthzRole, Name: "viewer"}},
							},
						},
					},
				},
			}, nil
		})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.ListNamespaceRoleBindings(ctx, gen.ListNamespaceRoleBindingsRequestObject{NamespaceName: "acme"})
		require.NoError(t, err)

		assert.Equal(t, "acme", capturedNS)

		typed, ok := resp.(gen.ListNamespaceRoleBindings200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		require.Len(t, typed.Items, 1)
		assert.Equal(t, "dev-binding", typed.Items[0].Metadata.Name)
		require.NotNil(t, typed.Items[0].Spec)
		assert.Equal(t, "groups", typed.Items[0].Spec.Entitlement.Claim)
		assert.Equal(t, "devs", typed.Items[0].Spec.Entitlement.Value)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().ListNamespaceRoleBindings(mock.Anything, "acme", mock.Anything).Return(nil, svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.ListNamespaceRoleBindings(ctx, gen.ListNamespaceRoleBindingsRequestObject{NamespaceName: "acme"})
		require.NoError(t, err)
		assert.IsType(t, gen.ListNamespaceRoleBindings403JSONResponse{}, resp)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().ListNamespaceRoleBindings(mock.Anything, "acme", mock.Anything).Return(nil, &svcpkg.ValidationError{Msg: "bad selector"})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.ListNamespaceRoleBindings(ctx, gen.ListNamespaceRoleBindingsRequestObject{NamespaceName: "acme"})
		require.NoError(t, err)
		assert.IsType(t, gen.ListNamespaceRoleBindings400JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().ListNamespaceRoleBindings(mock.Anything, "acme", mock.Anything).Return(nil, errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.ListNamespaceRoleBindings(ctx, gen.ListNamespaceRoleBindingsRequestObject{NamespaceName: "acme"})
		require.NoError(t, err)
		assert.IsType(t, gen.ListNamespaceRoleBindings500JSONResponse{}, resp)
	})
}

func TestCreateNamespaceRoleBindingHandler(t *testing.T) {
	ctx := testContext()

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.CreateNamespaceRoleBinding(ctx, gen.CreateNamespaceRoleBindingRequestObject{NamespaceName: "acme", Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateNamespaceRoleBinding400JSONResponse{}, resp)
	})

	t.Run("success forwards namespace, converts body and response", func(t *testing.T) {
		var capturedNS string
		var capturedBinding *openchoreov1alpha1.AuthzRoleBinding
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateNamespaceRoleBinding(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, ns string, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
			capturedNS = ns
			capturedBinding = binding
			binding.Name = "dev-binding"
			binding.Namespace = ns
			return binding, nil
		})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRoleBinding{
			Metadata: gen.ObjectMeta{Name: "dev-binding"},
			Spec: &gen.AuthzRoleBindingSpec{
				Entitlement:  gen.AuthzEntitlementClaim{Claim: "groups", Value: "devs"},
				RoleMappings: []gen.AuthzRoleMapping{{RoleRef: gen.AuthzRoleRef{Kind: "AuthzRole", Name: "viewer"}}},
			},
		}
		resp, err := h.CreateNamespaceRoleBinding(ctx, gen.CreateNamespaceRoleBindingRequestObject{NamespaceName: "acme", Body: &body})
		require.NoError(t, err)

		assert.Equal(t, "acme", capturedNS)
		require.NotNil(t, capturedBinding)
		assert.Equal(t, "groups", capturedBinding.Spec.Entitlement.Claim)
		assert.Equal(t, "devs", capturedBinding.Spec.Entitlement.Value)
		require.Len(t, capturedBinding.Spec.RoleMappings, 1)
		assert.Equal(t, "viewer", capturedBinding.Spec.RoleMappings[0].RoleRef.Name)

		typed, ok := resp.(gen.CreateNamespaceRoleBinding201JSONResponse)
		require.True(t, ok, "expected 201 response, got %T", resp)
		assert.Equal(t, "dev-binding", typed.Metadata.Name)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateNamespaceRoleBinding(mock.Anything, "acme", mock.Anything).Return(nil, svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRoleBinding{}
		resp, err := h.CreateNamespaceRoleBinding(ctx, gen.CreateNamespaceRoleBindingRequestObject{NamespaceName: "acme", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateNamespaceRoleBinding403JSONResponse{}, resp)
	})

	t.Run("already exists returns 409", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateNamespaceRoleBinding(mock.Anything, "acme", mock.Anything).Return(nil, authzsvc.ErrRoleBindingAlreadyExists)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRoleBinding{}
		resp, err := h.CreateNamespaceRoleBinding(ctx, gen.CreateNamespaceRoleBindingRequestObject{NamespaceName: "acme", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateNamespaceRoleBinding409JSONResponse{}, resp)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateNamespaceRoleBinding(mock.Anything, "acme", mock.Anything).Return(nil, &svcpkg.ValidationError{Msg: "entitlement required"})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRoleBinding{}
		resp, err := h.CreateNamespaceRoleBinding(ctx, gen.CreateNamespaceRoleBindingRequestObject{NamespaceName: "acme", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateNamespaceRoleBinding400JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().CreateNamespaceRoleBinding(mock.Anything, "acme", mock.Anything).Return(nil, errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRoleBinding{}
		resp, err := h.CreateNamespaceRoleBinding(ctx, gen.CreateNamespaceRoleBindingRequestObject{NamespaceName: "acme", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateNamespaceRoleBinding500JSONResponse{}, resp)
	})
}

func TestGetNamespaceRoleBindingHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success converts and returns binding", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetNamespaceRoleBinding(mock.Anything, "acme", "dev-binding").Return(&openchoreov1alpha1.AuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "dev-binding", Namespace: "acme"},
			Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
				Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
				RoleMappings: []openchoreov1alpha1.RoleMapping{
					{RoleRef: openchoreov1alpha1.RoleRef{Kind: openchoreov1alpha1.RoleRefKindAuthzRole, Name: "viewer"}},
				},
				Effect: openchoreov1alpha1.EffectAllow,
			},
		}, nil)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.GetNamespaceRoleBinding(ctx, gen.GetNamespaceRoleBindingRequestObject{NamespaceName: "acme", Name: "dev-binding"})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetNamespaceRoleBinding200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "dev-binding", typed.Metadata.Name)
		require.NotNil(t, typed.Metadata.Namespace)
		assert.Equal(t, "acme", *typed.Metadata.Namespace)
		require.NotNil(t, typed.Spec)
		assert.Equal(t, "groups", typed.Spec.Entitlement.Claim)
		assert.Equal(t, "devs", typed.Spec.Entitlement.Value)
		require.Len(t, typed.Spec.RoleMappings, 1)
		assert.Equal(t, gen.AuthzRoleRefKind("AuthzRole"), typed.Spec.RoleMappings[0].RoleRef.Kind)
		assert.Equal(t, "viewer", typed.Spec.RoleMappings[0].RoleRef.Name)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetNamespaceRoleBinding(mock.Anything, "acme", "dev-binding").Return(nil, svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.GetNamespaceRoleBinding(ctx, gen.GetNamespaceRoleBindingRequestObject{NamespaceName: "acme", Name: "dev-binding"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetNamespaceRoleBinding403JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetNamespaceRoleBinding(mock.Anything, "acme", "missing").Return(nil, authzsvc.ErrRoleBindingNotFound)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.GetNamespaceRoleBinding(ctx, gen.GetNamespaceRoleBindingRequestObject{NamespaceName: "acme", Name: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetNamespaceRoleBinding404JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().GetNamespaceRoleBinding(mock.Anything, "acme", "dev-binding").Return(nil, errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.GetNamespaceRoleBinding(ctx, gen.GetNamespaceRoleBindingRequestObject{NamespaceName: "acme", Name: "dev-binding"})
		require.NoError(t, err)
		assert.IsType(t, gen.GetNamespaceRoleBinding500JSONResponse{}, resp)
	})
}

func TestUpdateNamespaceRoleBindingHandler(t *testing.T) {
	ctx := testContext()

	t.Run("nil body returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.UpdateNamespaceRoleBinding(ctx, gen.UpdateNamespaceRoleBindingRequestObject{NamespaceName: "acme", Name: "dev-binding", Body: nil})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateNamespaceRoleBinding400JSONResponse{}, resp)
	})

	t.Run("success injects URL name, forwards namespace and converts response", func(t *testing.T) {
		var capturedNS string
		var capturedBinding *openchoreov1alpha1.AuthzRoleBinding
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateNamespaceRoleBinding(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, ns string, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
			capturedNS = ns
			capturedBinding = binding
			return &openchoreov1alpha1.AuthzRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "dev-binding", Namespace: ns},
				Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
					Entitlement: openchoreov1alpha1.EntitlementClaim{Claim: "groups", Value: "devs"},
				},
			}, nil
		})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRoleBinding{
			Metadata: gen.ObjectMeta{Name: "body-name-ignored"},
		}
		resp, err := h.UpdateNamespaceRoleBinding(ctx, gen.UpdateNamespaceRoleBindingRequestObject{NamespaceName: "acme", Name: "dev-binding", Body: &body})
		require.NoError(t, err)

		assert.Equal(t, "acme", capturedNS)
		require.NotNil(t, capturedBinding)
		assert.Equal(t, "dev-binding", capturedBinding.Name, "handler must override body name with URL path name")

		typed, ok := resp.(gen.UpdateNamespaceRoleBinding200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "dev-binding", typed.Metadata.Name)
		require.NotNil(t, typed.Spec)
		assert.Equal(t, "groups", typed.Spec.Entitlement.Claim)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateNamespaceRoleBinding(mock.Anything, "acme", mock.Anything).Return(nil, svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRoleBinding{}
		resp, err := h.UpdateNamespaceRoleBinding(ctx, gen.UpdateNamespaceRoleBindingRequestObject{NamespaceName: "acme", Name: "dev-binding", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateNamespaceRoleBinding403JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateNamespaceRoleBinding(mock.Anything, "acme", mock.Anything).Return(nil, authzsvc.ErrRoleBindingNotFound)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRoleBinding{}
		resp, err := h.UpdateNamespaceRoleBinding(ctx, gen.UpdateNamespaceRoleBindingRequestObject{NamespaceName: "acme", Name: "missing", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateNamespaceRoleBinding404JSONResponse{}, resp)
	})

	t.Run("validation error returns 400", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateNamespaceRoleBinding(mock.Anything, "acme", mock.Anything).Return(nil, &svcpkg.ValidationError{Msg: "entitlement required"})
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRoleBinding{}
		resp, err := h.UpdateNamespaceRoleBinding(ctx, gen.UpdateNamespaceRoleBindingRequestObject{NamespaceName: "acme", Name: "dev-binding", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateNamespaceRoleBinding400JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().UpdateNamespaceRoleBinding(mock.Anything, "acme", mock.Anything).Return(nil, errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		body := gen.AuthzRoleBinding{}
		resp, err := h.UpdateNamespaceRoleBinding(ctx, gen.UpdateNamespaceRoleBindingRequestObject{NamespaceName: "acme", Name: "dev-binding", Body: &body})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateNamespaceRoleBinding500JSONResponse{}, resp)
	})
}

func TestDeleteNamespaceRoleBindingHandler(t *testing.T) {
	ctx := testContext()

	t.Run("success returns 204", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().DeleteNamespaceRoleBinding(mock.Anything, "acme", "dev-binding").Return(nil)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.DeleteNamespaceRoleBinding(ctx, gen.DeleteNamespaceRoleBindingRequestObject{NamespaceName: "acme", Name: "dev-binding"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteNamespaceRoleBinding204Response{}, resp)
	})

	t.Run("forbidden returns 403", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().DeleteNamespaceRoleBinding(mock.Anything, "acme", "dev-binding").Return(svcpkg.ErrForbidden)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.DeleteNamespaceRoleBinding(ctx, gen.DeleteNamespaceRoleBindingRequestObject{NamespaceName: "acme", Name: "dev-binding"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteNamespaceRoleBinding403JSONResponse{}, resp)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().DeleteNamespaceRoleBinding(mock.Anything, "acme", "missing").Return(authzsvc.ErrRoleBindingNotFound)
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.DeleteNamespaceRoleBinding(ctx, gen.DeleteNamespaceRoleBindingRequestObject{NamespaceName: "acme", Name: "missing"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteNamespaceRoleBinding404JSONResponse{}, resp)
	})

	t.Run("generic error returns 500", func(t *testing.T) {
		svc := authzmocks.NewMockService(t)
		svc.EXPECT().DeleteNamespaceRoleBinding(mock.Anything, "acme", "dev-binding").Return(errors.New("unexpected"))
		h := newHandlerWithAuthzService(t, svc, &config.Config{})

		resp, err := h.DeleteNamespaceRoleBinding(ctx, gen.DeleteNamespaceRoleBindingRequestObject{NamespaceName: "acme", Name: "dev-binding"})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteNamespaceRoleBinding500JSONResponse{}, resp)
	})
}

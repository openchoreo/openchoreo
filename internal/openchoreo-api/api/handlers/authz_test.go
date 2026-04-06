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
		svc.EXPECT().ListActions(mock.Anything).Return(nil, errors.New("boom"))
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

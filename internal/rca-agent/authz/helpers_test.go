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
	"github.com/stretchr/testify/require"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

func TestComponentScopeAuthz_ComponentScope(t *testing.T) {
	t.Parallel()
	rt, id, hierarchy := ComponentScopeAuthz("ns-1", "proj-1", "comp-1")

	assert.Equal(t, ResourceTypeComponent, rt)
	assert.Equal(t, "comp-1", id)
	assert.Equal(t, "ns-1", hierarchy.Namespace)
	assert.Equal(t, "proj-1", hierarchy.Project)
	assert.Equal(t, "comp-1", hierarchy.Component)
}

func TestComponentScopeAuthz_ProjectScope(t *testing.T) {
	t.Parallel()
	rt, id, hierarchy := ComponentScopeAuthz("ns-1", "proj-1", "")

	assert.Equal(t, ResourceTypeProject, rt)
	assert.Equal(t, "proj-1", id)
	assert.Equal(t, "ns-1", hierarchy.Namespace)
	assert.Equal(t, "proj-1", hierarchy.Project)
	assert.Empty(t, hierarchy.Component)
}

func TestComponentScopeAuthz_NamespaceScope(t *testing.T) {
	t.Parallel()
	rt, id, hierarchy := ComponentScopeAuthz("ns-1", "", "")

	assert.Equal(t, ResourceTypeNamespace, rt)
	assert.Equal(t, "ns-1", id)
	assert.Equal(t, "ns-1", hierarchy.Namespace)
	assert.Empty(t, hierarchy.Project)
}

func TestComponentScopeAuthz_Empty(t *testing.T) {
	t.Parallel()
	rt, id, hierarchy := ComponentScopeAuthz("", "", "")

	assert.Equal(t, ResourceTypeUnknown, rt)
	assert.Empty(t, id)
	assert.Empty(t, hierarchy.Namespace)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func authedContext() context.Context {
	return auth.SetSubjectContext(context.Background(), &auth.SubjectContext{
		ID:                "test-user",
		Type:              "user",
		EntitlementClaim:  "groups",
		EntitlementValues: []string{"org-admins"},
	})
}

type stubPDP struct {
	decision bool
	err      error
}

func (s *stubPDP) Evaluate(_ context.Context, _ *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &authzcore.Decision{Decision: s.decision, Context: &authzcore.DecisionContext{}}, nil
}

func (s *stubPDP) BatchEvaluate(_ context.Context, _ *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	return nil, nil
}

func (s *stubPDP) GetSubjectProfile(_ context.Context, _ *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, nil
}

func TestCheckAuthorization_Allowed(t *testing.T) {
	t.Parallel()
	err := CheckAuthorization(
		authedContext(), testLogger(), &stubPDP{decision: true},
		ActionViewRCAReport, ResourceTypeProject, "proj-1",
		authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "proj-1"},
	)
	require.NoError(t, err)
}

func TestCheckAuthorization_Denied(t *testing.T) {
	t.Parallel()
	err := CheckAuthorization(
		authedContext(), testLogger(), &stubPDP{decision: false},
		ActionViewRCAReport, ResourceTypeProject, "proj-1",
		authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "proj-1"},
	)
	require.ErrorIs(t, err, ErrAuthzForbidden)
}

func TestCheckAuthorization_NilPDP(t *testing.T) {
	t.Parallel()
	err := CheckAuthorization(
		authedContext(), testLogger(), nil,
		ActionViewRCAReport, ResourceTypeProject, "proj-1",
		authzcore.ResourceHierarchy{},
	)
	require.NoError(t, err)
}

func TestCheckAuthorization_NoSubjectContext(t *testing.T) {
	t.Parallel()
	err := CheckAuthorization(
		context.Background(), testLogger(), &stubPDP{decision: true},
		ActionViewRCAReport, ResourceTypeProject, "proj-1",
		authzcore.ResourceHierarchy{},
	)
	require.ErrorIs(t, err, ErrAuthzUnauthorized)
}

func TestCheckAuthorization_PDPError(t *testing.T) {
	t.Parallel()
	err := CheckAuthorization(
		authedContext(), testLogger(), &stubPDP{err: fmt.Errorf("connection refused")},
		ActionViewRCAReport, ResourceTypeProject, "proj-1",
		authzcore.ResourceHierarchy{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authorization evaluation failed")
}

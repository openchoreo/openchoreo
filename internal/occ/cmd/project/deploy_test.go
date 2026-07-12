// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func makePipeline(paths ...gen.PromotionPath) *gen.DeploymentPipeline {
	return &gen.DeploymentPipeline{
		Metadata: gen.ObjectMeta{Name: "test-pipeline"},
		Spec:     &gen.DeploymentPipelineSpec{PromotionPaths: &paths},
	}
}

func promotionPath(source string, targets ...string) gen.PromotionPath {
	refs := make([]gen.TargetEnvironmentRef, len(targets))
	for i, t := range targets {
		refs[i] = gen.TargetEnvironmentRef{Name: t}
	}
	pp := gen.PromotionPath{TargetEnvironmentRefs: refs}
	pp.SourceEnvironmentRef.Name = source
	return pp
}

func bindingFor(name, project, env string, release *string) gen.ProjectReleaseBinding {
	b := gen.ProjectReleaseBinding{
		Metadata: gen.ObjectMeta{Name: name},
		Spec: &gen.ProjectReleaseBindingSpec{
			Environment:    env,
			ProjectRelease: release,
		},
	}
	b.Spec.Owner.ProjectName = project
	return b
}

func ptr(s string) *string { return &s }

// --- validation ---

func TestDeploy_ValidationError(t *testing.T) {
	p := New(mocks.NewMockInterface(t))
	assert.Error(t, p.Deploy(DeployParams{Namespace: "", ProjectName: "online-store"}))
	assert.Error(t, p.Deploy(DeployParams{Namespace: "acme", ProjectName: ""}))
}

// --- deploy (no --to) ---

func TestDeploy_CreatesBinding_LeavesReleaseUnset(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "acme", "online-store").
		Return(makePipeline(promotionPath("dev", "prod")), nil)
	mc.EXPECT().ListProjectReleaseBindings(mock.Anything, "acme", mock.Anything).
		Return(&gen.ProjectReleaseBindingList{Items: []gen.ProjectReleaseBinding{}}, nil)
	mc.EXPECT().CreateProjectReleaseBinding(mock.Anything, "acme", mock.MatchedBy(func(b gen.ProjectReleaseBinding) bool {
		return b.Metadata.Name == "online-store-dev" &&
			b.Spec != nil &&
			b.Spec.Environment == "dev" &&
			b.Spec.Owner.ProjectName == "online-store" &&
			b.Spec.ProjectRelease == nil // left unset for the controller to seed
	})).Return(&gen.ProjectReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "online-store-dev"},
		Spec:     &gen.ProjectReleaseBindingSpec{Environment: "dev"},
	}, nil)

	p := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, p.Deploy(DeployParams{Namespace: "acme", ProjectName: "online-store"}))
	})
	assert.Contains(t, out, "Successfully deployed project 'online-store' to environment 'dev'")
	assert.Contains(t, out, "Binding: online-store-dev")
}

func TestDeploy_AlreadyDeployed_NoRelease(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "acme", "online-store").
		Return(makePipeline(promotionPath("dev", "prod")), nil)
	mc.EXPECT().ListProjectReleaseBindings(mock.Anything, "acme", mock.Anything).
		Return(&gen.ProjectReleaseBindingList{Items: []gen.ProjectReleaseBinding{
			// A binding for a different project in the same env must be ignored.
			bindingFor("other-store-dev", "other-store", "dev", ptr("other-abc")),
			bindingFor("online-store-dev", "online-store", "dev", ptr("online-store-abc")),
		}}, nil)
	// No Create/Update expected — already deployed and no explicit --release.

	p := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, p.Deploy(DeployParams{Namespace: "acme", ProjectName: "online-store"}))
	})
	assert.Contains(t, out, "already deployed to environment 'dev'")
}

// findBinding must follow pagination: when the matching binding is on a later
// page, deploy must detect it and not create a duplicate.
func TestDeploy_FindBinding_FollowsPagination(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "acme", "online-store").
		Return(makePipeline(promotionPath("dev", "prod")), nil)

	// Page 1: no dev binding, but signals a next page.
	mc.EXPECT().ListProjectReleaseBindings(mock.Anything, "acme", mock.MatchedBy(func(p *gen.ListProjectReleaseBindingsParams) bool {
		return p.Cursor == nil
	})).Return(&gen.ProjectReleaseBindingList{
		Items:      []gen.ProjectReleaseBinding{bindingFor("online-store-prod", "online-store", "prod", ptr("rel-prod"))},
		Pagination: gen.Pagination{NextCursor: ptr("page-2")},
	}, nil)

	// Page 2: the dev binding lives here.
	mc.EXPECT().ListProjectReleaseBindings(mock.Anything, "acme", mock.MatchedBy(func(p *gen.ListProjectReleaseBindingsParams) bool {
		return p.Cursor != nil && *p.Cursor == "page-2"
	})).Return(&gen.ProjectReleaseBindingList{
		Items:      []gen.ProjectReleaseBinding{bindingFor("online-store-dev", "online-store", "dev", ptr("rel-dev"))},
		Pagination: gen.Pagination{},
	}, nil)

	// No Create/Update: the existing dev binding was found on page 2.
	p := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, p.Deploy(DeployParams{Namespace: "acme", ProjectName: "online-store"}))
	})
	assert.Contains(t, out, "already deployed to environment 'dev'")
}

func TestDeploy_WithReleasePin_Create(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "acme", "online-store").
		Return(makePipeline(promotionPath("dev", "prod")), nil)
	mc.EXPECT().ListProjectReleaseBindings(mock.Anything, "acme", mock.Anything).
		Return(&gen.ProjectReleaseBindingList{Items: []gen.ProjectReleaseBinding{}}, nil)
	mc.EXPECT().CreateProjectReleaseBinding(mock.Anything, "acme", mock.MatchedBy(func(b gen.ProjectReleaseBinding) bool {
		return b.Spec != nil && b.Spec.ProjectRelease != nil && *b.Spec.ProjectRelease == "online-store-xyz"
	})).Return(&gen.ProjectReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "online-store-dev"},
		Spec:     &gen.ProjectReleaseBindingSpec{Environment: "dev", ProjectRelease: ptr("online-store-xyz")},
	}, nil)

	p := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, p.Deploy(DeployParams{Namespace: "acme", ProjectName: "online-store", Release: "online-store-xyz"}))
	})
	assert.Contains(t, out, "Release: online-store-xyz")
}

func TestDeploy_WithReleasePin_UpdatesExisting(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "acme", "online-store").
		Return(makePipeline(promotionPath("dev", "prod")), nil)
	mc.EXPECT().ListProjectReleaseBindings(mock.Anything, "acme", mock.Anything).
		Return(&gen.ProjectReleaseBindingList{Items: []gen.ProjectReleaseBinding{
			bindingFor("online-store-dev", "online-store", "dev", ptr("old")),
		}}, nil)
	mc.EXPECT().UpdateProjectReleaseBinding(mock.Anything, "acme", "online-store-dev", mock.MatchedBy(func(b gen.ProjectReleaseBinding) bool {
		return b.Spec != nil && b.Spec.ProjectRelease != nil && *b.Spec.ProjectRelease == "new"
	})).Return(&gen.ProjectReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "online-store-dev"},
		Spec:     &gen.ProjectReleaseBindingSpec{Environment: "dev", ProjectRelease: ptr("new")},
	}, nil)

	p := New(mc)
	require.NoError(t, p.Deploy(DeployParams{Namespace: "acme", ProjectName: "online-store", Release: "new"}))
}

func TestDeploy_PipelineError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "acme", "online-store").
		Return(nil, fmt.Errorf("no pipeline"))

	p := New(mc)
	assert.EqualError(t, p.Deploy(DeployParams{Namespace: "acme", ProjectName: "online-store"}), "no pipeline")
}

// --- promote (--to) ---

func TestPromote_UsesSourceRelease_Create(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	// Linear pipeline dev -> staging -> prod; promoting to staging sources from dev.
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "acme", "online-store").
		Return(makePipeline(promotionPath("dev", "staging"), promotionPath("staging", "prod")), nil)
	// findBinding is called twice: source (dev) then target (staging).
	mc.EXPECT().ListProjectReleaseBindings(mock.Anything, "acme", mock.Anything).
		Return(&gen.ProjectReleaseBindingList{Items: []gen.ProjectReleaseBinding{
			bindingFor("online-store-dev", "online-store", "dev", ptr("online-store-abc")),
		}}, nil)
	mc.EXPECT().CreateProjectReleaseBinding(mock.Anything, "acme", mock.MatchedBy(func(b gen.ProjectReleaseBinding) bool {
		return b.Metadata.Name == "online-store-staging" &&
			b.Spec != nil && b.Spec.Environment == "staging" &&
			b.Spec.ProjectRelease != nil && *b.Spec.ProjectRelease == "online-store-abc"
	})).Return(&gen.ProjectReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "online-store-staging"},
		Spec:     &gen.ProjectReleaseBindingSpec{Environment: "staging", ProjectRelease: ptr("online-store-abc")},
	}, nil)

	p := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, p.Deploy(DeployParams{Namespace: "acme", ProjectName: "online-store", To: "staging"}))
	})
	assert.Contains(t, out, "environment 'staging'")
	assert.Contains(t, out, "Release: online-store-abc")
}

func TestPromote_NoSourceRelease_Error(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "acme", "online-store").
		Return(makePipeline(promotionPath("dev", "staging")), nil)
	mc.EXPECT().ListProjectReleaseBindings(mock.Anything, "acme", mock.Anything).
		Return(&gen.ProjectReleaseBindingList{Items: []gen.ProjectReleaseBinding{
			bindingFor("online-store-dev", "online-store", "dev", nil),
		}}, nil)

	p := New(mc)
	err := p.Deploy(DeployParams{Namespace: "acme", ProjectName: "online-store", To: "staging"})
	assert.ErrorContains(t, err, "no release pinned for source environment 'dev'")
}

func TestPromote_ExplicitRelease_UpdatesTarget(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "acme", "online-store").
		Return(makePipeline(promotionPath("dev", "staging")), nil)
	// With --release provided, only the target lookup happens.
	mc.EXPECT().ListProjectReleaseBindings(mock.Anything, "acme", mock.Anything).
		Return(&gen.ProjectReleaseBindingList{Items: []gen.ProjectReleaseBinding{
			bindingFor("online-store-staging", "online-store", "staging", ptr("old")),
		}}, nil)
	mc.EXPECT().UpdateProjectReleaseBinding(mock.Anything, "acme", "online-store-staging", mock.MatchedBy(func(b gen.ProjectReleaseBinding) bool {
		return b.Spec != nil && b.Spec.ProjectRelease != nil && *b.Spec.ProjectRelease == "pin-me"
	})).Return(&gen.ProjectReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "online-store-staging"},
		Spec:     &gen.ProjectReleaseBindingSpec{Environment: "staging", ProjectRelease: ptr("pin-me")},
	}, nil)

	p := New(mc)
	require.NoError(t, p.Deploy(DeployParams{Namespace: "acme", ProjectName: "online-store", To: "staging", Release: "pin-me"}))
}

func TestPromote_UnknownTargetEnv_Error(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "acme", "online-store").
		Return(makePipeline(promotionPath("dev", "staging")), nil)

	p := New(mc)
	err := p.Deploy(DeployParams{Namespace: "acme", ProjectName: "online-store", To: "prod"})
	assert.ErrorContains(t, err, "no promotion path found for target environment 'prod'")
}

// --- --set / environmentConfigs ---

func TestApplyEnvConfigOverrides_Empty(t *testing.T) {
	b := &gen.ProjectReleaseBinding{Spec: &gen.ProjectReleaseBindingSpec{Environment: "dev"}}
	out, err := applyEnvConfigOverrides(b, nil)
	require.NoError(t, err)
	assert.Same(t, b, out) // unchanged, same pointer
}

func TestApplyEnvConfigOverrides_Invalid(t *testing.T) {
	b := &gen.ProjectReleaseBinding{Spec: &gen.ProjectReleaseBindingSpec{}}
	_, err := applyEnvConfigOverrides(b, []string{"noequals"})
	assert.ErrorContains(t, err, "invalid --set format")
}

func TestApplyEnvConfigOverrides_Merge(t *testing.T) {
	b := &gen.ProjectReleaseBinding{Spec: &gen.ProjectReleaseBindingSpec{Environment: "dev"}}
	out, err := applyEnvConfigOverrides(b, []string{"replicas=3", "tier=gold"})
	require.NoError(t, err)
	require.NotNil(t, out.Spec.EnvironmentConfigs)
	ec := *out.Spec.EnvironmentConfigs
	assert.Equal(t, float64(3), ec["replicas"]) // numbers stay numeric
	assert.Equal(t, "gold", ec["tier"])
	assert.Equal(t, "dev", out.Spec.Environment) // existing fields preserved
}

func TestDeploy_WithSet_Create(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "acme", "online-store").
		Return(makePipeline(promotionPath("dev", "prod")), nil)
	mc.EXPECT().ListProjectReleaseBindings(mock.Anything, "acme", mock.Anything).
		Return(&gen.ProjectReleaseBindingList{Items: []gen.ProjectReleaseBinding{}}, nil)
	mc.EXPECT().CreateProjectReleaseBinding(mock.Anything, "acme", mock.MatchedBy(func(b gen.ProjectReleaseBinding) bool {
		if b.Spec == nil || b.Spec.EnvironmentConfigs == nil {
			return false
		}
		ec := *b.Spec.EnvironmentConfigs
		return ec["replicas"] == float64(3) && b.Spec.ProjectRelease == nil
	})).Return(&gen.ProjectReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "online-store-dev"},
		Spec:     &gen.ProjectReleaseBindingSpec{Environment: "dev"},
	}, nil)

	p := New(mc)
	require.NoError(t, p.Deploy(DeployParams{Namespace: "acme", ProjectName: "online-store", Set: []string{"replicas=3"}}))
}

func TestDeploy_WithSet_UpdatesExistingWithoutRelease(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "acme", "online-store").
		Return(makePipeline(promotionPath("dev", "prod")), nil)
	mc.EXPECT().ListProjectReleaseBindings(mock.Anything, "acme", mock.Anything).
		Return(&gen.ProjectReleaseBindingList{Items: []gen.ProjectReleaseBinding{
			bindingFor("online-store-dev", "online-store", "dev", ptr("online-store-abc")),
		}}, nil)
	// --set given, so the existing binding is updated (not an early "already deployed" return),
	// and the controller-seeded release pin is preserved.
	mc.EXPECT().UpdateProjectReleaseBinding(mock.Anything, "acme", "online-store-dev", mock.MatchedBy(func(b gen.ProjectReleaseBinding) bool {
		if b.Spec == nil || b.Spec.EnvironmentConfigs == nil {
			return false
		}
		ec := *b.Spec.EnvironmentConfigs
		return ec["replicas"] == float64(5) &&
			b.Spec.ProjectRelease != nil && *b.Spec.ProjectRelease == "online-store-abc"
	})).Return(&gen.ProjectReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "online-store-dev"},
		Spec:     &gen.ProjectReleaseBindingSpec{Environment: "dev"},
	}, nil)

	p := New(mc)
	require.NoError(t, p.Deploy(DeployParams{Namespace: "acme", ProjectName: "online-store", Set: []string{"replicas=5"}}))
}

// --- cmd wiring ---

func TestDeployCmd_FactoryError(t *testing.T) {
	cmd := newDeployCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"online-store"})
	assert.EqualError(t, err, "factory failed")
}

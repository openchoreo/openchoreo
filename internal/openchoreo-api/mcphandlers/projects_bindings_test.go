// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	deploymentpipelinemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/deploymentpipeline/mocks"
	projectmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project/mocks"
	projectreleasebindingmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/projectreleasebinding/mocks"
)

const (
	testBindingPipeline = "default"
	// testEnvironmentName ("dev") is defined in components_test.go.
	testEnvStaging = "staging"
	testEnvProd    = "prod"
)

// createdProject is the Project returned by ProjectService.CreateProject, with the
// pipeline ref already defaulted by the service.
func createdProject() *openchoreov1alpha1.Project {
	return &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: testProject, Namespace: testNS},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
				Kind: openchoreov1alpha1.DeploymentPipelineRefKindDeploymentPipeline,
				Name: testBindingPipeline,
			},
		},
	}
}

// pipelineWithEnvs builds a DeploymentPipeline whose promotion paths reference the
// given environments as dev -> (staging -> prod).
func pipelineWithEnvs() *openchoreov1alpha1.DeploymentPipeline {
	return &openchoreov1alpha1.DeploymentPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: testBindingPipeline, Namespace: testNS},
		Spec: openchoreov1alpha1.DeploymentPipelineSpec{
			PromotionPaths: []openchoreov1alpha1.PromotionPath{
				{
					SourceEnvironmentRef:  openchoreov1alpha1.EnvironmentRef{Name: testEnvironmentName},
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{Name: testEnvStaging}},
				},
				{
					SourceEnvironmentRef:  openchoreov1alpha1.EnvironmentRef{Name: testEnvStaging},
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{Name: testEnvProd}},
				},
			},
		},
	}
}

func createProjectReq() *gen.CreateProjectJSONRequestBody {
	return &gen.CreateProjectJSONRequestBody{Metadata: gen.ObjectMeta{Name: testProject}}
}

// expectProjectCreated wires a project service that returns the defaulted project.
func expectProjectCreated(t *testing.T) *projectmocks.MockService {
	t.Helper()
	projSvc := projectmocks.NewMockService(t)
	projSvc.EXPECT().
		CreateProject(mock.Anything, testNS, mock.Anything).
		Return(createdProject(), nil)
	return projSvc
}

func TestCreateProjectCreatesDefaultBindings(t *testing.T) {
	ctx := context.Background()

	t.Run("creates one unpinned binding per pipeline environment", func(t *testing.T) {
		dpSvc := deploymentpipelinemocks.NewMockService(t)
		dpSvc.EXPECT().
			GetDeploymentPipeline(mock.Anything, testNS, testBindingPipeline).
			Return(pipelineWithEnvs(), nil)

		var seen []*openchoreov1alpha1.ProjectReleaseBinding
		rbSvc := projectreleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			CreateProjectReleaseBinding(mock.Anything, testNS, mock.Anything).
			Run(func(_ context.Context, _ string, rb *openchoreov1alpha1.ProjectReleaseBinding) {
				seen = append(seen, rb)
			}).
			Return(&openchoreov1alpha1.ProjectReleaseBinding{}, nil).
			Times(3)

		h := newTestHandler(
			withProjectService(expectProjectCreated(t)),
			withDeploymentPipelineService(dpSvc),
			withProjectReleaseBindingService(rbSvc),
		)
		result, err := h.CreateProject(ctx, testNS, createProjectReq(), false)
		require.NoError(t, err)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "created", m["action"])
		assert.Equal(t,
			[]string{testProject + "-" + testEnvironmentName, testProject + "-" + testEnvStaging, testProject + "-" + testEnvProd},
			m["releaseBindings"])
		assert.NotContains(t, m, "releaseBindingFailures")

		// Envs are deduped across promotion paths (staging appears as both a target
		// and a source) and preserve promotion order.
		require.Len(t, seen, 3)
		for i, env := range []string{testEnvironmentName, testEnvStaging, testEnvProd} {
			assert.Equal(t, testProject+"-"+env, seen[i].Name)
			assert.Equal(t, testNS, seen[i].Namespace)
			assert.Equal(t, env, seen[i].Spec.Environment)
			assert.Equal(t, testProject, seen[i].Spec.Owner.ProjectName)
			// The pin is seeded by the Project controller once the first release lands.
			assert.Empty(t, seen[i].Spec.ProjectRelease)
		}
	})

	t.Run("skip_bindings creates the project alone", func(t *testing.T) {
		// No pipeline or binding service is wired: creating a binding would nil-panic,
		// proving the skip path never touches them.
		h := newTestHandler(withProjectService(expectProjectCreated(t)))
		result, err := h.CreateProject(ctx, testNS, createProjectReq(), true)
		require.NoError(t, err)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "created", m["action"])
		assert.NotContains(t, m, "releaseBindings")
		assert.Contains(t, m["releaseBindingsNote"], "Skipped by skip_bindings")
	})

	t.Run("partial binding failure keeps the project and reports the failures", func(t *testing.T) {
		dpSvc := deploymentpipelinemocks.NewMockService(t)
		dpSvc.EXPECT().
			GetDeploymentPipeline(mock.Anything, testNS, testBindingPipeline).
			Return(pipelineWithEnvs(), nil)

		rbSvc := projectreleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			CreateProjectReleaseBinding(mock.Anything, testNS,
				mock.MatchedBy(func(rb *openchoreov1alpha1.ProjectReleaseBinding) bool {
					return rb.Spec.Environment != testEnvStaging
				})).
			Return(&openchoreov1alpha1.ProjectReleaseBinding{}, nil).
			Times(2)
		rbSvc.EXPECT().
			CreateProjectReleaseBinding(mock.Anything, testNS,
				mock.MatchedBy(func(rb *openchoreov1alpha1.ProjectReleaseBinding) bool {
					return rb.Spec.Environment == testEnvStaging
				})).
			Return(nil, errors.New("forbidden"))

		h := newTestHandler(
			withProjectService(expectProjectCreated(t)),
			withDeploymentPipelineService(dpSvc),
			withProjectReleaseBindingService(rbSvc),
		)
		result, err := h.CreateProject(ctx, testNS, createProjectReq(), false)
		require.NoError(t, err, "a binding failure must not fail project creation")

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "created", m["action"])
		assert.Equal(t, []string{testProject + "-" + testEnvironmentName, testProject + "-" + testEnvProd}, m["releaseBindings"])

		failures, ok := m["releaseBindingFailures"].([]map[string]any)
		require.True(t, ok)
		require.Len(t, failures, 1)
		assert.Equal(t, testEnvStaging, failures[0]["environment"])
		assert.Equal(t, testProject+"-"+testEnvStaging, failures[0]["name"])
		assert.Equal(t, "forbidden", failures[0]["error"])
		assert.Contains(t, m["releaseBindingsNote"], "retry the failed environments")
	})

	t.Run("unresolvable pipeline keeps the project and explains why", func(t *testing.T) {
		dpSvc := deploymentpipelinemocks.NewMockService(t)
		dpSvc.EXPECT().
			GetDeploymentPipeline(mock.Anything, testNS, testBindingPipeline).
			Return(nil, errors.New("not found"))

		h := newTestHandler(
			withProjectService(expectProjectCreated(t)),
			withDeploymentPipelineService(dpSvc),
		)
		result, err := h.CreateProject(ctx, testNS, createProjectReq(), false)
		require.NoError(t, err)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "created", m["action"])
		assert.NotContains(t, m, "releaseBindings")
		assert.Contains(t, m["releaseBindingsNote"], "failed to resolve deployment pipeline")
	})

	t.Run("pipeline with no environments reports the reason", func(t *testing.T) {
		dpSvc := deploymentpipelinemocks.NewMockService(t)
		dpSvc.EXPECT().
			GetDeploymentPipeline(mock.Anything, testNS, testBindingPipeline).
			Return(&openchoreov1alpha1.DeploymentPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: testBindingPipeline, Namespace: testNS},
			}, nil)

		h := newTestHandler(
			withProjectService(expectProjectCreated(t)),
			withDeploymentPipelineService(dpSvc),
		)
		result, err := h.CreateProject(ctx, testNS, createProjectReq(), false)
		require.NoError(t, err)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.NotContains(t, m, "releaseBindings")
		assert.Contains(t, m["releaseBindingsNote"], "defines no environments")
	})
}

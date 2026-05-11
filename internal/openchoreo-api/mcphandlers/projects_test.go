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
	projectmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project/mocks"
)

func TestCreateProject(t *testing.T) {
	ctx := context.Background()

	makeCreated := func() *openchoreov1alpha1.Project {
		return &openchoreov1alpha1.Project{ObjectMeta: metav1.ObjectMeta{Name: "my-proj", Namespace: testNS}}
	}

	t.Run("happy path with all fields and custom DeploymentPipelineRef Kind", func(t *testing.T) {
		projSvc := projectmocks.NewMockService(t)
		dpKind := gen.ProjectSpecDeploymentPipelineRefKind("DeploymentPipeline")
		displayName := "My Project"
		annotations := map[string]string{"openchoreo.dev/display-name": displayName}
		projSvc.EXPECT().
			CreateProject(mock.Anything, testNS, mock.MatchedBy(func(p *openchoreov1alpha1.Project) bool {
				return p.Name == "my-proj" &&
					p.Namespace == testNS &&
					p.Spec.DeploymentPipelineRef.Name == "my-pipeline" &&
					string(p.Spec.DeploymentPipelineRef.Kind) == "DeploymentPipeline" &&
					p.Annotations["openchoreo.dev/display-name"] == displayName
			})).
			Return(makeCreated(), nil)

		req := &gen.CreateProjectJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: "my-proj", Annotations: &annotations},
			Spec: &gen.ProjectSpec{
				DeploymentPipelineRef: &struct {
					Kind *gen.ProjectSpecDeploymentPipelineRefKind `json:"kind,omitempty"`
					Name string                                    `json:"name"`
				}{
					Kind: &dpKind,
					Name: "my-pipeline",
				},
			},
		}
		h := newTestHandler(withProjectService(projSvc))
		_, err := h.CreateProject(ctx, testNS, req)
		require.NoError(t, err)
	})

	t.Run("default DeploymentPipelineRef Kind when nil", func(t *testing.T) {
		projSvc := projectmocks.NewMockService(t)
		projSvc.EXPECT().
			CreateProject(mock.Anything, testNS, mock.MatchedBy(func(p *openchoreov1alpha1.Project) bool {
				return p.Spec.DeploymentPipelineRef.Kind == openchoreov1alpha1.DeploymentPipelineRefKindDeploymentPipeline
			})).
			Return(makeCreated(), nil)

		req := &gen.CreateProjectJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: "my-proj"},
			Spec: &gen.ProjectSpec{
				DeploymentPipelineRef: &struct {
					Kind *gen.ProjectSpecDeploymentPipelineRefKind `json:"kind,omitempty"`
					Name string                                    `json:"name"`
				}{
					Kind: nil, // no kind — should default
					Name: "my-pipeline",
				},
			},
		}
		h := newTestHandler(withProjectService(projSvc))
		_, err := h.CreateProject(ctx, testNS, req)
		require.NoError(t, err)
	})

	t.Run("nil spec: default DeploymentPipelineRef Kind applied", func(t *testing.T) {
		projSvc := projectmocks.NewMockService(t)
		projSvc.EXPECT().
			CreateProject(mock.Anything, testNS, mock.MatchedBy(func(p *openchoreov1alpha1.Project) bool {
				return p.Spec.DeploymentPipelineRef.Kind == openchoreov1alpha1.DeploymentPipelineRefKindDeploymentPipeline
			})).
			Return(makeCreated(), nil)

		req := &gen.CreateProjectJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: "my-proj"},
			Spec:     nil,
		}
		h := newTestHandler(withProjectService(projSvc))
		_, err := h.CreateProject(ctx, testNS, req)
		require.NoError(t, err)
	})

	t.Run("empty annotation values cleaned", func(t *testing.T) {
		projSvc := projectmocks.NewMockService(t)
		annotations := map[string]string{
			"openchoreo.dev/display-name": "",
			"openchoreo.dev/description":  "",
		}
		projSvc.EXPECT().
			CreateProject(mock.Anything, testNS, mock.MatchedBy(func(p *openchoreov1alpha1.Project) bool {
				_, hasDisplay := p.Annotations["openchoreo.dev/display-name"]
				_, hasDesc := p.Annotations["openchoreo.dev/description"]
				return !hasDisplay && !hasDesc
			})).
			Return(makeCreated(), nil)

		req := &gen.CreateProjectJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: "my-proj", Annotations: &annotations},
		}
		h := newTestHandler(withProjectService(projSvc))
		_, err := h.CreateProject(ctx, testNS, req)
		require.NoError(t, err)
	})

	t.Run("service error propagated", func(t *testing.T) {
		projSvc := projectmocks.NewMockService(t)
		projSvc.EXPECT().CreateProject(mock.Anything, testNS, mock.Anything).Return(nil, errors.New("create failed"))

		req := &gen.CreateProjectJSONRequestBody{Metadata: gen.ObjectMeta{Name: "my-proj"}}
		h := newTestHandler(withProjectService(projSvc))
		_, err := h.CreateProject(ctx, testNS, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create failed")
	})
}

func TestUpdateProject(t *testing.T) {
	ctx := context.Background()

	t.Run("updates deployment pipeline while preserving existing metadata", func(t *testing.T) {
		const (
			projectName = "my-proj"
			oldPipeline = "default"
		)
		newPipeline := "custom-pipeline"
		newDisplayName := "Updated Project"
		newDescription := "Updated project description"

		projSvc := projectmocks.NewMockService(t)
		existing := &openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{
				Name:        projectName,
				Namespace:   testNS,
				Labels:      map[string]string{"team": "integrations"},
				Annotations: map[string]string{"openchoreo.dev/display-name": "My Project", "custom": "keep"},
			},
			Spec: openchoreov1alpha1.ProjectSpec{
				DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
					Kind: openchoreov1alpha1.DeploymentPipelineRefKindDeploymentPipeline,
					Name: oldPipeline,
				},
			},
		}
		updated := existing.DeepCopy()
		updated.Spec.DeploymentPipelineRef.Name = newPipeline

		projSvc.EXPECT().
			GetProject(mock.Anything, testNS, projectName).
			Return(existing, nil)

		var updateReq *openchoreov1alpha1.Project
		projSvc.EXPECT().
			UpdateProject(mock.Anything, testNS, mock.Anything).
			Run(func(ctx context.Context, namespaceName string, project *openchoreov1alpha1.Project) {
				updateReq = project
			}).
			Return(updated, nil)

		h := newTestHandler(withProjectService(projSvc))
		result, err := h.UpdateProject(ctx, testNS, projectName, &gen.PatchProjectRequest{
			DeploymentPipeline: &newPipeline,
			DisplayName:        &newDisplayName,
			Description:        &newDescription,
		})
		require.NoError(t, err)

		require.NotNil(t, updateReq)
		assert.Equal(t, projectName, updateReq.Name)
		assert.Equal(t, testNS, updateReq.Namespace)
		assert.Equal(t, existing.Labels, updateReq.Labels)
		assert.Equal(t, "keep", updateReq.Annotations["custom"])
		assert.Equal(t, newDisplayName, updateReq.Annotations["openchoreo.dev/display-name"])
		assert.Equal(t, newDescription, updateReq.Annotations["openchoreo.dev/description"])
		assert.Equal(t, openchoreov1alpha1.DeploymentPipelineRefKindDeploymentPipeline, updateReq.Spec.DeploymentPipelineRef.Kind)
		assert.Equal(t, newPipeline, updateReq.Spec.DeploymentPipelineRef.Name)

		typed, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "updated", typed["action"])
		assert.Equal(t, projectName, typed["name"])
		assert.Equal(t, newPipeline, typed["deploymentPipelineRef"])
	})

	t.Run("wraps get error", func(t *testing.T) {
		expected := errors.New("get failed")
		projSvc := projectmocks.NewMockService(t)
		projSvc.EXPECT().
			GetProject(mock.Anything, testNS, "my-proj").
			Return(nil, expected)

		h := newTestHandler(withProjectService(projSvc))
		_, err := h.UpdateProject(ctx, testNS, "my-proj", &gen.PatchProjectRequest{})
		require.ErrorIs(t, err, expected)
		assert.Contains(t, err.Error(), "UpdateProject: GetProject namespace=test-ns project=my-proj")
	})

	t.Run("wraps update error", func(t *testing.T) {
		expected := errors.New("update failed")
		project := &openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-proj",
				Namespace: testNS,
			},
		}

		projSvc := projectmocks.NewMockService(t)
		projSvc.EXPECT().
			GetProject(mock.Anything, testNS, "my-proj").
			Return(project, nil)
		projSvc.EXPECT().
			UpdateProject(mock.Anything, testNS, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withProjectService(projSvc))
		_, err := h.UpdateProject(ctx, testNS, "my-proj", nil)
		require.ErrorIs(t, err, expected)
		assert.Contains(t, err.Error(),
			"UpdateProject: UpdateProject namespace=test-ns project=my-proj deploymentPipeline=")
	})
}

func TestDeleteProject(t *testing.T) {
	ctx := context.Background()

	t.Run("delete returns action: deleted", func(t *testing.T) {
		projSvc := projectmocks.NewMockService(t)
		projSvc.EXPECT().DeleteProject(mock.Anything, testNS, "my-proj").Return(nil)

		h := newTestHandler(withProjectService(projSvc))
		result, err := h.DeleteProject(ctx, testNS, "my-proj")
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "deleted", m["action"])
		assert.Equal(t, "my-proj", m["name"])
		assert.Equal(t, testNS, m["namespace"])
	})

	t.Run("service delete error propagated", func(t *testing.T) {
		expected := errors.New("not found")
		projSvc := projectmocks.NewMockService(t)
		projSvc.EXPECT().DeleteProject(mock.Anything, testNS, "my-proj").Return(expected)

		h := newTestHandler(withProjectService(projSvc))
		_, err := h.DeleteProject(ctx, testNS, "my-proj")
		require.ErrorIs(t, err, expected)
	})
}

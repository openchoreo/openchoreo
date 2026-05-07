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

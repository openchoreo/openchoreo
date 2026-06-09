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
)

func TestCreateDeploymentPipeline(t *testing.T) {
	ctx := context.Background()

	makeCreated := func() *openchoreov1alpha1.DeploymentPipeline {
		return &openchoreov1alpha1.DeploymentPipeline{ObjectMeta: metav1.ObjectMeta{Name: "my-dp", Namespace: testNS}}
	}

	t.Run("happy path with promotion paths", func(t *testing.T) {
		dpSvc := deploymentpipelinemocks.NewMockService(t)
		dpSvc.EXPECT().
			CreateDeploymentPipeline(mock.Anything, testNS, mock.MatchedBy(func(dp *openchoreov1alpha1.DeploymentPipeline) bool {
				if len(dp.Spec.PromotionPaths) != 1 {
					return false
				}
				path := dp.Spec.PromotionPaths[0]
				return path.SourceEnvironmentRef.Name == "dev" &&
					len(path.TargetEnvironmentRefs) == 1 &&
					path.TargetEnvironmentRefs[0].Name == "staging"
			})).
			Return(makeCreated(), nil)

		paths := []gen.PromotionPath{
			{
				SourceEnvironmentRef: struct {
					Kind *gen.PromotionPathSourceEnvironmentRefKind `json:"kind,omitempty"`
					Name string                                     `json:"name"`
				}{Name: "dev"},
				TargetEnvironmentRefs: []gen.TargetEnvironmentRef{
					{Name: "staging"},
				},
			},
		}
		req := &gen.CreateDeploymentPipelineJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: "my-dp"},
			Spec:     &gen.DeploymentPipelineSpec{PromotionPaths: &paths},
		}
		h := newTestHandler(withDeploymentPipelineService(dpSvc))
		_, err := h.CreateDeploymentPipeline(ctx, testNS, req)
		require.NoError(t, err)
	})

	t.Run("nil promotion paths", func(t *testing.T) {
		dpSvc := deploymentpipelinemocks.NewMockService(t)
		dpSvc.EXPECT().
			CreateDeploymentPipeline(mock.Anything, testNS, mock.MatchedBy(func(dp *openchoreov1alpha1.DeploymentPipeline) bool {
				return len(dp.Spec.PromotionPaths) == 0
			})).
			Return(makeCreated(), nil)

		req := &gen.CreateDeploymentPipelineJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: "my-dp"},
			Spec:     &gen.DeploymentPipelineSpec{PromotionPaths: nil},
		}
		h := newTestHandler(withDeploymentPipelineService(dpSvc))
		_, err := h.CreateDeploymentPipeline(ctx, testNS, req)
		require.NoError(t, err)
	})

	t.Run("annotation cleanup on empty values", func(t *testing.T) {
		dpSvc := deploymentpipelinemocks.NewMockService(t)
		annotations := map[string]string{
			"openchoreo.dev/display-name": "",
			"openchoreo.dev/description":  "",
		}
		dpSvc.EXPECT().
			CreateDeploymentPipeline(mock.Anything, testNS, mock.MatchedBy(func(dp *openchoreov1alpha1.DeploymentPipeline) bool {
				_, hasDisplay := dp.Annotations["openchoreo.dev/display-name"]
				_, hasDesc := dp.Annotations["openchoreo.dev/description"]
				return !hasDisplay && !hasDesc
			})).
			Return(makeCreated(), nil)

		req := &gen.CreateDeploymentPipelineJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: "my-dp", Annotations: &annotations},
		}
		h := newTestHandler(withDeploymentPipelineService(dpSvc))
		_, err := h.CreateDeploymentPipeline(ctx, testNS, req)
		require.NoError(t, err)
	})
}

func TestUpdateDeploymentPipeline(t *testing.T) {
	ctx := context.Background()

	freshExisting := func() *openchoreov1alpha1.DeploymentPipeline {
		return &openchoreov1alpha1.DeploymentPipeline{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "my-dp",
				Namespace:   testNS,
				Annotations: map[string]string{"existing-key": testExistingVal},
			},
		}
	}

	t.Run("annotations merged and promotion paths replaced", func(t *testing.T) {
		dpSvc := deploymentpipelinemocks.NewMockService(t)
		dpSvc.EXPECT().GetDeploymentPipeline(mock.Anything, testNS, "my-dp").Return(freshExisting(), nil)
		newAnnotations := map[string]string{"new-key": testNewVal}
		paths := []gen.PromotionPath{
			{
				SourceEnvironmentRef: struct {
					Kind *gen.PromotionPathSourceEnvironmentRefKind `json:"kind,omitempty"`
					Name string                                     `json:"name"`
				}{Name: "dev"},
				TargetEnvironmentRefs: []gen.TargetEnvironmentRef{{Name: "prod"}},
			},
		}
		dpSvc.EXPECT().
			UpdateDeploymentPipeline(mock.Anything, testNS, mock.MatchedBy(func(dp *openchoreov1alpha1.DeploymentPipeline) bool {
				return dp.Annotations["existing-key"] == testExistingVal &&
					dp.Annotations["new-key"] == testNewVal &&
					len(dp.Spec.PromotionPaths) == 1 &&
					dp.Spec.PromotionPaths[0].SourceEnvironmentRef.Name == "dev"
			})).
			Return(freshExisting(), nil)

		req := &gen.UpdateDeploymentPipelineJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: "my-dp", Annotations: &newAnnotations},
			Spec:     &gen.DeploymentPipelineSpec{PromotionPaths: &paths},
		}
		h := newTestHandler(withDeploymentPipelineService(dpSvc))
		_, err := h.UpdateDeploymentPipeline(ctx, testNS, req)
		require.NoError(t, err)
	})

	t.Run("GetDeploymentPipeline error propagated", func(t *testing.T) {
		dpSvc := deploymentpipelinemocks.NewMockService(t)
		dpSvc.EXPECT().GetDeploymentPipeline(mock.Anything, testNS, "my-dp").Return(nil, errors.New("not found"))

		req := &gen.UpdateDeploymentPipelineJSONRequestBody{Metadata: gen.ObjectMeta{Name: "my-dp"}}
		h := newTestHandler(withDeploymentPipelineService(dpSvc))
		_, err := h.UpdateDeploymentPipeline(ctx, testNS, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

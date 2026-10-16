// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestReleaseBindingHandlerPromotion(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			ctx := testContext()
			project := &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{Name: "test-proj", Namespace: "test-ns"},
				Spec: openchoreov1alpha1.ProjectSpec{
					DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "default"},
				},
			}
			pipeline := &openchoreov1alpha1.DeploymentPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "test-ns"},
				Spec: openchoreov1alpha1.DeploymentPipelineSpec{PromotionPaths: []openchoreov1alpha1.PromotionPath{
					{SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: "staging"},
						TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{Name: "production"}}},
				}},
			}
			objects := []client.Object{testComponentForRB(), project, pipeline}
			if operation == "update" {
				existing := testReleaseBindingObj("production-rb")
				existing.Spec.Environment = "production"
				existing.Spec.ReleaseName = "old-release"
				objects = append(objects, existing)
			}
			svc := newReleaseBindingService(t, objects, &allowAllPDP{})
			h := newHandlerWithReleaseBindingService(svc)
			body := validReleaseBindingBody("production-rb")
			body.Spec.Environment = "production"
			body.Spec.ReleaseName = ptr.To("release-a")
			if operation == "create" {
				resp, err := h.CreateReleaseBinding(ctx, gen.CreateReleaseBindingRequestObject{
					NamespaceName: "test-ns", Body: body,
				})
				require.NoError(t, err)
				typed, ok := resp.(gen.CreateReleaseBinding400JSONResponse)
				require.True(t, ok, "expected 400, got %T", resp)
				assert.Contains(t, typed.Error, "must be referenced by a ReleaseBinding in staging")
			} else {
				resp, err := h.UpdateReleaseBinding(ctx, gen.UpdateReleaseBindingRequestObject{
					NamespaceName: "test-ns", ReleaseBindingName: "production-rb", Body: body,
				})
				require.NoError(t, err)
				typed, ok := resp.(gen.UpdateReleaseBinding400JSONResponse)
				require.True(t, ok, "expected 400, got %T", resp)
				assert.Contains(t, typed.Error, "must be referenced by a ReleaseBinding in staging")
			}
		})
	}
}

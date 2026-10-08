// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func TestMCPReleaseBindingPromotion(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			ctx := context.Background()
			pipeline := &openchoreov1alpha1.DeploymentPipeline{
				ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: testNS},
				Spec: openchoreov1alpha1.DeploymentPipelineSpec{PromotionPaths: []openchoreov1alpha1.PromotionPath{
					{SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: "staging"},
						TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{Name: "production"}}},
				}},
			}
			objects := []client.Object{
				pipeline, testutil.NewProject(testNS, testProject), testutil.NewComponent(testNS, testProject, testComponent),
			}
			bindingName := testComponent + "-production"
			if operation == "update" {
				existing := testutil.NewReleaseBinding(testNS, testProject, testComponent, "production", bindingName)
				existing.Spec.ReleaseName = "old-release"
				objects = append(objects, existing)
			}
			svc := releasebindingsvc.NewService(testutil.NewFakeClient(objects...), testutil.TestLogger())
			h := newTestHandler(withReleaseBindingService(svc))
			req := &gen.ReleaseBindingSpec{Environment: "production", ReleaseName: ptr.To("release-a")}
			req.Owner.ProjectName = testProject
			req.Owner.ComponentName = testComponent
			var result any
			var err error
			if operation == "create" {
				result, err = h.CreateReleaseBinding(ctx, testNS, req)
			} else {
				result, err = h.UpdateReleaseBinding(ctx, testNS, bindingName, req)
			}
			require.Nil(t, result)
			var validationErr *services.ValidationError
			require.ErrorAs(t, err, &validationErr)
			assert.Contains(t, validationErr.Msg, "must be deployed to staging")
		})
	}
}

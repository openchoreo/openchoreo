// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

var _ = Describe("ReleaseBinding direct hotfix deployments", func() {
	It("renders target creates and updates without a source binding", func() {
		const (
			pipelineName = "hotfix-pipeline"
			projectName  = "hotfix-project"
			component    = "hotfix-component"
			dataPlane    = "hotfix-dataplane"
			sourceEnv    = "hotfix-staging"
			targetEnv    = "hotfix-production"
			bindingName  = "hotfix-production-binding"
			releaseA     = "hotfix-release-a"
			releaseB     = "hotfix-release-b"
		)
		project := projectFixture(projectName)
		project.Spec.DeploymentPipelineRef.Name = pipelineName
		pipeline := &openchoreov1alpha1.DeploymentPipeline{
			ObjectMeta: metav1.ObjectMeta{Name: pipelineName, Namespace: ns},
			Spec: openchoreov1alpha1.DeploymentPipelineSpec{
				PromotionPaths: []openchoreov1alpha1.PromotionPath{{
					SourceEnvironmentRef:  openchoreov1alpha1.EnvironmentRef{Name: sourceEnv},
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{Name: targetEnv}},
				}},
			},
		}
		for _, obj := range []client.Object{
			pipeline, project, componentFixture(component, projectName), dpFixture(dataPlane),
			envFixture(sourceEnv, dataPlane), envFixture(targetEnv, dataPlane),
			crFixture(releaseA, projectName, component), crFixture(releaseB, projectName, component),
		} {
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			DeferCleanup(func() { Expect(k8sClient.Delete(ctx, obj)).To(Succeed()) })
		}
		renderedName := component + "-" + targetEnv
		DeferCleanup(func() {
			forceDelete(bindingName)
			forceDeleteRelease(renderedName)
		})

		r := testReconcilerWithCachedClient()
		Expect(k8sClient.Create(ctx, rbFixture(bindingName, projectName, component, targetEnv, releaseA, true))).To(Succeed())
		for _, release := range []string{releaseA, releaseB} {
			if release == releaseB {
				rb := fetchRB(bindingName)
				rb.Spec.ReleaseName = release
				Expect(k8sClient.Update(ctx, rb)).To(Succeed())
			}
			reconcileUntil(r, bindingName, func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				cond := conditionFor(rb, string(ConditionReleaseSynced))
				return cond != nil && cond.Status == metav1.ConditionTrue && cond.ObservedGeneration == rb.Generation
			})
			rendered := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: renderedName}, rendered)).To(Succeed())
			Expect(rendered.Labels).To(HaveKeyWithValue(labels.LabelKeyComponentReleaseName, release))
		}
	})
})

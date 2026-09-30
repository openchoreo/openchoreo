// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	componentpipeline "github.com/openchoreo/openchoreo/internal/pipeline/component"
)

// namespacedDeploymentTemplate places the Deployment in the data plane namespace
// the pipeline derives, the way real component types do. Delivery events are
// created beside the workload they describe, so they need that namespace.
var namespacedDeploymentTemplate = &runtime.RawExtension{
	Raw: []byte(`{` +
		`"apiVersion":"apps/v1",` +
		`"kind":"Deployment",` +
		`"metadata":{"name":"test-deployment","namespace":"${metadata.namespace}"},` +
		`"spec":{` +
		`"selector":{"matchLabels":{"app":"test"}},` +
		`"template":{` +
		`"metadata":{"labels":{"app":"test"}},` +
		`"spec":{"containers":[{"name":"app","image":"nginx:latest"}]}` +
		`}}}`,
	),
}

// Regression coverage for #4842: a component that declares resource dependencies
// never emitted delivery events. The reconcile returned at the resource-dependency
// guard -- without a requeue -- before it reached reconcileDelivery, although the
// RenderedRelease carrying the rollout had already been written, so the rollout
// went healthy on the data plane and was never counted.
var _ = Describe("ReleaseBinding delivery events", func() {
	Context("when a resource dependency is still pending", func() {
		const (
			rbName   = "rb-delivery-dep"
			crName   = "cr-delivery-dep"
			envName  = "env-delivery-dep"
			dpName   = "dp-delivery-dep"
			compName = "comp-delivery-dep"
			projName = "proj-delivery-dep"
			depRef   = "missing-db"
		)
		releaseName := compName + "-" + envName

		var workloadNamespace string

		AfterEach(func() {
			forceDelete(rbName)
			forceDeleteRelease(releaseName)
			for _, obj := range []client.Object{
				&openchoreov1alpha1.ComponentRelease{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName}},
				&openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName}},
				&openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName}},
				&openchoreov1alpha1.Component{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName}},
				&openchoreov1alpha1.Project{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: projName}},
			} {
				_ = k8sClient.Delete(ctx, obj)
			}
			if workloadNamespace != "" {
				_ = k8sClient.DeleteAllOf(ctx, &corev1.Event{}, client.InNamespace(workloadNamespace))
			}
		})

		It("emits DeploymentStarted and DeploymentSucceeded once the rollout is healthy", func() {
			By("Creating fixtures with a resource dependency that does not exist")
			Expect(k8sClient.Create(ctx, projectFixture(projName))).To(Succeed())
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, componentFixture(compName, projName))).To(Succeed())
			cr := crFixture(crName, projName, compName)
			cr.Spec.ComponentType.Spec.Resources[0].Template = namespacedDeploymentTemplate
			cr.Spec.Workload.Dependencies = &openchoreov1alpha1.WorkloadDependencies{
				Resources: []openchoreov1alpha1.WorkloadResourceDependency{
					{Ref: depRef, EnvBindings: map[string]string{"host": "DB_HOST"}},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, projName, compName, envName, crName, true),
			)).To(Succeed())

			// The envtest API server stands in for the data plane: the events are
			// created in it, alongside the objects the test inspects.
			r := &Reconciler{
				Client:              k8sCachedClient,
				Scheme:              k8sCachedClient.Scheme(),
				Pipeline:            componentpipeline.NewPipeline(),
				PlaneClientProvider: &deliveryPlaneProvider{cl: k8sClient},
			}

			By("Reconciling until the resource-dependency guard engages")
			reconcileUntil(r, rbName, func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				return len(rb.Status.PendingResourceDependencies) > 0
			})

			By("Marking the rendered rollout applied and healthy, as the renderedrelease controller would")
			rendered := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: releaseName}, rendered)).To(Succeed())
			deployment := findDeploymentManifest(rendered)
			workloadNamespace = deployment.Namespace
			Expect(workloadNamespace).NotTo(BeEmpty(), "the rendered Deployment should carry its data plane namespace")
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: workloadNamespace},
			}))).To(Succeed())

			rendered.Status.Resources = []openchoreov1alpha1.RenderedManifestStatus{{
				ID:           "deployment",
				Group:        "apps",
				Version:      "v1",
				Kind:         "Deployment",
				Name:         deployment.Name,
				Namespace:    workloadNamespace,
				HealthStatus: openchoreov1alpha1.HealthStatusHealthy,
			}}
			rendered.Status.Conditions = []metav1.Condition{appliedCondition(rendered.Generation)}
			Expect(k8sClient.Status().Update(ctx, rendered)).To(Succeed())

			By("Reconciling again: the guard still blocks, but the rollout is reported")
			var events []corev1.Event
			Eventually(func(g Gomega) {
				_, err := r.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: ns, Name: rbName},
				})
				g.Expect(err).NotTo(HaveOccurred())

				list := &corev1.EventList{}
				g.Expect(k8sClient.List(ctx, list, client.InNamespace(workloadNamespace))).To(Succeed())
				events = list.Items
				g.Expect(findEventByReason(events, reasonDeploymentStarted)).NotTo(BeNil())
				g.Expect(findEventByReason(events, reasonDeploymentSucceeded)).NotTo(BeNil())
			}, timeout, interval).Should(Succeed())

			rb := &openchoreov1alpha1.ReleaseBinding{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: rbName}, rb)).To(Succeed())
			Expect(rb.Status.PendingResourceDependencies).NotTo(BeEmpty(),
				"the dependency is still missing, so the guard must still have returned early")
			Expect(rb.Status.Delivery).NotTo(BeNil())
			Expect(rb.Status.Delivery.SucceededAt).NotTo(BeNil(), "the emitted phase should be recorded on the binding")
		})
	})
})

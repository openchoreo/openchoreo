// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("ComponentRelease Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		componentrelease := &openchoreov1alpha1.ComponentRelease{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ComponentRelease")
			err := k8sClient.Get(ctx, typeNamespacedName, componentrelease)
			if err != nil && errors.IsNotFound(err) {
				resource := &openchoreov1alpha1.ComponentRelease{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: openchoreov1alpha1.ComponentReleaseSpec{
						Owner: openchoreov1alpha1.ComponentReleaseOwner{
							ProjectName:   "test-project",
							ComponentName: "test-component",
						},
						ComponentType: openchoreov1alpha1.ComponentTypeSpec{
							WorkloadType: "deployment",
							Resources: []openchoreov1alpha1.ResourceTemplate{
								{
									ID: "deployment",
									Template: &runtime.RawExtension{
										Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test"},"spec":{"replicas":1,"selector":{"matchLabels":{"app":"test"}},"template":{"metadata":{"labels":{"app":"test"}},"spec":{"containers":[{"name":"test","image":"nginx"}]}}}}`),
									},
								},
							},
						},
						ComponentProfile: &openchoreov1alpha1.ComponentProfile{
							Parameters: &runtime.RawExtension{
								Raw: []byte(`{"replicas":1,"image":"nginx:latest"}`),
							},
							Traits: []openchoreov1alpha1.ComponentTrait{
								{
									Name:         "test-trait",
									InstanceName: "test-instance",
									Parameters: &runtime.RawExtension{
										Raw: []byte(`{"minReplicas":2,"maxReplicas":10}`),
									},
								},
							},
						},
						Traits: map[string]openchoreov1alpha1.TraitSpec{
							"test-instance": {
								Schema: openchoreov1alpha1.TraitSchema{
									Parameters: &runtime.RawExtension{
										Raw: []byte(`{"minReplicas":{"type":"integer"},"maxReplicas":{"type":"integer"}}`),
									},
								},
								Creates: []openchoreov1alpha1.TraitCreate{
									{
										Template: &runtime.RawExtension{
											Raw: []byte(`{"apiVersion":"autoscaling/v2","kind":"HorizontalPodAutoscaler","metadata":{"name":"test-hpa"},"spec":{"minReplicas":2,"maxReplicas":10}}`),
										},
									},
								},
							},
						},
						Workload: openchoreov1alpha1.WorkloadTemplateSpec{
							Container: openchoreov1alpha1.Container{
								Image:   "nginx:latest",
								Command: []string{"/bin/sh"},
								Args:    []string{"-c", "nginx -g 'daemon off;'"},
							},
							Endpoints: map[string]openchoreov1alpha1.WorkloadEndpoint{
								"http": {
									Type: openchoreov1alpha1.EndpointTypeHTTP,
									Port: 8080,
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Cleanup logic after each test, like removing the resource instance.
			resource := &openchoreov1alpha1.ComponentRelease{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ComponentRelease")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})

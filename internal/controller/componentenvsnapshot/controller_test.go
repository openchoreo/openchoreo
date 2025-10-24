// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentenvsnapshot

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("ComponentEnvSnapshot Controller", func() {
	Context("When reconciling a ComponentEnvSnapshot resource", func() {
		const snapshotName = "test-componentenvsnapshot"
		const namespace = "default"

		snapshotNamespacedName := types.NamespacedName{
			Name:      snapshotName,
			Namespace: namespace,
		}

		It("should successfully reconcile the resource", func() {
			By("Creating the ComponentEnvSnapshot resource")
			snapshot := &openchoreov1alpha1.ComponentEnvSnapshot{}
			err := k8sClient.Get(ctx, snapshotNamespacedName, snapshot)
			if err != nil && errors.IsNotFound(err) {
				snapshot = &openchoreov1alpha1.ComponentEnvSnapshot{
					ObjectMeta: metav1.ObjectMeta{
						Name:      snapshotName,
						Namespace: namespace,
					},
					Spec: openchoreov1alpha1.ComponentEnvSnapshotSpec{
						Owner: openchoreov1alpha1.ComponentEnvSnapshotOwner{
							ProjectName:   "test-project",
							ComponentName: "test-component",
						},
						Environment: "dev",
						ComponentTypeDefinition: openchoreov1alpha1.ComponentTypeDefinition{
							ObjectMeta: metav1.ObjectMeta{
								Name: "test-ctd",
							},
							Spec: openchoreov1alpha1.ComponentTypeDefinitionSpec{
								WorkloadType: "deployment",
								Resources: []openchoreov1alpha1.ResourceTemplate{
									{
										ID: "deployment",
										Template: runtime.RawExtension{
											Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test"},"spec":{"replicas":1,"selector":{"matchLabels":{"app":"test"}},"template":{"metadata":{"labels":{"app":"test"}},"spec":{"containers":[{"name":"test","image":"nginx"}]}}}}`),
										},
									},
								},
							},
						},
						Component: openchoreov1alpha1.Component{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-component",
								Namespace: namespace,
							},
							Spec: openchoreov1alpha1.ComponentSpec{
								Owner: openchoreov1alpha1.ComponentOwner{
									ProjectName: "test-project",
								},
								ComponentType: "deployment/test-ctd",
							},
						},
						Workload: openchoreov1alpha1.Workload{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-component",
								Namespace: namespace,
							},
							Spec: openchoreov1alpha1.WorkloadSpec{
								Owner: openchoreov1alpha1.WorkloadOwner{
									ProjectName:   "test-project",
									ComponentName: "test-component",
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, snapshot)).To(Succeed())
			}

			By("Reconciling the ComponentEnvSnapshot resource")
			snapshotReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err = snapshotReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: snapshotNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the ComponentEnvSnapshot resource exists")
			Eventually(func() error {
				return k8sClient.Get(ctx, snapshotNamespacedName, snapshot)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())

			By("Cleaning up the ComponentEnvSnapshot resource")
			Expect(k8sClient.Delete(ctx, snapshot)).To(Succeed())
		})
	})
})

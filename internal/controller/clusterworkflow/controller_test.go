// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflow

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("ClusterWorkflow Controller", func() {
	const timeout = time.Second * 10
	const interval = time.Millisecond * 500

	createClusterWorkflow := func(name string) *openchoreov1alpha1.ClusterWorkflow {
		return &openchoreov1alpha1.ClusterWorkflow{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: openchoreov1alpha1.ClusterWorkflowSpec{
				RunTemplate: &runtime.RawExtension{
					Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"generateName":"test-"}}`),
				},
			},
		}
	}

	cleanupClusterWorkflow := func(name string) {
		cw := &openchoreov1alpha1.ClusterWorkflow{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, cw)
		if err == nil {
			Expect(k8sClient.Delete(ctx, cw)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, cw)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		}
	}

	Context("Basic Reconciliation", func() {
		const cwName = "test-reconcile-basic"

		AfterEach(func() {
			cleanupClusterWorkflow(cwName)
		})

		It("should successfully reconcile a ClusterWorkflow", func() {
			cw := createClusterWorkflow(cwName)
			Expect(k8sClient.Create(ctx, cw)).To(Succeed())

			reconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cwName},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: cwName}, cw)
			}, timeout, interval).Should(Succeed())
		})

		It("should setup with manager successfully", func() {
			mgr, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme: k8sClient.Scheme(),
				Metrics: metricsserver.Options{
					BindAddress: "0",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			reconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			Expect(reconciler.SetupWithManager(mgr)).To(Succeed())
		})

		It("should return no error for a non-existent ClusterWorkflow", func() {
			reconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "non-existent-clusterworkflow"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})

	Context("Cluster-Scoped Behavior", func() {
		It("should be accessible without namespace", func() {
			const name = "test-cluster-scoped"
			defer cleanupClusterWorkflow(name)

			cw := createClusterWorkflow(name)
			Expect(k8sClient.Create(ctx, cw)).To(Succeed())

			fetched := &openchoreov1alpha1.ClusterWorkflow{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, fetched)).To(Succeed())
			Expect(fetched.Namespace).To(BeEmpty())
		})
	})
})

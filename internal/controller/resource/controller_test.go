// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("Resource Controller", func() {
	Context("type resolution", func() {
		var reconciler *Reconciler

		BeforeEach(func() {
			reconciler = &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		It("sets Ready=False, Reason=ResourceTypeNotFound when the referenced ResourceType does not exist", func() {
			res := &openchoreov1alpha1.Resource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "stage1-missing-type",
					Namespace: "default",
				},
				Spec: openchoreov1alpha1.ResourceSpec{
					Owner: openchoreov1alpha1.ResourceOwner{ProjectName: "test-project"},
					Type: openchoreov1alpha1.ResourceTypeRef{
						Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
						Name: "non-existent-type",
					},
				},
			}
			Expect(k8sClient.Create(ctx, res)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, res)
			})

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(res),
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &openchoreov1alpha1.Resource{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(res), updated)).To(Succeed())

			cond := meta.FindStatusCondition(updated.Status.Conditions, "Ready")
			Expect(cond).NotTo(BeNil(), "expected Ready condition to be set")
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("ResourceTypeNotFound"))
		})

		It("does not set Ready=False when the namespaced ResourceType exists (release creation lands later)", func() {
			rt := &openchoreov1alpha1.ResourceType{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "stage1-mysql",
					Namespace: "default",
				},
				Spec: openchoreov1alpha1.ResourceTypeSpec{
					Resources: []openchoreov1alpha1.ResourceTypeManifest{
						{
							ID: "claim",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"x"}}`),
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, rt)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, rt)
			})

			res := &openchoreov1alpha1.Resource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "stage1-resolves-namespaced",
					Namespace: "default",
				},
				Spec: openchoreov1alpha1.ResourceSpec{
					Owner: openchoreov1alpha1.ResourceOwner{ProjectName: "test-project"},
					Type: openchoreov1alpha1.ResourceTypeRef{
						Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
						Name: rt.Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, res)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, res)
			})

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(res),
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &openchoreov1alpha1.Resource{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(res), updated)).To(Succeed())

			cond := meta.FindStatusCondition(updated.Status.Conditions, "Ready")
			if cond != nil {
				Expect(cond.Status).NotTo(Equal(metav1.ConditionFalse),
					"Ready=False not expected when type resolves; got Reason=%s", cond.Reason)
			}
		})

		It("resolves spec.type.kind=ClusterResourceType against the cluster-scoped sibling without setting Ready=False", func() {
			crt := &openchoreov1alpha1.ClusterResourceType{
				ObjectMeta: metav1.ObjectMeta{
					Name: "stage1-cluster-mysql",
				},
				Spec: openchoreov1alpha1.ClusterResourceTypeSpec{
					Resources: []openchoreov1alpha1.ResourceTypeManifest{
						{
							ID: "claim",
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"x"}}`),
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, crt)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, crt)
			})

			res := &openchoreov1alpha1.Resource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "stage1-resolves-cluster",
					Namespace: "default",
				},
				Spec: openchoreov1alpha1.ResourceSpec{
					Owner: openchoreov1alpha1.ResourceOwner{ProjectName: "test-project"},
					Type: openchoreov1alpha1.ResourceTypeRef{
						Kind: openchoreov1alpha1.ResourceTypeRefKindClusterResourceType,
						Name: crt.Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, res)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, res)
			})

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(res),
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &openchoreov1alpha1.Resource{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(res), updated)).To(Succeed())

			cond := meta.FindStatusCondition(updated.Status.Conditions, "Ready")
			if cond != nil {
				Expect(cond.Status).NotTo(Equal(metav1.ConditionFalse),
					"Ready=False not expected when ClusterResourceType resolves; got Reason=%s", cond.Reason)
			}
		})
	})
})

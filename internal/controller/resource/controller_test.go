// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
					Name:       "stage1-missing-type",
					Namespace:  "default",
					Finalizers: []string{ResourceFinalizer},
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
					Name:       "stage1-resolves-namespaced",
					Namespace:  "default",
					Finalizers: []string{ResourceFinalizer},
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

		It("resolves spec.type.kind=ClusterResourceType against the cluster-scoped sibling without setting Ready=False (release creation lands later)", func() {
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
					Name:       "stage1-resolves-cluster",
					Namespace:  "default",
					Finalizers: []string{ResourceFinalizer},
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

	Context("ResourceRelease creation", func() {
		var reconciler *Reconciler

		BeforeEach(func() {
			reconciler = &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		// cleanupReleases removes every ResourceRelease in the namespace so each
		// spec starts from a clean slate. The Resource finalizer will eventually
		// do this cascade automatically; until then, tests are responsible.
		cleanupReleases := func(namespace string) {
			releases := &openchoreov1alpha1.ResourceReleaseList{}
			_ = k8sClient.List(ctx, releases, client.InNamespace(namespace))
			for i := range releases.Items {
				_ = k8sClient.Delete(ctx, &releases.Items[i])
			}
		}

		newResourceType := func(name string) *openchoreov1alpha1.ResourceType {
			rt := &openchoreov1alpha1.ResourceType{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
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
			// Refetch so server-applied defaults (RetainPolicy: Delete) are visible.
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rt), rt)).To(Succeed())
			return rt
		}

		newResource := func(name, rtName string, params *runtime.RawExtension) *openchoreov1alpha1.Resource {
			res := &openchoreov1alpha1.Resource{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  "default",
					Finalizers: []string{ResourceFinalizer},
				},
				Spec: openchoreov1alpha1.ResourceSpec{
					Owner: openchoreov1alpha1.ResourceOwner{ProjectName: "test-project"},
					Type: openchoreov1alpha1.ResourceTypeRef{
						Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
						Name: rtName,
					},
					Parameters: params,
				},
			}
			Expect(k8sClient.Create(ctx, res)).To(Succeed())
			return res
		}

		It("creates a ResourceRelease with the right shape and sets Ready=True, Reason=Reconciled", func() {
			rt := newResourceType("stage2-mysql")
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rt) })

			res := newResource("stage2-create", rt.Name, &runtime.RawExtension{Raw: []byte(`{"version":"8.0"}`)})
			DeferCleanup(func() {
				cleanupReleases("default")
				_ = k8sClient.Delete(ctx, res)
			})

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(res)})
			Expect(err).NotTo(HaveOccurred())

			updated := &openchoreov1alpha1.Resource{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(res), updated)).To(Succeed())

			By("populating Resource.status.latestRelease")
			Expect(updated.Status.LatestRelease).NotTo(BeNil())
			Expect(updated.Status.LatestRelease.Name).NotTo(BeEmpty())
			Expect(updated.Status.LatestRelease.Hash).NotTo(BeEmpty())
			Expect(updated.Status.LatestRelease.Name).To(Equal(fmt.Sprintf("%s-%s", res.Name, updated.Status.LatestRelease.Hash)))

			By("creating a ResourceRelease with the right shape")
			rr := &openchoreov1alpha1.ResourceRelease{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: updated.Status.LatestRelease.Name, Namespace: "default"}, rr)).To(Succeed())
			Expect(rr.Spec.Owner).To(Equal(openchoreov1alpha1.ResourceReleaseOwner{
				ProjectName:  "test-project",
				ResourceName: res.Name,
			}))
			Expect(rr.Spec.ResourceType.Kind).To(Equal(openchoreov1alpha1.ResourceTypeRefKindResourceType))
			Expect(rr.Spec.ResourceType.Name).To(Equal(rt.Name))
			Expect(rr.Spec.ResourceType.Spec).To(Equal(rt.Spec))
			Expect(rr.Spec.Parameters).NotTo(BeNil())
			Expect(rr.Spec.Parameters.Raw).To(Equal(res.Spec.Parameters.Raw))

			By("setting Ready=True, Reason=Reconciled")
			cond := meta.FindStatusCondition(updated.Status.Conditions, "Ready")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("Reconciled"))
		})

		It("is idempotent — reconciling twice does not create a duplicate ResourceRelease", func() {
			rt := newResourceType("stage2-idemp-mysql")
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rt) })

			res := newResource("stage2-idemp", rt.Name, &runtime.RawExtension{Raw: []byte(`{"version":"8.0"}`)})
			DeferCleanup(func() {
				cleanupReleases("default")
				_ = k8sClient.Delete(ctx, res)
			})

			req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(res)}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			releases := &openchoreov1alpha1.ResourceReleaseList{}
			Expect(k8sClient.List(ctx, releases, client.InNamespace("default"))).To(Succeed())

			ownedCount := 0
			for _, rr := range releases.Items {
				if rr.Spec.Owner.ResourceName == res.Name {
					ownedCount++
				}
			}
			Expect(ownedCount).To(Equal(1), "expected exactly one ResourceRelease owned by the Resource")
		})

		It("cuts a new ResourceRelease when spec.parameters changes; both old and new exist", func() {
			rt := newResourceType("stage2-edit-mysql")
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rt) })

			res := newResource("stage2-edit", rt.Name, &runtime.RawExtension{Raw: []byte(`{"version":"8.0"}`)})
			DeferCleanup(func() {
				cleanupReleases("default")
				_ = k8sClient.Delete(ctx, res)
			})

			req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(res)}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			first := &openchoreov1alpha1.Resource{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(res), first)).To(Succeed())
			firstReleaseName := first.Status.LatestRelease.Name
			firstHash := first.Status.LatestRelease.Hash

			By("editing Resource.spec.parameters")
			first.Spec.Parameters = &runtime.RawExtension{Raw: []byte(`{"version":"8.4"}`)}
			Expect(k8sClient.Update(ctx, first)).To(Succeed())

			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			second := &openchoreov1alpha1.Resource{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(res), second)).To(Succeed())
			secondReleaseName := second.Status.LatestRelease.Name
			secondHash := second.Status.LatestRelease.Hash

			Expect(secondHash).NotTo(Equal(firstHash))
			Expect(secondReleaseName).NotTo(Equal(firstReleaseName))

			By("preserving both releases (immutable, never overwritten)")
			oldRR := &openchoreov1alpha1.ResourceRelease{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: firstReleaseName, Namespace: "default"}, oldRR)).To(Succeed())
			newRR := &openchoreov1alpha1.ResourceRelease{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: secondReleaseName, Namespace: "default"}, newRR)).To(Succeed())
		})
	})
})

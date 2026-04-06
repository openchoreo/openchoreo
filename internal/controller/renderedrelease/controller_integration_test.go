// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// forceDelete removes the DataPlaneCleanupFinalizer from a RenderedRelease and deletes it.
// Safe to call even if the resource does not exist.
func forceDelete(ctx context.Context, nn types.NamespacedName) {
	r := &openchoreov1alpha1.RenderedRelease{}
	if err := k8sClient.Get(ctx, nn, r); err != nil {
		return
	}
	if controllerutil.ContainsFinalizer(r, DataPlaneCleanupFinalizer) {
		controllerutil.RemoveFinalizer(r, DataPlaneCleanupFinalizer)
		_ = k8sClient.Update(ctx, r)
	}
	_ = k8sClient.Delete(ctx, r)
}

// makeMinimalRelease returns a RenderedRelease with the minimum required spec fields.
func makeMinimalRelease(name, envName string) *openchoreov1alpha1.RenderedRelease {
	return &openchoreov1alpha1.RenderedRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: openchoreov1alpha1.RenderedReleaseSpec{
			Owner: openchoreov1alpha1.RenderedReleaseOwner{
				ProjectName:   "test-project",
				ComponentName: "test-component",
			},
			EnvironmentName: envName,
		},
	}
}

var _ = Describe("RenderedRelease Controller", func() {

	// ─────────────────────────────────────────────────────────────
	// Non-existent resource
	// ─────────────────────────────────────────────────────────────

	Context("when the RenderedRelease resource does not exist", func() {
		It("should return no error and not requeue", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "ghost-release", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	// ─────────────────────────────────────────────────────────────
	// New release — first reconcile adds finalizer
	// ─────────────────────────────────────────────────────────────

	Context("when a new RenderedRelease is created", func() {
		const releaseName = "release-first-reconcile"
		nn := types.NamespacedName{Name: releaseName, Namespace: "default"}

		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, makeMinimalRelease(releaseName, "my-env"))).To(Succeed())
		})

		AfterEach(func() {
			forceDelete(ctx, nn)
		})

		It("should add DataPlaneCleanupFinalizer on first reconcile", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			// First reconcile returns early after adding the finalizer — no requeue
			Expect(result.Requeue).To(BeFalse())

			got := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(got, DataPlaneCleanupFinalizer)).To(BeTrue())
		})

		It("should return error on second reconcile when environment does not exist", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			By("first reconcile: adds finalizer")
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			By("second reconcile: tries to get environment, which is missing")
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("my-env"))
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Deleted release — finalization flow
	// ─────────────────────────────────────────────────────────────

	Context("when a RenderedRelease with a finalizer is being deleted", func() {
		const releaseName = "release-finalizing"
		nn := types.NamespacedName{Name: releaseName, Namespace: "default"}

		BeforeEach(func() {
			By("creating release with pre-set finalizer")
			release := makeMinimalRelease(releaseName, "my-env")
			release.Finalizers = []string{DataPlaneCleanupFinalizer}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())

			By("deleting release (sets DeletionTimestamp, finalizer blocks removal)")
			Expect(k8sClient.Delete(ctx, release)).To(Succeed())
		})

		AfterEach(func() {
			forceDelete(ctx, nn)
		})

		It("should set Finalizing condition on first finalize reconcile", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			// Condition is set and status updated; returns empty Result (no requeue)
			Expect(result.Requeue).To(BeFalse())

			got := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())

			cond := apimeta.FindStatusCondition(got.Status.Conditions, string(ConditionFinalizing))
			Expect(cond).NotTo(BeNil(), "Finalizing condition should be set")
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(ReasonCleanupInProgress)))
		})

		It("should return error on second finalize reconcile when environment does not exist", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			By("first reconcile: sets Finalizing condition")
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			By("second reconcile: Finalizing condition already set, tries to get DP client")
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("my-env"))
		})

		It("should keep the finalizer while cleanup is pending", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			// After one reconcile the finalizer should still be present (cleanup didn't succeed)
			_, _ = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})

			got := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, nn, got)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(got, DataPlaneCleanupFinalizer)).To(BeTrue())
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Deleted release — no finalizer (immediate deletion)
	// ─────────────────────────────────────────────────────────────

	Context("when a RenderedRelease without a finalizer is deleted", func() {
		It("should return no error (resource already gone)", func() {
			const releaseName = "release-no-finalizer"
			nn := types.NamespacedName{Name: releaseName, Namespace: "default"}

			release := makeMinimalRelease(releaseName, "my-env")
			// Explicitly no Finalizers
			Expect(k8sClient.Create(ctx, release)).To(Succeed())
			Expect(k8sClient.Delete(ctx, release)).To(Succeed())

			// Without a finalizer the API server removes the object immediately
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nn, &openchoreov1alpha1.RenderedRelease{})
				return apierrors.IsNotFound(err)
			}, "5s", "100ms").Should(BeTrue())

			// Reconcile on a missing resource should be a no-op
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Deleted release — finalizer absent, DeletionTimestamp present
	// ─────────────────────────────────────────────────────────────

	Context("when a RenderedRelease has a DeletionTimestamp but no finalizer", func() {
		const releaseName = "release-del-no-finalizer"
		nn := types.NamespacedName{Name: releaseName, Namespace: "default"}

		BeforeEach(func() {
			By("creating with finalizer so we can control DeletionTimestamp")
			release := makeMinimalRelease(releaseName, "env-x")
			release.Finalizers = []string{DataPlaneCleanupFinalizer}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())

			By("deleting to set DeletionTimestamp")
			Expect(k8sClient.Delete(ctx, release)).To(Succeed())

			By("stripping the finalizer so finalize() returns early")
			fetched := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			controllerutil.RemoveFinalizer(fetched, DataPlaneCleanupFinalizer)
			Expect(k8sClient.Update(ctx, fetched)).To(Succeed())
		})

		AfterEach(func() {
			forceDelete(ctx, nn)
		})

		It("should return no error when finalizer is absent during finalization", func() {
			// After removing the finalizer the API server may already delete the object;
			// either way reconcile should succeed without error.
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ─────────────────────────────────────────────────────────────
	// Status persistence
	// ─────────────────────────────────────────────────────────────

	Context("when status is updated on a Release", func() {
		const releaseName = "release-status-persist"
		nn := types.NamespacedName{Name: releaseName, Namespace: "default"}

		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, makeMinimalRelease(releaseName, "env-status"))).To(Succeed())
		})

		AfterEach(func() {
			forceDelete(ctx, nn)
		})

		It("should persist status conditions via status subresource", func() {
			By("fetching release")
			release := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, nn, release)).To(Succeed())

			By("updating status with a condition")
			release.Status.Conditions = []metav1.Condition{
				{
					Type:               "TestCondition",
					Status:             metav1.ConditionTrue,
					Reason:             "TestReason",
					Message:            "test message",
					LastTransitionTime: metav1.Now(),
					ObservedGeneration: release.Generation,
				},
			}
			Expect(k8sClient.Status().Update(ctx, release)).To(Succeed())

			By("re-fetching and verifying condition persisted")
			fetched := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			cond := apimeta.FindStatusCondition(fetched.Status.Conditions, "TestCondition")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("TestReason"))
		})

		It("should persist resource inventory in status", func() {
			By("fetching release")
			release := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, nn, release)).To(Succeed())

			By("writing resource status entries")
			release.Status.Resources = []openchoreov1alpha1.ResourceStatus{
				{
					ID:           "res-1",
					Group:        "apps",
					Version:      "v1",
					Kind:         "Deployment",
					Name:         "my-deploy",
					Namespace:    "dp-ns",
					HealthStatus: openchoreov1alpha1.HealthStatusHealthy,
				},
			}
			Expect(k8sClient.Status().Update(ctx, release)).To(Succeed())

			By("re-fetching and verifying resources persisted")
			fetched := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
			Expect(fetched.Status.Resources).To(HaveLen(1))
			Expect(fetched.Status.Resources[0].ID).To(Equal("res-1"))
			Expect(fetched.Status.Resources[0].HealthStatus).To(Equal(openchoreov1alpha1.HealthStatusHealthy))
		})
	})
})

// ─────────────────────────────────────────────────────────────
// ensureNamespaces
// ─────────────────────────────────────────────────────────────

var _ = Describe("ensureNamespaces", func() {
	makeNS := func(name string) *corev1.Namespace {
		return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	}

	deleteNS := func(name string) {
		ns := &corev1.Namespace{}
		ns.Name = name
		_ = k8sClient.Delete(ctx, ns)
	}

	Context("with an empty namespace list", func() {
		It("should be a no-op", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			Expect(r.ensureNamespaces(ctx, k8sClient, nil)).To(Succeed())
		})
	})

	Context("when namespace does not exist", func() {
		const nsName = "test-ensure-ns-new"

		AfterEach(func() { deleteNS(nsName) })

		It("should create the namespace", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			Expect(r.ensureNamespaces(ctx, k8sClient, []*corev1.Namespace{makeNS(nsName)})).To(Succeed())

			existing := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, existing)).To(Succeed())
		})
	})

	Context("when namespace already exists", func() {
		const nsName = "test-ensure-ns-exists"

		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, makeNS(nsName))).To(Succeed())
		})
		AfterEach(func() { deleteNS(nsName) })

		It("should not return an error", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			Expect(r.ensureNamespaces(ctx, k8sClient, []*corev1.Namespace{makeNS(nsName)})).To(Succeed())
		})
	})

	Context("when multiple namespaces are provided", func() {
		nsNames := []string{"test-multi-ns-a", "test-multi-ns-b", "test-multi-ns-c"}

		AfterEach(func() {
			for _, name := range nsNames {
				deleteNS(name)
			}
		})

		It("should create all namespaces", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			nsList := make([]*corev1.Namespace, len(nsNames))
			for i, name := range nsNames {
				nsList[i] = makeNS(name)
			}
			Expect(r.ensureNamespaces(ctx, k8sClient, nsList)).To(Succeed())

			for _, name := range nsNames {
				existing := &corev1.Namespace{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, existing)).To(Succeed())
			}
		})
	})
})

// ─────────────────────────────────────────────────────────────
// applyResources and deleteResources
// ─────────────────────────────────────────────────────────────

var _ = Describe("applyResources and deleteResources", func() {
	const testNS = "default"
	configMapGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}

	makeTrackedCM := func(name, resourceID, releaseUID string) *unstructured.Unstructured {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(configMapGVK)
		obj.SetName(name)
		obj.SetNamespace(testNS)
		obj.SetLabels(map[string]string{
			labels.LabelKeyManagedBy:                 ControllerName,
			labels.LabelKeyRenderedReleaseResourceID: resourceID,
			labels.LabelKeyRenderedReleaseUID:        releaseUID,
			labels.LabelKeyRenderedReleaseName:       "test-release",
			labels.LabelKeyRenderedReleaseNamespace:  testNS,
		})
		return obj
	}

	deleteCM := func(name string) {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(configMapGVK)
		obj.SetName(name)
		obj.SetNamespace(testNS)
		_ = k8sClient.Delete(ctx, obj)
	}

	Context("with an empty resource list", func() {
		It("applyResources should be a no-op", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			Expect(r.applyResources(ctx, k8sClient, nil)).To(Succeed())
		})

		It("deleteResources should be a no-op", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			Expect(r.deleteResources(ctx, k8sClient, nil)).To(Succeed())
		})
	})

	Context("when applying a ConfigMap resource", func() {
		const cmName = "test-apply-cm"
		const resourceID = "apply-res-1"
		const releaseUID = "apply-uid-1"

		AfterEach(func() { deleteCM(cmName) })

		It("should apply the resource with tracking labels", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			obj := makeTrackedCM(cmName, resourceID, releaseUID)
			Expect(r.applyResources(ctx, k8sClient, []*unstructured.Unstructured{obj})).To(Succeed())

			existing := &unstructured.Unstructured{}
			existing.SetGroupVersionKind(configMapGVK)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: testNS}, existing)).To(Succeed())
			Expect(existing.GetLabels()[labels.LabelKeyRenderedReleaseResourceID]).To(Equal(resourceID))
		})

		It("should be idempotent when applied twice", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			obj := makeTrackedCM(cmName, resourceID, releaseUID)
			Expect(r.applyResources(ctx, k8sClient, []*unstructured.Unstructured{obj})).To(Succeed())
			obj2 := makeTrackedCM(cmName, resourceID, releaseUID)
			Expect(r.applyResources(ctx, k8sClient, []*unstructured.Unstructured{obj2})).To(Succeed())
		})
	})

	Context("when deleting a previously applied resource", func() {
		const cmName = "test-delete-cm"
		const resourceID = "delete-res-1"
		const releaseUID = "delete-uid-1"

		BeforeEach(func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			obj := makeTrackedCM(cmName, resourceID, releaseUID)
			Expect(r.applyResources(ctx, k8sClient, []*unstructured.Unstructured{obj})).To(Succeed())
		})

		AfterEach(func() { deleteCM(cmName) })

		It("should delete the resource", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			obj := makeTrackedCM(cmName, resourceID, releaseUID)
			Expect(r.deleteResources(ctx, k8sClient, []*unstructured.Unstructured{obj})).To(Succeed())

			existing := &unstructured.Unstructured{}
			existing.SetGroupVersionKind(configMapGVK)
			err := k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: testNS}, existing)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})
})

// ─────────────────────────────────────────────────────────────
// listLiveResourcesByGVKs
// ─────────────────────────────────────────────────────────────

var _ = Describe("listLiveResourcesByGVKs", func() {
	const testNS = "default"
	configMapGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}

	makeTrackedCM := func(name, releaseUID string) *unstructured.Unstructured {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(configMapGVK)
		obj.SetName(name)
		obj.SetNamespace(testNS)
		obj.SetLabels(map[string]string{
			labels.LabelKeyManagedBy:          ControllerName,
			labels.LabelKeyRenderedReleaseUID: releaseUID,
		})
		return obj
	}

	deleteCM := func(name string) {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(configMapGVK)
		obj.SetName(name)
		obj.SetNamespace(testNS)
		_ = k8sClient.Delete(ctx, obj)
	}

	makeRelease := func(uid string) *openchoreov1alpha1.RenderedRelease {
		r := &openchoreov1alpha1.RenderedRelease{}
		r.UID = types.UID(uid)
		return r
	}

	Context("when no resources match the label selector", func() {
		It("should return an empty list", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			release := makeRelease("nonexistent-uid-99999")
			result, err := r.listLiveResourcesByGVKs(ctx, k8sClient, release, []schema.GroupVersionKind{configMapGVK})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})

	Context("when resources with matching labels exist", func() {
		const releaseUID = "list-live-uid-match"
		const cmName = "test-list-cm-match"

		BeforeEach(func() {
			obj := makeTrackedCM(cmName, releaseUID)
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
		})
		AfterEach(func() { deleteCM(cmName) })

		It("should find the matching resources", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			release := makeRelease(releaseUID)
			result, err := r.listLiveResourcesByGVKs(ctx, k8sClient, release, []schema.GroupVersionKind{configMapGVK})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(1))
			Expect(result[0].GetName()).To(Equal(cmName))
		})
	})

	Context("when resources without matching labels exist", func() {
		const cmName = "test-list-cm-nomatch"

		BeforeEach(func() {
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(configMapGVK)
			obj.SetName(cmName)
			obj.SetNamespace(testNS)
			// No tracking labels
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
		})
		AfterEach(func() { deleteCM(cmName) })

		It("should exclude resources without matching labels", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			release := makeRelease("uid-for-nomatch-test")
			result, err := r.listLiveResourcesByGVKs(ctx, k8sClient, release, []schema.GroupVersionKind{configMapGVK})
			Expect(err).NotTo(HaveOccurred())
			for _, res := range result {
				Expect(res.GetName()).NotTo(Equal(cmName))
			}
		})
	})

	Context("when an unknown GVK is queried", func() {
		It("should continue without returning an error", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			unknownGVK := schema.GroupVersionKind{Group: "unknown.example.com", Version: "v1", Kind: "NonExistentResource"}
			release := makeRelease("some-uid-unknown-gvk")
			result, err := r.listLiveResourcesByGVKs(ctx, k8sClient, release, []schema.GroupVersionKind{unknownGVK})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})

	Context("when list fails for non-NoMatch reasons", func() {
		It("should return an error", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			cancelledCtx, cancel := context.WithCancel(ctx)
			cancel()
			release := makeRelease("uid-list-failure")
			_, err := r.listLiveResourcesByGVKs(cancelledCtx, k8sClient, release, []schema.GroupVersionKind{configMapGVK})
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when multiple GVKs are queried", func() {
		const releaseUID = "list-live-uid-multi"
		const cmName = "test-list-cm-multi"

		BeforeEach(func() {
			obj := makeTrackedCM(cmName, releaseUID)
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
		})
		AfterEach(func() { deleteCM(cmName) })

		It("should collect resources from each GVK", func() {
			r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			release := makeRelease(releaseUID)
			serviceGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}
			result, err := r.listLiveResourcesByGVKs(ctx, k8sClient, release, []schema.GroupVersionKind{configMapGVK, serviceGVK})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(1))
			Expect(result[0].GetName()).To(Equal(cmName))
		})
	})
})

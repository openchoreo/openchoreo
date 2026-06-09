// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

var _ = Describe("Finalizer", func() {
	finCtx := context.Background()

	newRes := func(name string, withFinalizer bool) *openchoreov1alpha1.Resource {
		om := metav1.ObjectMeta{Name: name, Namespace: "default"}
		if withFinalizer {
			om.Finalizers = []string{ResourceFinalizer}
		}
		return &openchoreov1alpha1.Resource{
			ObjectMeta: om,
			Spec: openchoreov1alpha1.ResourceSpec{
				Owner: openchoreov1alpha1.ResourceOwner{ProjectName: "p"},
				Type: openchoreov1alpha1.ResourceTypeRef{
					Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
					Name: "rt",
				},
			},
		}
	}

	newRelease := func(name, ownerResource string) *openchoreov1alpha1.ResourceRelease {
		return &openchoreov1alpha1.ResourceRelease{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: openchoreov1alpha1.ResourceReleaseSpec{
				Owner: openchoreov1alpha1.ResourceReleaseOwner{
					ProjectName:  "p",
					ResourceName: ownerResource,
				},
				ResourceType: openchoreov1alpha1.ResourceReleaseResourceType{
					Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
					Name: "rt",
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
				},
			},
		}
	}

	newBinding := func(name, ownerResource string) *openchoreov1alpha1.ResourceReleaseBinding {
		return &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
					ProjectName:  "p",
					ResourceName: ownerResource,
				},
				Environment: "dev",
			},
		}
	}

	buildClient := func(objs ...client.Object) client.Client {
		return fake.NewClientBuilder().
			WithScheme(scheme.Scheme).
			WithObjects(objs...).
			WithStatusSubresource(&openchoreov1alpha1.Resource{}).
			WithIndex(&openchoreov1alpha1.ResourceRelease{},
				controller.IndexKeyResourceReleaseOwnerResourceName,
				controller.IndexResourceReleaseOwner).
			WithIndex(&openchoreov1alpha1.ResourceReleaseBinding{},
				controller.IndexKeyResourceReleaseBindingOwnerResourceName,
				controller.IndexResourceReleaseBindingOwner).
			Build()
	}

	It("adds the resource-cleanup finalizer on first reconcile", func() {
		res := newRes("fin-add", false)
		cli := buildClient(res)
		r := &Reconciler{Client: cli, Scheme: scheme.Scheme}

		_, err := r.Reconcile(finCtx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(res)})
		Expect(err).NotTo(HaveOccurred())

		updated := &openchoreov1alpha1.Resource{}
		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(res), updated)).To(Succeed())
		Expect(updated.Finalizers).To(ContainElement(ResourceFinalizer))
	})

	It("holds the finalizer while ResourceReleaseBindings reference the Resource", func() {
		res := newRes("fin-block", true)
		binding := newBinding("fin-block-binding", res.Name)
		cli := buildClient(res, binding)
		r := &Reconciler{Client: cli, Scheme: scheme.Scheme}

		Expect(cli.Delete(finCtx, res)).To(Succeed())
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(res)}

		// 1st reconcile: finalize branch, sets Finalizing condition, returns.
		_, err := r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())

		// 2nd reconcile: binding still present, requeue without acting.
		result, err := r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))

		updated := &openchoreov1alpha1.Resource{}
		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(res), updated)).To(Succeed())
		Expect(updated.DeletionTimestamp).NotTo(BeNil())
		Expect(updated.Finalizers).To(ContainElement(ResourceFinalizer))

		cond := meta.FindStatusCondition(updated.Status.Conditions, "Finalizing")
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	})

	It("is a no-op when the resource-cleanup finalizer is not present (already removed elsewhere)", func() {
		// Post-cleanup state: DeletionTimestamp set, ResourceFinalizer gone,
		// another finalizer holding the object so it hasn't been GC'd yet.
		now := metav1.Now()
		res := newRes("fin-noop", false)
		res.DeletionTimestamp = &now
		res.Finalizers = []string{"other.example.com/protection"}

		cli := buildClient()
		r := &Reconciler{Client: cli, Scheme: scheme.Scheme}

		result, err := r.finalize(finCtx, res.DeepCopy(), res)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())
		Expect(meta.FindStatusCondition(res.Status.Conditions, "Finalizing")).To(BeNil(),
			"no Finalizing condition expected when our finalizer is absent")
	})

	It("cascade-deletes ResourceReleases and clears the finalizer once bindings are gone", func() {
		res := newRes("fin-cascade", true)
		release := newRelease("fin-cascade-r1", res.Name)
		cli := buildClient(res, release)
		r := &Reconciler{Client: cli, Scheme: scheme.Scheme}

		Expect(cli.Delete(finCtx, res)).To(Succeed())
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(res)}

		// 1st: sets Finalizing condition, returns.
		_, err := r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())
		// 2nd: no bindings present, deletes the release, requeues.
		_, err = r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())
		// 3rd: no releases left, clears the finalizer.
		_, err = r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())

		// Resource is gone (fake client GCs once last finalizer is removed and DeletionTimestamp is set).
		err = cli.Get(finCtx, client.ObjectKeyFromObject(res), &openchoreov1alpha1.Resource{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Resource should have been GC'd after finalizer cleared")

		err = cli.Get(finCtx, client.ObjectKeyFromObject(release), &openchoreov1alpha1.ResourceRelease{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "owned ResourceRelease should be deleted")
	})
})

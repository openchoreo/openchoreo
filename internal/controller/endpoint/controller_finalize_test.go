// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

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

	newEp := func(name string, withFinalizer bool) *openchoreov1alpha1.Endpoint {
		ep := newEndpoint(name, "p", clusterTypeRef("t"), componentBackend("stable", "greeter", "http", 1))
		if !withFinalizer {
			ep.Finalizers = nil
		}
		return ep
	}

	newRelease := func(name, ownerEndpoint string) *openchoreov1alpha1.EndpointRelease {
		return &openchoreov1alpha1.EndpointRelease{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: openchoreov1alpha1.EndpointReleaseSpec{
				Owner: openchoreov1alpha1.EndpointReleaseOwner{ProjectName: "p", EndpointName: ownerEndpoint},
				EndpointType: openchoreov1alpha1.EndpointReleaseEndpointType{
					Kind: openchoreov1alpha1.EndpointTypeRefKindClusterEndpointType,
					Name: "t",
					Spec: openchoreov1alpha1.EndpointTypeSpec{
						Resources: []openchoreov1alpha1.ResourceTemplate{{
							ID:       "route",
							Template: &runtime.RawExtension{Raw: []byte(routeTemplate)},
						}},
					},
				},
				Visibility:  []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityExternal},
				BackendRefs: []openchoreov1alpha1.EndpointBackendRef{componentBackend("stable", "greeter", "http", 1)},
			},
		}
	}

	newBinding := func(name, ownerEndpoint string, withFinalizer bool) *openchoreov1alpha1.EndpointReleaseBinding {
		om := metav1.ObjectMeta{Name: name, Namespace: "default"}
		if withFinalizer {
			om.Finalizers = []string{"test-keep"}
		}
		return &openchoreov1alpha1.EndpointReleaseBinding{
			ObjectMeta: om,
			Spec: openchoreov1alpha1.EndpointReleaseBindingSpec{
				Owner:       openchoreov1alpha1.EndpointReleaseBindingOwner{ProjectName: "p", EndpointName: ownerEndpoint},
				Environment: "dev",
			},
		}
	}

	buildClient := func(objs ...client.Object) client.Client {
		return fake.NewClientBuilder().
			WithScheme(scheme.Scheme).
			WithObjects(objs...).
			WithStatusSubresource(&openchoreov1alpha1.Endpoint{}).
			WithIndex(&openchoreov1alpha1.EndpointRelease{},
				controller.IndexKeyEndpointReleaseOwnerEndpointName,
				controller.IndexEndpointReleaseOwner).
			WithIndex(&openchoreov1alpha1.EndpointReleaseBinding{},
				controller.IndexKeyEndpointReleaseBindingOwnerEndpointName,
				controller.IndexEndpointReleaseBindingOwner).
			Build()
	}

	It("adds the endpoint-cleanup finalizer on first reconcile", func() {
		ep := newEp("fin-add", false)
		cli := buildClient(ep)
		r := &Reconciler{Client: cli, Scheme: scheme.Scheme}

		_, err := r.Reconcile(finCtx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(ep)})
		Expect(err).NotTo(HaveOccurred())

		updated := &openchoreov1alpha1.Endpoint{}
		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(ep), updated)).To(Succeed())
		Expect(updated.Finalizers).To(ContainElement(EndpointFinalizer))
	})

	It("deletes EndpointReleaseBindings and holds the finalizer until they are gone", func() {
		ep := newEp("fin-block", true)
		binding := newBinding("fin-block-binding", ep.Name, true)
		other := newBinding("fin-block-other", "another-endpoint", false)
		cli := buildClient(ep, binding, other)
		r := &Reconciler{Client: cli, Scheme: scheme.Scheme}

		Expect(cli.Delete(finCtx, ep)).To(Succeed())
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(ep)}

		// 1st reconcile: finalize branch, sets Finalizing condition, returns.
		_, err := r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())

		// 2nd reconcile: deletes the binding, which its own finalizer holds; requeue.
		result, err := r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))

		updated := &openchoreov1alpha1.Endpoint{}
		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(ep), updated)).To(Succeed())
		Expect(updated.Finalizers).To(ContainElement(EndpointFinalizer))
		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionFinalizing))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Message).To(Equal("Waiting for EndpointReleaseBindings to be deleted"))

		updatedBinding := &openchoreov1alpha1.EndpointReleaseBinding{}
		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(binding), updatedBinding)).To(Succeed())
		Expect(updatedBinding.DeletionTimestamp).NotTo(BeNil())

		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(other), &openchoreov1alpha1.EndpointReleaseBinding{})).
			To(Succeed(), "a binding owned by another Endpoint must be left alone")
	})

	It("is a no-op when the endpoint-cleanup finalizer is not present (already removed elsewhere)", func() {
		now := metav1.Now()
		ep := newEp("fin-noop", false)
		ep.DeletionTimestamp = &now
		ep.Finalizers = []string{"other.example.com/protection"}

		cli := buildClient()
		r := &Reconciler{Client: cli, Scheme: scheme.Scheme}

		result, err := r.finalize(finCtx, ep.DeepCopy(), ep)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())
		Expect(meta.FindStatusCondition(ep.Status.Conditions, string(ConditionFinalizing))).To(BeNil())
	})

	It("cascade-deletes EndpointReleases and clears the finalizer once bindings are gone", func() {
		ep := newEp("fin-cascade", true)
		release := newRelease("fin-cascade-r1", ep.Name)
		other := newRelease("fin-cascade-other", "another-endpoint")
		cli := buildClient(ep, release, other)
		r := &Reconciler{Client: cli, Scheme: scheme.Scheme}

		Expect(cli.Delete(finCtx, ep)).To(Succeed())
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(ep)}

		// 1st: sets Finalizing condition. 2nd: deletes the release. 3rd: clears the finalizer.
		for range 3 {
			_, err := r.Reconcile(finCtx, req)
			Expect(err).NotTo(HaveOccurred())
		}

		err := cli.Get(finCtx, client.ObjectKeyFromObject(ep), &openchoreov1alpha1.Endpoint{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Endpoint should have been GC'd after finalizer cleared")

		err = cli.Get(finCtx, client.ObjectKeyFromObject(release), &openchoreov1alpha1.EndpointRelease{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "owned EndpointRelease should be deleted")

		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(other), &openchoreov1alpha1.EndpointRelease{})).
			To(Succeed(), "a release owned by another Endpoint must be left alone")
	})
})

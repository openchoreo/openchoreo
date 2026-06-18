// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

var _ = Describe("ProjectReleaseBinding controller — finalize", func() {
	finCtx := context.Background()

	newBinding := func(name string, withFinalizer bool) *openchoreov1alpha1.ProjectReleaseBinding {
		om := metav1.ObjectMeta{Name: name, Namespace: "default"}
		if withFinalizer {
			om.Finalizers = []string{ProjectReleaseBindingFinalizer}
		}
		return &openchoreov1alpha1.ProjectReleaseBinding{
			ObjectMeta: om,
			Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
				Owner: openchoreov1alpha1.ProjectReleaseBindingOwner{
					ProjectName: "p",
				},
				Environment:    "dev",
				ProjectRelease: name + "-release",
			},
		}
	}

	// newRenderedRelease produces an RR with the same name the controller
	// derives from {ProjectName=p, Environment=dev} via makeRenderedReleaseName,
	// which is what every binding in this suite uses.
	newRenderedRelease := func() *openchoreov1alpha1.RenderedRelease {
		return &openchoreov1alpha1.RenderedRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dpkubernetes.GenerateK8sName("p_p", "dev"),
				Namespace: "default",
			},
			Spec: openchoreov1alpha1.RenderedReleaseSpec{
				Owner: openchoreov1alpha1.RenderedReleaseOwner{
					ProjectName: "p",
				},
				EnvironmentName: "dev",
				TargetPlane:     openchoreov1alpha1.TargetPlaneDataPlane,
			},
		}
	}

	buildClient := func(objs ...client.Object) client.Client {
		return fake.NewClientBuilder().
			WithScheme(scheme.Scheme).
			WithObjects(objs...).
			WithStatusSubresource(&openchoreov1alpha1.ProjectReleaseBinding{}).
			Build()
	}

	newReconciler := func(cli client.Client) *Reconciler {
		return &Reconciler{
			Client: cli,
			Scheme: scheme.Scheme,
		}
	}

	It("cascades the RenderedRelease delete and clears the finalizer", func() {
		b := newBinding("delete-binding", true)
		rr := newRenderedRelease()
		Expect(controllerutil.SetControllerReference(b, rr, scheme.Scheme)).To(Succeed())
		cli := buildClient(b, rr)
		r := newReconciler(cli)

		Expect(cli.Delete(finCtx, b)).To(Succeed())
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)}

		// 1st reconcile: set Finalizing condition, return.
		_, err := r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())

		updated := &openchoreov1alpha1.ProjectReleaseBinding{}
		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(b), updated)).To(Succeed())
		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionFinalizing))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))

		// 2nd reconcile: cascade-delete the RR, requeue.
		result, err := r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))

		// 3rd reconcile: RR is gone, finalizer cleared, binding GC'd.
		_, err = r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())

		err = cli.Get(finCtx, client.ObjectKeyFromObject(b), &openchoreov1alpha1.ProjectReleaseBinding{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "binding should be GC'd after finalizer is cleared")

		err = cli.Get(finCtx, client.ObjectKeyFromObject(rr), &openchoreov1alpha1.RenderedRelease{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "RenderedRelease should be deleted")
	})

	It("clears the finalizer cleanly when the RenderedRelease is already absent", func() {
		b := newBinding("no-rr-binding", true)
		// No RenderedRelease in the fake client.
		cli := buildClient(b)
		r := newReconciler(cli)

		Expect(cli.Delete(finCtx, b)).To(Succeed())
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)}

		// 1st: Finalizing condition. 2nd: RR not found, finalizer cleared.
		_, err := r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())
		_, err = r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())

		err = cli.Get(finCtx, client.ObjectKeyFromObject(b), &openchoreov1alpha1.ProjectReleaseBinding{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("is a no-op when the projectreleasebinding-cleanup finalizer is not present", func() {
		// Post-cleanup state: binding has DeletionTimestamp, our finalizer
		// is gone, but a stranger finalizer is still holding it open.
		now := metav1.Now()
		b := newBinding("noop-binding", false)
		b.DeletionTimestamp = &now
		b.Finalizers = []string{"other.example.com/protection"}

		cli := buildClient()
		r := newReconciler(cli)

		result, err := r.finalize(finCtx, b.DeepCopy(), b)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())
		Expect(meta.FindStatusCondition(b.Status.Conditions, string(ConditionFinalizing))).To(BeNil(),
			"no Finalizing condition expected when our finalizer is absent")
	})

	It("ensureFinalizer is a no-op once the binding is being deleted", func() {
		now := metav1.Now()
		b := newBinding("ef-deleting", false)
		b.DeletionTimestamp = &now
		r := newReconciler(buildClient())

		added, err := r.ensureFinalizer(finCtx, b)
		Expect(err).NotTo(HaveOccurred())
		Expect(added).To(BeFalse())
		Expect(b.Finalizers).NotTo(ContainElement(ProjectReleaseBindingFinalizer))
	})

	It("ensureFinalizer is a no-op when the finalizer is already present", func() {
		b := newBinding("ef-present", true)
		r := newReconciler(buildClient(b))

		added, err := r.ensureFinalizer(finCtx, b)
		Expect(err).NotTo(HaveOccurred())
		Expect(added).To(BeFalse())
	})

	It("requeues without re-deleting when the RenderedRelease is already terminating", func() {
		b := newBinding("rr-terminating", true)
		rr := newRenderedRelease()
		// A residual finalizer keeps the RR around after Delete, so it lingers
		// with a DeletionTimestamp — the state the binding should wait on.
		rr.Finalizers = []string{"keep.example.com/hold"}
		Expect(controllerutil.SetControllerReference(b, rr, scheme.Scheme)).To(Succeed())
		cli := buildClient(b, rr)
		r := newReconciler(cli)

		Expect(cli.Delete(finCtx, rr)).To(Succeed())
		Expect(cli.Delete(finCtx, b)).To(Succeed())
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)}

		_, err := r.Reconcile(finCtx, req) // 1st: Finalizing condition
		Expect(err).NotTo(HaveOccurred())
		result, err := r.Reconcile(finCtx, req) // 2nd: RR present + terminating → requeue
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))

		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(rr), &openchoreov1alpha1.RenderedRelease{})).To(Succeed())
		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(b), &openchoreov1alpha1.ProjectReleaseBinding{})).To(Succeed(),
			"binding finalizer should be retained while its RenderedRelease lingers")
	})

	It("propagates a transient error getting the RenderedRelease", func() {
		b := newBinding("rr-get-err", true)
		transientErr := errors.New("simulated transient API error")
		cli := fake.NewClientBuilder().
			WithScheme(scheme.Scheme).
			WithObjects(b).
			WithStatusSubresource(&openchoreov1alpha1.ProjectReleaseBinding{}).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*openchoreov1alpha1.RenderedRelease); ok {
						return transientErr
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}).
			Build()
		r := newReconciler(cli)

		Expect(cli.Delete(finCtx, b)).To(Succeed())
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)}

		_, err := r.Reconcile(finCtx, req) // 1st: Finalizing condition
		Expect(err).NotTo(HaveOccurred())
		_, err = r.Reconcile(finCtx, req) // 2nd: RR Get fails → error propagates
		Expect(err).To(HaveOccurred())

		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(b), &openchoreov1alpha1.ProjectReleaseBinding{})).To(Succeed(),
			"binding finalizer must be retained when the RR lookup fails")
	})

	It("propagates an error deleting the RenderedRelease", func() {
		b := newBinding("rr-del-err", true)
		rr := newRenderedRelease()
		Expect(controllerutil.SetControllerReference(b, rr, scheme.Scheme)).To(Succeed())
		cli := fake.NewClientBuilder().
			WithScheme(scheme.Scheme).
			WithObjects(b, rr).
			WithStatusSubresource(&openchoreov1alpha1.ProjectReleaseBinding{}).
			WithInterceptorFuncs(interceptor.Funcs{
				Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					if _, ok := obj.(*openchoreov1alpha1.RenderedRelease); ok {
						return errors.New("simulated delete error")
					}
					return c.Delete(ctx, obj, opts...)
				},
			}).
			Build()
		r := newReconciler(cli)

		Expect(cli.Delete(finCtx, b)).To(Succeed())
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)}

		_, err := r.Reconcile(finCtx, req) // 1st: Finalizing condition
		Expect(err).NotTo(HaveOccurred())
		_, err = r.Reconcile(finCtx, req) // 2nd: RR Delete fails → error propagates
		Expect(err).To(HaveOccurred())
	})

	It("propagates an error removing the finalizer", func() {
		b := newBinding("rmfin-err", true)
		// No RenderedRelease → finalize falls through to the finalizer removal.
		cli := fake.NewClientBuilder().
			WithScheme(scheme.Scheme).
			WithObjects(b).
			WithStatusSubresource(&openchoreov1alpha1.ProjectReleaseBinding{}).
			WithInterceptorFuncs(interceptor.Funcs{
				Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
					if _, ok := obj.(*openchoreov1alpha1.ProjectReleaseBinding); ok {
						return errors.New("simulated update error")
					}
					return c.Update(ctx, obj, opts...)
				},
			}).
			Build()
		r := newReconciler(cli)

		Expect(cli.Delete(finCtx, b)).To(Succeed())
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)}

		_, err := r.Reconcile(finCtx, req) // 1st: Finalizing condition (status update, not intercepted)
		Expect(err).NotTo(HaveOccurred())
		_, err = r.Reconcile(finCtx, req) // 2nd: RR absent → remove finalizer Update fails
		Expect(err).To(HaveOccurred())
	})
})

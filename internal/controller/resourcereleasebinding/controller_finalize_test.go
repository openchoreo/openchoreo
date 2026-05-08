// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	resourcepipeline "github.com/openchoreo/openchoreo/internal/pipeline/resource"
)

var _ = Describe("ResourceReleaseBinding controller — finalize", func() {
	finCtx := context.Background()

	newBinding := func(name string, withFinalizer bool, retain openchoreov1alpha1.ResourceRetainPolicy) *openchoreov1alpha1.ResourceReleaseBinding {
		om := metav1.ObjectMeta{Name: name, Namespace: "default"}
		if withFinalizer {
			om.Finalizers = []string{ResourceReleaseBindingFinalizer}
		}
		return &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: om,
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
					ProjectName:  "p",
					ResourceName: "r",
				},
				Environment:     "dev",
				ResourceRelease: name + "-release",
				RetainPolicy:    retain,
			},
		}
	}

	newSnapshot := func(name string, retain openchoreov1alpha1.ResourceRetainPolicy) *openchoreov1alpha1.ResourceRelease {
		return &openchoreov1alpha1.ResourceRelease{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: openchoreov1alpha1.ResourceReleaseSpec{
				Owner: openchoreov1alpha1.ResourceReleaseOwner{ProjectName: "p", ResourceName: "r"},
				ResourceType: openchoreov1alpha1.ResourceReleaseResourceType{
					Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
					Name: "rt",
					Spec: openchoreov1alpha1.ResourceTypeSpec{
						RetainPolicy: retain,
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

	// newRenderedRelease produces an RR with the same name the controller
	// derives from {ResourceName=r, Environment=dev} via
	// makeRenderedReleaseName, which is what every binding in this suite uses.
	newRenderedRelease := func() *openchoreov1alpha1.RenderedRelease {
		return &openchoreov1alpha1.RenderedRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dpkubernetes.GenerateK8sName("r_r", "dev"),
				Namespace: "default",
			},
			Spec: openchoreov1alpha1.RenderedReleaseSpec{
				Owner: openchoreov1alpha1.RenderedReleaseOwner{
					ProjectName:  "p",
					ResourceName: "r",
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
			WithStatusSubresource(&openchoreov1alpha1.ResourceReleaseBinding{}).
			Build()
	}

	newReconciler := func(cli client.Client) *Reconciler {
		return &Reconciler{
			Client:   cli,
			Scheme:   scheme.Scheme,
			Pipeline: resourcepipeline.NewPipeline(),
		}
	}

	It("cascades the RenderedRelease delete and clears the finalizer when retainPolicy=Delete", func() {
		b := newBinding("delete-binding", true, openchoreov1alpha1.ResourceRetainPolicyDelete)
		rr := newRenderedRelease()
		Expect(controllerutil.SetControllerReference(b, rr, scheme.Scheme)).To(Succeed())
		cli := buildClient(b, rr)
		r := newReconciler(cli)

		Expect(cli.Delete(finCtx, b)).To(Succeed())
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)}

		// 1st reconcile: set Finalizing condition, return.
		_, err := r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())

		updated := &openchoreov1alpha1.ResourceReleaseBinding{}
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

		err = cli.Get(finCtx, client.ObjectKeyFromObject(b), &openchoreov1alpha1.ResourceReleaseBinding{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "binding should be GC'd after finalizer is cleared")

		err = cli.Get(finCtx, client.ObjectKeyFromObject(rr), &openchoreov1alpha1.RenderedRelease{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "RenderedRelease should be deleted")

		// snapshot is unused but we keep it referenced so the helper is exercised
		// the same way 4.3-4.5 will use it.
		_ = newSnapshot
	})

	It("does not silently fall through to Delete when the snapshot lookup hits a transient API error", func() {
		// Binding pins a release with no retainPolicy override; effective
		// resolution would normally read the snapshot's default. We inject
		// a transient (non-NotFound) error on the snapshot Get so the
		// controller must requeue rather than treat the missing answer as
		// "Delete".
		b := newBinding("transient-binding", true, "")
		snap := newSnapshot(b.Spec.ResourceRelease, openchoreov1alpha1.ResourceRetainPolicyRetain)
		rr := newRenderedRelease()
		Expect(controllerutil.SetControllerReference(b, rr, scheme.Scheme)).To(Succeed())

		transientErr := errors.New("simulated transient API error")
		cli := fake.NewClientBuilder().
			WithScheme(scheme.Scheme).
			WithObjects(b, snap, rr).
			WithStatusSubresource(&openchoreov1alpha1.ResourceReleaseBinding{}).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*openchoreov1alpha1.ResourceRelease); ok {
						return transientErr
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}).
			Build()
		r := newReconciler(cli)

		Expect(cli.Delete(finCtx, b)).To(Succeed())
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)}

		// 1st reconcile sets Finalizing. 2nd reconcile hits the transient
		// error; we expect the error to propagate so controller-runtime
		// requeues with backoff.
		_, err := r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())
		_, err = r.Reconcile(finCtx, req)
		Expect(err).To(HaveOccurred(), "transient snapshot Get error must propagate")

		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(rr), &openchoreov1alpha1.RenderedRelease{})).To(Succeed(),
			"transient errors must not silently cascade the RenderedRelease delete")
	})

	It("clears the finalizer cleanly when the RenderedRelease is already absent", func() {
		b := newBinding("no-rr-binding", true, openchoreov1alpha1.ResourceRetainPolicyDelete)
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

		err = cli.Get(finCtx, client.ObjectKeyFromObject(b), &openchoreov1alpha1.ResourceReleaseBinding{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("is a no-op when the resourcereleasebinding-cleanup finalizer is not present", func() {
		// Post-cleanup state: binding has DeletionTimestamp, our finalizer
		// is gone, but a stranger finalizer is still holding it open.
		now := metav1.Now()
		b := newBinding("noop-binding", false, openchoreov1alpha1.ResourceRetainPolicyDelete)
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

	It("respects a retainPolicy flip from Retain to Delete after deletion is in flight", func() {
		b := newBinding("flip-binding", true, openchoreov1alpha1.ResourceRetainPolicyRetain)
		rr := newRenderedRelease()
		Expect(controllerutil.SetControllerReference(b, rr, scheme.Scheme)).To(Succeed())
		cli := buildClient(b, rr)
		r := newReconciler(cli)

		Expect(cli.Delete(finCtx, b)).To(Succeed())
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)}

		// Initial reconciles under Retain: binding stays put.
		_, err := r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())
		_, err = r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())

		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(b), &openchoreov1alpha1.ResourceReleaseBinding{})).To(Succeed())

		By("flipping retainPolicy to Delete on the live binding")
		current := &openchoreov1alpha1.ResourceReleaseBinding{}
		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(b), current)).To(Succeed())
		current.Spec.RetainPolicy = openchoreov1alpha1.ResourceRetainPolicyDelete
		Expect(cli.Update(finCtx, current)).To(Succeed())

		// Three more reconciles: condition reset (RetainHold → Finalizing),
		// cascade-delete RR, clear finalizer.
		for i := 0; i < 3; i++ {
			_, err = r.Reconcile(finCtx, req)
			Expect(err).NotTo(HaveOccurred())
		}

		err = cli.Get(finCtx, client.ObjectKeyFromObject(b), &openchoreov1alpha1.ResourceReleaseBinding{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
		err = cli.Get(finCtx, client.ObjectKeyFromObject(rr), &openchoreov1alpha1.RenderedRelease{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("falls back to the snapshot's retainPolicy when the binding does not override it (Retain)", func() {
		b := newBinding("snapshot-retain-binding", true, "")
		snap := newSnapshot(b.Spec.ResourceRelease, openchoreov1alpha1.ResourceRetainPolicyRetain)
		rr := newRenderedRelease()
		Expect(controllerutil.SetControllerReference(b, rr, scheme.Scheme)).To(Succeed())
		cli := buildClient(b, snap, rr)
		r := newReconciler(cli)

		Expect(cli.Delete(finCtx, b)).To(Succeed())
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)}

		_, err := r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())
		_, err = r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())

		updated := &openchoreov1alpha1.ResourceReleaseBinding{}
		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(b), updated)).To(Succeed())
		Expect(updated.Finalizers).To(ContainElement(ResourceReleaseBindingFinalizer))
		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(rr), &openchoreov1alpha1.RenderedRelease{})).To(Succeed())
	})

	It("binding-level Retain overrides snapshot Delete", func() {
		b := newBinding("override-retain-binding", true, openchoreov1alpha1.ResourceRetainPolicyRetain)
		snap := newSnapshot(b.Spec.ResourceRelease, openchoreov1alpha1.ResourceRetainPolicyDelete)
		rr := newRenderedRelease()
		Expect(controllerutil.SetControllerReference(b, rr, scheme.Scheme)).To(Succeed())
		cli := buildClient(b, snap, rr)
		r := newReconciler(cli)

		Expect(cli.Delete(finCtx, b)).To(Succeed())
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)}

		_, err := r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())
		_, err = r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())

		updated := &openchoreov1alpha1.ResourceReleaseBinding{}
		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(b), updated)).To(Succeed(),
			"Retain on the binding wins over Delete on the snapshot")
		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(rr), &openchoreov1alpha1.RenderedRelease{})).To(Succeed())
	})

	It("binding-level Delete overrides snapshot Retain", func() {
		b := newBinding("override-delete-binding", true, openchoreov1alpha1.ResourceRetainPolicyDelete)
		snap := newSnapshot(b.Spec.ResourceRelease, openchoreov1alpha1.ResourceRetainPolicyRetain)
		rr := newRenderedRelease()
		Expect(controllerutil.SetControllerReference(b, rr, scheme.Scheme)).To(Succeed())
		cli := buildClient(b, snap, rr)
		r := newReconciler(cli)

		Expect(cli.Delete(finCtx, b)).To(Succeed())
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)}

		// Three reconciles: Finalizing → cascade → clear.
		for i := 0; i < 3; i++ {
			_, err := r.Reconcile(finCtx, req)
			Expect(err).NotTo(HaveOccurred())
		}

		err := cli.Get(finCtx, client.ObjectKeyFromObject(b), &openchoreov1alpha1.ResourceReleaseBinding{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(),
			"Delete on the binding wins over Retain on the snapshot; binding GC'd")
		err = cli.Get(finCtx, client.ObjectKeyFromObject(rr), &openchoreov1alpha1.RenderedRelease{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("blocks finalizer clearance when retainPolicy=Retain (binding override)", func() {
		b := newBinding("retain-binding", true, openchoreov1alpha1.ResourceRetainPolicyRetain)
		rr := newRenderedRelease()
		Expect(controllerutil.SetControllerReference(b, rr, scheme.Scheme)).To(Succeed())
		cli := buildClient(b, rr)
		r := newReconciler(cli)

		Expect(cli.Delete(finCtx, b)).To(Succeed())
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)}

		// 1st reconcile: set Finalizing condition.
		_, err := r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())

		// 2nd reconcile: Retain → flip Reason to RetainHold, exit.
		result, err := r.Reconcile(finCtx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero(), "Retain holds the finalizer; no requeue needed")

		updated := &openchoreov1alpha1.ResourceReleaseBinding{}
		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(b), updated)).To(Succeed(),
			"binding should still exist (finalizer holds it)")
		Expect(updated.DeletionTimestamp).NotTo(BeNil())
		Expect(updated.Finalizers).To(ContainElement(ResourceReleaseBindingFinalizer))

		cond := meta.FindStatusCondition(updated.Status.Conditions, string(ConditionFinalizing))
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(string(ReasonRetainHold)))

		Expect(cli.Get(finCtx, client.ObjectKeyFromObject(rr), &openchoreov1alpha1.RenderedRelease{})).To(Succeed(),
			"RenderedRelease must remain when retainPolicy=Retain")
	})
})

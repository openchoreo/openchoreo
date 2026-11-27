// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// settleTime is how long a spec waits, after a binding reaches a NotFound reason, for the
// reconciles it set off itself to finish. Its own status write is an update event on
// the binding, which triggers one more reconcile. If that reconcile were still pending
// when the spec creates the missing object, it would see the object without any watch.
const settleTime = time.Second

// The other specs call Reconcile by hand, so they never exercise the watches that
// SetupWithManager registers. These start the real controller and only create
// objects, leaving it to the watches to wake the binding.
var _ = Describe("ReleaseBinding controller watches", func() {
	// startController runs the controller on a fresh manager until the spec ends.
	// A fresh manager has its own cache, so registering the shared and controller
	// field indexes again does not conflict with the suite's manager.
	startController := func() {
		GinkgoHelper()
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:     k8sClient.Scheme(),
			Metrics:    metricsserver.Options{BindAddress: "0"},
			Controller: config.Controller{SkipNameValidation: ptr.To(true)},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(controller.SetupSharedIndexes(ctx, mgr)).To(Succeed())

		r := &Reconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}
		Expect(r.SetupWithManager(mgr)).To(Succeed())

		mgrCtx, stop := context.WithCancel(ctx)
		stopped := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			defer close(stopped)
			Expect(mgr.Start(mgrCtx)).To(Succeed())
		}()
		// DeferCleanup runs last-in, first-out. Callers register their object cleanup
		// before calling this, so the controller stops before the objects are deleted.
		// Otherwise it could put the finalizer back while forceDelete is removing it.
		DeferCleanup(func() {
			stop()
			<-stopped
		})
	}

	syncedReason := func(name string) func() string {
		return func() string {
			rb := &openchoreov1alpha1.ReleaseBinding{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, rb); err != nil {
				return ""
			}
			if cond := conditionFor(rb, string(ConditionReleaseSynced)); cond != nil {
				return cond.Reason
			}
			return ""
		}
	}

	deleteObj := func(obj client.Object) {
		_ = k8sClient.Delete(ctx, obj)
	}

	It("wakes a binding stuck on EnvironmentNotFound when the Environment is created", func() {
		const (
			rbName  = "rb-watch-env"
			crName  = "cr-watch-env"
			envName = "env-watch-env"
			dpName  = "dp-watch-env-missing"
		)
		DeferCleanup(forceDelete, rbName)
		DeferCleanup(deleteObj, &openchoreov1alpha1.ComponentRelease{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName}})
		DeferCleanup(deleteObj, &openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName}})
		startController()

		By("Creating the binding before its Environment")
		Expect(k8sClient.Create(ctx, crFixture(crName, "proj", "comp"))).To(Succeed())
		Expect(k8sClient.Create(ctx, rbFixture(rbName, "proj", "comp", envName, crName, true))).To(Succeed())
		Eventually(syncedReason(rbName), timeout, interval).Should(Equal(string(ReasonEnvironmentNotFound)))
		Consistently(syncedReason(rbName), settleTime, interval).Should(Equal(string(ReasonEnvironmentNotFound)))

		By("Creating the Environment; the watch must wake the binding without a manual reconcile")
		// The data plane is deliberately missing, so the next gate is DataPlaneNotFound.
		// Moving there shows the binding was reconciled again after the Environment appeared.
		// NotFound exits schedule no requeue, so only the watch can move the binding on.
		Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
		Eventually(syncedReason(rbName), timeout, interval).Should(Equal(string(ReasonDataPlaneNotFound)))
	})

	It("wakes a binding on an Environment without dataPlaneRef when the default DataPlane is created", func() {
		const (
			rbName  = "rb-watch-default-dp"
			crName  = "cr-watch-default-dp"
			envName = "env-watch-default-dp"
		)
		DeferCleanup(forceDelete, rbName)
		DeferCleanup(deleteObj, &openchoreov1alpha1.ComponentRelease{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName}})
		DeferCleanup(deleteObj, &openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName}})
		DeferCleanup(deleteObj, &openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: controller.DefaultPlaneName}})
		startController()

		By("Creating an Environment with no dataPlaneRef while no default data plane exists")
		Expect(k8sClient.Create(ctx, envFixture(envName, ""))).To(Succeed())
		Expect(k8sClient.Create(ctx, crFixture(crName, "proj", "comp-watch-default-dp"))).To(Succeed())
		Expect(k8sClient.Create(ctx, rbFixture(rbName, "proj", "comp-watch-default-dp", envName, crName, true))).To(Succeed())
		Eventually(syncedReason(rbName), timeout, interval).Should(Equal(string(ReasonDataPlaneNotFound)))
		Consistently(syncedReason(rbName), settleTime, interval).Should(Equal(string(ReasonDataPlaneNotFound)))

		By("Creating the default DataPlane; the fallback match must wake the binding")
		// The Component is deliberately missing, so the next gate is ComponentNotFound.
		Expect(k8sClient.Create(ctx, dpFixture(controller.DefaultPlaneName))).To(Succeed())
		Eventually(syncedReason(rbName), timeout, interval).Should(Equal(string(ReasonComponentNotFound)))
	})
})

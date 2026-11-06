// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// These specs check that the controller-wide requeue intervals on the Reconciler reach
// the result of a reconcile. Precedence and jitter are covered by the unit tests of
// getStableRequeueInterval and getProgressingRequeueInterval.
var _ = Describe("RenderedRelease requeue intervals", func() {
	configMapManifest := func(name string) string {
		return fmt.Sprintf(`{"apiVersion": "v1", "kind": "ConfigMap",
			"metadata": {"name": %q, "namespace": "default"}, "data": {"key": "value"}}`, name)
	}
	// envtest runs no Deployment controller, so the Deployment never reports an observed
	// generation and stays Progressing: the transitioning requeue path.
	deploymentManifest := func(name string) string {
		return fmt.Sprintf(`{"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": {"name": %q, "namespace": "default"},
			"spec": {"replicas": 1, "selector": {"matchLabels": {"app": %q}},
				"template": {"metadata": {"labels": {"app": %q}},
					"spec": {"containers": [{"name": "nginx", "image": "nginx:latest"}]}}}}`, name, name, name)
	}

	// reconcileToRequeue creates a release holding one resource on its own plane and
	// environment, lets configure adjust the reconciler and the release, and returns the
	// result of the first reconcile that reaches the requeue decision.
	reconcileToRequeue := func(name, manifest string,
		configure func(*Reconciler, *openchoreov1alpha1.RenderedRelease)) reconcile.Result {
		GinkgoHelper()
		envName, cdpName, planeID := "env-"+name, "cdp-"+name, "plane-"+name
		DeferCleanup(func() {
			forceDelete(ctx, types.NamespacedName{Name: name, Namespace: testNs})
			forceDeleteEnvironment(ctx, types.NamespacedName{Name: envName, Namespace: testNs})
			forceDeleteClusterDataPlane(ctx, cdpName)
		})

		Expect(k8sClient.Create(ctx, makeClusterDataPlane(cdpName, planeID))).To(Succeed())
		Expect(k8sClient.Create(ctx, makeEnvironment(envName, cdpName))).To(Succeed())

		release := makeMinimalRelease(name, envName)
		release.Spec.Resources = []openchoreov1alpha1.RenderedManifest{
			{ID: name, Object: &runtime.RawExtension{Raw: []byte(manifest)}},
		}
		r := testReconcilerWithDPClient(k8sClient, planeID, cdpName)
		configure(r, release)
		Expect(k8sClient.Create(ctx, release)).To(Succeed())

		mustReconcile(r, reconcileRequest(name)) // adds the finalizer
		mustReconcile(r, reconcileRequest(name)) // applies and persists status
		return mustReconcile(r, reconcileRequest(name))
	}

	expectJittered := func(result reconcile.Result, base time.Duration) {
		GinkgoHelper()
		Expect(result.RequeueAfter).To(BeNumerically(">=", base))
		Expect(result.RequeueAfter).To(BeNumerically("<", base+base/5))
	}

	It("requeues a stable release after 1m when no stable interval is configured", func() {
		result := reconcileToRequeue("rri-stable-default", configMapManifest("rri-stable-default"),
			func(*Reconciler, *openchoreov1alpha1.RenderedRelease) {})
		expectJittered(result, time.Minute)
	})

	It("requeues a stable release after the configured stable interval", func() {
		result := reconcileToRequeue("rri-stable-flag", configMapManifest("rri-stable-flag"),
			func(r *Reconciler, _ *openchoreov1alpha1.RenderedRelease) {
				r.StableInterval = 2 * time.Minute
			})
		expectJittered(result, 2*time.Minute)
	})

	It("lets spec.interval override the configured stable interval", func() {
		result := reconcileToRequeue("rri-stable-spec", configMapManifest("rri-stable-spec"),
			func(r *Reconciler, release *openchoreov1alpha1.RenderedRelease) {
				r.StableInterval = 2 * time.Minute
				release.Spec.Interval = &metav1.Duration{Duration: 7 * time.Minute}
			})
		expectJittered(result, 7*time.Minute)
	})

	It("does not requeue a stable release whose spec.interval is 0", func() {
		result := reconcileToRequeue("rri-stable-zero", configMapManifest("rri-stable-zero"),
			func(r *Reconciler, release *openchoreov1alpha1.RenderedRelease) {
				r.StableInterval = 2 * time.Minute
				release.Spec.Interval = &metav1.Duration{Duration: 0}
			})
		Expect(result.RequeueAfter).To(BeZero())
	})

	It("requeues a transitioning release after the configured progressing interval", func() {
		result := reconcileToRequeue("rri-progressing-flag", deploymentManifest("rri-progressing-flag"),
			func(r *Reconciler, _ *openchoreov1alpha1.RenderedRelease) {
				r.ProgressingInterval = 30 * time.Second
			})
		expectJittered(result, 30*time.Second)
	})
})

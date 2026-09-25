// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("ReleaseBinding controller hook wiring", func() {
	// The hook watches (owned WorkflowRuns, Environment, Hook, ClusterHook) must
	// build against a real manager, and the event recorder must come from the
	// manager so hook events are emitted in production. The mapping itself is
	// pinned by the releaseBindingsFor* unit tests.
	It("SetupWithManager registers the hook watches and the event recorder", func() {
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:     scheme.Scheme,
			Metrics:    metricsserver.Options{BindAddress: "0"},
			Controller: config.Controller{SkipNameValidation: ptr.To(true)},
		})
		Expect(err).NotTo(HaveOccurred())
		r := hookReconciler()
		Expect(r.SetupWithManager(mgr)).To(Succeed())
		Expect(r.Recorder).NotTo(BeNil())
	})

	// A hook-resolution API failure must fail the reconcile (requeue with
	// backoff) before anything is rendered: failing open here would deploy a
	// release past a scan that never ran.
	Context("when resolving the environment's hooks fails", func() {
		const (
			project  = "hooks-err-proj"
			compName = "hooks-err-comp"
			envName  = "hooks-err-env"
			dpName   = "hooks-err-dp"
			rbName   = "rb-hooks-err"
			crName   = "cr-hooks-err"
		)

		AfterEach(func() {
			forceDelete(rbName)
			forceDeleteRelease(compName + "-" + envName)
			for _, obj := range []client.Object{
				&openchoreov1alpha1.ComponentRelease{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName}},
				&openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName}},
				&openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName}},
				&openchoreov1alpha1.Component{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName}},
				&openchoreov1alpha1.Project{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: project}},
			} {
				_ = k8sClient.Delete(ctx, obj)
			}
		})

		It("returns the error and renders nothing", func() {
			Expect(k8sClient.Create(ctx, crFixture(crName, project, compName))).To(Succeed())
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envWithHooks(envName, dpName, &openchoreov1alpha1.HookSet{
				PreDeploy: []openchoreov1alpha1.HookBinding{
					hookBinding("scan", openchoreov1alpha1.HookModeSync, openchoreov1alpha1.HookFailurePolicyBlock)}}))).To(Succeed())
			Expect(k8sClient.Create(ctx, componentFixture(compName, project))).To(Succeed())
			Expect(k8sClient.Create(ctx, projectFixture(project))).To(Succeed())
			Expect(k8sClient.Create(ctx, rbFixture(rbName, project, compName, envName, crName, true))).To(Succeed())

			boom := errors.New("apiserver unavailable")
			r := hookReconciler()
			wc, err := client.NewWithWatch(cfg, client.Options{Scheme: scheme.Scheme})
			Expect(err).NotTo(HaveOccurred())
			r.Client = interceptor.NewClient(wc, interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*openchoreov1alpha1.ClusterHook); ok {
						return boom
					}
					return c.Get(ctx, key, obj, opts...)
				},
			})

			_, err = r.Reconcile(ctx, reconcileRequest(rbName))
			Expect(err).To(MatchError(ContainSubstring("failed to evaluate pre-deploy hooks")))
			Expect(errors.Is(err, boom)).To(BeTrue())
			Expect(releaseExists(compName + "-" + envName)).To(BeFalse())
		})
	})
})

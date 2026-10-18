// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	endpointpipeline "github.com/openchoreo/openchoreo/internal/pipeline/endpoint"
)

// These drive the full reconcile through a fake client registered with the
// production field indexes.
var _ = Describe("Backend resolution and rendering", func() {
	fctx := context.Background()

	var dpNS string

	BeforeEach(func() {
		dpNS = dataPlaneNamespace(newBinding("probe", ""))
	})

	build := func(objs ...client.Object) (*Reconciler, client.Client) {
		base := []client.Object{newEnvironment(), newDataPlane(), newProject(), newEndpointCR()}
		cli := fake.NewClientBuilder().
			WithScheme(scheme.Scheme).
			WithObjects(append(base, objs...)...).
			WithStatusSubresource(&openchoreov1alpha1.EndpointReleaseBinding{}, &openchoreov1alpha1.RenderedRelease{}).
			WithIndex(&openchoreov1alpha1.ReleaseBinding{},
				controller.IndexKeyReleaseBindingOwnerEnv, controller.IndexReleaseBindingOwnerEnv).
			WithIndex(&openchoreov1alpha1.EndpointReleaseBinding{},
				controller.IndexKeyEndpointReleaseBindingOwnerEndpointName, controller.IndexEndpointReleaseBindingOwner).
			Build()
		return &Reconciler{Client: cli, Scheme: scheme.Scheme, Pipeline: endpointpipeline.NewPipeline()}, cli
	}

	// reconcileBinding runs the finalizer-adding pass and the reconcile pass.
	reconcileBinding := func(r *Reconciler, cli client.Client, b *openchoreov1alpha1.EndpointReleaseBinding) *openchoreov1alpha1.EndpointReleaseBinding {
		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)}
		for range 2 {
			_, err := r.Reconcile(fctx, req)
			Expect(err).NotTo(HaveOccurred())
		}
		updated := &openchoreov1alpha1.EndpointReleaseBinding{}
		Expect(cli.Get(fctx, req.NamespacedName, updated)).To(Succeed())
		return updated
	}

	condition := func(b *openchoreov1alpha1.EndpointReleaseBinding, ct controller.ConditionType) *metav1.Condition {
		cond := meta.FindStatusCondition(b.Status.Conditions, string(ct))
		Expect(cond).NotTo(BeNil(), "expected %s condition", ct)
		return cond
	}

	renderedRelease := func(cli client.Client, b *openchoreov1alpha1.EndpointReleaseBinding) (*openchoreov1alpha1.RenderedRelease, error) {
		rr := &openchoreov1alpha1.RenderedRelease{}
		err := cli.Get(fctx, client.ObjectKey{Namespace: testNamespace, Name: makeRenderedReleaseName(b)}, rr)
		return rr, err
	}

	routeBackendRefs := func(rr *openchoreov1alpha1.RenderedRelease) []any {
		Expect(rr.Spec.Resources).To(HaveLen(1))
		var route map[string]any
		Expect(json.Unmarshal(rr.Spec.Resources[0].Object.Raw, &route)).To(Succeed())
		rule := route["spec"].(map[string]any)["rules"].([]any)[0].(map[string]any)
		return rule["backendRefs"].([]any)
	}

	It("renders a weighted route across two deployed component backends", func() {
		release := newRelease(testRelease, componentBackend("stable", "greeter", 90), componentBackend("canary", "greeter-v2", 10))
		b := newBinding("weighted", testRelease)
		r, cli := build(release, b, deployedComponent("greeter", dpNS), deployedComponent("greeter-v2", dpNS))

		updated := reconcileBinding(r, cli, b)

		Expect(condition(updated, ConditionBackendsResolved).Status).To(Equal(metav1.ConditionTrue))
		Expect(condition(updated, ConditionSynced).Status).To(Equal(metav1.ConditionTrue))
		Expect(updated.Status.Backends).To(HaveLen(2))
		for _, st := range updated.Status.Backends {
			Expect(st.Resolved).To(BeTrue(), "backend %s", st.Name)
			Expect(st.ServiceURL).NotTo(BeNil())
		}

		rr, err := renderedRelease(cli, updated)
		Expect(err).NotTo(HaveOccurred())
		Expect(rr.Spec.Owner).To(Equal(openchoreov1alpha1.RenderedReleaseOwner{
			ProjectName: testProject, EndpointName: testEndpoint,
		}))
		Expect(rr.Spec.EnvironmentName).To(Equal(testEnvName))
		Expect(rr.Spec.TargetPlane).To(Equal(openchoreov1alpha1.TargetPlaneDataPlane))
		Expect(routeBackendRefs(rr)).To(Equal([]any{
			map[string]any{"name": "greeter-svc", "port": float64(8080), "weight": float64(90)},
			map[string]any{"name": "greeter-v2-svc", "port": float64(8080), "weight": float64(10)},
		}))

		// Nothing observed on the data plane yet, so the binding is not Ready.
		ready := condition(updated, ConditionReady)
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal(string(ReasonResourcesProgressing)))
	})

	It("becomes Ready once the data plane reports the rendered route healthy", func() {
		release := newRelease(testRelease, componentBackend("stable", "greeter", 1))
		b := newBinding("ready", testRelease)
		r, cli := build(release, b, deployedComponent("greeter", dpNS))
		reconcileBinding(r, cli, b)

		rr, err := renderedRelease(cli, b)
		Expect(err).NotTo(HaveOccurred())
		rr.Status.Resources = []openchoreov1alpha1.RenderedManifestStatus{{
			ID: "route", Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute",
			Name: rr.Name, HealthStatus: openchoreov1alpha1.HealthStatusHealthy,
		}}
		Expect(cli.Status().Update(fctx, rr)).To(Succeed())

		updated := reconcileBinding(r, cli, b)
		Expect(condition(updated, ConditionResourcesReady).Status).To(Equal(metav1.ConditionTrue))
		Expect(condition(updated, ConditionReady).Status).To(Equal(metav1.ConditionTrue))
	})

	It("blocks rendering with a readable reason when a weighted backend is not deployed", func() {
		release := newRelease(testRelease, componentBackend("stable", "greeter", 90), componentBackend("canary", "greeter-v2", 10))
		b := newBinding("blocked", testRelease)
		r, cli := build(release, b, deployedComponent("greeter", dpNS))

		updated := reconcileBinding(r, cli, b)

		backends := condition(updated, ConditionBackendsResolved)
		Expect(backends.Status).To(Equal(metav1.ConditionFalse))
		Expect(backends.Message).To(Equal(`Backend(s) not available in environment "dev": ` +
			`canary (component "greeter-v2" is not deployed in environment "dev")`))

		ready := condition(updated, ConditionReady)
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal(string(ReasonBackendsUnresolved)))
		Expect(ready.Message).To(Equal(backends.Message), "Ready should surface the backend problem, not a generic one")

		Expect(updated.Status.Backends).To(ConsistOf(
			HaveField("Name", "stable"),
			And(HaveField("Name", "canary"), HaveField("Resolved", false), HaveField("Reason", BackendReasonNotDeployed)),
		))
		_, err := renderedRelease(cli, updated)
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "nothing should be rendered while a weighted backend is missing")
	})

	It("renders without an undeployed backend staged at weight 0", func() {
		release := newRelease(testRelease, componentBackend("stable", "greeter", 100), componentBackend("canary", "greeter-v2", 0))
		b := newBinding("staged", testRelease)
		r, cli := build(release, b, deployedComponent("greeter", dpNS))

		updated := reconcileBinding(r, cli, b)

		Expect(condition(updated, ConditionBackendsResolved).Status).To(Equal(metav1.ConditionTrue))
		rr, err := renderedRelease(cli, updated)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeBackendRefs(rr)).To(HaveLen(1))
		Expect(updated.Status.Backends).To(ContainElement(
			And(HaveField("Name", "canary"), HaveField("Resolved", false), HaveField("Reason", BackendReasonNotDeployed)),
		), "the staged backend is still reported, just not routed to")
	})

	It("leaves the existing RenderedRelease untouched when a backend disappears", func() {
		release := newRelease(testRelease, componentBackend("stable", "greeter", 90), componentBackend("canary", "greeter-v2", 10))
		b := newBinding("disappears", testRelease)
		canary := deployedComponent("greeter-v2", dpNS)
		r, cli := build(release, b, deployedComponent("greeter", dpNS), canary)
		reconcileBinding(r, cli, b)
		before, err := renderedRelease(cli, b)
		Expect(err).NotTo(HaveOccurred())

		Expect(cli.Delete(fctx, canary)).To(Succeed())
		updated := reconcileBinding(r, cli, b)

		Expect(condition(updated, ConditionBackendsResolved).Status).To(Equal(metav1.ConditionFalse))
		after, err := renderedRelease(cli, updated)
		Expect(err).NotTo(HaveOccurred())
		Expect(after.Spec).To(Equal(before.Spec), "the route keeps both backends rather than shifting the canary's share")
	})

	DescribeTable("reports why a component backend cannot be used",
		func(rb func() *openchoreov1alpha1.ReleaseBinding, reason, message string) {
			release := newRelease(testRelease, componentBackend("stable", "greeter", 1))
			b := newBinding("unusable", testRelease)
			r, cli := build(release, b, rb())

			updated := reconcileBinding(r, cli, b)
			Expect(updated.Status.Backends).To(HaveLen(1))
			st := updated.Status.Backends[0]
			Expect(st.Resolved).To(BeFalse())
			Expect(st.Reason).To(Equal(reason))
			Expect(st.Message).To(ContainSubstring(message))
		},
		Entry("undeployed", func() *openchoreov1alpha1.ReleaseBinding {
			rb := deployedComponent("greeter", dpNS)
			rb.Spec.State = openchoreov1alpha1.ReleaseStateUndeploy
			return rb
		}, BackendReasonNotDeployed, `component "greeter" is undeployed`),
		Entry("endpoint without an address yet", func() *openchoreov1alpha1.ReleaseBinding {
			rb := deployedComponent("greeter", dpNS)
			rb.Status.Endpoints = nil
			return rb
		}, BackendReasonEndpointNotAvailable, `endpoint "http" of component "greeter" has no in-cluster address`),
		Entry("UDP endpoint", func() *openchoreov1alpha1.ReleaseBinding {
			rb := deployedComponent("greeter", dpNS)
			rb.Status.Endpoints[0].Type = openchoreov1alpha1.EndpointProtocolUDP
			return rb
		}, BackendReasonUnsupportedProtocol, "uses protocol UDP"),
		Entry("Service in another namespace", func() *openchoreov1alpha1.ReleaseBinding {
			return deployedComponent("greeter", "some-other-namespace")
		}, BackendReasonNamespaceMismatch, `is served from namespace "some-other-namespace"`),
	)

	It("reports RenderingFailed when a type validation rule rejects the backends", func() {
		release := newRelease(testRelease, componentBackend("stable", "greeter", 1))
		release.Spec.EndpointType.Spec.Validations = []openchoreov1alpha1.ValidationRule{{
			Rule:    `${backends.all(b, b.protocol == "gRPC")}`,
			Message: "this type only routes gRPC backends",
		}}
		b := newBinding("validation", testRelease)
		r, cli := build(release, b, deployedComponent("greeter", dpNS))

		updated := reconcileBinding(r, cli, b)
		synced := condition(updated, ConditionSynced)
		Expect(synced.Status).To(Equal(metav1.ConditionFalse))
		Expect(synced.Reason).To(Equal(string(ReasonRenderingFailed)))
		Expect(synced.Message).To(ContainSubstring("this type only routes gRPC backends"))
	})

	It("deletes the RenderedRelease before releasing the finalizer", func() {
		release := newRelease(testRelease, componentBackend("stable", "greeter", 1))
		b := newBinding("finalize", testRelease)
		r, cli := build(release, b, deployedComponent("greeter", dpNS))
		reconcileBinding(r, cli, b)
		_, err := renderedRelease(cli, b)
		Expect(err).NotTo(HaveOccurred())

		live := &openchoreov1alpha1.EndpointReleaseBinding{}
		Expect(cli.Get(fctx, client.ObjectKeyFromObject(b), live)).To(Succeed())
		Expect(cli.Delete(fctx, live)).To(Succeed())

		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)}
		// 1st: Finalizing condition. 2nd: deletes the RenderedRelease. 3rd: releases the finalizer.
		for range 3 {
			_, err := r.Reconcile(fctx, req)
			Expect(err).NotTo(HaveOccurred())
		}

		_, err = renderedRelease(cli, b)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
		err = cli.Get(fctx, req.NamespacedName, &openchoreov1alpha1.EndpointReleaseBinding{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("maps a component's ReleaseBinding only to bindings routing to it in that environment", func() {
		release := newRelease(testRelease, componentBackend("stable", "greeter", 1))
		other := newRelease("other-release", componentBackend("stable", "billing", 1))
		routes := newBinding("routes", testRelease)
		wrongEnv := newBinding("wrong-env", testRelease)
		wrongEnv.Spec.Environment = "prod"
		otherComponent := newBinding("other-component", other.Name)
		r, _ := build(release, other, routes, wrongEnv, otherComponent)

		requests := r.listBindingsForReleaseBinding(fctx, deployedComponent("greeter", dpNS))
		Expect(requests).To(ConsistOf(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(routes)}))
	})
})

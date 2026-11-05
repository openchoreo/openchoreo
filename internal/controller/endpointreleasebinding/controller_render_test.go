// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	"context"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/renderedrelease"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/networkpolicy"
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
		seen := map[string]bool{}
		for _, obj := range objs {
			if rb, ok := obj.(*openchoreov1alpha1.ReleaseBinding); ok {
				for _, in := range releaseBindingInputs(rb) {
					key := fmt.Sprintf("%T/%s", in, in.GetName())
					if !seen[key] {
						seen[key] = true
						base = append(base, in)
					}
				}
			}
		}
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

	// reportHealthy records every rendered resource as healthy, as the data
	// plane would once it has applied them.
	reportHealthy := func(cli client.Client, rr *openchoreov1alpha1.RenderedRelease) {
		rr.Status.Resources = nil
		for _, res := range rr.Spec.Resources {
			var obj unstructured.Unstructured
			Expect(json.Unmarshal(res.Object.Raw, &obj.Object)).To(Succeed())
			gvk := obj.GroupVersionKind()
			rr.Status.Resources = append(rr.Status.Resources, openchoreov1alpha1.RenderedManifestStatus{
				ID: res.ID, Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind,
				Name: obj.GetName(), HealthStatus: openchoreov1alpha1.HealthStatusHealthy,
			})
		}
		Expect(cli.Status().Update(fctx, rr)).To(Succeed())
	}

	// manifest returns the RenderedRelease resource with the given ID.
	manifest := func(rr *openchoreov1alpha1.RenderedRelease, id string) map[string]any {
		for _, res := range rr.Spec.Resources {
			if res.ID == id {
				var obj map[string]any
				Expect(json.Unmarshal(res.Object.Raw, &obj)).To(Succeed())
				return obj
			}
		}
		Fail(fmt.Sprintf("RenderedRelease has no resource %q", id))
		return nil
	}

	routeBackendRefs := func(rr *openchoreov1alpha1.RenderedRelease) []any {
		route := manifest(rr, "route")
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
		reportHealthy(cli, rr)

		// Healthy entries alone are not enough: they may describe the previous
		// routes until the RenderedRelease reports its current generation applied.
		updated := reconcileBinding(r, cli, b)
		Expect(condition(updated, ConditionResourcesReady).Reason).To(Equal(string(ReasonResourcesProgressing)))
		Expect(condition(updated, ConditionReady).Status).To(Equal(metav1.ConditionFalse))

		Expect(cli.Get(fctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())
		meta.SetStatusCondition(&rr.Status.Conditions, metav1.Condition{
			Type: renderedrelease.ConditionResourcesApplied, Status: metav1.ConditionTrue,
			Reason: "Applied", ObservedGeneration: rr.Generation,
		})
		Expect(cli.Status().Update(fctx, rr)).To(Succeed())

		updated = reconcileBinding(r, cli, b)
		Expect(condition(updated, ConditionResourcesReady).Status).To(Equal(metav1.ConditionTrue))
		Expect(condition(updated, ConditionReady).Status).To(Equal(metav1.ConditionTrue))
	})

	It("refuses to render when the Endpoint belongs to another project", func() {
		release := newRelease(testRelease, componentBackend("stable", "greeter", 1))
		b := newBinding("foreign", testRelease)
		r, cli := build(release, b, deployedComponent("greeter", dpNS))
		ep := &openchoreov1alpha1.Endpoint{}
		Expect(cli.Get(fctx, client.ObjectKey{Namespace: testNamespace, Name: testEndpoint}, ep)).To(Succeed())
		ep.Spec.Owner.ProjectName = "billing"
		Expect(cli.Update(fctx, ep)).To(Succeed())

		updated := reconcileBinding(r, cli, b)

		synced := condition(updated, ConditionSynced)
		Expect(synced.Status).To(Equal(metav1.ConditionFalse))
		Expect(synced.Reason).To(Equal(string(ReasonInvalidReleaseConfiguration)))
		Expect(synced.Message).To(ContainSubstring(`project "billing" of Endpoint`))
		_, err := renderedRelease(cli, updated)
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "nothing should be rendered for a foreign Endpoint")
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

	It("renders the environment's weights from spec.backendOverrides without a new release", func() {
		release := newRelease(testRelease, componentBackend("stable", "greeter", 100), componentBackend("canary", "greeter-v2", 0))
		b := newBinding("override", testRelease)
		b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{
			{Name: "stable", Weight: ptr.To[int32](75)},
			{Name: "canary", Weight: ptr.To[int32](25)},
		}
		r, cli := build(release, b, deployedComponent("greeter", dpNS), deployedComponent("greeter-v2", dpNS))

		updated := reconcileBinding(r, cli, b)

		Expect(condition(updated, ConditionSynced).Status).To(Equal(metav1.ConditionTrue))
		Expect(condition(updated, ConditionBackendOverridesApplied).Status).To(Equal(metav1.ConditionTrue))
		Expect(updated.Status.Backends).To(ConsistOf(
			And(HaveField("Name", "stable"), HaveField("Weight", int32(75))),
			And(HaveField("Name", "canary"), HaveField("Weight", int32(25))),
		), "status reports the effective weights")
		rr, err := renderedRelease(cli, updated)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeBackendRefs(rr)).To(Equal([]any{
			map[string]any{"name": "greeter-svc", "port": float64(8080), "weight": float64(75)},
			map[string]any{"name": "greeter-v2-svc", "port": float64(8080), "weight": float64(25)},
		}))
	})

	It("ignores and reports an override for a backend the release no longer has", func() {
		// The release retired the canary; this environment's overrides still
		// name it, as after promoting a release that removed a backend.
		release := newRelease(testRelease, componentBackend("stable", "greeter", 100))
		b := newBinding("retired", testRelease)
		b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{
			{Name: "stable", Weight: ptr.To[int32](100)},
			{Name: "canary", Weight: ptr.To[int32](0)},
		}
		r, cli := build(release, b, deployedComponent("greeter", dpNS))

		updated := reconcileBinding(r, cli, b)

		applied := condition(updated, ConditionBackendOverridesApplied)
		Expect(applied.Status).To(Equal(metav1.ConditionFalse))
		Expect(applied.Reason).To(Equal(string(ReasonBackendOverridesIgnored)))
		Expect(applied.Message).To(ContainSubstring("canary"))
		Expect(condition(updated, ConditionSynced).Status).To(Equal(metav1.ConditionTrue),
			"an ignored override must not hold the binding back")
		rr, err := renderedRelease(cli, updated)
		Expect(err).NotTo(HaveOccurred())
		Expect(routeBackendRefs(rr)).To(Equal([]any{
			map[string]any{"name": "greeter-svc", "port": float64(8080), "weight": float64(100)},
		}))
	})

	It("does not render when the overrides leave every backend at weight 0", func() {
		release := newRelease(testRelease, componentBackend("stable", "greeter", 100))
		b := newBinding("zero", testRelease)
		b.Spec.BackendOverrides = []openchoreov1alpha1.EndpointBackendOverride{{Name: "stable", Weight: ptr.To[int32](0)}}
		r, cli := build(release, b, deployedComponent("greeter", dpNS))

		updated := reconcileBinding(r, cli, b)

		backends := condition(updated, ConditionBackendsResolved)
		Expect(backends.Status).To(Equal(metav1.ConditionFalse))
		Expect(backends.Reason).To(Equal(string(ReasonNoWeightedBackend)))
		_, err := renderedRelease(cli, updated)
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "a route to nowhere must not be rendered")
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
		Entry("no release to read the container port from", func() *openchoreov1alpha1.ReleaseBinding {
			rb := deployedComponent("greeter", dpNS)
			rb.Spec.ReleaseName = ""
			return rb
		}, BackendReasonEndpointNotAvailable, `endpoint "http" of component "greeter" is not in the release deployed`),
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

	withURLs := func(release *openchoreov1alpha1.EndpointRelease, urls *openchoreov1alpha1.EndpointTypeURLs) *openchoreov1alpha1.EndpointRelease {
		release.Spec.EndpointType.Spec.URLs = urls
		return release
	}
	hostFromBackends := `${backends.map(b, b.name).join("-")}.example.com`

	It("resolves the type's URLs into status.urls for the requested visibilities", func() {
		release := withURLs(newRelease(testRelease, componentBackend("stable", "greeter", 90), componentBackend("canary", "greeter-v2", 10)),
			&openchoreov1alpha1.EndpointTypeURLs{
				External: &openchoreov1alpha1.EndpointURLTemplate{
					Scheme: "https", Host: "${metadata.endpointName}.${metadata.environmentName}.example.com", Port: "8443", Path: "/api",
					Security: &openchoreov1alpha1.EndpointSecurityTemplate{
						Scheme: "apiKey", APIKey: &openchoreov1alpha1.EndpointAPIKeyTemplate{In: "header", Name: "api-key"},
					},
				},
				Internal: &openchoreov1alpha1.EndpointURLTemplate{Scheme: "http", Host: hostFromBackends},
			})
		b := newBinding("urls", testRelease)
		r, cli := build(release, b, deployedComponent("greeter", dpNS), deployedComponent("greeter-v2", dpNS))

		updated := reconcileBinding(r, cli, b)

		Expect(condition(updated, ConditionURLsResolved).Status).To(Equal(metav1.ConditionTrue))
		Expect(updated.Status.URLs).To(Equal(&openchoreov1alpha1.EndpointReleaseBindingURLs{
			External: &openchoreov1alpha1.ResolvedEndpointURL{
				EndpointURL: openchoreov1alpha1.EndpointURL{
					Scheme: "https", Host: testEndpoint + "." + testEnvName + ".example.com", Port: 8443, Path: "/api",
				},
				Security: &openchoreov1alpha1.EndpointSecurity{
					Scheme: openchoreov1alpha1.EndpointSecuritySchemeAPIKey,
					APIKey: &openchoreov1alpha1.EndpointAPIKey{In: openchoreov1alpha1.EndpointAPIKeyInHeader, Name: "api-key"},
				},
			},
		}), "the release's Endpoint requests external only, so the internal URL is not resolved")
	})

	It("keeps the previous URLs while rendering is blocked", func() {
		release := withURLs(newRelease(testRelease, componentBackend("stable", "greeter", 90), componentBackend("canary", "greeter-v2", 10)),
			&openchoreov1alpha1.EndpointTypeURLs{External: &openchoreov1alpha1.EndpointURLTemplate{Scheme: "http", Host: hostFromBackends}})
		b := newBinding("urls-blocked", testRelease)
		canary := deployedComponent("greeter-v2", dpNS)
		r, cli := build(release, b, deployedComponent("greeter", dpNS), canary)
		before := reconcileBinding(r, cli, b)
		Expect(before.Status.URLs).NotTo(BeNil())
		Expect(before.Status.URLs.External.Host).To(Equal("stable-canary.example.com"))

		Expect(cli.Delete(fctx, canary)).To(Succeed())
		updated := reconcileBinding(r, cli, b)

		Expect(condition(updated, ConditionBackendsResolved).Status).To(Equal(metav1.ConditionFalse))
		Expect(updated.Status.URLs).To(Equal(before.Status.URLs), "URLs describe the RenderedRelease that is still deployed")
	})

	It("reports a failing URL without blocking the RenderedRelease", func() {
		release := withURLs(newRelease(testRelease, componentBackend("stable", "greeter", 1)),
			&openchoreov1alpha1.EndpointTypeURLs{
				Project:  &openchoreov1alpha1.EndpointURLTemplate{Scheme: "http", Host: "${metadata.endpointName}"},
				External: &openchoreov1alpha1.EndpointURLTemplate{Scheme: "https", Host: "${parameters.missing}"},
			})
		b := newBinding("urls-failing", testRelease)
		r, cli := build(release, b, deployedComponent("greeter", dpNS))

		updated := reconcileBinding(r, cli, b)

		urls := condition(updated, ConditionURLsResolved)
		Expect(urls.Status).To(Equal(metav1.ConditionFalse))
		Expect(urls.Reason).To(Equal(string(ReasonURLResolutionFailed)))
		Expect(urls.Message).To(ContainSubstring(`url "external"`))
		Expect(updated.Status.URLs).NotTo(BeNil())
		Expect(updated.Status.URLs.Project.Host).To(Equal(testEndpoint), "the project URL is implicit and still resolves")
		Expect(updated.Status.URLs.External).To(BeNil())
		Expect(condition(updated, ConditionSynced).Status).To(Equal(metav1.ConditionTrue))

		// Once the data plane has applied the route and reports it healthy, the
		// broken URL is what keeps the binding from being Ready.
		rr, err := renderedRelease(cli, updated)
		Expect(err).NotTo(HaveOccurred(), "routing is unaffected by a broken URL")
		meta.SetStatusCondition(&rr.Status.Conditions, metav1.Condition{
			Type: renderedrelease.ConditionResourcesApplied, Status: metav1.ConditionTrue,
			Reason: "Applied", ObservedGeneration: rr.Generation,
		})
		reportHealthy(cli, rr)
		updated = reconcileBinding(r, cli, b)
		ready := condition(updated, ConditionReady)
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal(string(ReasonURLResolutionFailed)))
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

	It("leaves another binding's RenderedRelease in place when a duplicate binding is deleted", func() {
		release := newRelease(testRelease, componentBackend("stable", "greeter", 1))
		active := newBinding("active", testRelease)
		// Same Endpoint and environment, so it resolves to the same RenderedRelease name.
		duplicate := newBinding("duplicate", testRelease)
		duplicate.Finalizers = []string{EndpointReleaseBindingFinalizer}
		r, cli := build(release, active, duplicate, deployedComponent("greeter", dpNS))
		reconcileBinding(r, cli, active)
		_, err := renderedRelease(cli, active)
		Expect(err).NotTo(HaveOccurred())

		live := &openchoreov1alpha1.EndpointReleaseBinding{}
		Expect(cli.Get(fctx, client.ObjectKeyFromObject(duplicate), live)).To(Succeed())
		Expect(cli.Delete(fctx, live)).To(Succeed())

		req := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(duplicate)}
		// 1st: Finalizing condition. 2nd: finds a RenderedRelease it does not own and releases the finalizer.
		for range 2 {
			_, err := r.Reconcile(fctx, req)
			Expect(err).NotTo(HaveOccurred())
		}

		err = cli.Get(fctx, req.NamespacedName, &openchoreov1alpha1.EndpointReleaseBinding{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "the duplicate binding is released")
		rr, err := renderedRelease(cli, active)
		Expect(err).NotTo(HaveOccurred(), "the active binding's RenderedRelease must survive")
		Expect(rr.DeletionTimestamp.IsZero()).To(BeTrue())
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

	Context("network policies", func() {
		policyName := endpointpipeline.NetworkPolicyID

		It("renders a policy per backend admitting the gateways to its pods on the container port", func() {
			release := newRelease(testRelease, componentBackend("stable", "greeter", 90), componentBackend("canary", "greeter-v2", 10))
			b := newBinding("policies", testRelease)
			r, cli := build(release, b, deployedComponent("greeter", dpNS), deployedComponent("greeter-v2", dpNS))

			updated := reconcileBinding(r, cli, b)
			rr, err := renderedRelease(cli, updated)
			Expect(err).NotTo(HaveOccurred())
			Expect(rr.Spec.Resources).To(HaveLen(3), "the route and one policy per backend")

			policy := manifest(rr, policyName("stable"))
			Expect(policy["kind"]).To(Equal("NetworkPolicy"))
			Expect(policy["metadata"].(map[string]any)["namespace"]).To(Equal(dpNS))
			spec := policy["spec"].(map[string]any)
			selectors := spec["podSelector"].(map[string]any)["matchLabels"].(map[string]any)
			Expect(selectors).To(HaveKeyWithValue(labels.LabelKeyComponentName, "greeter"))
			Expect(selectors).To(HaveKeyWithValue(labels.LabelKeyComponentUID, "greeter-uid"))
			Expect(selectors).To(HaveKeyWithValue(labels.LabelKeyEnvironmentName, testEnvName))
			Expect(selectors).NotTo(HaveKey(labels.LabelKeyReleaseLine))
			ingress := spec["ingress"].([]any)
			Expect(ingress).To(HaveLen(1), "external visibility admits only the gateways")
			rule := ingress[0].(map[string]any)
			Expect(rule["ports"]).To(Equal([]any{map[string]any{"protocol": "TCP", "port": float64(9090)}}),
				"the policy opens the container port, not the Service port the route targets")
			Expect(rule["from"].([]any)[0].(map[string]any)["podSelector"]).To(Equal(map[string]any{
				"matchExpressions": []any{map[string]any{"key": labels.LabelKeySystemComponent, "operator": "Exists"}},
			}))

			canary := manifest(rr, policyName("canary"))
			canarySelectors := canary["spec"].(map[string]any)["podSelector"].(map[string]any)["matchLabels"].(map[string]any)
			Expect(canarySelectors).To(HaveKeyWithValue(labels.LabelKeyComponentUID, "greeter-v2-uid"))
		})

		It("opens the workload endpoint's targetPort when it differs from its port", func() {
			release := newRelease(testRelease, componentBackend("stable", "greeter", 1))
			b := newBinding("target-port", testRelease)
			rb := deployedComponent("greeter", dpNS)
			r, cli := build(release, b, rb)

			// The Service forwards port 8080 to 9091 on the pod, where the policy is enforced.
			cr := &openchoreov1alpha1.ComponentRelease{}
			Expect(cli.Get(fctx, client.ObjectKey{Namespace: testNamespace, Name: rb.Spec.ReleaseName}, cr)).To(Succeed())
			ep := cr.Spec.Workload.Endpoints["http"]
			ep.Port, ep.TargetPort = 8080, 9091
			cr.Spec.Workload.Endpoints["http"] = ep
			Expect(cli.Update(fctx, cr)).To(Succeed())

			updated := reconcileBinding(r, cli, b)
			rr, err := renderedRelease(cli, updated)
			Expect(err).NotTo(HaveOccurred())
			rule := manifest(rr, policyName("stable"))["spec"].(map[string]any)["ingress"].([]any)[0].(map[string]any)
			Expect(rule["ports"]).To(Equal([]any{map[string]any{"protocol": "TCP", "port": float64(9091)}}))
		})

		It("refuses to render a type resource that takes a network policy's ID", func() {
			release := newRelease(testRelease, componentBackend("stable", "greeter", 1))
			release.Spec.EndpointType.Spec.Resources[0].ID = policyName("stable")
			b := newBinding("id-collision", testRelease)
			r, cli := build(release, b, deployedComponent("greeter", dpNS))

			updated := reconcileBinding(r, cli, b)
			synced := condition(updated, ConditionSynced)
			Expect(synced.Status).To(Equal(metav1.ConditionFalse))
			Expect(synced.Reason).To(Equal(string(ReasonRenderingFailed)))
			Expect(synced.Message).To(ContainSubstring(`share the ID "openchoreo-networkpolicy-stable"`))
			_, err := renderedRelease(cli, updated)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "nothing is written while two resources share an ID")
		})

		It("admits the other projects of the namespace for namespace visibility", func() {
			release := newRelease(testRelease, componentBackend("stable", "greeter", 1))
			release.Spec.Visibility = []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityNamespace}
			b := newBinding("namespace-policy", testRelease)
			r, cli := build(release, b, deployedComponent("greeter", dpNS))

			updated := reconcileBinding(r, cli, b)
			rr, err := renderedRelease(cli, updated)
			Expect(err).NotTo(HaveOccurred())
			ingress := manifest(rr, policyName("stable"))["spec"].(map[string]any)["ingress"].([]any)
			Expect(ingress).To(HaveLen(1))
			Expect(ingress[0].(map[string]any)["from"]).To(Equal([]any{map[string]any{
				"namespaceSelector": map[string]any{"matchLabels": map[string]any{
					labels.LabelKeyNamespaceName:   testNamespace,
					labels.LabelKeyEnvironmentName: testEnvName,
				}},
			}}))
		})

		It("selects a release line's pods for a backend on that line", func() {
			release := newRelease(testRelease, lineBackend(1))
			b := newBinding("line-policy", testRelease)
			r, cli := build(release, b, newReleaseLine("greeter"), deployedLine("greeter", dpNS))

			updated := reconcileBinding(r, cli, b)
			rr, err := renderedRelease(cli, updated)
			Expect(err).NotTo(HaveOccurred())
			selectors := manifest(rr, policyName("next"))["spec"].(map[string]any)["podSelector"].(map[string]any)["matchLabels"].(map[string]any)
			Expect(selectors).To(HaveKeyWithValue(labels.LabelKeyReleaseLine, testLine))
			Expect(selectors).To(HaveKeyWithValue(labels.LabelKeyReleaseLineUID, "greeter-next-uid"))
			Expect(selectors).To(HaveKeyWithValue(labels.LabelKeyReleaseLineComponentUID, "greeter-uid"))
			Expect(selectors).NotTo(HaveKey(labels.LabelKeyComponentUID), "the main line's pods must not match")
		})

		It("renders a CiliumNetworkPolicy when the data plane selects Cilium", func() {
			release := newRelease(testRelease, componentBackend("stable", "greeter", 1))
			b := newBinding("cilium-policy", testRelease)
			r, cli := build(release, b, deployedComponent("greeter", dpNS))
			dp := &openchoreov1alpha1.DataPlane{}
			Expect(cli.Get(fctx, client.ObjectKey{Namespace: testNamespace, Name: "primary"}, dp)).To(Succeed())
			dp.Annotations = map[string]string{networkpolicy.ProviderAnnotation: string(networkpolicy.ProviderCilium)}
			Expect(cli.Update(fctx, dp)).To(Succeed())

			updated := reconcileBinding(r, cli, b)
			rr, err := renderedRelease(cli, updated)
			Expect(err).NotTo(HaveOccurred())
			policy := manifest(rr, policyName("stable"))
			Expect(policy["kind"]).To(Equal("CiliumNetworkPolicy"))
			Expect(policy["spec"].(map[string]any)).To(HaveKey("endpointSelector"))
		})
	})

	Context("release lines", func() {
		It("routes a backend without a line to the main line while a release line is deployed beside it", func() {
			release := newRelease(testRelease, componentBackend("stable", "greeter", 1))
			b := newBinding("main-line", testRelease)
			r, cli := build(release, b, newReleaseLine("greeter"),
				deployedComponent("greeter", dpNS), deployedLine("greeter", dpNS))

			updated := reconcileBinding(r, cli, b)

			Expect(condition(updated, ConditionBackendsResolved).Status).To(Equal(metav1.ConditionTrue))
			rr, err := renderedRelease(cli, updated)
			Expect(err).NotTo(HaveOccurred())
			Expect(routeBackendRefs(rr)).To(Equal([]any{
				map[string]any{"name": "greeter-svc", "port": float64(8080), "weight": float64(1)},
			}))
		})

		It("splits traffic between a component's main line and one of its release lines", func() {
			release := newRelease(testRelease,
				componentBackend("stable", "greeter", 90), lineBackend(10))
			b := newBinding("canary-line", testRelease)
			r, cli := build(release, b, newReleaseLine("greeter"),
				deployedComponent("greeter", dpNS), deployedLine("greeter", dpNS))

			updated := reconcileBinding(r, cli, b)

			Expect(condition(updated, ConditionBackendsResolved).Status).To(Equal(metav1.ConditionTrue))
			rr, err := renderedRelease(cli, updated)
			Expect(err).NotTo(HaveOccurred())
			Expect(routeBackendRefs(rr)).To(Equal([]any{
				map[string]any{"name": "greeter-svc", "port": float64(8080), "weight": float64(90)},
				map[string]any{"name": "greeter-next-svc", "port": float64(8080), "weight": float64(10)},
			}))
		})

		DescribeTable("reports why a backend on a release line does not resolve",
			func(objs []client.Object, reason, message string) {
				release := newRelease(testRelease, lineBackend(1))
				b := newBinding("line-unresolved", testRelease)
				r, cli := build(append([]client.Object{release, b, deployedComponent("greeter", dpNS)}, objs...)...)

				updated := reconcileBinding(r, cli, b)
				Expect(updated.Status.Backends).To(HaveLen(1))
				st := updated.Status.Backends[0]
				Expect(st.Resolved).To(BeFalse())
				Expect(st.Reason).To(Equal(reason))
				Expect(st.Message).To(ContainSubstring(message))
			},
			Entry("line does not exist", []client.Object{},
				BackendReasonReleaseLineNotFound, `component "greeter" has no release line "next"`),
			Entry("line belongs to another component", []client.Object{newReleaseLine("billing")},
				BackendReasonReleaseLineNotFound, `component "greeter" has no release line "next"`),
			Entry("line exists but is not deployed", []client.Object{newReleaseLine("greeter")},
				BackendReasonNotDeployed, `release line "next" of component "greeter" is not deployed`),
		)

		It("maps a ReleaseLine only to bindings routing to that line", func() {
			toLine := newRelease(testRelease, lineBackend(1))
			toMain := newRelease("main-release", componentBackend("stable", "greeter", 1))
			routes := newBinding("routes-to-line", toLine.Name)
			routesProd := newBinding("routes-to-line-prod", toLine.Name)
			routesProd.Spec.Environment = "prod"
			mainOnly := newBinding("routes-to-main", toMain.Name)
			r, _ := build(toLine, toMain, routes, routesProd, mainOnly)

			requests := r.listBindingsForReleaseLine(fctx, newReleaseLine("greeter"))
			Expect(requests).To(ConsistOf(
				reconcile.Request{NamespacedName: client.ObjectKeyFromObject(routes)},
				reconcile.Request{NamespacedName: client.ObjectKeyFromObject(routesProd)},
			))
		})
	})
})

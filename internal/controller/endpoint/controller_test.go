// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const routeTemplate = `{"apiVersion":"gateway.networking.k8s.io/v1","kind":"HTTPRoute","metadata":{"name":"x"}}`

// pathPrefixSchema declares a required pathPrefix with a default, so an
// Endpoint that omits it is valid once defaults are applied.
const pathPrefixSchema = `{"type":"object","required":["pathPrefix"],"properties":{"pathPrefix":{"type":"string","default":"/"}}}`

func newClusterEndpointType(name, paramsSchema string) *openchoreov1alpha1.ClusterEndpointType {
	cet := &openchoreov1alpha1.ClusterEndpointType{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: openchoreov1alpha1.ClusterEndpointTypeSpec{
			Resources: []openchoreov1alpha1.ResourceTemplate{{
				ID:       "route",
				Template: &runtime.RawExtension{Raw: []byte(routeTemplate)},
			}},
		},
	}
	if paramsSchema != "" {
		cet.Spec.Parameters = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(paramsSchema)},
		}
	}
	return cet
}

// newWorkload returns a Workload whose endpoints are all exposed externally,
// which is what an Endpoint backend requires.
func newWorkload(project, component string, endpoints ...string) *openchoreov1alpha1.Workload {
	eps := map[string]openchoreov1alpha1.WorkloadEndpoint{}
	for _, name := range endpoints {
		eps[name] = openchoreov1alpha1.WorkloadEndpoint{
			Type:       openchoreov1alpha1.EndpointProtocolHTTP,
			Port:       8080,
			Visibility: []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityExternal},
		}
	}
	return &openchoreov1alpha1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: project + "-" + component, Namespace: "default"},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{ProjectName: project, ComponentName: component},
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
				Container: openchoreov1alpha1.Container{Image: "nginx"},
				Endpoints: eps,
			},
		},
	}
}

func componentBackend(name, component, endpoint string, weight int32) openchoreov1alpha1.EndpointBackendRef {
	return openchoreov1alpha1.EndpointBackendRef{
		Name:      name,
		Kind:      openchoreov1alpha1.EndpointBackendKindComponent,
		Component: &openchoreov1alpha1.EndpointComponentBackend{Name: component, Endpoint: endpoint},
		Weight:    ptr.To(weight),
	}
}

func newEndpoint(
	name, project string, typeRef openchoreov1alpha1.EndpointTypeRef, backends ...openchoreov1alpha1.EndpointBackendRef,
) *openchoreov1alpha1.Endpoint {
	return &openchoreov1alpha1.Endpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  "default",
			Finalizers: []string{EndpointFinalizer},
		},
		Spec: openchoreov1alpha1.EndpointSpec{
			Owner:       openchoreov1alpha1.EndpointOwner{ProjectName: project},
			Type:        typeRef,
			Visibility:  []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityExternal},
			BackendRefs: backends,
		},
	}
}

func clusterTypeRef(name string) openchoreov1alpha1.EndpointTypeRef {
	return openchoreov1alpha1.EndpointTypeRef{Kind: openchoreov1alpha1.EndpointTypeRefKindClusterEndpointType, Name: name}
}

// create persists objects and registers their cleanup.
func create(objs ...client.Object) {
	for _, obj := range objs {
		Expect(k8sClient.Create(ctx, obj)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj) })
	}
}

var _ = Describe("Endpoint Controller", func() {
	var reconciler *Reconciler

	BeforeEach(func() {
		reconciler = &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	})

	// reconcileAndGet reconciles ep once and returns its persisted state.
	reconcileAndGet := func(ep *openchoreov1alpha1.Endpoint) *openchoreov1alpha1.Endpoint {
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(ep)})
		Expect(err).NotTo(HaveOccurred())
		updated := &openchoreov1alpha1.Endpoint{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ep), updated)).To(Succeed())
		return updated
	}

	expectReady := func(ep *openchoreov1alpha1.Endpoint, status metav1.ConditionStatus, reason string) *metav1.Condition {
		cond := meta.FindStatusCondition(ep.Status.Conditions, string(ConditionReady))
		Expect(cond).NotTo(BeNil(), "expected Ready condition to be set")
		Expect(cond.Status).To(Equal(status))
		Expect(cond.Reason).To(Equal(reason))
		return cond
	}

	It("sets Ready=False, Reason=EndpointTypeNotFound when the referenced type does not exist", func() {
		ep := newEndpoint("ep-missing-type", "p-missing-type", clusterTypeRef("does-not-exist"),
			componentBackend("stable", "greeter", "http", 1))
		create(ep)

		updated := reconcileAndGet(ep)
		cond := expectReady(updated, metav1.ConditionFalse, string(ReasonEndpointTypeNotFound))
		Expect(cond.Message).To(ContainSubstring(`ClusterEndpointType "does-not-exist" not found`))
		Expect(updated.Status.LatestRelease).To(BeNil())
	})

	It("cuts an EndpointRelease snapshotting the Endpoint and its ClusterEndpointType", func() {
		cet := newClusterEndpointType("cet-release", pathPrefixSchema)
		ep := newEndpoint("ep-release", "p-release", clusterTypeRef(cet.Name),
			componentBackend("stable", "greeter", "http", 90),
			componentBackend("canary", "greeter-v2", "http", 10))
		ep.Spec.Parameters = &runtime.RawExtension{Raw: []byte(`{"pathPrefix":"/greeter"}`)}
		create(cet,
			newWorkload("p-release", "greeter", "http"),
			newWorkload("p-release", "greeter-v2", "http"),
			ep)

		updated := reconcileAndGet(ep)
		expectReady(updated, metav1.ConditionTrue, string(ReasonReconciled))
		Expect(updated.Status.LatestRelease).NotTo(BeNil())
		Expect(updated.Status.LatestRelease.Name).To(Equal(ep.Name + "-" + updated.Status.LatestRelease.Hash))

		release := &openchoreov1alpha1.EndpointRelease{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: updated.Status.LatestRelease.Name},
			release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		Expect(release.Spec.Owner).To(Equal(openchoreov1alpha1.EndpointReleaseOwner{
			ProjectName: "p-release", EndpointName: ep.Name,
		}))
		Expect(release.Spec.EndpointType.Kind).To(Equal(openchoreov1alpha1.EndpointTypeRefKindClusterEndpointType))
		Expect(release.Spec.EndpointType.Name).To(Equal(cet.Name))
		Expect(release.Spec.EndpointType.Spec.Resources).To(HaveLen(1))
		Expect(release.Spec.Visibility).To(Equal(ep.Spec.Visibility))
		Expect(release.Spec.BackendRefs).To(Equal(updated.Spec.BackendRefs))
		Expect(string(release.Spec.Parameters.Raw)).To(MatchJSON(`{"pathPrefix":"/greeter"}`))
	})

	It("keeps the same release when nothing changed and cuts a new one when the spec changes", func() {
		cet := newClusterEndpointType("cet-rehash", pathPrefixSchema)
		ep := newEndpoint("ep-rehash", "p-rehash", clusterTypeRef(cet.Name),
			componentBackend("stable", "greeter", "http", 1))
		create(cet, newWorkload("p-rehash", "greeter", "http"), ep)

		first := reconcileAndGet(ep).Status.LatestRelease
		Expect(first).NotTo(BeNil())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.EndpointRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: first.Name},
			})
		})

		Expect(reconcileAndGet(ep).Status.LatestRelease).To(Equal(first), "an unchanged spec must not cut a new release")

		current := &openchoreov1alpha1.Endpoint{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ep), current)).To(Succeed())
		current.Spec.BackendRefs[0].Weight = ptr.To[int32](5)
		Expect(k8sClient.Update(ctx, current)).To(Succeed())

		second := reconcileAndGet(ep).Status.LatestRelease
		Expect(second).NotTo(BeNil())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.EndpointRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: second.Name},
			})
		})
		Expect(second.Name).NotTo(Equal(first.Name))

		// The earlier release is left in place for bindings still pinned to it.
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: first.Name},
			&openchoreov1alpha1.EndpointRelease{})).To(Succeed())
	})

	It("resolves a namespaced EndpointType when kind is omitted", func() {
		et := &openchoreov1alpha1.EndpointType{
			ObjectMeta: metav1.ObjectMeta{Name: "et-namespaced", Namespace: "default"},
			Spec: openchoreov1alpha1.EndpointTypeSpec{
				Resources: []openchoreov1alpha1.ResourceTemplate{{
					ID:       "route",
					Template: &runtime.RawExtension{Raw: []byte(routeTemplate)},
				}},
			},
		}
		ep := newEndpoint("ep-namespaced", "p-namespaced", openchoreov1alpha1.EndpointTypeRef{Name: et.Name},
			componentBackend("stable", "greeter", "http", 1))
		create(et, newWorkload("p-namespaced", "greeter", "http"), ep)

		updated := reconcileAndGet(ep)
		expectReady(updated, metav1.ConditionTrue, string(ReasonReconciled))
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.EndpointRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: updated.Status.LatestRelease.Name},
			})
		})

		release := &openchoreov1alpha1.EndpointRelease{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: updated.Status.LatestRelease.Name},
			release)).To(Succeed())
		Expect(release.Spec.EndpointType.Kind).To(Equal(openchoreov1alpha1.EndpointTypeRefKindEndpointType))
	})

	It("accepts omitted parameters that the schema defaults", func() {
		cet := newClusterEndpointType("cet-defaults", pathPrefixSchema)
		ep := newEndpoint("ep-defaults", "p-defaults", clusterTypeRef(cet.Name),
			componentBackend("stable", "greeter", "http", 1))
		create(cet, newWorkload("p-defaults", "greeter", "http"), ep)

		updated := reconcileAndGet(ep)
		expectReady(updated, metav1.ConditionTrue, string(ReasonReconciled))
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.EndpointRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: updated.Status.LatestRelease.Name},
			})
		})
	})

	It("sets Ready=False, Reason=InvalidParameters and cuts no release when parameters violate the schema", func() {
		cet := newClusterEndpointType("cet-invalid-params", pathPrefixSchema)
		ep := newEndpoint("ep-invalid-params", "p-invalid-params", clusterTypeRef(cet.Name),
			componentBackend("stable", "greeter", "http", 1))
		ep.Spec.Parameters = &runtime.RawExtension{Raw: []byte(`{"pathPrefix":42}`)}
		create(cet, newWorkload("p-invalid-params", "greeter", "http"), ep)

		updated := reconcileAndGet(ep)
		cond := expectReady(updated, metav1.ConditionFalse, string(ReasonInvalidParameters))
		Expect(cond.Message).To(ContainSubstring("pathPrefix"))
		Expect(updated.Status.LatestRelease).To(BeNil())
	})

	It("sets Ready=False, Reason=BackendNotFound when a backend component has no Workload", func() {
		cet := newClusterEndpointType("cet-no-workload", "")
		ep := newEndpoint("ep-no-workload", "p-no-workload", clusterTypeRef(cet.Name),
			componentBackend("stable", "greeter", "http", 1),
			componentBackend("canary", "missing", "http", 1))
		create(cet, newWorkload("p-no-workload", "greeter", "http"), ep)

		updated := reconcileAndGet(ep)
		cond := expectReady(updated, metav1.ConditionFalse, string(ReasonBackendNotFound))
		Expect(cond.Message).To(Equal(`backend "canary": component "missing" in project "p-no-workload" has no Workload`))
		Expect(updated.Status.LatestRelease).To(BeNil())
	})

	It("does not match a Workload of the same component name in another project", func() {
		cet := newClusterEndpointType("cet-other-project", "")
		ep := newEndpoint("ep-other-project", "p-mine", clusterTypeRef(cet.Name),
			componentBackend("stable", "greeter", "http", 1))
		create(cet, newWorkload("p-theirs", "greeter", "http"), ep)

		updated := reconcileAndGet(ep)
		expectReady(updated, metav1.ConditionFalse, string(ReasonBackendNotFound))
	})

	It("sets Ready=False, Reason=BackendNotExposed and cuts no release when the workload endpoint is not gateway-visible", func() {
		cet := newClusterEndpointType("cet-not-exposed", "")
		ep := newEndpoint("ep-not-exposed", "p-not-exposed", clusterTypeRef(cet.Name),
			componentBackend("stable", "greeter", "http", 1))
		w := newWorkload("p-not-exposed", "greeter", "http")
		w.Spec.Endpoints["http"] = openchoreov1alpha1.WorkloadEndpoint{
			Type:       openchoreov1alpha1.EndpointProtocolHTTP,
			Port:       8080,
			Visibility: []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityNamespace},
		}
		create(cet, w, ep)

		updated := reconcileAndGet(ep)
		cond := expectReady(updated, metav1.ConditionFalse, string(ReasonBackendNotExposed))
		Expect(cond.Message).To(Equal(`backend "stable": workload endpoint "http" of component "greeter" ` +
			`is not exposed through the gateway (visibility: project, namespace); set internal or external visibility on it`))
		Expect(updated.Status.LatestRelease).To(BeNil())
	})

	It("accepts a backend whose workload endpoint is internal", func() {
		cet := newClusterEndpointType("cet-internal", "")
		ep := newEndpoint("ep-internal", "p-internal", clusterTypeRef(cet.Name),
			componentBackend("stable", "greeter", "http", 1))
		w := newWorkload("p-internal", "greeter", "http")
		w.Spec.Endpoints["http"] = openchoreov1alpha1.WorkloadEndpoint{
			Type:       openchoreov1alpha1.EndpointProtocolHTTP,
			Port:       8080,
			Visibility: []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityInternal},
		}
		create(cet, w, ep)

		updated := reconcileAndGet(ep)
		expectReady(updated, metav1.ConditionTrue, string(ReasonReconciled))
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.EndpointRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: updated.Status.LatestRelease.Name},
			})
		})
	})

	It("reports every offending backend, taking the reason from the first", func() {
		cet := newClusterEndpointType("cet-mixed", "")
		ep := newEndpoint("ep-mixed", "p-mixed", clusterTypeRef(cet.Name),
			componentBackend("stable", "greeter", "http", 1),
			componentBackend("canary", "missing", "http", 1))
		w := newWorkload("p-mixed", "greeter", "http")
		w.Spec.Endpoints["http"] = openchoreov1alpha1.WorkloadEndpoint{Type: openchoreov1alpha1.EndpointProtocolHTTP, Port: 8080}
		create(cet, w, ep)

		updated := reconcileAndGet(ep)
		cond := expectReady(updated, metav1.ConditionFalse, string(ReasonBackendNotExposed))
		Expect(cond.Message).To(ContainSubstring(`backend "stable": workload endpoint "http"`))
		Expect(cond.Message).To(ContainSubstring(`backend "canary": component "missing" in project "p-mixed" has no Workload`))
	})

	It("sets Ready=False, Reason=BackendNotFound naming the available endpoints when the workload endpoint is missing", func() {
		cet := newClusterEndpointType("cet-no-endpoint", "")
		ep := newEndpoint("ep-no-endpoint", "p-no-endpoint", clusterTypeRef(cet.Name),
			componentBackend("stable", "greeter", "grpc", 1))
		create(cet, newWorkload("p-no-endpoint", "greeter", "http", "admin"), ep)

		updated := reconcileAndGet(ep)
		cond := expectReady(updated, metav1.ConditionFalse, string(ReasonBackendNotFound))
		Expect(cond.Message).To(Equal(
			`backend "stable": component "greeter" has no workload endpoint "grpc" (available: admin, http)`))
		Expect(updated.Status.LatestRelease).To(BeNil())
	})
})

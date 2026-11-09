// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// These check which EndpointReleaseBindings each watch mapper wakes, through a
// fake client registered with the production owner index.
var _ = Describe("Watch mappers", func() {
	fctx := context.Background()

	reconcilerWith := func(objs ...client.Object) *Reconciler {
		cli := fake.NewClientBuilder().
			WithScheme(scheme.Scheme).
			WithObjects(objs...).
			WithIndex(&openchoreov1alpha1.EndpointReleaseBinding{},
				controller.IndexKeyEndpointReleaseBindingOwnerEndpointName, controller.IndexEndpointReleaseBindingOwner).
			Build()
		return &Reconciler{Client: cli, Scheme: scheme.Scheme}
	}

	// binding returns a binding of endpoint in namespace, targeting env and
	// pinned to release.
	binding := func(name, namespace, project, endpoint, env, release string) *openchoreov1alpha1.EndpointReleaseBinding {
		b := newBinding(name, release)
		b.Namespace = namespace
		b.Spec.Owner = openchoreov1alpha1.EndpointReleaseBindingOwner{ProjectName: project, EndpointName: endpoint}
		b.Spec.Environment = env
		return b
	}

	environment := func(name, namespace string, kind openchoreov1alpha1.DataPlaneRefKind, dataPlane string) *openchoreov1alpha1.Environment {
		return &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{Kind: kind, Name: dataPlane},
			},
		}
	}

	names := func(requests []reconcile.Request) []string {
		out := make([]string, 0, len(requests))
		for _, r := range requests {
			out = append(out, r.Namespace+"/"+r.Name)
		}
		return out
	}

	Describe("listBindingsForReleaseBinding", func() {
		greeterRelease := newRelease("greeter-api-r1", componentBackend("stable", "greeter", 1))
		otherRelease := newRelease("orders-api-r1", componentBackend("stable", "orders", 1))

		It("wakes only the bindings in the component's project and environment whose release routes to it", func() {
			r := reconcilerWith(
				greeterRelease, otherRelease,
				binding("routes-to-greeter", testNamespace, testProject, testEndpoint, testEnvName, greeterRelease.Name),
				binding("routes-elsewhere", testNamespace, testProject, "orders-api", testEnvName, otherRelease.Name),
				binding("other-environment", testNamespace, testProject, testEndpoint, "staging", greeterRelease.Name),
				binding("other-project", testNamespace, "billing", testEndpoint, testEnvName, greeterRelease.Name),
				binding("no-release-yet", testNamespace, testProject, testEndpoint, testEnvName, ""),
				binding("release-missing", testNamespace, testProject, testEndpoint, testEnvName, "greeter-api-gone"),
			)

			got := r.listBindingsForReleaseBinding(fctx, deployedComponent("greeter", "dp-ns"))

			Expect(names(got)).To(ConsistOf(testNamespace + "/routes-to-greeter"))
		})

		It("wakes nothing when no binding routes to the component", func() {
			r := reconcilerWith(otherRelease,
				binding("routes-elsewhere", testNamespace, testProject, "orders-api", testEnvName, otherRelease.Name))

			Expect(r.listBindingsForReleaseBinding(fctx, deployedComponent("greeter", "dp-ns"))).To(BeEmpty())
		})
	})

	Describe("routesToComponent", func() {
		It("matches a Component backend by component name", func() {
			release := newRelease("r", componentBackend("a", "greeter", 1), componentBackend("b", "orders", 0))
			Expect(routesToComponent(release, "orders")).To(BeTrue())
			Expect(routesToComponent(release, "billing")).To(BeFalse())
		})

		It("ignores backends without a component", func() {
			release := newRelease("r", openchoreov1alpha1.EndpointBackendRef{Name: "bare"})
			Expect(routesToComponent(release, "greeter")).To(BeFalse())
		})
	})

	It("listBindingsForEndpointRelease wakes the endpoint's bindings pinned to the release", func() {
		r := reconcilerWith(
			binding("pinned", testNamespace, testProject, testEndpoint, testEnvName, testRelease),
			binding("pinned-elsewhere", testNamespace, testProject, testEndpoint, "staging", "greeter-api-older"),
			binding("other-endpoint", testNamespace, testProject, "orders-api", testEnvName, testRelease),
		)

		got := r.listBindingsForEndpointRelease(fctx, newRelease(testRelease))

		Expect(names(got)).To(ConsistOf(testNamespace + "/pinned"))
	})

	It("listBindingsForEndpoint wakes every binding of the endpoint and no other", func() {
		r := reconcilerWith(
			binding("dev", testNamespace, testProject, testEndpoint, testEnvName, testRelease),
			binding("staging", testNamespace, testProject, testEndpoint, "staging", ""),
			binding("other-endpoint", testNamespace, testProject, "orders-api", testEnvName, testRelease),
			binding("other-namespace", "acme", testProject, testEndpoint, testEnvName, testRelease),
		)

		got := r.listBindingsForEndpoint(fctx, newEndpointCR())

		Expect(names(got)).To(ConsistOf(testNamespace+"/dev", testNamespace+"/staging"))
	})

	It("listBindingsForEnvironment wakes the bindings targeting the environment in its namespace", func() {
		r := reconcilerWith(
			binding("dev-a", testNamespace, testProject, testEndpoint, testEnvName, ""),
			binding("dev-b", testNamespace, "billing", "invoices-api", testEnvName, ""),
			binding("staging", testNamespace, testProject, testEndpoint, "staging", ""),
			binding("dev-other-namespace", "acme", testProject, testEndpoint, testEnvName, ""),
		)

		got := r.listBindingsForEnvironment(fctx, newEnvironment())

		Expect(names(got)).To(ConsistOf(testNamespace+"/dev-a", testNamespace+"/dev-b"))
	})

	It("listBindingsForProject wakes the bindings owned by the project in its namespace", func() {
		r := reconcilerWith(
			binding("dev", testNamespace, testProject, testEndpoint, testEnvName, ""),
			binding("staging", testNamespace, testProject, "orders-api", "staging", ""),
			binding("other-project", testNamespace, "billing", "invoices-api", testEnvName, ""),
			binding("other-namespace", "acme", testProject, testEndpoint, testEnvName, ""),
		)

		got := r.listBindingsForProject(fctx, newProject())

		Expect(names(got)).To(ConsistOf(testNamespace+"/dev", testNamespace+"/staging"))
	})

	Describe("listBindingsForDataPlane", func() {
		It("wakes the bindings whose environment uses the DataPlane", func() {
			r := reconcilerWith(
				newEnvironment(), // dev -> DataPlane "primary"
				environment("staging", testNamespace, openchoreov1alpha1.DataPlaneRefKindDataPlane, "secondary"),
				environment("prod", testNamespace, openchoreov1alpha1.DataPlaneRefKindClusterDataPlane, "primary"),
				binding("dev", testNamespace, testProject, testEndpoint, testEnvName, ""),
				binding("staging", testNamespace, testProject, testEndpoint, "staging", ""),
				binding("prod", testNamespace, testProject, testEndpoint, "prod", ""),
			)

			got := r.listBindingsForDataPlane(fctx, newDataPlane())

			Expect(names(got)).To(ConsistOf(testNamespace + "/dev"))
		})

		It("wakes nothing when no environment uses the DataPlane", func() {
			r := reconcilerWith(
				environment("staging", testNamespace, openchoreov1alpha1.DataPlaneRefKindDataPlane, "secondary"),
				binding("staging", testNamespace, testProject, testEndpoint, "staging", ""),
			)

			Expect(r.listBindingsForDataPlane(fctx, newDataPlane())).To(BeEmpty())
		})
	})

	It("listBindingsForClusterDataPlane wakes bindings in any namespace whose environment uses it", func() {
		shared := &openchoreov1alpha1.ClusterDataPlane{ObjectMeta: metav1.ObjectMeta{Name: "shared"}}
		r := reconcilerWith(
			environment("dev", "acme", openchoreov1alpha1.DataPlaneRefKindClusterDataPlane, "shared"),
			environment("prod", "globex", openchoreov1alpha1.DataPlaneRefKindClusterDataPlane, "shared"),
			// Same name as the ClusterDataPlane, but a namespaced DataPlane.
			environment("dev", testNamespace, openchoreov1alpha1.DataPlaneRefKindDataPlane, "shared"),
			binding("acme-dev", "acme", testProject, testEndpoint, "dev", ""),
			binding("globex-prod", "globex", testProject, testEndpoint, "prod", ""),
			binding("default-dev", testNamespace, testProject, testEndpoint, "dev", ""),
		)

		got := r.listBindingsForClusterDataPlane(fctx, shared)

		Expect(names(got)).To(ConsistOf("acme/acme-dev", "globex/globex-prod"))
	})
})

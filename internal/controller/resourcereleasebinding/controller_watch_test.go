// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

var _ = Describe("ResourceRelease watch mapper", func() {
	mapperCtx := context.Background()

	newBinding := func(name, ns, releaseName string) *openchoreov1alpha1.ResourceReleaseBinding {
		return &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
					ProjectName:  "p",
					ResourceName: "r",
				},
				Environment:     "dev",
				ResourceRelease: releaseName,
			},
		}
	}

	buildClient := func(objs ...client.Object) client.Client {
		s := runtime.NewScheme()
		Expect(openchoreov1alpha1.AddToScheme(s)).To(Succeed())
		return fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(objs...).
			WithIndex(&openchoreov1alpha1.ResourceReleaseBinding{},
				resourceReleaseRefIndex, indexResourceReleaseRef).
			Build()
	}

	It("enqueues only bindings in the same namespace whose spec.resourceRelease matches", func() {
		matching := newBinding("matches", "ns1", "r-abc123")
		differentRelease := newBinding("other-release", "ns1", "r-def456")
		differentNs := newBinding("other-ns", "ns2", "r-abc123")
		unsetPin := newBinding("unset-pin", "ns1", "")

		r := &Reconciler{Client: buildClient(matching, differentRelease, differentNs, unsetPin)}
		release := &openchoreov1alpha1.ResourceRelease{
			ObjectMeta: metav1.ObjectMeta{Name: "r-abc123", Namespace: "ns1"},
		}

		requests := r.listResourceReleaseBindingsForResourceRelease(mapperCtx, release)

		Expect(requests).To(HaveLen(1))
		Expect(requests[0].Namespace).To(Equal("ns1"))
		Expect(requests[0].Name).To(Equal("matches"))
	})

	It("returns no requests when no binding references the release", func() {
		r := &Reconciler{Client: buildClient(newBinding("a", "ns1", "r-other"))}
		release := &openchoreov1alpha1.ResourceRelease{
			ObjectMeta: metav1.ObjectMeta{Name: "r-abc123", Namespace: "ns1"},
		}

		Expect(r.listResourceReleaseBindingsForResourceRelease(mapperCtx, release)).To(BeEmpty())
	})
})

var _ = Describe("Environment and data plane watch mappers", func() {
	mapperCtx := context.Background()

	newBinding := func(name, ns, env string) *openchoreov1alpha1.ResourceReleaseBinding {
		return &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
					ProjectName:  "p",
					ResourceName: "r",
				},
				Environment: env,
			},
		}
	}

	newEnv := func(name, ns string, ref *openchoreov1alpha1.DataPlaneRef) *openchoreov1alpha1.Environment {
		return &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       openchoreov1alpha1.EnvironmentSpec{DataPlaneRef: ref},
		}
	}

	dpRef := func(kind openchoreov1alpha1.DataPlaneRefKind, name string) *openchoreov1alpha1.DataPlaneRef {
		return &openchoreov1alpha1.DataPlaneRef{Kind: kind, Name: name}
	}

	buildClient := func(objs ...client.Object) client.Client {
		s := runtime.NewScheme()
		Expect(openchoreov1alpha1.AddToScheme(s)).To(Succeed())
		return fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(objs...).
			WithIndex(&openchoreov1alpha1.ResourceReleaseBinding{},
				environmentRefIndex, indexEnvironmentRef).
			Build()
	}

	keys := func(requests []reconcile.Request) []string {
		out := make([]string, 0, len(requests))
		for _, req := range requests {
			out = append(out, req.Namespace+"/"+req.Name)
		}
		return out
	}

	// Shared fixture: ns1 has one Environment per data plane shape (namespaced
	// DataPlane, a DataPlane with a different name, ClusterDataPlane with the
	// same name, no ref); ns2 repeats the DataPlane, ClusterDataPlane and no-ref
	// shapes to cover namespace scoping.
	fixture := func() []client.Object {
		return []client.Object{
			newEnv("dev", "ns1", dpRef(openchoreov1alpha1.DataPlaneRefKindDataPlane, "dp1")),
			newEnv("qa", "ns1", dpRef(openchoreov1alpha1.DataPlaneRefKindDataPlane, "dp2")),
			newEnv("staging", "ns1", dpRef(openchoreov1alpha1.DataPlaneRefKindClusterDataPlane, "dp1")),
			newEnv("prod", "ns1", nil),
			newEnv("dev", "ns2", dpRef(openchoreov1alpha1.DataPlaneRefKindDataPlane, "dp1")),
			newEnv("staging", "ns2", dpRef(openchoreov1alpha1.DataPlaneRefKindClusterDataPlane, "dp1")),
			newEnv("prod", "ns2", nil),
			newBinding("db-dev", "ns1", "dev"),
			newBinding("cache-dev", "ns1", "dev"),
			newBinding("db-qa", "ns1", "qa"),
			newBinding("db-staging", "ns1", "staging"),
			newBinding("db-prod", "ns1", "prod"),
			newBinding("db-dev", "ns2", "dev"),
			newBinding("db-staging", "ns2", "staging"),
			newBinding("db-prod", "ns2", "prod"),
		}
	}

	Describe("Environment", func() {
		It("enqueues only bindings in the same namespace that target the environment", func() {
			r := &Reconciler{Client: buildClient(fixture()...)}
			env := newEnv("dev", "ns1", nil)

			Expect(keys(r.listResourceReleaseBindingsForEnvironment(mapperCtx, env))).
				To(ConsistOf("ns1/db-dev", "ns1/cache-dev"))
		})

		It("returns no requests when no binding targets the environment", func() {
			r := &Reconciler{Client: buildClient(fixture()...)}
			env := newEnv("unused", "ns1", nil)

			Expect(r.listResourceReleaseBindingsForEnvironment(mapperCtx, env)).To(BeEmpty())
		})
	})

	Describe("DataPlane", func() {
		It("enqueues bindings whose environment references the DataPlane by kind and name in its namespace", func() {
			r := &Reconciler{Client: buildClient(fixture()...)}
			dp := &openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Name: "dp1", Namespace: "ns1"}}

			// ns1/qa points at a different DataPlane, ns1/staging points at a
			// ClusterDataPlane with the same name and ns1/prod has no
			// dataPlaneRef; none must match. ns2/dev is out of the DataPlane's
			// namespace.
			Expect(keys(r.listResourceReleaseBindingsForDataPlane(mapperCtx, dp))).
				To(ConsistOf("ns1/db-dev", "ns1/cache-dev"))
		})

		It("enqueues bindings of environments without a dataPlaneRef when the default DataPlane lands", func() {
			r := &Reconciler{Client: buildClient(fixture()...)}
			dp := &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Name: controller.DefaultPlaneName, Namespace: "ns1"},
			}

			// Only ns1/prod falls back to the default DataPlane in ns1; ns2/prod
			// is out of the DataPlane's namespace.
			Expect(keys(r.listResourceReleaseBindingsForDataPlane(mapperCtx, dp))).
				To(ConsistOf("ns1/db-prod"))
		})

		It("returns no requests when no environment references the DataPlane", func() {
			r := &Reconciler{Client: buildClient(fixture()...)}
			dp := &openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Name: "unused", Namespace: "ns1"}}

			Expect(r.listResourceReleaseBindingsForDataPlane(mapperCtx, dp)).To(BeEmpty())
		})
	})

	Describe("ClusterDataPlane", func() {
		It("enqueues bindings across namespaces whose environment references the ClusterDataPlane", func() {
			r := &Reconciler{Client: buildClient(fixture()...)}
			cdp := &openchoreov1alpha1.ClusterDataPlane{ObjectMeta: metav1.ObjectMeta{Name: "dp1"}}

			// The dev environments point at a namespaced DataPlane with the
			// same name and must not match.
			Expect(keys(r.listResourceReleaseBindingsForClusterDataPlane(mapperCtx, cdp))).
				To(ConsistOf("ns1/db-staging", "ns2/db-staging"))
		})

		It("enqueues bindings of environments without a dataPlaneRef across namespaces when the default ClusterDataPlane lands", func() {
			r := &Reconciler{Client: buildClient(fixture()...)}
			cdp := &openchoreov1alpha1.ClusterDataPlane{
				ObjectMeta: metav1.ObjectMeta{Name: controller.DefaultPlaneName},
			}

			Expect(keys(r.listResourceReleaseBindingsForClusterDataPlane(mapperCtx, cdp))).
				To(ConsistOf("ns1/db-prod", "ns2/db-prod"))
		})

		It("returns no requests when no environment references the ClusterDataPlane", func() {
			r := &Reconciler{Client: buildClient(fixture()...)}
			cdp := &openchoreov1alpha1.ClusterDataPlane{ObjectMeta: metav1.ObjectMeta{Name: "unused"}}

			Expect(r.listResourceReleaseBindingsForClusterDataPlane(mapperCtx, cdp)).To(BeEmpty())
		})
	})
})

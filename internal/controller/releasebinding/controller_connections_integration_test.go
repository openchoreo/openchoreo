// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// providerRBFixture builds a provider ReleaseBinding that is discoverable through
// the owner/env field index. Providers carry no finalizer so that AfterEach deletion
// completes immediately; with a finalizer the delete would hang in Terminating, since
// nothing reconciles these providers to remove it.
func providerRBFixture(name, project, component, envName string) *openchoreov1alpha1.ReleaseBinding {
	return &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner: openchoreov1alpha1.ReleaseBindingOwner{
				ProjectName:   project,
				ComponentName: component,
			},
			Environment: envName,
			ReleaseName: name + "-release",
		},
	}
}

// setProviderEndpoints writes the provider RB's status.endpoints via the status
// subresource so the resolver can read them through the cache.
func setProviderEndpoints(rb *openchoreov1alpha1.ReleaseBinding, endpoints []openchoreov1alpha1.EndpointURLStatus) {
	GinkgoHelper()
	fetched := &openchoreov1alpha1.ReleaseBinding{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: rb.Name}, fetched)).To(Succeed())
	fetched.Status.Endpoints = endpoints
	Expect(k8sClient.Status().Update(ctx, fetched)).To(Succeed())
}

// hasPendingReason reports whether any of the RB's PendingConnections carries a
// Reason containing substr. Used to wait for an EXPECTED terminal pending shape
// rather than any transient pending state surfaced by the cached client.
func hasPendingReason(rb *openchoreov1alpha1.ReleaseBinding, substr string) bool {
	for _, pc := range rb.Status.PendingConnections {
		if strings.Contains(pc.Reason, substr) {
			return true
		}
	}
	return false
}

// These specs exercise connection resolution from the consumer ReleaseBinding's point of view —
// it is the only RB that reconciles. For each dependency the reconcile records a ConnectionTarget
// (defaulting an empty Project to the consumer's project), then looks up the provider ReleaseBinding
// through the owner+env field index (consumer namespace; key = provider project/component +
// environment) and reads URLs from the provider's status.Endpoints. Results land in
// status.ResolvedConnections / status.PendingConnections and drive the ConnectionsResolved
// condition; while any connection is unresolved the reconcile returns before marking ReleaseSynced
// (the connection stability guard, without requesting an explicit requeue).
//
// The provider ReleaseBindings are passive fixtures. Reconciliation is driven manually and only the
// consumer RB is ever reconciled (the suite registers no controller), so the providers are never
// reconciled; their status.Endpoints is hand-set via setProviderEndpoints to stand in for what a
// provider reconcile would publish. They carry no finalizer so AfterEach deletion completes
// immediately (a finalizer would block, since nothing reconciles them to remove it). So only the
// consumer's resolution logic is under test.
var _ = Describe("ReleaseBinding connection resolution", func() {
	// S1: no provider ReleaseBinding exists for the connection's owner+env. The consumer still
	// records the ConnectionTarget (empty Project => consumer-project fallback), marks the
	// connection pending "not found", and the guard blocks ReleaseSynced.
	Context("S1: when the provider ReleaseBinding is not found", func() {
		const (
			rbName   = "rb-conn-s1"
			crName   = "cr-conn-s1"
			envName  = "env-conn-s1"
			dpName   = "dp-conn-s1"
			compName = "comp-conn-s1"
			projName = "proj-conn-s1"
			provComp = "provider-s1"
			epName   = "api"
		)

		AfterEach(func() {
			forceDelete(rbName)
			forceDeleteRelease(compName + "-" + envName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: projName},
			})
		})

		It("surfaces a pending connection with default-project fallback and blocks ReleaseSynced", func() {
			By("Creating fixtures (project, component, env, dp)")
			Expect(k8sClient.Create(ctx, projectFixture(projName))).To(Succeed())
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, componentFixture(compName, projName))).To(Succeed())

			By("Creating ComponentRelease with a connection (empty Project => default fallback)")
			cr := crFixture(crName, projName, compName)
			cr.Spec.Workload.Dependencies = &openchoreov1alpha1.WorkloadDependencies{
				Endpoints: []openchoreov1alpha1.WorkloadConnection{
					{
						Component:   provComp,
						Name:        epName,
						Visibility:  string(openchoreov1alpha1.EndpointVisibilityProject),
						EnvBindings: openchoreov1alpha1.ConnectionEnvBindings{Host: "DEP_HOST"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			By("Creating consumer ReleaseBinding")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, projName, compName, envName, crName, true),
			)).To(Succeed())

			By("Reconciling until the not-found pending connection is surfaced")
			r := testReconcilerWithCachedClient()
			rb := reconcileUntil(r, rbName, func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				return hasPendingReason(rb, "ReleaseBinding not found")
			})

			By("ConnectionTargets reflect the workload connection with default project fallback")
			Expect(rb.Status.ConnectionTargets).To(HaveLen(1))
			Expect(rb.Status.ConnectionTargets[0].Component).To(Equal(provComp))
			Expect(rb.Status.ConnectionTargets[0].Endpoint).To(Equal(epName))
			Expect(rb.Status.ConnectionTargets[0].Project).To(Equal(projName))
			Expect(rb.Status.ConnectionTargets[0].Environment).To(Equal(envName))

			By("PendingConnections surface the not-found reason with default project")
			Expect(rb.Status.PendingConnections).To(HaveLen(1))
			Expect(rb.Status.PendingConnections[0].Project).To(Equal(projName))
			Expect(rb.Status.PendingConnections[0].Reason).To(ContainSubstring("ReleaseBinding not found"))
			Expect(rb.Status.ResolvedConnections).To(BeEmpty())

			By("ConditionConnectionsResolved=False/ConnectionsPending")
			cond := conditionFor(rb, string(ConditionConnectionsResolved))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonConnectionsPending)))

			By("ConditionReleaseSynced is NOT True (guard blocked sync)")
			synced := conditionFor(rb, string(ConditionReleaseSynced))
			Expect(synced == nil || synced.Status != metav1.ConditionTrue).To(BeTrue())
		})
	})

	// S2: two provider ReleaseBindings share the same owner+env index key, so the consumer's
	// lookup is ambiguous. It refuses to pick one and marks the connection pending with a
	// cardinality-violation reason.
	Context("S2: when multiple provider ReleaseBindings match", func() {
		const (
			rbName   = "rb-conn-s2"
			crName   = "cr-conn-s2"
			envName  = "env-conn-s2"
			dpName   = "dp-conn-s2"
			compName = "comp-conn-s2"
			projName = "proj-conn-s2"
			provComp = "provider-s2"
			provA    = "prov-s2-a"
			provB    = "prov-s2-b"
			epName   = "api"
		)

		AfterEach(func() {
			forceDelete(rbName)
			forceDelete(provA)
			forceDelete(provB)
			forceDeleteRelease(compName + "-" + envName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: projName},
			})
		})

		It("surfaces a cardinality-violation pending connection", func() {
			By("Creating fixtures")
			Expect(k8sClient.Create(ctx, projectFixture(projName))).To(Succeed())
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, componentFixture(compName, projName))).To(Succeed())

			By("Creating two provider RBs with the same owner+env index key")
			Expect(k8sClient.Create(ctx, providerRBFixture(provA, projName, provComp, envName))).To(Succeed())
			Expect(k8sClient.Create(ctx, providerRBFixture(provB, projName, provComp, envName))).To(Succeed())

			By("Creating ComponentRelease with a connection to the duplicated provider")
			cr := crFixture(crName, projName, compName)
			cr.Spec.Workload.Dependencies = &openchoreov1alpha1.WorkloadDependencies{
				Endpoints: []openchoreov1alpha1.WorkloadConnection{
					{
						Project:     projName,
						Component:   provComp,
						Name:        epName,
						Visibility:  string(openchoreov1alpha1.EndpointVisibilityProject),
						EnvBindings: openchoreov1alpha1.ConnectionEnvBindings{Host: "DEP_HOST"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			By("Creating consumer ReleaseBinding")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, projName, compName, envName, crName, true),
			)).To(Succeed())

			By("Reconciling until the multiple-bindings pending connection is surfaced")
			r := testReconcilerWithCachedClient()
			rb := reconcileUntil(r, rbName, func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				return hasPendingReason(rb, "multiple ReleaseBindings found")
			})

			By("PendingConnections surface the multiple-bindings reason")
			Expect(rb.Status.PendingConnections).To(HaveLen(1))
			Expect(rb.Status.PendingConnections[0].Reason).To(ContainSubstring("multiple ReleaseBindings found"))

			By("ConditionConnectionsResolved=False/ConnectionsPending")
			cond := conditionFor(rb, string(ConditionConnectionsResolved))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonConnectionsPending)))
		})
	})

	// S3: the single provider ReleaseBinding exists but is in the Undeploy state. The consumer
	// finds it, treats an undeployed provider as unavailable, and marks the connection pending
	// "undeployed" rather than resolving to a stale URL.
	Context("S3: when the provider component is undeployed", func() {
		const (
			rbName   = "rb-conn-s3"
			crName   = "cr-conn-s3"
			envName  = "env-conn-s3"
			dpName   = "dp-conn-s3"
			compName = "comp-conn-s3"
			projName = "proj-conn-s3"
			provComp = "provider-s3"
			provRB   = "prov-s3"
			epName   = "api"
		)

		AfterEach(func() {
			forceDelete(rbName)
			forceDelete(provRB)
			forceDeleteRelease(compName + "-" + envName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: projName},
			})
		})

		It("surfaces an undeployed pending connection", func() {
			By("Creating fixtures")
			Expect(k8sClient.Create(ctx, projectFixture(projName))).To(Succeed())
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, componentFixture(compName, projName))).To(Succeed())

			By("Creating an undeployed provider RB")
			prov := providerRBFixture(provRB, projName, provComp, envName)
			prov.Spec.State = openchoreov1alpha1.ReleaseStateUndeploy
			Expect(k8sClient.Create(ctx, prov)).To(Succeed())

			By("Creating ComponentRelease with a connection to the undeployed provider")
			cr := crFixture(crName, projName, compName)
			cr.Spec.Workload.Dependencies = &openchoreov1alpha1.WorkloadDependencies{
				Endpoints: []openchoreov1alpha1.WorkloadConnection{
					{
						Project:     projName,
						Component:   provComp,
						Name:        epName,
						Visibility:  string(openchoreov1alpha1.EndpointVisibilityProject),
						EnvBindings: openchoreov1alpha1.ConnectionEnvBindings{Host: "DEP_HOST"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			By("Creating consumer ReleaseBinding")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, projName, compName, envName, crName, true),
			)).To(Succeed())

			By("Reconciling until the undeployed pending connection is surfaced")
			r := testReconcilerWithCachedClient()
			rb := reconcileUntil(r, rbName, func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				return hasPendingReason(rb, "component is undeployed")
			})

			By("PendingConnections surface the undeployed reason")
			Expect(rb.Status.PendingConnections).To(HaveLen(1))
			Expect(rb.Status.PendingConnections[0].Reason).To(ContainSubstring("component is undeployed"))

			By("ConditionConnectionsResolved=False/ConnectionsPending")
			cond := conditionFor(rb, string(ConditionConnectionsResolved))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonConnectionsPending)))
		})
	})

	// S4: the provider ReleaseBinding is deployed but its status advertises a different endpoint
	// name. The consumer finds the provider, fails to match the requested endpoint, and marks the
	// connection pending "not yet resolved".
	Context("S4: when the requested endpoint is absent from provider status", func() {
		const (
			rbName   = "rb-conn-s4"
			crName   = "cr-conn-s4"
			envName  = "env-conn-s4"
			dpName   = "dp-conn-s4"
			compName = "comp-conn-s4"
			projName = "proj-conn-s4"
			provComp = "provider-s4"
			provRB   = "prov-s4"
			epName   = "api"
		)

		AfterEach(func() {
			forceDelete(rbName)
			forceDelete(provRB)
			forceDeleteRelease(compName + "-" + envName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: projName},
			})
		})

		It("surfaces a not-yet-resolved pending connection", func() {
			By("Creating fixtures")
			Expect(k8sClient.Create(ctx, projectFixture(projName))).To(Succeed())
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, componentFixture(compName, projName))).To(Succeed())

			By("Creating a deployed provider RB whose status has a different endpoint name")
			Expect(k8sClient.Create(ctx, providerRBFixture(provRB, projName, provComp, envName))).To(Succeed())
			setProviderEndpoints(&openchoreov1alpha1.ReleaseBinding{ObjectMeta: metav1.ObjectMeta{Name: provRB}},
				[]openchoreov1alpha1.EndpointURLStatus{
					{Name: "other", ServiceURL: &openchoreov1alpha1.EndpointURL{Scheme: "http", Host: "svc", Port: 8080}},
				})

			By("Creating ComponentRelease with a connection to the missing endpoint")
			cr := crFixture(crName, projName, compName)
			cr.Spec.Workload.Dependencies = &openchoreov1alpha1.WorkloadDependencies{
				Endpoints: []openchoreov1alpha1.WorkloadConnection{
					{
						Project:     projName,
						Component:   provComp,
						Name:        epName,
						Visibility:  string(openchoreov1alpha1.EndpointVisibilityProject),
						EnvBindings: openchoreov1alpha1.ConnectionEnvBindings{Host: "DEP_HOST"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			By("Creating consumer ReleaseBinding")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, projName, compName, envName, crName, true),
			)).To(Succeed())

			By("Reconciling until the not-yet-resolved pending connection is surfaced")
			r := testReconcilerWithCachedClient()
			rb := reconcileUntil(r, rbName, func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				return hasPendingReason(rb, "not yet resolved")
			})

			By("PendingConnections surface the not-yet-resolved reason")
			Expect(rb.Status.PendingConnections).To(HaveLen(1))
			Expect(rb.Status.PendingConnections[0].Reason).To(ContainSubstring("not yet resolved"))

			By("ConditionConnectionsResolved=False/ConnectionsPending")
			cond := conditionFor(rb, string(ConditionConnectionsResolved))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonConnectionsPending)))
		})
	})

	// S5: the provider endpoint is exposed externally only (external URLs set, no internal
	// ServiceURL). The consumer requests namespace visibility — which, like project visibility,
	// is backed by the internal ServiceURL (connections may only ask for project/namespace) —
	// finds no internal URL, and marks the connection pending "no URL for visibility".
	Context("S5: when the endpoint exists but has no URL for the requested visibility", func() {
		const (
			rbName   = "rb-conn-s5"
			crName   = "cr-conn-s5"
			envName  = "env-conn-s5"
			dpName   = "dp-conn-s5"
			compName = "comp-conn-s5"
			projName = "proj-conn-s5"
			provComp = "provider-s5"
			provRB   = "prov-s5"
			epName   = "api"
		)

		AfterEach(func() {
			forceDelete(rbName)
			forceDelete(provRB)
			forceDeleteRelease(compName + "-" + envName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: projName},
			})
		})

		It("surfaces a no-URL-for-visibility pending connection", func() {
			By("Creating fixtures")
			Expect(k8sClient.Create(ctx, projectFixture(projName))).To(Succeed())
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, componentFixture(compName, projName))).To(Succeed())

			By("Creating a deployed provider RB whose endpoint is external-only (nil internal ServiceURL)")
			Expect(k8sClient.Create(ctx, providerRBFixture(provRB, projName, provComp, envName))).To(Succeed())
			setProviderEndpoints(&openchoreov1alpha1.ReleaseBinding{ObjectMeta: metav1.ObjectMeta{Name: provRB}},
				[]openchoreov1alpha1.EndpointURLStatus{
					{
						Name:       epName,
						ServiceURL: nil,
						ExternalURLs: &openchoreov1alpha1.EndpointGatewayURLs{
							HTTP: &openchoreov1alpha1.EndpointURL{Scheme: "http", Host: "public.example.com", Port: 80},
						},
					},
				})

			By("Creating ComponentRelease with a namespace-visibility connection")
			cr := crFixture(crName, projName, compName)
			cr.Spec.Workload.Dependencies = &openchoreov1alpha1.WorkloadDependencies{
				Endpoints: []openchoreov1alpha1.WorkloadConnection{
					{
						Project:     projName,
						Component:   provComp,
						Name:        epName,
						Visibility:  string(openchoreov1alpha1.EndpointVisibilityNamespace),
						EnvBindings: openchoreov1alpha1.ConnectionEnvBindings{Host: "DEP_HOST"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			By("Creating consumer ReleaseBinding")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, projName, compName, envName, crName, true),
			)).To(Succeed())

			By("Reconciling until the no-URL-for-visibility pending connection is surfaced")
			r := testReconcilerWithCachedClient()
			rb := reconcileUntil(r, rbName, func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				return hasPendingReason(rb, "has no URL for visibility")
			})

			By("PendingConnections surface the no-URL-for-visibility reason")
			Expect(rb.Status.PendingConnections).To(HaveLen(1))
			Expect(rb.Status.PendingConnections[0].Reason).To(ContainSubstring("has no URL for visibility"))

			By("ConditionConnectionsResolved=False/ConnectionsPending")
			cond := conditionFor(rb, string(ConditionConnectionsResolved))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonConnectionsPending)))
		})
	})

	// S6: the provider ReleaseBinding is deployed with the endpoint initially absent, so the
	// consumer first marks the connection pending "not yet resolved". After the provider publishes
	// the endpoint's ServiceURL, the next consumer reconcile flips it to resolved with the expected
	// host (project visibility).
	Context("S6: when a pending connection later resolves (project visibility)", func() {
		const (
			rbName   = "rb-conn-s6"
			crName   = "cr-conn-s6"
			envName  = "env-conn-s6"
			dpName   = "dp-conn-s6"
			compName = "comp-conn-s6"
			projName = "proj-conn-s6"
			provComp = "provider-s6"
			provRB   = "prov-s6"
			epName   = "api"
			svcHost  = "provider-s6-svc.default.svc.cluster.local"
		)

		AfterEach(func() {
			forceDelete(rbName)
			forceDelete(provRB)
			forceDeleteRelease(compName + "-" + envName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: projName},
			})
		})

		It("transitions from pending to resolved when the provider exposes the endpoint", func() {
			By("Creating fixtures")
			Expect(k8sClient.Create(ctx, projectFixture(projName))).To(Succeed())
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, componentFixture(compName, projName))).To(Succeed())

			By("Creating a deployed provider RB with the endpoint initially absent")
			Expect(k8sClient.Create(ctx, providerRBFixture(provRB, projName, provComp, envName))).To(Succeed())

			By("Creating ComponentRelease with a project-visibility connection")
			cr := crFixture(crName, projName, compName)
			cr.Spec.Workload.Dependencies = &openchoreov1alpha1.WorkloadDependencies{
				Endpoints: []openchoreov1alpha1.WorkloadConnection{
					{
						Project:     projName,
						Component:   provComp,
						Name:        epName,
						Visibility:  string(openchoreov1alpha1.EndpointVisibilityProject),
						EnvBindings: openchoreov1alpha1.ConnectionEnvBindings{Host: "DEP_HOST"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			By("Creating consumer ReleaseBinding")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, projName, compName, envName, crName, true),
			)).To(Succeed())

			By("Reconciling until the not-yet-resolved pending connection is surfaced")
			r := testReconcilerWithCachedClient()
			reconcileUntil(r, rbName, func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				return hasPendingReason(rb, "not yet resolved")
			})

			By("Adding the endpoint with a ServiceURL to the provider status")
			setProviderEndpoints(&openchoreov1alpha1.ReleaseBinding{ObjectMeta: metav1.ObjectMeta{Name: provRB}},
				[]openchoreov1alpha1.EndpointURLStatus{
					{Name: epName, ServiceURL: &openchoreov1alpha1.EndpointURL{Scheme: "http", Host: svcHost, Port: 8080}},
				})

			By("Reconciling until ConnectionsResolved=True")
			rb := reconcileUntil(r, rbName, func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				cond := conditionFor(rb, string(ConditionConnectionsResolved))
				return cond != nil && cond.Status == metav1.ConditionTrue
			})

			By("ResolvedConnections has the expected URL host and PendingConnections is empty")
			Expect(rb.Status.ResolvedConnections).To(HaveLen(1))
			Expect(rb.Status.ResolvedConnections[0].URL.Host).To(Equal(svcHost))
			Expect(rb.Status.PendingConnections).To(BeEmpty())

			By("ConditionConnectionsResolved=True/AllConnectionsResolved")
			cond := conditionFor(rb, string(ConditionConnectionsResolved))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(ReasonAllConnectionsResolved)))
		})
	})

	// S6n: same pending->resolved recovery as S6, but for a namespace-visibility connection whose
	// provider ReleaseBinding lives in a different project. The consumer resolves cross-project via
	// the connection's explicit provider project and records that project + namespace visibility
	// on the resolved connection.
	Context("S6n: when a pending connection later resolves (namespace visibility)", func() {
		const (
			rbName   = "rb-conn-s6n"
			crName   = "cr-conn-s6n"
			envName  = "env-conn-s6n"
			dpName   = "dp-conn-s6n"
			compName = "comp-conn-s6n"
			projName = "proj-conn-s6n"
			provProj = "proj-conn-s6n-provider"
			provComp = "provider-s6n"
			provRB   = "prov-s6n"
			epName   = "api"
			svcHost  = "provider-s6n-svc.default.svc.cluster.local"
		)

		AfterEach(func() {
			forceDelete(rbName)
			forceDelete(provRB)
			forceDeleteRelease(compName + "-" + envName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: projName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: provProj},
			})
		})

		It("resolves a namespace-visibility connection to a provider in another project", func() {
			By("Creating fixtures (consumer project + provider project)")
			Expect(k8sClient.Create(ctx, projectFixture(projName))).To(Succeed())
			Expect(k8sClient.Create(ctx, projectFixture(provProj))).To(Succeed())
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, componentFixture(compName, projName))).To(Succeed())

			By("Creating a deployed provider RB in another project")
			Expect(k8sClient.Create(ctx, providerRBFixture(provRB, provProj, provComp, envName))).To(Succeed())

			By("Creating ComponentRelease with a namespace-visibility connection (explicit provider project)")
			cr := crFixture(crName, projName, compName)
			cr.Spec.Workload.Dependencies = &openchoreov1alpha1.WorkloadDependencies{
				Endpoints: []openchoreov1alpha1.WorkloadConnection{
					{
						Project:     provProj,
						Component:   provComp,
						Name:        epName,
						Visibility:  string(openchoreov1alpha1.EndpointVisibilityNamespace),
						EnvBindings: openchoreov1alpha1.ConnectionEnvBindings{Host: "DEP_HOST"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			By("Creating consumer ReleaseBinding")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, projName, compName, envName, crName, true),
			)).To(Succeed())

			By("Reconciling until the not-yet-resolved pending connection is surfaced")
			r := testReconcilerWithCachedClient()
			reconcileUntil(r, rbName, func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				return hasPendingReason(rb, "not yet resolved")
			})

			By("Adding the endpoint with a ServiceURL to the provider status")
			setProviderEndpoints(&openchoreov1alpha1.ReleaseBinding{ObjectMeta: metav1.ObjectMeta{Name: provRB}},
				[]openchoreov1alpha1.EndpointURLStatus{
					{Name: epName, ServiceURL: &openchoreov1alpha1.EndpointURL{Scheme: "http", Host: svcHost, Port: 8080}},
				})

			By("Reconciling until ConnectionsResolved=True")
			rb := reconcileUntil(r, rbName, func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				cond := conditionFor(rb, string(ConditionConnectionsResolved))
				return cond != nil && cond.Status == metav1.ConditionTrue
			})

			By("ResolvedConnections reflects the provider project and host")
			Expect(rb.Status.ResolvedConnections).To(HaveLen(1))
			Expect(rb.Status.ResolvedConnections[0].Project).To(Equal(provProj))
			Expect(rb.Status.ResolvedConnections[0].URL.Host).To(Equal(svcHost))
			Expect(rb.Status.ResolvedConnections[0].Visibility).To(Equal(openchoreov1alpha1.EndpointVisibilityNamespace))
			Expect(rb.Status.PendingConnections).To(BeEmpty())

			By("ConditionConnectionsResolved=True/AllConnectionsResolved")
			cond := conditionFor(rb, string(ConditionConnectionsResolved))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(ReasonAllConnectionsResolved)))
		})
	})

	// S7: end-to-end projection. The provider ReleaseBinding exposes the endpoint's ServiceURL up
	// front, so the consumer resolves the connection immediately; because all connections are
	// resolved, the render projects the resolved host into the Deployment manifest as the DEP_HOST
	// container env var via the dependencies.envVars template binding.
	Context("S7: when a resolved connection wires env vars into the rendered Deployment", func() {
		const (
			rbName   = "rb-conn-s7"
			crName   = "cr-conn-s7"
			envName  = "env-conn-s7"
			dpName   = "dp-conn-s7"
			compName = "comp-conn-s7"
			projName = "proj-conn-s7"
			provComp = "provider-s7"
			provRB   = "prov-s7"
			epName   = "api"
			svcHost  = "provider-s7-svc.default.svc.cluster.local"
		)

		AfterEach(func() {
			forceDelete(rbName)
			forceDelete(provRB)
			forceDeleteRelease(compName + "-" + envName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: projName},
			})
		})

		It("projects the resolved host into container.env", func() {
			By("Creating fixtures")
			Expect(k8sClient.Create(ctx, projectFixture(projName))).To(Succeed())
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, componentFixture(compName, projName))).To(Succeed())

			By("Creating a deployed provider RB exposing the endpoint via ServiceURL")
			Expect(k8sClient.Create(ctx, providerRBFixture(provRB, projName, provComp, envName))).To(Succeed())
			setProviderEndpoints(&openchoreov1alpha1.ReleaseBinding{ObjectMeta: metav1.ObjectMeta{Name: provRB}},
				[]openchoreov1alpha1.EndpointURLStatus{
					{Name: epName, ServiceURL: &openchoreov1alpha1.EndpointURL{Scheme: "http", Host: svcHost, Port: 8080}},
				})

			By("Creating ComponentRelease with a connection and a template projecting dependencies.envVars")
			cr := crFixture(crName, projName, compName)
			cr.Spec.ComponentType.Spec.Resources[0].Template = depEnvTemplate
			cr.Spec.Workload.Dependencies = &openchoreov1alpha1.WorkloadDependencies{
				Endpoints: []openchoreov1alpha1.WorkloadConnection{
					{
						Project:     projName,
						Component:   provComp,
						Name:        epName,
						Visibility:  string(openchoreov1alpha1.EndpointVisibilityProject),
						EnvBindings: openchoreov1alpha1.ConnectionEnvBindings{Host: "DEP_HOST"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			By("Creating consumer ReleaseBinding")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, projName, compName, envName, crName, true),
			)).To(Succeed())

			By("Reconciling until ConnectionsResolved=True")
			r := testReconcilerWithCachedClient()
			rb := reconcileUntil(r, rbName, func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				cond := conditionFor(rb, string(ConditionConnectionsResolved))
				return cond != nil && cond.Status == metav1.ConditionTrue
			})

			By("ResolvedConnections has the expected host, PendingConnections empty")
			Expect(rb.Status.ResolvedConnections).To(HaveLen(1))
			Expect(rb.Status.ResolvedConnections[0].URL.Host).To(Equal(svcHost))
			Expect(rb.Status.PendingConnections).To(BeEmpty())
			cond := conditionFor(rb, string(ConditionConnectionsResolved))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(ReasonAllConnectionsResolved)))

			By("Fetching the rendered Release and parsing its Deployment manifest")
			rendered := &openchoreov1alpha1.RenderedRelease{}
			Expect(k8sClient.Get(ctx,
				types.NamespacedName{Namespace: ns, Name: compName + "-" + envName},
				rendered,
			)).To(Succeed())
			deploy := findDeploymentManifest(rendered)

			By("Container.env contains DEP_HOST=" + svcHost)
			Expect(deploy.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deploy.Spec.Template.Spec.Containers[0].Env).To(ContainElement(corev1.EnvVar{
				Name:  "DEP_HOST",
				Value: svcHost,
			}))
		})
	})

	// S8: partial resolution. Two connections, only one with a deployed provider ReleaseBinding. The
	// consumer resolves the satisfiable connection (A) and keeps it in ResolvedConnections while the
	// other (B) stays pending "not found"; the aggregate ConnectionsResolved condition is
	// all-or-nothing, so it stays False and ReleaseSynced remains blocked until every connection
	// resolves.
	Context("S8: when one of multiple connections is unresolvable", func() {
		const (
			rbName    = "rb-conn-s8"
			crName    = "cr-conn-s8"
			envName   = "env-conn-s8"
			dpName    = "dp-conn-s8"
			compName  = "comp-conn-s8"
			projName  = "proj-conn-s8"
			provCompA = "provider-s8-a"
			provCompB = "provider-s8-b"
			provRBA   = "prov-s8-a"
			epName    = "api"
			svcHost   = "provider-s8-a-svc.default.svc.cluster.local"
		)

		AfterEach(func() {
			forceDelete(rbName)
			forceDelete(provRBA)
			forceDeleteRelease(compName + "-" + envName)
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: crName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: envName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: dpName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: compName},
			})
			_ = k8sClient.Delete(ctx, &openchoreov1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: projName},
			})
		})

		It("keeps the overall condition pending while still resolving the satisfiable connection", func() {
			By("Creating fixtures")
			Expect(k8sClient.Create(ctx, projectFixture(projName))).To(Succeed())
			Expect(k8sClient.Create(ctx, dpFixture(dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, envFixture(envName, dpName))).To(Succeed())
			Expect(k8sClient.Create(ctx, componentFixture(compName, projName))).To(Succeed())

			By("Creating a deployed provider RB for connection A (resolvable)")
			Expect(k8sClient.Create(ctx, providerRBFixture(provRBA, projName, provCompA, envName))).To(Succeed())
			setProviderEndpoints(&openchoreov1alpha1.ReleaseBinding{ObjectMeta: metav1.ObjectMeta{Name: provRBA}},
				[]openchoreov1alpha1.EndpointURLStatus{
					{Name: epName, ServiceURL: &openchoreov1alpha1.EndpointURL{Scheme: "http", Host: svcHost, Port: 8080}},
				})

			By("Creating ComponentRelease with two connections (B has no provider)")
			cr := crFixture(crName, projName, compName)
			cr.Spec.Workload.Dependencies = &openchoreov1alpha1.WorkloadDependencies{
				Endpoints: []openchoreov1alpha1.WorkloadConnection{
					{
						Project:     projName,
						Component:   provCompA,
						Name:        epName,
						Visibility:  string(openchoreov1alpha1.EndpointVisibilityProject),
						EnvBindings: openchoreov1alpha1.ConnectionEnvBindings{Host: "DEP_A_HOST"},
					},
					{
						Project:     projName,
						Component:   provCompB,
						Name:        epName,
						Visibility:  string(openchoreov1alpha1.EndpointVisibilityProject),
						EnvBindings: openchoreov1alpha1.ConnectionEnvBindings{Host: "DEP_B_HOST"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			By("Creating consumer ReleaseBinding")
			Expect(k8sClient.Create(ctx,
				rbFixture(rbName, projName, compName, envName, crName, true),
			)).To(Succeed())

			By("Reconciling until the terminal shape (A resolved, B pending) is reached")
			r := testReconcilerWithCachedClient()
			rb := reconcileUntil(r, rbName, func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				if len(rb.Status.ResolvedConnections) != 1 || len(rb.Status.PendingConnections) != 1 {
					return false
				}
				return rb.Status.ResolvedConnections[0].Component == provCompA &&
					rb.Status.PendingConnections[0].Component == provCompB
			})

			By("Exactly one connection resolved and one pending")
			Expect(rb.Status.ConnectionTargets).To(HaveLen(2))
			Expect(rb.Status.ResolvedConnections).To(HaveLen(1))
			Expect(rb.Status.ResolvedConnections[0].Component).To(Equal(provCompA))
			Expect(rb.Status.ResolvedConnections[0].URL.Host).To(Equal(svcHost))
			Expect(rb.Status.PendingConnections).To(HaveLen(1))
			Expect(rb.Status.PendingConnections[0].Component).To(Equal(provCompB))
			Expect(rb.Status.PendingConnections[0].Reason).To(ContainSubstring("ReleaseBinding not found"))

			By("Overall ConditionConnectionsResolved=False/ConnectionsPending (not all resolved)")
			cond := conditionFor(rb, string(ConditionConnectionsResolved))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(ReasonConnectionsPending)))
		})
	})
})

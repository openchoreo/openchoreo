// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package connectionbinding

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	ns       = "default"
	timeout  = 10 * time.Second
	interval = 250 * time.Millisecond
)

// --- Fixtures ---

func connectionBindingFixture(name string, connections []openchoreov1alpha1.ConnectionTarget) *openchoreov1alpha1.ConnectionBinding {
	return &openchoreov1alpha1.ConnectionBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: openchoreov1alpha1.ConnectionBindingSpec{
			ReleaseBindingRef: "rb-source",
			Environment:       "dev",
			Connections:       connections,
		},
	}
}

func releaseBindingFixture(name, project, component string) *openchoreov1alpha1.ReleaseBinding {
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
			Environment: "dev",
			State:       openchoreov1alpha1.ReleaseStateActive,
		},
	}
}

func connectionTarget(project, component string, visibility openchoreov1alpha1.EndpointVisibility) openchoreov1alpha1.ConnectionTarget {
	return openchoreov1alpha1.ConnectionTarget{
		Namespace:  ns,
		Project:    project,
		Component:  component,
		Endpoint:   "http-ep",
		Visibility: visibility,
	}
}

// --- Helpers ---

func fetchCB(g Gomega, name string) *openchoreov1alpha1.ConnectionBinding {
	cb := &openchoreov1alpha1.ConnectionBinding{}
	g.ExpectWithOffset(1, k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, cb)).To(Succeed())
	return cb
}

func allResolvedCondition(cb *openchoreov1alpha1.ConnectionBinding) *metav1.Condition {
	return apimeta.FindStatusCondition(cb.Status.Conditions, "AllResolved")
}

// --- Tests ---

var _ = Describe("ConnectionBinding Controller", func() {
	AfterEach(func() {
		// Clean up all ConnectionBindings and ReleaseBindings
		Expect(k8sClient.DeleteAllOf(ctx, &openchoreov1alpha1.ConnectionBinding{}, client.InNamespace(ns))).To(Succeed())
		Expect(k8sClient.DeleteAllOf(ctx, &openchoreov1alpha1.ReleaseBinding{}, client.InNamespace(ns))).To(Succeed())
		// Wait for cleanup to complete
		Eventually(func(g Gomega) {
			var cbList openchoreov1alpha1.ConnectionBindingList
			g.Expect(k8sClient.List(ctx, &cbList, client.InNamespace(ns))).To(Succeed())
			g.Expect(cbList.Items).To(BeEmpty())
			var rbList openchoreov1alpha1.ReleaseBindingList
			g.Expect(k8sClient.List(ctx, &rbList, client.InNamespace(ns))).To(Succeed())
			g.Expect(rbList.Items).To(BeEmpty())
		}, timeout, interval).Should(Succeed())
	})

	It("should resolve all connections when target ReleaseBinding has endpoint URLs", func() {
		By("creating a ReleaseBinding with endpoint status")
		rb := releaseBindingFixture("rb-resolved", "proj-a", "comp-a")
		Expect(k8sClient.Create(ctx, rb)).To(Succeed())

		By("patching the ReleaseBinding status with a ServiceURL")
		rb.Status.Endpoints = []openchoreov1alpha1.EndpointURLStatus{
			{
				Name: "http-ep",
				ServiceURL: &openchoreov1alpha1.EndpointURL{
					Scheme: "http",
					Host:   "comp-a.default.svc.cluster.local",
					Port:   8080,
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, rb)).To(Succeed())

		By("creating a ConnectionBinding targeting that endpoint")
		cb := connectionBindingFixture("cb-resolved", []openchoreov1alpha1.ConnectionTarget{
			connectionTarget("proj-a", "comp-a", openchoreov1alpha1.EndpointVisibilityProject),
		})
		Expect(k8sClient.Create(ctx, cb)).To(Succeed())

		By("verifying the connection is resolved")
		Eventually(func(g Gomega) {
			cb := fetchCB(g, "cb-resolved")
			g.Expect(cb.Status.Resolved).To(HaveLen(1))
			g.Expect(cb.Status.Resolved[0].Endpoint).To(Equal("http-ep"))
			g.Expect(cb.Status.Resolved[0].URL.Host).To(Equal("comp-a.default.svc.cluster.local"))
			g.Expect(cb.Status.Resolved[0].URL.Port).To(Equal(int32(8080)))
			g.Expect(cb.Status.Pending).To(BeEmpty())

			cond := allResolvedCondition(cb)
			g.Expect(cond).NotTo(BeNil())
			g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(cond.Reason).To(Equal("AllResolved"))
		}, timeout, interval).Should(Succeed())
	})

	It("should mark connection as pending when target ReleaseBinding is not found", func() {
		By("creating a ConnectionBinding targeting a non-existent component")
		cb := connectionBindingFixture("cb-not-found", []openchoreov1alpha1.ConnectionTarget{
			connectionTarget("proj-missing", "comp-missing", openchoreov1alpha1.EndpointVisibilityProject),
		})
		Expect(k8sClient.Create(ctx, cb)).To(Succeed())

		By("verifying the connection is pending")
		Eventually(func(g Gomega) {
			cb := fetchCB(g, "cb-not-found")
			g.Expect(cb.Status.Pending).To(HaveLen(1))
			g.Expect(cb.Status.Pending[0].Reason).To(ContainSubstring("ReleaseBinding not found"))
			g.Expect(cb.Status.Resolved).To(BeEmpty())

			cond := allResolvedCondition(cb)
			g.Expect(cond).NotTo(BeNil())
			g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(cond.Reason).To(Equal("ConnectionsPending"))
		}, timeout, interval).Should(Succeed())
	})

	It("should mark connection as pending when component is undeployed", func() {
		By("creating an undeployed ReleaseBinding")
		rb := releaseBindingFixture("rb-undeploy", "proj-b", "comp-b")
		rb.Spec.State = openchoreov1alpha1.ReleaseStateUndeploy
		Expect(k8sClient.Create(ctx, rb)).To(Succeed())

		By("creating a ConnectionBinding targeting the undeployed component")
		cb := connectionBindingFixture("cb-undeploy", []openchoreov1alpha1.ConnectionTarget{
			connectionTarget("proj-b", "comp-b", openchoreov1alpha1.EndpointVisibilityProject),
		})
		Expect(k8sClient.Create(ctx, cb)).To(Succeed())

		By("verifying the connection is pending with undeployed reason")
		Eventually(func(g Gomega) {
			cb := fetchCB(g, "cb-undeploy")
			g.Expect(cb.Status.Pending).To(HaveLen(1))
			g.Expect(cb.Status.Pending[0].Reason).To(Equal("component is undeployed"))
			g.Expect(cb.Status.Resolved).To(BeEmpty())

			cond := allResolvedCondition(cb)
			g.Expect(cond).NotTo(BeNil())
			g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		}, timeout, interval).Should(Succeed())
	})

	It("should mark connection as pending when endpoint is not yet resolved", func() {
		By("creating a ReleaseBinding with no endpoints in status")
		rb := releaseBindingFixture("rb-no-ep", "proj-c", "comp-c")
		Expect(k8sClient.Create(ctx, rb)).To(Succeed())

		By("creating a ConnectionBinding targeting an endpoint")
		cb := connectionBindingFixture("cb-no-ep", []openchoreov1alpha1.ConnectionTarget{
			connectionTarget("proj-c", "comp-c", openchoreov1alpha1.EndpointVisibilityProject),
		})
		Expect(k8sClient.Create(ctx, cb)).To(Succeed())

		By("verifying the connection is pending with not-yet-resolved reason")
		Eventually(func(g Gomega) {
			cb := fetchCB(g, "cb-no-ep")
			g.Expect(cb.Status.Pending).To(HaveLen(1))
			g.Expect(cb.Status.Pending[0].Reason).To(ContainSubstring("not yet resolved"))
			g.Expect(cb.Status.Resolved).To(BeEmpty())

			cond := allResolvedCondition(cb)
			g.Expect(cond).NotTo(BeNil())
			g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		}, timeout, interval).Should(Succeed())
	})

	It("should handle mixed resolved and pending connections", func() {
		By("creating a ReleaseBinding with endpoints")
		rbOk := releaseBindingFixture("rb-mix-ok", "proj-d", "comp-d")
		Expect(k8sClient.Create(ctx, rbOk)).To(Succeed())
		rbOk.Status.Endpoints = []openchoreov1alpha1.EndpointURLStatus{
			{
				Name: "http-ep",
				ServiceURL: &openchoreov1alpha1.EndpointURL{
					Scheme: "http",
					Host:   "comp-d.default.svc.cluster.local",
					Port:   8080,
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, rbOk)).To(Succeed())

		By("creating a ReleaseBinding without endpoints")
		rbPending := releaseBindingFixture("rb-mix-pending", "proj-d", "comp-e")
		Expect(k8sClient.Create(ctx, rbPending)).To(Succeed())

		By("creating a ConnectionBinding with connections to both")
		cb := connectionBindingFixture("cb-mixed", []openchoreov1alpha1.ConnectionTarget{
			connectionTarget("proj-d", "comp-d", openchoreov1alpha1.EndpointVisibilityProject),
			connectionTarget("proj-d", "comp-e", openchoreov1alpha1.EndpointVisibilityProject),
		})
		Expect(k8sClient.Create(ctx, cb)).To(Succeed())

		By("verifying 1 resolved and 1 pending")
		Eventually(func(g Gomega) {
			cb := fetchCB(g, "cb-mixed")
			g.Expect(cb.Status.Resolved).To(HaveLen(1))
			g.Expect(cb.Status.Pending).To(HaveLen(1))

			cond := allResolvedCondition(cb)
			g.Expect(cond).NotTo(BeNil())
			g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(cond.Reason).To(Equal("ConnectionsPending"))
			g.Expect(cond.Message).To(Equal("1 of 2 connections pending"))
		}, timeout, interval).Should(Succeed())
	})

	It("should re-reconcile when ReleaseBinding status is updated with endpoints", func() {
		By("creating a ReleaseBinding with no endpoints")
		rb := releaseBindingFixture("rb-watch", "proj-e", "comp-f")
		Expect(k8sClient.Create(ctx, rb)).To(Succeed())

		By("creating a ConnectionBinding targeting the endpoint")
		cb := connectionBindingFixture("cb-watch", []openchoreov1alpha1.ConnectionTarget{
			connectionTarget("proj-e", "comp-f", openchoreov1alpha1.EndpointVisibilityProject),
		})
		Expect(k8sClient.Create(ctx, cb)).To(Succeed())

		By("verifying the connection starts as pending")
		Eventually(func(g Gomega) {
			cb := fetchCB(g, "cb-watch")
			g.Expect(cb.Status.Pending).To(HaveLen(1))
			g.Expect(cb.Status.Pending[0].Reason).To(ContainSubstring("not yet resolved"))
		}, timeout, interval).Should(Succeed())

		By("updating the ReleaseBinding status with endpoints")
		// Re-fetch to get the latest resource version
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "rb-watch", Namespace: ns}, rb)).To(Succeed())
		rb.Status.Endpoints = []openchoreov1alpha1.EndpointURLStatus{
			{
				Name: "http-ep",
				ServiceURL: &openchoreov1alpha1.EndpointURL{
					Scheme: "http",
					Host:   "comp-f.default.svc.cluster.local",
					Port:   8080,
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, rb)).To(Succeed())

		By("verifying the ConnectionBinding transitions to all resolved")
		Eventually(func(g Gomega) {
			cb := fetchCB(g, "cb-watch")
			g.Expect(cb.Status.Resolved).To(HaveLen(1))
			g.Expect(cb.Status.Resolved[0].URL.Host).To(Equal("comp-f.default.svc.cluster.local"))
			g.Expect(cb.Status.Pending).To(BeEmpty())

			cond := allResolvedCondition(cb)
			g.Expect(cond).NotTo(BeNil())
			g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		}, timeout, interval).Should(Succeed())
	})

	It("should select the correct URL based on visibility level", func() {
		By("creating a ReleaseBinding with all URL types")
		rb := releaseBindingFixture("rb-visibility", "proj-f", "comp-g")
		Expect(k8sClient.Create(ctx, rb)).To(Succeed())

		rb.Status.Endpoints = []openchoreov1alpha1.EndpointURLStatus{
			{
				Name: "http-ep",
				ServiceURL: &openchoreov1alpha1.EndpointURL{
					Scheme: "http",
					Host:   "comp-g.default.svc.cluster.local",
					Port:   8080,
				},
				InternalURLs: &openchoreov1alpha1.EndpointGatewayURLs{
					HTTPS: &openchoreov1alpha1.EndpointURL{
						Scheme: "https",
						Host:   "comp-g.internal.example.com",
						Port:   443,
					},
				},
				ExternalURLs: &openchoreov1alpha1.EndpointGatewayURLs{
					HTTPS: &openchoreov1alpha1.EndpointURL{
						Scheme: "https",
						Host:   "comp-g.external.example.com",
						Port:   443,
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, rb)).To(Succeed())

		By("creating a ConnectionBinding for project visibility")
		cbProject := connectionBindingFixture("cb-vis-project", []openchoreov1alpha1.ConnectionTarget{
			connectionTarget("proj-f", "comp-g", openchoreov1alpha1.EndpointVisibilityProject),
		})
		Expect(k8sClient.Create(ctx, cbProject)).To(Succeed())

		By("creating a ConnectionBinding for namespace visibility")
		cbNamespace := connectionBindingFixture("cb-vis-namespace", []openchoreov1alpha1.ConnectionTarget{
			connectionTarget("proj-f", "comp-g", openchoreov1alpha1.EndpointVisibilityNamespace),
		})
		Expect(k8sClient.Create(ctx, cbNamespace)).To(Succeed())

		By("creating a ConnectionBinding for internal visibility")
		cbInternal := connectionBindingFixture("cb-vis-internal", []openchoreov1alpha1.ConnectionTarget{
			connectionTarget("proj-f", "comp-g", openchoreov1alpha1.EndpointVisibilityInternal),
		})
		Expect(k8sClient.Create(ctx, cbInternal)).To(Succeed())

		By("creating a ConnectionBinding for external visibility")
		cbExternal := connectionBindingFixture("cb-vis-external", []openchoreov1alpha1.ConnectionTarget{
			connectionTarget("proj-f", "comp-g", openchoreov1alpha1.EndpointVisibilityExternal),
		})
		Expect(k8sClient.Create(ctx, cbExternal)).To(Succeed())

		By("verifying project visibility resolves to ServiceURL")
		Eventually(func(g Gomega) {
			cb := fetchCB(g, "cb-vis-project")
			g.Expect(cb.Status.Resolved).To(HaveLen(1))
			g.Expect(cb.Status.Resolved[0].URL.Host).To(Equal("comp-g.default.svc.cluster.local"))
			g.Expect(cb.Status.Resolved[0].URL.Port).To(Equal(int32(8080)))
		}, timeout, interval).Should(Succeed())

		By("verifying namespace visibility resolves to ServiceURL")
		Eventually(func(g Gomega) {
			cb := fetchCB(g, "cb-vis-namespace")
			g.Expect(cb.Status.Resolved).To(HaveLen(1))
			g.Expect(cb.Status.Resolved[0].URL.Host).To(Equal("comp-g.default.svc.cluster.local"))
			g.Expect(cb.Status.Resolved[0].URL.Port).To(Equal(int32(8080)))
		}, timeout, interval).Should(Succeed())

		By("verifying internal visibility resolves to InternalURLs.HTTPS")
		Eventually(func(g Gomega) {
			cb := fetchCB(g, "cb-vis-internal")
			g.Expect(cb.Status.Resolved).To(HaveLen(1))
			g.Expect(cb.Status.Resolved[0].URL.Host).To(Equal("comp-g.internal.example.com"))
			g.Expect(cb.Status.Resolved[0].URL.Port).To(Equal(int32(443)))
			g.Expect(cb.Status.Resolved[0].URL.Scheme).To(Equal("https"))
		}, timeout, interval).Should(Succeed())

		By("verifying external visibility resolves to ExternalURLs.HTTPS")
		Eventually(func(g Gomega) {
			cb := fetchCB(g, "cb-vis-external")
			g.Expect(cb.Status.Resolved).To(HaveLen(1))
			g.Expect(cb.Status.Resolved[0].URL.Host).To(Equal("comp-g.external.example.com"))
			g.Expect(cb.Status.Resolved[0].URL.Port).To(Equal(int32(443)))
			g.Expect(cb.Status.Resolved[0].URL.Scheme).To(Equal("https"))
		}, timeout, interval).Should(Succeed())
	})
})

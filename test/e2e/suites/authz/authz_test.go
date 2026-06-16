// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

// deleteBindingAndWaitForRevocation deletes a ClusterAuthzRoleBinding and waits
// until the Casbin PDP has observed the revocation. The probe function should
// return true when the subject is denied (revocation confirmed).
func deleteBindingAndWaitForRevocation(_ context.Context, name string, probe func() bool) {
	output, err := framework.Kubectl(kubeContext, "delete", "clusterauthzrolebinding", name, "--ignore-not-found", "--wait=false")
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "WARNING: failed to delete clusterauthzrolebinding %s: %s\n", name, output)
	}
	Eventually(func() bool {
		return probe()
	}, framework.DefaultTimeout, 2*time.Second).Should(BeTrue(),
		"binding %s revocation did not propagate in time", name)
}

var _ = Describe("Authorization", Ordered, Label("tier2"), func() {
	SetDefaultEventuallyTimeout(framework.DefaultTimeout)
	SetDefaultEventuallyPollingInterval(framework.DefaultPolling)

	ctx := context.Background()

	// ── Test 1: Unauthenticated request → 401 ──────────────────────────
	Describe("Unauthenticated access", func() {
		It("should return 401 for request without token", func() {
			unauthClient := newUnauthClient()
			resp, err := unauthClient.ListNamespacesWithResponse(ctx, &gen.ListNamespacesParams{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusUnauthorized),
				"expected 401 for unauthenticated request, got %d", resp.StatusCode())
		})
	})

	// ── Test 2: No binding → 403 ───────────────────────────────────────
	Describe("No binding", func() {
		It("should return 403 for authenticated request with no role binding", func() {
			By("verifying subject cannot create a namespace")
			createResp, err := subjectClient.CreateNamespaceWithResponse(ctx,
				newNamespace(fmt.Sprintf("e2e-authz-noperm-%s", testNs)))
			Expect(err).NotTo(HaveOccurred())
			Expect(createResp.StatusCode()).To(Equal(http.StatusForbidden),
				"expected 403 for subject with no binding, got %d: %s",
				createResp.StatusCode(), string(createResp.Body))

			By("verifying list returns empty (filtered by per-item authz)")
			resp, err := subjectClient.ListNamespacesWithResponse(ctx, &gen.ListNamespacesParams{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusOK))
			Expect(resp.JSON200).NotTo(BeNil())
			Expect(resp.JSON200.Items).To(BeEmpty(),
				"expected empty namespace list for subject with no binding")
		})
	})

	// ── Test 3: Admin role allows everything ────────────────────────────
	Describe("Admin role", func() {
		bindingName := fmt.Sprintf("e2e-authz-admin-%s", testNs)

		BeforeAll(func() {
			By("creating admin ClusterAuthzRoleBinding for test subject")
			yaml := clusterAuthzRoleBindingYAML(bindingName, "admin", "allow")
			output, err := framework.KubectlApplyLiteral(kubeContext, yaml)
			Expect(err).NotTo(HaveOccurred(), "create admin binding: %s", output)

			By("waiting for binding to propagate")
			Eventually(func(g Gomega) {
				resp, err := subjectClient.ListNamespacesWithResponse(ctx, &gen.ListNamespacesParams{})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(Equal(http.StatusOK))
				g.Expect(resp.JSON200).NotTo(BeNil())
				g.Expect(resp.JSON200.Items).NotTo(BeEmpty(),
					"admin binding should grant namespace:view")
			}).Should(Succeed())
		})

		AfterAll(func() {
			// Always clean up authz bindings to prevent leakage into subsequent tests.
			deleteBindingAndWaitForRevocation(ctx, bindingName, func() bool {
				resp, _ := subjectClient.CreateNamespaceWithResponse(ctx,
					newNamespace(fmt.Sprintf("e2e-authz-revoke-probe-%s", testNs)))
				return resp != nil && resp.StatusCode() == http.StatusForbidden
			})
		})

		It("should allow listing namespaces", func() {
			resp, err := subjectClient.ListNamespacesWithResponse(ctx, &gen.ListNamespacesParams{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusOK))
			Expect(resp.JSON200.Items).NotTo(BeEmpty())
		})

		It("should allow listing projects in the test namespace", func() {
			resp, err := subjectClient.ListProjectsWithResponse(ctx, testNs, &gen.ListProjectsParams{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusOK))
			Expect(resp.JSON200).NotTo(BeNil())
			names := make([]string, 0, len(resp.JSON200.Items))
			for _, p := range resp.JSON200.Items {
				names = append(names, p.Metadata.Name)
			}
			Expect(names).To(ContainElement(projectName),
				"admin should see the test project")
		})

		It("should allow creating a component type in the test namespace", func() {
			ctYAML := fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: e2e-authz-ct
  namespace: %s
spec:
  workloadType: deployment
  resources:
    - id: deployment
      template:
        apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: "${metadata.name}"
          namespace: "${metadata.namespace}"
        spec:
          replicas: 1
          selector:
            matchLabels: "${metadata.podSelectors}"
          template:
            metadata:
              labels: "${metadata.podSelectors}"
            spec:
              containers:
                - name: main
                  image: "${workload.container.image}"
`, testNs)
			output, err := framework.KubectlApplyLiteral(kubeContext, ctYAML)
			Expect(err).NotTo(HaveOccurred(), "apply ComponentType: %s", output)

			resp, err := subjectClient.ListComponentTypesWithResponse(ctx, testNs, &gen.ListComponentTypesParams{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusOK))
			Expect(resp.JSON200).NotTo(BeNil())

			names := make([]string, 0, len(resp.JSON200.Items))
			for _, ct := range resp.JSON200.Items {
				names = append(names, ct.Metadata.Name)
			}
			Expect(names).To(ContainElement("e2e-authz-ct"))
		})
	})

	// ── Test 4: Developer role — partial access ─────────────────────────
	Describe("Developer role", func() {
		bindingName := fmt.Sprintf("e2e-authz-dev-%s", testNs)

		BeforeAll(func() {
			By("creating developer ClusterAuthzRoleBinding for test subject")
			yaml := clusterAuthzRoleBindingYAML(bindingName, "developer", "allow")
			output, err := framework.KubectlApplyLiteral(kubeContext, yaml)
			Expect(err).NotTo(HaveOccurred(), "create developer binding: %s", output)

			By("waiting for binding to propagate")
			Eventually(func(g Gomega) {
				resp, err := subjectClient.ListProjectsWithResponse(ctx, testNs, &gen.ListProjectsParams{})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(Equal(http.StatusOK))
				g.Expect(resp.JSON200).NotTo(BeNil())
				g.Expect(resp.JSON200.Items).NotTo(BeEmpty(),
					"developer binding should grant project:view")
			}).Should(Succeed())
		})

		AfterAll(func() {
			// Probe with component:create — developer allows it, so it flips
			// from 201 to 403 when the binding is deleted.
			deleteBindingAndWaitForRevocation(ctx, bindingName, func() bool {
				comp := newComponent("e2e-authz-dev-revoke-probe", projectName, "deployment/service")
				resp, _ := subjectClient.CreateComponentWithResponse(ctx, testNs, comp)
				if resp != nil && resp.StatusCode() == http.StatusCreated {
					_, _ = subjectClient.DeleteComponentWithResponse(ctx, testNs, "e2e-authz-dev-revoke-probe")
					return false
				}
				return resp != nil && resp.StatusCode() == http.StatusForbidden
			})
		})

		It("should allow creating a component (developer has component:create)", func() {
			comp := newComponent(compName, projectName, "deployment/service")
			resp, err := subjectClient.CreateComponentWithResponse(ctx, testNs, comp)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusCreated),
				"developer should be able to create component, got %d: %s",
				resp.StatusCode(), string(resp.Body))

			By("cleaning up the component")
			_, _ = subjectClient.DeleteComponentWithResponse(ctx, testNs, compName)
			Eventually(func(g Gomega) {
				framework.AssertResourceGone(g, kubeContext, testNs, "component", compName)
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())
		})

		It("should deny creating an environment (developer lacks environment:create)", func() {
			resp, err := subjectClient.CreateEnvironmentWithResponse(ctx, testNs, newEnvironment("e2e-authz-env-denied"))
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusForbidden),
				"developer should not be able to create environment, got %d: %s",
				resp.StatusCode(), string(resp.Body))
		})

		It("should deny deleting a namespace (developer lacks namespace:delete)", func() {
			resp, err := subjectClient.DeleteNamespaceWithResponse(ctx, testNs)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusForbidden),
				"developer should not be able to delete namespace, got %d: %s",
				resp.StatusCode(), string(resp.Body))
		})
	})

	// ── Test 5: Namespace-scoped role isolation ─────────────────────────
	Describe("Namespace scoping", func() {
		nsA := fmt.Sprintf("e2e-authz-nsa-%s", testNs)
		nsB := fmt.Sprintf("e2e-authz-nsb-%s", testNs)
		roleName := "e2e-authz-ns-viewer"
		bindingName := "e2e-authz-ns-viewer-binding"
		clusterBindingName := fmt.Sprintf("e2e-authz-nsread-%s", testNs)

		BeforeAll(func() {
			By("creating two test namespaces with platform resources")
			for _, ns := range []string{nsA, nsB} {
				output, err := framework.KubectlApplyLiteral(kubeContext, cpNamespaceYAML(ns))
				Expect(err).NotTo(HaveOccurred(), "create namespace %s: %s", ns, output)
				output, err = framework.KubectlApplyLiteral(kubeContext, platformResourcesYAML(ns))
				Expect(err).NotTo(HaveOccurred(), "apply platform resources %s: %s", ns, output)
			}

			By("creating AuthzRole in namespace A only")
			roleYAML := authzRoleYAML(nsA, roleName, []string{"project:view", "component:view"})
			output, err := framework.KubectlApplyLiteral(kubeContext, roleYAML)
			Expect(err).NotTo(HaveOccurred(), "create AuthzRole: %s", output)

			By("creating AuthzRoleBinding in namespace A for test subject")
			rbYAML := authzRoleBindingYAML(nsA, bindingName, roleName, "allow")
			output, err = framework.KubectlApplyLiteral(kubeContext, rbYAML)
			Expect(err).NotTo(HaveOccurred(), "create AuthzRoleBinding: %s", output)

			By("granting namespace-reader at cluster scope so subject can reach namespace endpoints")
			clusterYAML := clusterAuthzRoleBindingYAML(clusterBindingName, "namespace-reader", "allow")
			output, err = framework.KubectlApplyLiteral(kubeContext, clusterYAML)
			Expect(err).NotTo(HaveOccurred(), "create cluster binding: %s", output)

			By("waiting for bindings to propagate")
			Eventually(func(g Gomega) {
				resp, err := subjectClient.ListProjectsWithResponse(ctx, nsA, &gen.ListProjectsParams{})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(Equal(http.StatusOK))
				g.Expect(resp.JSON200).NotTo(BeNil())
				g.Expect(resp.JSON200.Items).NotTo(BeEmpty(),
					"namespace-scoped binding should grant project:view in ns-A")
			}).Should(Succeed())
		})

		AfterAll(func() {
			deleteBindingAndWaitForRevocation(ctx, clusterBindingName, func() bool {
				resp, _ := subjectClient.ListNamespacesWithResponse(ctx, &gen.ListNamespacesParams{})
				return resp != nil && resp.JSON200 != nil && len(resp.JSON200.Items) == 0
			})
			if os.Getenv("E2E_KEEP_RESOURCES") != "true" {
				_, _ = framework.Kubectl(kubeContext, "delete", "namespace", nsA, "--ignore-not-found", "--wait=false")
				_, _ = framework.Kubectl(kubeContext, "delete", "namespace", nsB, "--ignore-not-found", "--wait=false")
			}
		})

		It("should list projects in namespace A (binding exists)", func() {
			resp, err := subjectClient.ListProjectsWithResponse(ctx, nsA, &gen.ListProjectsParams{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusOK))
			Expect(resp.JSON200).NotTo(BeNil())
			Expect(resp.JSON200.Items).NotTo(BeEmpty(),
				"should see projects in namespace A")
		})

		It("should return empty project list for namespace B (no binding)", func() {
			resp, err := subjectClient.ListProjectsWithResponse(ctx, nsB, &gen.ListProjectsParams{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusOK))
			Expect(resp.JSON200).NotTo(BeNil())
			Expect(resp.JSON200.Items).To(BeEmpty(),
				"should not see projects in namespace B (no AuthzRoleBinding)")
		})
	})

	// ── Test 6: Deny policy overrides allow ─────────────────────────────
	Describe("Deny override", func() {
		allowBindingName := fmt.Sprintf("e2e-authz-allow-%s", testNs)
		denyRoleName := fmt.Sprintf("e2e-authz-deny-role-%s", testNs)
		denyBindingName := fmt.Sprintf("e2e-authz-deny-%s", testNs)

		BeforeAll(func() {
			By("creating developer allow binding for test subject")
			allowYAML := clusterAuthzRoleBindingYAML(allowBindingName, "developer", "allow")
			output, err := framework.KubectlApplyLiteral(kubeContext, allowYAML)
			Expect(err).NotTo(HaveOccurred(), "create allow binding: %s", output)

			By("creating deny role for component:create")
			denyRoleYAML := clusterAuthzRoleYAML(denyRoleName, []string{"component:create"})
			output, err = framework.KubectlApplyLiteral(kubeContext, denyRoleYAML)
			Expect(err).NotTo(HaveOccurred(), "create deny role: %s", output)

			By("creating deny binding for test subject")
			denyYAML := clusterAuthzRoleBindingYAML(denyBindingName, denyRoleName, "deny")
			output, err = framework.KubectlApplyLiteral(kubeContext, denyYAML)
			Expect(err).NotTo(HaveOccurred(), "create deny binding: %s", output)

			By("waiting for allow binding to propagate")
			Eventually(func(g Gomega) {
				resp, err := subjectClient.ListProjectsWithResponse(ctx, testNs, &gen.ListProjectsParams{})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(Equal(http.StatusOK))
				g.Expect(resp.JSON200).NotTo(BeNil())
				g.Expect(resp.JSON200.Items).NotTo(BeEmpty(),
					"developer allow binding should grant project:view")
			}).Should(Succeed())

			By("waiting for deny binding to propagate")
			Eventually(func(g Gomega) {
				comp := newComponent("e2e-authz-deny-wait", projectName, "deployment/service")
				resp, err := subjectClient.CreateComponentWithResponse(ctx, testNs, comp)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(Equal(http.StatusForbidden),
					"deny binding should block component:create")
			}).Should(Succeed())
		})

		AfterAll(func() {
			// Revoke deny binding first so the allow binding's component:create is unblocked.
			deleteBindingAndWaitForRevocation(ctx, denyBindingName, func() bool {
				comp := newComponent("e2e-authz-deny-probe", projectName, "deployment/service")
				resp, _ := subjectClient.CreateComponentWithResponse(ctx, testNs, comp)
				if resp != nil && resp.StatusCode() == http.StatusCreated {
					_, _ = subjectClient.DeleteComponentWithResponse(ctx, testNs, "e2e-authz-deny-probe")
					return true
				}
				return false
			})
			// Then revoke the allow binding.
			deleteBindingAndWaitForRevocation(ctx, allowBindingName, func() bool {
				resp, _ := subjectClient.ListProjectsWithResponse(ctx, testNs, &gen.ListProjectsParams{})
				return resp != nil && resp.JSON200 != nil && len(resp.JSON200.Items) == 0
			})
			if os.Getenv("E2E_KEEP_RESOURCES") != "true" {
				_, _ = framework.Kubectl(kubeContext, "delete", "clusterauthzrole", denyRoleName, "--ignore-not-found", "--wait=false")
			}
		})

		It("should allow viewing components (allowed by developer, not denied)", func() {
			resp, err := subjectClient.ListComponentsWithResponse(ctx, testNs, &gen.ListComponentsParams{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusOK),
				"component:view should be allowed (developer allows, deny does not cover view)")
		})

		It("should deny creating component (explicitly denied)", func() {
			comp := newComponent("e2e-authz-denied-comp", projectName, "deployment/service")
			resp, err := subjectClient.CreateComponentWithResponse(ctx, testNs, comp)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusForbidden),
				"component:create should be denied (explicit deny overrides developer allow), got %d: %s",
				resp.StatusCode(), string(resp.Body))
		})
	})

	// ── Test 7: Binding update propagation ──────────────────────────────
	Describe("Binding update propagation", func() {
		bindingName := fmt.Sprintf("e2e-authz-prop-%s", testNs)

		AfterAll(func() {
			deleteBindingAndWaitForRevocation(ctx, bindingName, func() bool {
				resp, _ := subjectClient.CreateNamespaceWithResponse(ctx,
					newNamespace(fmt.Sprintf("e2e-authz-prop-cleanup-%s", testNs)))
				return resp != nil && resp.StatusCode() == http.StatusForbidden
			})
		})

		It("should grant access after binding is created", func() {
			By("verifying subject cannot create namespace initially")
			resp, err := subjectClient.CreateNamespaceWithResponse(ctx,
				newNamespace(fmt.Sprintf("e2e-authz-prop-%s", testNs)))
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusForbidden),
				"expected 403 before binding exists")

			By("creating admin binding")
			yaml := clusterAuthzRoleBindingYAML(bindingName, "admin", "allow")
			output, err := framework.KubectlApplyLiteral(kubeContext, yaml)
			Expect(err).NotTo(HaveOccurred(), "create binding: %s", output)

			By("waiting for binding to propagate — subject should gain access")
			Eventually(func(g Gomega) {
				resp, err := subjectClient.ListNamespacesWithResponse(ctx, &gen.ListNamespacesParams{})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(Equal(http.StatusOK))
				g.Expect(resp.JSON200).NotTo(BeNil())
				g.Expect(resp.JSON200.Items).NotTo(BeEmpty(),
					"admin binding should propagate and grant namespace:view")
			}).Should(Succeed())
		})

		It("should revoke access after binding is deleted", func() {
			By("verifying subject has access before deletion")
			resp, err := subjectClient.ListNamespacesWithResponse(ctx, &gen.ListNamespacesParams{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusOK))
			Expect(resp.JSON200).NotTo(BeNil())
			Expect(resp.JSON200.Items).NotTo(BeEmpty())

			By("deleting the binding and waiting for revocation")
			deleteBindingAndWaitForRevocation(ctx, bindingName, func() bool {
				createResp, _ := subjectClient.CreateNamespaceWithResponse(ctx,
					newNamespace(fmt.Sprintf("e2e-authz-revoke-%s", testNs)))
				return createResp != nil && createResp.StatusCode() == http.StatusForbidden
			})
		})
	})

	// ── Test 8: Scoped cluster binding (namespace restriction) ──────────
	Describe("Scoped cluster binding", func() {
		scopedBindingName := fmt.Sprintf("e2e-authz-scoped-%s", testNs)
		nsB := fmt.Sprintf("e2e-authz-scopedb-%s", testNs)

		BeforeAll(func() {
			By("creating a second namespace for scope testing")
			output, err := framework.KubectlApplyLiteral(kubeContext, cpNamespaceYAML(nsB))
			Expect(err).NotTo(HaveOccurred(), "create namespace B: %s", output)
			output, err = framework.KubectlApplyLiteral(kubeContext, platformResourcesYAML(nsB))
			Expect(err).NotTo(HaveOccurred(), "apply platform resources B: %s", output)

			By("creating developer binding scoped to test namespace only")
			yaml := scopedClusterAuthzRoleBindingYAML(scopedBindingName, "developer", "allow", testNs)
			output, err = framework.KubectlApplyLiteral(kubeContext, yaml)
			Expect(err).NotTo(HaveOccurred(), "create scoped binding: %s", output)

			By("waiting for binding to propagate")
			Eventually(func(g Gomega) {
				resp, err := subjectClient.ListProjectsWithResponse(ctx, testNs, &gen.ListProjectsParams{})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(Equal(http.StatusOK))
				g.Expect(resp.JSON200).NotTo(BeNil())
				g.Expect(resp.JSON200.Items).NotTo(BeEmpty(),
					"scoped developer binding should grant project:view in testNs")
			}).Should(Succeed())
		})

		AfterAll(func() {
			deleteBindingAndWaitForRevocation(ctx, scopedBindingName, func() bool {
				resp, _ := subjectClient.ListProjectsWithResponse(ctx, testNs, &gen.ListProjectsParams{})
				return resp != nil && resp.JSON200 != nil && len(resp.JSON200.Items) == 0
			})
			if os.Getenv("E2E_KEEP_RESOURCES") != "true" {
				_, _ = framework.Kubectl(kubeContext, "delete", "namespace", nsB, "--ignore-not-found", "--wait=false")
			}
		})

		It("should allow creating component in scoped namespace", func() {
			comp := newComponent("e2e-authz-scoped-comp", projectName, "deployment/service")
			resp, err := subjectClient.CreateComponentWithResponse(ctx, testNs, comp)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusCreated),
				"should allow component:create in scoped namespace, got %d: %s",
				resp.StatusCode(), string(resp.Body))

			_, _ = subjectClient.DeleteComponentWithResponse(ctx, testNs, "e2e-authz-scoped-comp")
			Eventually(func(g Gomega) {
				framework.AssertResourceGone(g, kubeContext, testNs, "component", "e2e-authz-scoped-comp")
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())
		})

		It("should deny creating component in out-of-scope namespace", func() {
			comp := newComponent("e2e-authz-oos-comp", projectName, "deployment/service")
			resp, err := subjectClient.CreateComponentWithResponse(ctx, nsB, comp)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusForbidden),
				"should deny component:create in out-of-scope namespace, got %d: %s",
				resp.StatusCode(), string(resp.Body))
		})
	})

	// ── Test 9: Role update propagation ─────────────────────────────────
	Describe("Role update propagation", func() {
		roleName := fmt.Sprintf("e2e-authz-evolving-%s", testNs)
		bindingName := fmt.Sprintf("e2e-authz-evolving-bind-%s", testNs)

		BeforeAll(func() {
			By("creating a role with only project:view")
			roleYAML := clusterAuthzRoleYAML(roleName, []string{"project:view", "namespace:view"})
			output, err := framework.KubectlApplyLiteral(kubeContext, roleYAML)
			Expect(err).NotTo(HaveOccurred(), "create role: %s", output)

			By("binding the role to test subject")
			bindYAML := clusterAuthzRoleBindingYAML(bindingName, roleName, "allow")
			output, err = framework.KubectlApplyLiteral(kubeContext, bindYAML)
			Expect(err).NotTo(HaveOccurred(), "create binding: %s", output)

			By("waiting for initial binding to propagate")
			Eventually(func(g Gomega) {
				resp, err := subjectClient.ListProjectsWithResponse(ctx, testNs, &gen.ListProjectsParams{})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(Equal(http.StatusOK))
				g.Expect(resp.JSON200).NotTo(BeNil())
				g.Expect(resp.JSON200.Items).NotTo(BeEmpty())
			}).Should(Succeed())
		})

		AfterAll(func() {
			deleteBindingAndWaitForRevocation(ctx, bindingName, func() bool {
				resp, _ := subjectClient.ListProjectsWithResponse(ctx, testNs, &gen.ListProjectsParams{})
				return resp != nil && resp.JSON200 != nil && len(resp.JSON200.Items) == 0
			})
			if os.Getenv("E2E_KEEP_RESOURCES") != "true" {
				_, _ = framework.Kubectl(kubeContext, "delete", "clusterauthzrole", roleName, "--ignore-not-found", "--wait=false")
			}
		})

		It("should deny component creation initially (role lacks component:create)", func() {
			comp := newComponent("e2e-authz-role-evolve", projectName, "deployment/service")
			resp, err := subjectClient.CreateComponentWithResponse(ctx, testNs, comp)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusForbidden),
				"initial role should not grant component:create, got %d", resp.StatusCode())
		})

		It("should allow component creation after role is updated to include component:create", func() {
			By("updating the role to add component actions")
			updatedRoleYAML := clusterAuthzRoleYAML(roleName, []string{
				"project:view", "namespace:view",
				"component:create", "component:view", "component:delete",
			})
			output, err := framework.KubectlApplyLiteral(kubeContext, updatedRoleYAML)
			Expect(err).NotTo(HaveOccurred(), "update role: %s", output)

			By("waiting for role update to propagate")
			Eventually(func(g Gomega) {
				comp := newComponent("e2e-authz-role-evolve", projectName, "deployment/service")
				resp, err := subjectClient.CreateComponentWithResponse(ctx, testNs, comp)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(Equal(http.StatusCreated),
					"updated role should grant component:create, got %d: %s",
					resp.StatusCode(), string(resp.Body))
			}).Should(Succeed())

			By("cleaning up")
			_, _ = subjectClient.DeleteComponentWithResponse(ctx, testNs, "e2e-authz-role-evolve")
			Eventually(func(g Gomega) {
				framework.AssertResourceGone(g, kubeContext, testNs, "component", "e2e-authz-role-evolve")
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())
		})
	})

	// ── Test 10: Server-side list filtering ─────────────────────────────
	Describe("Server-side list filtering", func() {
		nsA := fmt.Sprintf("e2e-authz-filta-%s", testNs)
		nsB := fmt.Sprintf("e2e-authz-filtb-%s", testNs)
		roleName := "e2e-authz-filter-viewer"
		rbName := "e2e-authz-filter-binding"
		clusterBindingName := fmt.Sprintf("e2e-authz-filt-clust-%s", testNs)
		projA := "filter-proj-a"
		projB := "filter-proj-b"

		BeforeAll(func() {
			By("creating two namespaces with projects")
			for _, ns := range []string{nsA, nsB} {
				output, err := framework.KubectlApplyLiteral(kubeContext, cpNamespaceYAML(ns))
				Expect(err).NotTo(HaveOccurred(), "create namespace %s: %s", ns, output)
				output, err = framework.KubectlApplyLiteral(kubeContext, platformResourcesYAML(ns))
				Expect(err).NotTo(HaveOccurred(), "apply platform resources %s: %s", ns, output)
			}

			By("creating additional project in ns-A via admin")
			projAYAML := fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: %s
  namespace: %s
  labels:
    openchoreo.dev/name: %s
spec:
  deploymentPipelineRef:
    name: default
  type:
    kind: ClusterProjectType
    name: default
`, projA, nsA, projA)
			output, err := framework.KubectlApplyLiteral(kubeContext, projAYAML)
			Expect(err).NotTo(HaveOccurred(), "create project A: %s", output)

			projBYAML := fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: %s
  namespace: %s
  labels:
    openchoreo.dev/name: %s
spec:
  deploymentPipelineRef:
    name: default
  type:
    kind: ClusterProjectType
    name: default
`, projB, nsB, projB)
			output, err = framework.KubectlApplyLiteral(kubeContext, projBYAML)
			Expect(err).NotTo(HaveOccurred(), "create project B: %s", output)

			By("creating namespace-scoped AuthzRole in ns-A")
			roleYAML := authzRoleYAML(nsA, roleName, []string{"project:view", "component:view"})
			output, err = framework.KubectlApplyLiteral(kubeContext, roleYAML)
			Expect(err).NotTo(HaveOccurred(), "create role: %s", output)

			By("creating AuthzRoleBinding in ns-A only")
			bindYAML := authzRoleBindingYAML(nsA, rbName, roleName, "allow")
			output, err = framework.KubectlApplyLiteral(kubeContext, bindYAML)
			Expect(err).NotTo(HaveOccurred(), "create binding: %s", output)

			By("granting namespace-reader at cluster scope for endpoint access")
			clusterYAML := clusterAuthzRoleBindingYAML(clusterBindingName, "namespace-reader", "allow")
			output, err = framework.KubectlApplyLiteral(kubeContext, clusterYAML)
			Expect(err).NotTo(HaveOccurred(), "create cluster binding: %s", output)

			By("waiting for bindings to propagate")
			Eventually(func(g Gomega) {
				resp, err := subjectClient.ListProjectsWithResponse(ctx, nsA, &gen.ListProjectsParams{})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(Equal(http.StatusOK))
				g.Expect(resp.JSON200).NotTo(BeNil())
				g.Expect(resp.JSON200.Items).NotTo(BeEmpty())
			}).Should(Succeed())
		})

		AfterAll(func() {
			deleteBindingAndWaitForRevocation(ctx, clusterBindingName, func() bool {
				resp, _ := subjectClient.ListNamespacesWithResponse(ctx, &gen.ListNamespacesParams{})
				return resp != nil && resp.JSON200 != nil && len(resp.JSON200.Items) == 0
			})
			if os.Getenv("E2E_KEEP_RESOURCES") != "true" {
				_, _ = framework.Kubectl(kubeContext, "delete", "namespace", nsA, "--ignore-not-found", "--wait=false")
				_, _ = framework.Kubectl(kubeContext, "delete", "namespace", nsB, "--ignore-not-found", "--wait=false")
			}
		})

		It("should include projects from authorized namespace in list", func() {
			resp, err := subjectClient.ListProjectsWithResponse(ctx, nsA, &gen.ListProjectsParams{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusOK))
			Expect(resp.JSON200).NotTo(BeNil())

			names := make([]string, 0, len(resp.JSON200.Items))
			for _, p := range resp.JSON200.Items {
				names = append(names, p.Metadata.Name)
			}
			Expect(names).To(ContainElement(projA),
				"should see project in authorized namespace A")
		})

		It("should return empty list for unauthorized namespace", func() {
			resp, err := subjectClient.ListProjectsWithResponse(ctx, nsB, &gen.ListProjectsParams{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusOK))
			Expect(resp.JSON200).NotTo(BeNil())
			Expect(resp.JSON200.Items).To(BeEmpty(),
				"should not see projects in unauthorized namespace B")
		})
	})

	// ── Test 11: Authz resource self-management ─────────────────────────
	Describe("Authz resource self-management", func() {
		adminBindingName := fmt.Sprintf("e2e-authz-selfadmin-%s", testNs)
		devBindingName := fmt.Sprintf("e2e-authz-selfdev-%s", testNs)

		Context("admin identity can manage authz resources", func() {
			BeforeAll(func() {
				By("creating admin binding for test subject")
				yaml := clusterAuthzRoleBindingYAML(adminBindingName, "admin", "allow")
				output, err := framework.KubectlApplyLiteral(kubeContext, yaml)
				Expect(err).NotTo(HaveOccurred(), "create admin binding: %s", output)

				By("waiting for propagation")
				Eventually(func(g Gomega) {
					resp, err := subjectClient.ListNamespacesWithResponse(ctx, &gen.ListNamespacesParams{})
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(resp.StatusCode()).To(Equal(http.StatusOK))
					g.Expect(resp.JSON200).NotTo(BeNil())
					g.Expect(resp.JSON200.Items).NotTo(BeEmpty())
				}).Should(Succeed())
			})

			AfterAll(func() {
				deleteBindingAndWaitForRevocation(ctx, adminBindingName, func() bool {
					resp, _ := subjectClient.CreateNamespaceWithResponse(ctx,
						newNamespace(fmt.Sprintf("e2e-authz-self-revoke-%s", testNs)))
					return resp != nil && resp.StatusCode() == http.StatusForbidden
				})
			})

			It("should allow listing namespace roles via API", func() {
				resp, err := subjectClient.ListNamespaceRolesWithResponse(ctx, testNs, &gen.ListNamespaceRolesParams{})
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode()).To(Equal(http.StatusOK),
					"admin should be able to list authz roles, got %d: %s",
					resp.StatusCode(), string(resp.Body))
			})

			It("should allow listing namespace role bindings via API", func() {
				resp, err := subjectClient.ListNamespaceRoleBindingsWithResponse(ctx, testNs, &gen.ListNamespaceRoleBindingsParams{})
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode()).To(Equal(http.StatusOK),
					"admin should be able to list authz role bindings, got %d: %s",
					resp.StatusCode(), string(resp.Body))
			})
		})

		Context("developer cannot manage authz resources", func() {
			BeforeAll(func() {
				By("creating developer binding for test subject")
				yaml := clusterAuthzRoleBindingYAML(devBindingName, "developer", "allow")
				output, err := framework.KubectlApplyLiteral(kubeContext, yaml)
				Expect(err).NotTo(HaveOccurred(), "create developer binding: %s", output)

				By("waiting for developer binding to take effect")
				Eventually(func(g Gomega) {
					resp, err := subjectClient.ListProjectsWithResponse(ctx, testNs, &gen.ListProjectsParams{})
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(resp.StatusCode()).To(Equal(http.StatusOK))
					g.Expect(resp.JSON200).NotTo(BeNil())
					g.Expect(resp.JSON200.Items).NotTo(BeEmpty())
				}).Should(Succeed())
			})

			AfterAll(func() {
				deleteBindingAndWaitForRevocation(ctx, devBindingName, func() bool {
					resp, _ := subjectClient.ListProjectsWithResponse(ctx, testNs, &gen.ListProjectsParams{})
					return resp != nil && resp.JSON200 != nil && len(resp.JSON200.Items) == 0
				})
			})

			It("should deny creating namespace role via API", func() {
				resp, err := subjectClient.CreateNamespaceRoleWithResponse(ctx, testNs, gen.AuthzRole{
					Metadata: gen.ObjectMeta{Name: "e2e-authz-denied-role"},
					Spec: &gen.AuthzRoleSpec{
						Actions: []string{"project:view"},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode()).To(Equal(http.StatusForbidden),
					"developer should not be able to create authz roles, got %d: %s",
					resp.StatusCode(), string(resp.Body))
			})

			It("should deny creating namespace role binding via API", func() {
				effect := gen.AuthzRoleBindingSpecEffectAllow
				resp, err := subjectClient.CreateNamespaceRoleBindingWithResponse(ctx, testNs, gen.AuthzRoleBinding{
					Metadata: gen.ObjectMeta{Name: "e2e-authz-denied-rb"},
					Spec: &gen.AuthzRoleBindingSpec{
						RoleMappings: []gen.AuthzRoleMapping{{
							RoleRef: gen.AuthzRoleRef{
								Kind: gen.AuthzRoleRefKindAuthzRole,
								Name: "e2e-authz-denied-role",
							},
						}},
						Entitlement: gen.AuthzEntitlementClaim{
							Claim: "sub",
							Value: "test",
						},
						Effect: &effect,
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode()).To(Equal(http.StatusForbidden),
					"developer should not be able to create authz role bindings, got %d: %s",
					resp.StatusCode(), string(resp.Body))
			})
		})
	})

	// ── Test 12: CEL condition — componentType scoping ──────────────────
	// The component service injects resource.componentType into the authz context.
	// ClusterComponentType "deployment/service" → attribute "service" (name after the slash).
	// Namespace-scoped ComponentType "alt-ct" in ns → attribute "{ns}/alt-ct".
	Describe("CEL condition", func() {
		altCTName := "deployment/e2e-authz-alt-ct"
		roleName := fmt.Sprintf("e2e-authz-cel-role-%s", testNs)
		bindingName := fmt.Sprintf("e2e-authz-cel-bind-%s", testNs)

		BeforeAll(func() {
			By("creating a namespace-scoped ComponentType as an alternative type")
			output, err := framework.KubectlApplyLiteral(kubeContext, componentTypeYAML(testNs, "e2e-authz-alt-ct"))
			Expect(err).NotTo(HaveOccurred(), "create alt ComponentType: %s", output)

			By("creating a role with component + view actions")
			roleYAML := clusterAuthzRoleYAML(roleName, []string{
				"component:create", "component:view", "component:delete",
				"project:view", "namespace:view",
			})
			output, err = framework.KubectlApplyLiteral(kubeContext, roleYAML)
			Expect(err).NotTo(HaveOccurred(), "create CEL role: %s", output)

			By("creating binding with CEL condition: only allow component:create for ClusterComponentType 'service'")
			// "deployment/service" → formatComponentTypeAttr strips the workloadType prefix → "service"
			bindYAML := clusterAuthzRoleBindingWithConditionsYAML(
				bindingName, roleName, "allow",
				"component:create", `resource.componentType == "service"`,
			)
			output, err = framework.KubectlApplyLiteral(kubeContext, bindYAML)
			Expect(err).NotTo(HaveOccurred(), "create CEL binding: %s", output)

			By("waiting for binding to propagate")
			Eventually(func(g Gomega) {
				resp, err := subjectClient.ListProjectsWithResponse(ctx, testNs, &gen.ListProjectsParams{})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(Equal(http.StatusOK))
				g.Expect(resp.JSON200).NotTo(BeNil())
				g.Expect(resp.JSON200.Items).NotTo(BeEmpty())
			}).Should(Succeed())
		})

		AfterAll(func() {
			deleteBindingAndWaitForRevocation(ctx, bindingName, func() bool {
				resp, _ := subjectClient.ListProjectsWithResponse(ctx, testNs, &gen.ListProjectsParams{})
				return resp != nil && resp.JSON200 != nil && len(resp.JSON200.Items) == 0
			})
			if os.Getenv("E2E_KEEP_RESOURCES") != "true" {
				_, _ = framework.Kubectl(kubeContext, "delete", "clusterauthzrole", roleName, "--ignore-not-found", "--wait=false")
			}
		})

		It("should allow component creation when CEL condition matches (ClusterComponentType 'deployment/service')", func() {
			comp := newComponent("e2e-authz-cel-match", projectName, "deployment/service")
			resp, err := subjectClient.CreateComponentWithResponse(ctx, testNs, comp)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusCreated),
				"CEL condition resource.componentType == 'service' should match ClusterComponentType 'deployment/service', got %d: %s",
				resp.StatusCode(), string(resp.Body))

			By("cleaning up")
			_, _ = subjectClient.DeleteComponentWithResponse(ctx, testNs, "e2e-authz-cel-match")
			Eventually(func(g Gomega) {
				framework.AssertResourceGone(g, kubeContext, testNs, "component", "e2e-authz-cel-match")
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())
		})

		It("should deny component creation when CEL condition does not match (namespace-scoped ComponentType)", func() {
			// Namespace-scoped CT "e2e-authz-alt-ct" → attribute "{testNs}/e2e-authz-alt-ct" ≠ "service"
			comp := newComponentWithType("e2e-authz-cel-nomatch", projectName, altCTName, gen.ComponentSpecComponentTypeKindComponentType)
			resp, err := subjectClient.CreateComponentWithResponse(ctx, testNs, comp)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusForbidden),
				"CEL condition should reject namespace-scoped ComponentType (attribute != 'service'), got %d: %s",
				resp.StatusCode(), string(resp.Body))
		})
	})

	// ── Test 13: Deny with CEL condition ────────────────────────────────
	// Proves: deny policy with CEL condition blocks matching requests but
	// allows non-matching ones (developer allow still applies).
	Describe("Deny with CEL condition", func() {
		allowBindingName := fmt.Sprintf("e2e-authz-dcel-allow-%s", testNs)
		denyRoleName := fmt.Sprintf("e2e-authz-dcel-deny-role-%s", testNs)
		denyBindingName := fmt.Sprintf("e2e-authz-dcel-deny-%s", testNs)
		altCTName := "deployment/e2e-authz-alt-ct"

		BeforeAll(func() {
			By("ensuring namespace-scoped ComponentType exists")
			output, err := framework.KubectlApplyLiteral(kubeContext, componentTypeYAML(testNs, "e2e-authz-alt-ct"))
			Expect(err).NotTo(HaveOccurred(), "create alt ComponentType: %s", output)

			By("creating developer allow binding")
			allowYAML := clusterAuthzRoleBindingYAML(allowBindingName, "developer", "allow")
			output, err = framework.KubectlApplyLiteral(kubeContext, allowYAML)
			Expect(err).NotTo(HaveOccurred(), "create allow binding: %s", output)

			By("creating deny role for component:create")
			denyRoleYAML := clusterAuthzRoleYAML(denyRoleName, []string{"component:create"})
			output, err = framework.KubectlApplyLiteral(kubeContext, denyRoleYAML)
			Expect(err).NotTo(HaveOccurred(), "create deny role: %s", output)

			By("creating deny binding with CEL condition: deny component:create only for ClusterComponentType 'service'")
			denyYAML := clusterAuthzRoleBindingWithConditionsYAML(
				denyBindingName, denyRoleName, "deny",
				"component:create", `resource.componentType == "service"`,
			)
			output, err = framework.KubectlApplyLiteral(kubeContext, denyYAML)
			Expect(err).NotTo(HaveOccurred(), "create deny+condition binding: %s", output)

			By("waiting for allow binding to propagate")
			Eventually(func(g Gomega) {
				resp, err := subjectClient.ListProjectsWithResponse(ctx, testNs, &gen.ListProjectsParams{})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(Equal(http.StatusOK))
				g.Expect(resp.JSON200).NotTo(BeNil())
				g.Expect(resp.JSON200.Items).NotTo(BeEmpty(),
					"developer allow should grant project:view")
			}).Should(Succeed())

			By("waiting for conditional deny binding to propagate")
			Eventually(func(g Gomega) {
				comp := newComponent("e2e-authz-dcel-wait", projectName, "deployment/service")
				resp, err := subjectClient.CreateComponentWithResponse(ctx, testNs, comp)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(Equal(http.StatusForbidden),
					"conditional deny should block component:create for 'deployment/service'")
			}).Should(Succeed())
		})

		AfterAll(func() {
			deleteBindingAndWaitForRevocation(ctx, denyBindingName, func() bool {
				// After deny is gone, developer allow should let us create with "deployment/service".
				comp := newComponent("e2e-authz-dcel-probe", projectName, "deployment/service")
				resp, _ := subjectClient.CreateComponentWithResponse(ctx, testNs, comp)
				if resp != nil && resp.StatusCode() == http.StatusCreated {
					_, _ = subjectClient.DeleteComponentWithResponse(ctx, testNs, "e2e-authz-dcel-probe")
					return true
				}
				return false
			})
			deleteBindingAndWaitForRevocation(ctx, allowBindingName, func() bool {
				resp, _ := subjectClient.ListProjectsWithResponse(ctx, testNs, &gen.ListProjectsParams{})
				return resp != nil && resp.JSON200 != nil && len(resp.JSON200.Items) == 0
			})
			if os.Getenv("E2E_KEEP_RESOURCES") != "true" {
				_, _ = framework.Kubectl(kubeContext, "delete", "clusterauthzrole", denyRoleName, "--ignore-not-found", "--wait=false")
			}
		})

		It("should deny component creation when deny condition matches (ClusterComponentType 'deployment/service')", func() {
			comp := newComponent("e2e-authz-dcel-blocked", projectName, "deployment/service")
			resp, err := subjectClient.CreateComponentWithResponse(ctx, testNs, comp)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusForbidden),
				"conditional deny should block component:create for ClusterComponentType 'deployment/service', got %d: %s",
				resp.StatusCode(), string(resp.Body))
		})

		It("should allow component creation when deny condition does not match (namespace-scoped ComponentType)", func() {
			// Namespace-scoped CT → attribute "{testNs}/e2e-authz-alt-ct" ≠ "service" → deny doesn't fire → developer allow applies.
			comp := newComponentWithType("e2e-authz-dcel-allowed", projectName, altCTName, gen.ComponentSpecComponentTypeKindComponentType)
			resp, err := subjectClient.CreateComponentWithResponse(ctx, testNs, comp)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusCreated),
				"deny condition should not match namespace-scoped ComponentType, developer allow should apply, got %d: %s",
				resp.StatusCode(), string(resp.Body))

			By("cleaning up")
			_, _ = subjectClient.DeleteComponentWithResponse(ctx, testNs, "e2e-authz-dcel-allowed")
			Eventually(func(g Gomega) {
				framework.AssertResourceGone(g, kubeContext, testNs, "component", "e2e-authz-dcel-allowed")
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())
		})
	})
})

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

var _ = Describe("OpenChoreo API", Ordered, Label("tier2"), func() {
	SetDefaultEventuallyTimeout(framework.DefaultTimeout)
	SetDefaultEventuallyPollingInterval(framework.DefaultPolling)

	var (
		client *gen.ClientWithResponses
		ctx    context.Context

		nsName       string
		kubectlNs    string
		projectName  string
		compName     string
		workloadName string

		ct1Name string
		ct2Name string
	)

	BeforeAll(func() {
		ctx = context.Background()
		nsName = fmt.Sprintf("e2e-api-%d", time.Now().UnixNano())
		kubectlNs = fmt.Sprintf("e2e-api-kctl-%d", time.Now().UnixNano())
		projectName = "e2e-api-proj"
		compName = "e2e-api-comp"
		workloadName = compName
		ct1Name = "e2e-api-ct-alpha"
		ct2Name = "e2e-api-ct-beta"

		By("waiting for API server to be reachable through kgateway")
		framework.WaitForHTTP(apiURL+"/health", 60*time.Second)

		By("obtaining OAuth2 token from Thunder IdP")
		var token string
		Eventually(func() error {
			var tokenErr error
			token, tokenErr = fetchToken()
			return tokenErr
		}, 60*time.Second, 5*time.Second).Should(Succeed(), "failed to obtain token from Thunder")
		Expect(token).NotTo(BeEmpty())

		client = newAPIClient(token)

		By("creating kubectl-managed namespace for catalog sync test")
		yaml := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %s
  labels:
    openchoreo.dev/control-plane: "true"
`, kubectlNs)
		output, err := framework.KubectlApplyLiteral(kubeContext, yaml)
		Expect(err).NotTo(HaveOccurred(), "kubectl apply namespace: %s", output)
	})

	AfterAll(func() {
		if os.Getenv("E2E_KEEP_RESOURCES") == "true" {
			By("skipping cleanup (E2E_KEEP_RESOURCES=true)")
			return
		}

		By("cleaning up test namespaces")
		if nsName != "" {
			_, _ = framework.Kubectl(kubeContext, "delete", "namespace", nsName, "--ignore-not-found", "--wait=false")
		}
		if kubectlNs != "" {
			_, _ = framework.Kubectl(kubeContext, "delete", "namespace", kubectlNs, "--ignore-not-found", "--wait=false")
		}
	})

	// ── Test 1: Unauthenticated request → 401 ───────────────────────────
	It("should reject unauthenticated requests with 401", func() {
		unauthClient := newUnauthClient()
		resp, err := unauthClient.ListNamespacesWithResponse(ctx, &gen.ListNamespacesParams{})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusUnauthorized),
			"expected 401 for unauthenticated request to protected endpoint")
	})

	// ── Test 2: Health and Ready endpoints ───────────────────────────────
	It("should serve health and ready endpoints", func() {
		By("checking /health")
		resp, err := client.GetHealthWithResponse(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusOK))

		By("checking /ready")
		resp2, err := client.GetReadyWithResponse(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp2.StatusCode()).To(Equal(http.StatusOK))

		By("checking /version")
		resp3, err := client.GetVersionWithResponse(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp3.StatusCode()).To(Equal(http.StatusOK))
	})

	// ── Test 3: Create namespace via API → verify K8s namespace ──────────
	It("should create namespace via API and verify it exists in K8s", func() {
		resp, err := client.CreateNamespaceWithResponse(ctx, newNamespace(nsName))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusCreated),
			"expected 201 for namespace creation, got %d: %s", resp.StatusCode(), string(resp.Body))
		Expect(resp.JSON201).NotTo(BeNil())
		Expect(resp.JSON201.Metadata.Name).To(Equal(nsName))

		By("verifying K8s namespace exists with control-plane label")
		Eventually(func(g Gomega) {
			framework.AssertClusterResourceExists(g, kubeContext, "namespace", nsName)
			output, err := framework.Kubectl(kubeContext,
				"get", "namespace", nsName,
				"-o", `jsonpath={.metadata.labels.openchoreo\.dev/control-plane}`,
			)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("true"))
		}, 30*time.Second, 2*time.Second).Should(Succeed())

		By("applying platform resources for release chain tests")
		output, err := framework.KubectlApplyLiteral(kubeContext, platformResourcesYAML(nsName))
		Expect(err).NotTo(HaveOccurred(), "apply platform resources: %s", output)
	})

	// ── Test 4: List namespaces includes kubectl-created namespace ────────
	It("should list namespaces including kubectl-created ones", func() {
		Eventually(func(g Gomega) {
			resp, err := client.ListNamespacesWithResponse(ctx, &gen.ListNamespacesParams{})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(resp.StatusCode()).To(Equal(http.StatusOK))
			g.Expect(resp.JSON200).NotTo(BeNil())

			names := make([]string, 0, len(resp.JSON200.Items))
			for _, ns := range resp.JSON200.Items {
				names = append(names, ns.Metadata.Name)
			}

			g.Expect(names).To(ContainElement(kubectlNs),
				"kubectl-created namespace should appear in API list")
			g.Expect(names).To(ContainElement(nsName),
				"API-created namespace should appear in API list")
		}, 30*time.Second, 2*time.Second).Should(Succeed())
	})

	// ── Test 5: Create project via API → verify CR exists ────────────────
	It("should create project via API and verify Project CR exists", func() {
		resp, err := client.CreateProjectWithResponse(ctx, nsName, newProject(projectName))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusCreated),
			"expected 201 for project creation, got %d: %s", resp.StatusCode(), string(resp.Body))
		Expect(resp.JSON201).NotTo(BeNil())
		Expect(resp.JSON201.Metadata.Name).To(Equal(projectName))

		By("verifying Project CR exists in K8s")
		Eventually(func(g Gomega) {
			framework.AssertResourceExists(g, kubeContext, nsName, "project", projectName)
		}, 30*time.Second, 2*time.Second).Should(Succeed())

		By("verifying project is returned by API GET")
		getResp, err := client.GetProjectWithResponse(ctx, nsName, projectName)
		Expect(err).NotTo(HaveOccurred())
		Expect(getResp.StatusCode()).To(Equal(http.StatusOK))
		Expect(getResp.JSON200).NotTo(BeNil())
		Expect(getResp.JSON200.Metadata.Name).To(Equal(projectName))
	})

	// ── Test 6: ComponentType list + pagination ──────────────────────────
	It("should list component types and paginate results", func() {
		By("applying suite-owned ComponentTypes via kubectl")
		output, err := framework.KubectlApplyLiteral(kubeContext, componentTypeYAML(nsName, ct1Name))
		Expect(err).NotTo(HaveOccurred(), "apply CT1: %s", output)
		output, err = framework.KubectlApplyLiteral(kubeContext, componentTypeYAML(nsName, ct2Name))
		Expect(err).NotTo(HaveOccurred(), "apply CT2: %s", output)

		By("listing all component types in the namespace")
		Eventually(func(g Gomega) {
			resp, err := client.ListComponentTypesWithResponse(ctx, nsName, &gen.ListComponentTypesParams{})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(resp.StatusCode()).To(Equal(http.StatusOK))
			g.Expect(resp.JSON200).NotTo(BeNil())

			names := make([]string, 0, len(resp.JSON200.Items))
			for _, ct := range resp.JSON200.Items {
				names = append(names, ct.Metadata.Name)
			}
			g.Expect(names).To(ContainElements(ct1Name, ct2Name))
		}, 30*time.Second, 2*time.Second).Should(Succeed())

		By("paginating with limit=1")
		limit := 1
		page1, err := client.ListComponentTypesWithResponse(ctx, nsName, &gen.ListComponentTypesParams{
			Limit: &limit,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(page1.StatusCode()).To(Equal(http.StatusOK))
		Expect(page1.JSON200).NotTo(BeNil())
		Expect(page1.JSON200.Items).To(HaveLen(1), "page 1 should have exactly 1 item")
		Expect(page1.JSON200.Pagination.NextCursor).NotTo(BeNil(),
			"page 1 should have a next cursor")

		By("fetching page 2 with cursor")
		cursor := *page1.JSON200.Pagination.NextCursor
		page2, err := client.ListComponentTypesWithResponse(ctx, nsName, &gen.ListComponentTypesParams{
			Limit:  &limit,
			Cursor: &cursor,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(page2.StatusCode()).To(Equal(http.StatusOK))
		Expect(page2.JSON200).NotTo(BeNil())
		Expect(page2.JSON200.Items).To(HaveLen(1), "page 2 should have exactly 1 item")

		page1Name := page1.JSON200.Items[0].Metadata.Name
		page2Name := page2.JSON200.Items[0].Metadata.Name
		Expect(page1Name).NotTo(Equal(page2Name),
			"page 1 and page 2 should return different ComponentTypes")
		Expect([]string{page1Name, page2Name}).To(ContainElements(ct1Name, ct2Name))
	})

	// ── Test 7: Component + Workload → release chain ─────────────────────
	It("should create component and workload via API and trigger release chain", func() {
		By("creating component via API")
		comp := newComponent(compName, projectName, "deployment/service")
		compResp, err := client.CreateComponentWithResponse(ctx, nsName, comp)
		Expect(err).NotTo(HaveOccurred())
		Expect(compResp.StatusCode()).To(Equal(http.StatusCreated),
			"expected 201 for component creation, got %d: %s", compResp.StatusCode(), string(compResp.Body))
		Expect(compResp.JSON201).NotTo(BeNil())

		By("creating workload for the component")
		wl := newWorkload(workloadName, compName, projectName)
		wlResp, err := client.CreateWorkloadWithResponse(ctx, nsName, wl)
		Expect(err).NotTo(HaveOccurred())
		Expect(wlResp.StatusCode()).To(Equal(http.StatusCreated),
			"expected 201 for workload creation, got %d: %s", wlResp.StatusCode(), string(wlResp.Body))

		By("verifying Component CR exists")
		Eventually(func(g Gomega) {
			framework.AssertResourceExists(g, kubeContext, nsName, "component", compName)
		}, 30*time.Second, 2*time.Second).Should(Succeed())

		By("verifying ComponentRelease appears")
		Eventually(func(g Gomega) {
			output, err := framework.KubectlGet(kubeContext, nsName, "componentrelease")
			g.Expect(err).NotTo(HaveOccurred(), "failed to list componentreleases: %s", output)
			g.Expect(output).To(ContainSubstring(compName))
		}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())

		By("verifying ComponentReleaseBinding appears")
		Eventually(func(g Gomega) {
			output, err := framework.KubectlGet(kubeContext, nsName, "componentreleasebinding")
			g.Expect(err).NotTo(HaveOccurred(), "failed to list componentreleasebindings: %s", output)
			g.Expect(output).To(ContainSubstring(compName))
		}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())
	})

	// ── Test 8: Get component + list release bindings via API ─────────────
	It("should read component and release binding details via API", func() {
		By("waiting for ComponentReleaseBinding to become Ready")
		rbName := fmt.Sprintf("%s-development", compName)
		Eventually(func(g Gomega) {
			framework.AssertComponentReleaseBindingReady(g, kubeContext, nsName, rbName)
		}, 5*time.Minute, framework.DefaultPolling).Should(Succeed())

		By("getting component via API")
		getResp, err := client.GetComponentWithResponse(ctx, nsName, compName)
		Expect(err).NotTo(HaveOccurred())
		Expect(getResp.StatusCode()).To(Equal(http.StatusOK))
		Expect(getResp.JSON200).NotTo(BeNil())
		Expect(getResp.JSON200.Metadata.Name).To(Equal(compName))

		By("listing release bindings via API")
		compFilter := compName
		rbResp, err := client.ListComponentReleaseBindingsWithResponse(ctx, nsName, &gen.ListComponentReleaseBindingsParams{
			Component: &compFilter,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(rbResp.StatusCode()).To(Equal(http.StatusOK))
		Expect(rbResp.JSON200).NotTo(BeNil())
		Expect(rbResp.JSON200.Items).NotTo(BeEmpty(), "expected at least one ComponentReleaseBinding")

		found := false
		for _, rb := range rbResp.JSON200.Items {
			if rb.Metadata.Name == rbName {
				found = true
				Expect(rb.Spec).NotTo(BeNil())
				Expect(rb.Spec.Environment).To(Equal("development"))
				Expect(rb.Spec.Owner.ComponentName).To(Equal(compName))
				break
			}
		}
		Expect(found).To(BeTrue(), "expected ComponentReleaseBinding %s in API response", rbName)
	})

	// ── Test 9: Delete component → verify CR cleanup ─────────────────────
	It("should delete component via API and clean up owned resources", func() {
		delResp, err := client.DeleteComponentWithResponse(ctx, nsName, compName)
		Expect(err).NotTo(HaveOccurred())
		Expect(delResp.StatusCode()).To(SatisfyAny(
			Equal(http.StatusOK),
			Equal(http.StatusNoContent),
		), "expected 200 or 204 for component deletion, got %d", delResp.StatusCode())

		By("verifying Component CR is removed")
		Eventually(func(g Gomega) {
			framework.AssertResourceGone(g, kubeContext, nsName, "component", compName)
		}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())

		By("verifying Workload is cleaned up")
		Eventually(func(g Gomega) {
			framework.AssertResourceGone(g, kubeContext, nsName, "workload", workloadName)
		}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())

		By("verifying ComponentRelease is cleaned up")
		Eventually(func(g Gomega) {
			output, err := framework.KubectlGet(kubeContext, nsName, "componentrelease")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).NotTo(ContainSubstring(compName))
		}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())

		By("verifying ComponentReleaseBinding is cleaned up")
		Eventually(func(g Gomega) {
			output, err := framework.KubectlGet(kubeContext, nsName, "componentreleasebinding")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).NotTo(ContainSubstring(compName))
		}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())
	})

	// ── Test 10: Delete project via API → verify cleanup ─────────────────
	It("should delete project via API and verify cleanup", func() {
		delResp, err := client.DeleteProjectWithResponse(ctx, nsName, projectName)
		Expect(err).NotTo(HaveOccurred())
		Expect(delResp.StatusCode()).To(SatisfyAny(
			Equal(http.StatusOK),
			Equal(http.StatusNoContent),
		), "expected 200 or 204 for project deletion")

		By("verifying Project CR is removed")
		Eventually(func(g Gomega) {
			framework.AssertResourceGone(g, kubeContext, nsName, "project", projectName)
		}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())
	})
})

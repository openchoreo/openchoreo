// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

const (
	mcpEndpoint  = "http://api.e2e-cp.local:28080/mcp"
	tokenURL     = "http://thunder.e2e-cp.local:28080/oauth2/token"
	clientID     = "service_mcp_client"
	clientSecret = "service_mcp_client_secret"
)

var (
	mcpRunID = fmt.Sprintf("%d", time.Now().UnixNano())
	mcpNs    = fmt.Sprintf("e2e-mcp-%s", mcpRunID)
	dpNs     string
)

var _ = Describe("MCP Server", Ordered, Label("tier2"), func() {
	var token string

	BeforeAll(func() {
		By("fetching an OAuth2 access token from Thunder IdP")
		var err error
		token, err = framework.FetchOAuth2Token(tokenURL, clientID, clientSecret)
		Expect(err).NotTo(HaveOccurred(), "failed to fetch OAuth2 token")
		Expect(token).NotTo(BeEmpty(), "access token should not be empty")
		fmt.Fprintf(GinkgoWriter, "OAuth2 token obtained successfully\n")
	})

	AfterAll(func() {
		if os.Getenv("E2E_KEEP_RESOURCES") == "true" {
			By("skipping cleanup because E2E_KEEP_RESOURCES=true")
			return
		}

		By("cleaning up data plane namespace")
		if dpNs != "" {
			_, _ = framework.Kubectl(kubeContext, "delete", "namespace", dpNs, "--ignore-not-found", "--wait=false")
		}

		By("cleaning up test namespace")
		_, _ = framework.Kubectl(kubeContext, "delete", "namespace", mcpNs, "--ignore-not-found", "--wait=false")
	})

	Context("authentication", func() {
		It("rejects unauthenticated requests with 401", func() {
			statusCode, headers, err := framework.MCPRawPOST(mcpEndpoint)
			Expect(err).NotTo(HaveOccurred(), "raw POST should succeed at HTTP level")
			Expect(statusCode).To(Equal(401), "unauthenticated request should return 401")
			Expect(headers.Get("WWW-Authenticate")).To(ContainSubstring("Bearer"),
				"401 response should include WWW-Authenticate header")
		})

		It("connects successfully with a valid token", func() {
			session, err := framework.NewMCPSession(context.Background(), framework.MCPClientConfig{
				Endpoint: mcpEndpoint,
				Token:    token,
			})
			Expect(err).NotTo(HaveOccurred(), "MCP session should connect with valid token")
			defer session.Close()

			names, err := framework.ListMCPToolNames(session)
			Expect(err).NotTo(HaveOccurred(), "tools/list should succeed")
			Expect(names).NotTo(BeEmpty(), "tool list should not be empty")
			fmt.Fprintf(GinkgoWriter, "Connected to MCP server, %d tools available\n", len(names))
		})
	})

	Context("tool listing", func() {
		It("returns expected core tools", func() {
			noDeprecated := false
			session, err := framework.NewMCPSession(context.Background(), framework.MCPClientConfig{
				Endpoint:               mcpEndpoint,
				Token:                  token,
				IncludeDeprecatedTools: &noDeprecated,
			})
			Expect(err).NotTo(HaveOccurred())
			defer session.Close()

			names, err := framework.ListMCPToolNames(session)
			Expect(err).NotTo(HaveOccurred())

			expectedTools := []string{
				"list_namespaces",
				"create_namespace",
				"list_projects",
				"create_project",
				"create_component",
				"list_components",
				"create_workload",
				"list_release_bindings",
				"list_environments",
			}
			for _, expected := range expectedTools {
				Expect(names).To(ContainElement(expected),
					"tools/list should include %s", expected)
			}
		})

		It("respects toolset narrowing via query parameter", func() {
			noDeprecated := false
			session, err := framework.NewMCPSession(context.Background(), framework.MCPClientConfig{
				Endpoint:               mcpEndpoint,
				Token:                  token,
				Toolsets:               []string{"namespace"},
				IncludeDeprecatedTools: &noDeprecated,
			})
			Expect(err).NotTo(HaveOccurred())
			defer session.Close()

			names, err := framework.ListMCPToolNames(session)
			Expect(err).NotTo(HaveOccurred())

			Expect(names).To(ContainElement("list_namespaces"),
				"namespace toolset should include list_namespaces")
			Expect(names).To(ContainElement("create_namespace"),
				"namespace toolset should include create_namespace")

			Expect(names).NotTo(ContainElement("create_project"),
				"narrowed to namespace toolset should not include create_project")
			Expect(names).NotTo(ContainElement("create_component"),
				"narrowed to namespace toolset should not include create_component")
		})
	})

	Context("namespace tool chain", func() {
		It("creates a namespace via MCP and verifies it exists on the cluster", func() {
			session, err := framework.NewMCPSession(context.Background(), framework.MCPClientConfig{
				Endpoint: mcpEndpoint,
				Token:    token,
			})
			Expect(err).NotTo(HaveOccurred())
			defer session.Close()

			By("creating a namespace via MCP create_namespace tool")
			_, err = framework.CallMCPTool(session, "create_namespace", map[string]any{
				"name":         mcpNs,
				"display_name": "MCP E2E Test",
				"description":  "Created by MCP e2e test",
			})
			Expect(err).NotTo(HaveOccurred(), "create_namespace should succeed")

			By("verifying the namespace exists on the cluster via kubectl")
			Eventually(func(g Gomega) {
				framework.AssertClusterResourceExists(g, kubeContext, "namespace", mcpNs)
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())

			By("listing namespaces via MCP and verifying the new namespace appears")
			var listResult struct {
				Namespaces []struct {
					Name string `json:"name"`
				} `json:"namespaces"`
			}
			err = framework.CallMCPToolJSON(session, "list_namespaces", nil, &listResult)
			Expect(err).NotTo(HaveOccurred(), "list_namespaces should succeed")

			found := false
			for _, ns := range listResult.Namespaces {
				if ns.Name == mcpNs {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "list_namespaces should include %s", mcpNs)
		})
	})

	Context("component tool chain", func() {
		const (
			projectName   = "e2e-mcp-proj"
			componentName = "e2e-mcp-echo"
			workloadName  = componentName + "-workload"
		)

		var session *mcp.ClientSession

		BeforeAll(func() {
			var err error
			session, err = framework.NewMCPSession(context.Background(), framework.MCPClientConfig{
				Endpoint: mcpEndpoint,
				Token:    token,
			})
			Expect(err).NotTo(HaveOccurred(), "MCP session should connect")

			By("applying platform resources (DeploymentPipeline + Environments) for auto-deploy")
			output, err := framework.KubectlApplyLiteral(kubeContext,
				platformResourcesYAML(mcpNs, []string{"development", "staging"}, nil))
			Expect(err).NotTo(HaveOccurred(), "failed to apply platform resources: %s", output)
		})

		AfterAll(func() {
			if session != nil {
				session.Close()
			}
		})

		It("creates a project via MCP", func() {
			_, err := framework.CallMCPTool(session, "create_project", map[string]any{
				"namespace_name":      mcpNs,
				"name":                projectName,
				"description":         "MCP e2e test project",
				"deployment_pipeline": "default",
			})
			Expect(err).NotTo(HaveOccurred(), "create_project should succeed")

			By("verifying the project CR exists on the cluster")
			Eventually(func(g Gomega) {
				framework.AssertResourceExists(g, kubeContext, mcpNs, "project", projectName)
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())
		})

		It("creates a component via MCP", func() {
			_, err := framework.CallMCPTool(session, "create_component", map[string]any{
				"namespace_name": mcpNs,
				"project_name":   projectName,
				"name":           componentName,
				"component_type": "deployment/service",
				"auto_deploy":    true,
			})
			Expect(err).NotTo(HaveOccurred(), "create_component should succeed")

			By("verifying the component CR exists on the cluster")
			Eventually(func(g Gomega) {
				framework.AssertResourceExists(g, kubeContext, mcpNs, "component", componentName)
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())
		})

		It("creates a workload via MCP", func() {
			_, err := framework.CallMCPTool(session, "create_workload", map[string]any{
				"namespace_name": mcpNs,
				"component_name": componentName,
				"workload_spec": map[string]any{
					"container": map[string]any{
						"image": "hashicorp/http-echo:0.2.3",
						"args":  []string{"-text=mcp-e2e", "-listen=:8080"},
					},
					"endpoints": map[string]any{
						"http": map[string]any{
							"type":       "HTTP",
							"port":       8080,
							"visibility": []string{"project"},
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred(), "create_workload should succeed")

			By("verifying the workload CR exists on the cluster")
			Eventually(func(g Gomega) {
				framework.AssertResourceExists(g, kubeContext, mcpNs, "workload", workloadName)
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())
		})

		It("verifies the release chain is created", func() {
			By("waiting for ComponentRelease to appear via component status")
			Eventually(func(g Gomega) {
				releaseName, err := framework.KubectlGetJsonpath(
					kubeContext, mcpNs, "component", componentName,
					"{.status.latestRelease.name}",
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(releaseName).NotTo(BeEmpty(), "component should have a latestRelease")
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())

			By("waiting for ReleaseBinding to appear")
			rbName := fmt.Sprintf("%s-development", componentName)
			Eventually(func(g Gomega) {
				framework.AssertResourceExists(g, kubeContext, mcpNs, "releasebinding", rbName)
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())

			By("discovering data plane namespace for cleanup")
			Eventually(func() error {
				var discoverErr error
				dpNs, discoverErr = framework.GetDPNamespace(kubeContext, mcpNs, projectName, "development")
				return discoverErr
			}, framework.DefaultTimeout, 5*time.Second).Should(Succeed(), "dp namespace not found")
			fmt.Fprintf(GinkgoWriter, "discovered dp namespace: %s\n", dpNs)
		})

		It("lists components and finds the created component", func() {
			var listResult struct {
				Components []struct {
					Name string `json:"name"`
				} `json:"components"`
			}
			err := framework.CallMCPToolJSON(session, "list_components", map[string]any{
				"namespace_name": mcpNs,
				"project_name":   projectName,
			}, &listResult)
			Expect(err).NotTo(HaveOccurred(), "list_components should succeed")

			found := false
			for _, c := range listResult.Components {
				if c.Name == componentName {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "list_components should include %s", componentName)
		})
	})
})

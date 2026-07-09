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

	// subjectClientID is a dedicated, permission-less client_credentials subject
	// seeded unbound by the Thunder bootstrap (install/k3d/common/values-thunder.yaml
	// `61-mcp-e2e-subject-app.sh`). It is OWNED EXCLUSIVELY by this MCP suite's
	// authorization context: the authz suite already binds/unbinds roles on
	// `customer-portal-client`, and `make e2e.test` runs the authz and mcp suite
	// packages concurrently (no `-p 1`), so sharing a subject would flake both.
	// Never create bindings for this client from any other suite, and never bind
	// `customer-portal-client` / `service_mcp_client` from this context.
	subjectClientID     = "mcp-e2e-subject-client"
	subjectClientSecret = "mcp-e2e-subject-secret" //nolint:gosec

	// mcpAuthzLabelKey labels every ClusterAuthzRole(Binding) this context creates
	// so the AfterAll can sweep leftovers by selector.
	mcpAuthzLabelKey = "e2e-mcp-authz/run"
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

			By("creating a ProjectReleaseBinding so the development cell namespace is provisioned")
			output, err := framework.KubectlApplyLiteral(kubeContext,
				projectReleaseBindingYAML(mcpNs, projectName, "development"))
			Expect(err).NotTo(HaveOccurred(), "failed to create ProjectReleaseBinding: %s", output)
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

	// authorization exercises the control-plane MCP tool-filter middleware
	// (pkg/mcp/tools/filter.go) end to end against a real OAuth subject and the
	// live PDP. It uses the dedicated permission-less subjectClientID so it never
	// collides with the concurrently-running authz suite's subject. All
	// ClusterAuthzRole(Binding) CRs are labelled mcpAuthzLabelKey=<mcpRunID> and
	// swept in this context's AfterAll.
	Context("authorization", Ordered, func() {
		var subjectToken string
		labelValue := mcpRunID

		// subjectSession is the default-config session (filterByAuthz=on) used by
		// every case that needs the visibility filter active.
		var subjectSession *mcp.ClientSession

		BeforeAll(func() {
			By("fetching a token for the permission-less MCP subject")
			Eventually(func() error {
				var tokenErr error
				subjectToken, tokenErr = framework.FetchClientCredentialsToken(
					tokenURL, subjectClientID, subjectClientSecret)
				return tokenErr
			}, 60*time.Second, 5*time.Second).Should(Succeed(),
				"failed to obtain token for %s", subjectClientID)
			Expect(subjectToken).NotTo(BeEmpty())

			var err error
			subjectSession, err = framework.NewMCPSession(context.Background(), framework.MCPClientConfig{
				Endpoint: mcpEndpoint,
				Token:    subjectToken,
			})
			Expect(err).NotTo(HaveOccurred(), "subject MCP session should connect")
		})

		AfterAll(func() {
			if subjectSession != nil {
				subjectSession.Close()
			}
			// Always sweep authz CRs even when keeping namespaces: a lingering
			// grant would poison later deny-by-default cases and reruns.
			By("sweeping leftover authz CRs by selector")
			selector := fmt.Sprintf("%s=%s", mcpAuthzLabelKey, labelValue)
			_, _ = framework.Kubectl(kubeContext, "delete", "clusterauthzrolebinding", "-l", selector, "--ignore-not-found", "--wait=false")
			_, _ = framework.Kubectl(kubeContext, "delete", "clusterauthzrole", "-l", selector, "--ignore-not-found", "--wait=false")
		})

		It("T1: deny-by-default visibility — unbound subject's tools/list excludes write tools", func() {
			// T1: the unbound subject's tools/list must HIDE write tools (create_namespace/project/component).
			// Verifies the MCP visibility filter (filter.go:113) over the real request path:
			// OAuth token -> MCP filter -> PDP. The PDP's allow/deny decisions are unit-tested in
			// internal/authz/casbin/pdp_test.go; what this adds is that the MCP layer actually consults
			// the PDP for a real subject. Meaningful only paired with T2 (an empty list passes vacuously).
			names, err := framework.ListMCPToolNames(subjectSession)
			Expect(err).NotTo(HaveOccurred(), "tools/list should succeed")
			Expect(names).NotTo(ContainElement("create_namespace"))
			Expect(names).NotTo(ContainElement("create_project"))
			Expect(names).NotTo(ContainElement("create_component"))
		})

		It("T2: deny-by-default enforcement — create_namespace rejected", func() {
			// T2: an unbound subject's create_namespace call must be REJECTED with the explicit
			// "missing permission \"namespace:create\"" error (filter.go:174). This is the real deny-by-default
			// proof and what makes T1's absence meaningful. Exercises the real OAuth -> MCP filter -> PDP
			// path (the PDP decision itself is unit-tested in pdp_test.go); this proves the live PDP is
			// wired into the MCP layer and denying.
			deniedNs := fmt.Sprintf("e2e-mcp-denied-%s", mcpRunID)
			framework.CallMCPToolExpectDenied(subjectSession, "create_namespace",
				map[string]any{"name": deniedNs},
				`not authorized to call tool "create_namespace": missing permission "namespace:create"`)

			By("verifying the namespace was not created")
			_, err := framework.Kubectl(kubeContext, "get", "namespace", deniedNs)
			Expect(err).To(HaveOccurred(), "denied create_namespace must not create %s", deniedNs)
		})

		It("T3: allow-after-grant round-trip — grant, call succeeds, tool visible, revoke, denied", func() {
			// T3: grant role -> call succeeds + tool becomes visible -> revoke -> denied again.
			// Verifies the full grant/propagate/revoke lifecycle through the MCP layer, not just denial.
			// Exercises real binding-propagation timing (eventual consistency) end to end; the PDP decision
			// logic is unit-tested in pdp_test.go.
			roleName := "e2e-mcp-authz-nscreate-" + mcpRunID
			bindingName := "e2e-mcp-authz-nscreate-bind-" + mcpRunID
			grantedNs := "e2e-mcp-granted-" + mcpRunID

			By("granting a ClusterAuthzRole + binding with namespace:create")
			out, err := framework.KubectlApplyLiteral(kubeContext,
				framework.ClusterAuthzRoleYAML(roleName, mcpAuthzLabelKey, labelValue,
					[]string{"namespace:create", "namespace:view"}))
			Expect(err).NotTo(HaveOccurred(), "create role: %s", out)
			out, err = framework.KubectlApplyLiteral(kubeContext,
				framework.ClusterAuthzRoleBindingYAML(bindingName, mcpAuthzLabelKey, labelValue,
					roleName, subjectClientID, "allow"))
			Expect(err).NotTo(HaveOccurred(), "create binding: %s", out)

			By("waiting for the grant to propagate, then create_namespace succeeds")
			Eventually(func(g Gomega) {
				_, callErr := framework.CallMCPTool(subjectSession, "create_namespace",
					map[string]any{"name": grantedNs})
				g.Expect(callErr).NotTo(HaveOccurred())
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())

			By("verifying create_namespace is now visible in tools/list")
			Eventually(func(g Gomega) {
				names, listErr := framework.ListMCPToolNames(subjectSession)
				g.Expect(listErr).NotTo(HaveOccurred())
				g.Expect(names).To(ContainElement("create_namespace"))
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())

			By("verifying the namespace exists on the cluster")
			Eventually(func(g Gomega) {
				framework.AssertClusterResourceExists(g, kubeContext, "namespace", grantedNs)
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())

			By("cleaning up the granted namespace and revoking the binding")
			_, _ = framework.Kubectl(kubeContext, "delete", "namespace", grantedNs, "--ignore-not-found", "--wait=false")
			probeNs := "e2e-mcp-revoke-probe-" + mcpRunID
			framework.DeleteClusterAuthzRoleBindingAndWaitForRevocation(kubeContext, bindingName,
				framework.DeniedProbe(subjectSession, "create_namespace", map[string]any{"name": probeNs},
					`missing permission "namespace:create"`,
					func() {
						_, _ = framework.Kubectl(kubeContext, "delete", "namespace", probeNs, "--ignore-not-found", "--wait=false")
					}))
			_, _ = framework.Kubectl(kubeContext, "delete", "clusterauthzrole", roleName, "--ignore-not-found", "--wait=false")
		})

		It("T4: scoped isolation — namespace-scoped grant allows ns-A, denies ns-B", func() {
			// T4: a namespace-scoped grant must allow create_project in ns-A and DENY it in ns-B.
			// Verifies per-call scope extraction (filter.go:299) -> multi-tenant isolation, flowing
			// OAuth -> MCP -> PDP for a real scoped binding. The scoped-decision logic itself is unit-tested
			// in pdp_test.go; this proves the MCP filter extracts and passes the scope per call.
			nsA := "e2e-mcp-scope-a-" + mcpRunID
			nsB := "e2e-mcp-scope-b-" + mcpRunID
			roleName := "e2e-mcp-authz-projcreate-" + mcpRunID
			bindingName := "e2e-mcp-authz-projcreate-bind-" + mcpRunID

			By("creating two control-plane namespaces with platform resources")
			for _, ns := range []string{nsA, nsB} {
				out, err := framework.KubectlApplyLiteral(kubeContext, cpNamespaceYAML(ns))
				Expect(err).NotTo(HaveOccurred(), "create namespace %s: %s", ns, out)
				out, err = framework.KubectlApplyLiteral(kubeContext,
					platformResourcesYAML(ns, []string{"development"}, nil))
				Expect(err).NotTo(HaveOccurred(), "apply platform resources %s: %s", ns, out)
			}

			By("granting project:create scoped to ns-A only")
			out, err := framework.KubectlApplyLiteral(kubeContext,
				framework.ClusterAuthzRoleYAML(roleName, mcpAuthzLabelKey, labelValue,
					[]string{"project:create", "project:view", "namespace:view"}))
			Expect(err).NotTo(HaveOccurred(), "create role: %s", out)
			out, err = framework.KubectlApplyLiteral(kubeContext,
				framework.ScopedClusterAuthzRoleBindingYAML(bindingName, mcpAuthzLabelKey, labelValue,
					roleName, subjectClientID, "allow", nsA))
			Expect(err).NotTo(HaveOccurred(), "create scoped binding: %s", out)

			By("create_project in ns-A succeeds once the scoped grant propagates")
			Eventually(func(g Gomega) {
				_, callErr := framework.CallMCPTool(subjectSession, "create_project", map[string]any{
					"namespace_name":      nsA,
					"name":                "scoped-proj",
					"deployment_pipeline": "default",
				})
				g.Expect(callErr).NotTo(HaveOccurred())
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())

			By("create_project in ns-B is denied (out of scope)")
			framework.CallMCPToolExpectDenied(subjectSession, "create_project", map[string]any{
				"namespace_name":      nsB,
				"name":                "scoped-proj-b",
				"deployment_pipeline": "default",
			}, `missing permission "project:create"`)

			By("revoking the binding and cleaning up")
			framework.DeleteClusterAuthzRoleBindingAndWaitForRevocation(kubeContext, bindingName,
				framework.DeniedProbe(subjectSession, "create_project", map[string]any{
					"namespace_name":      nsA,
					"name":                "scoped-proj-probe",
					"deployment_pipeline": "default",
				}, `missing permission "project:create"`, func() {
					// grant still active; drop the probe project so a later poll doesn't hit a conflict
					_, _ = framework.Kubectl(kubeContext, "delete", "project", "scoped-proj-probe", "-n", nsA, "--ignore-not-found", "--wait=false")
				}))
			_, _ = framework.Kubectl(kubeContext, "delete", "clusterauthzrole", roleName, "--ignore-not-found", "--wait=false")
			_, _ = framework.Kubectl(kubeContext, "delete", "namespace", nsA, "--ignore-not-found", "--wait=false")
			_, _ = framework.Kubectl(kubeContext, "delete", "namespace", nsB, "--ignore-not-found", "--wait=false")
		})

		It("T5: deny effect overrides allow", func() {
			// T5: a deny binding must OVERRIDE an allow binding (deny-precedence).
			// Security-critical PDP semantic: without it, access can't be revoked by adding a deny.
			// The deny-precedence decision is unit-tested in pdp_test.go; this proves it holds end to end
			// through the MCP path with real allow+deny bindings applied as CRs.
			allowRoleName := "e2e-mcp-authz-allowrole-" + mcpRunID
			allowBindingName := "e2e-mcp-authz-allow-" + mcpRunID
			denyRoleName := "e2e-mcp-authz-denyrole-" + mcpRunID
			denyBindingName := "e2e-mcp-authz-deny-" + mcpRunID

			By("granting an allow role (namespace:create + namespace:view)")
			out, err := framework.KubectlApplyLiteral(kubeContext,
				framework.ClusterAuthzRoleYAML(allowRoleName, mcpAuthzLabelKey, labelValue,
					[]string{"namespace:create", "namespace:view"}))
			Expect(err).NotTo(HaveOccurred(), "create allow role: %s", out)
			out, err = framework.KubectlApplyLiteral(kubeContext,
				framework.ClusterAuthzRoleBindingYAML(allowBindingName, mcpAuthzLabelKey, labelValue,
					allowRoleName, subjectClientID, "allow"))
			Expect(err).NotTo(HaveOccurred(), "create allow binding: %s", out)

			By("adding a deny role+binding covering namespace:create")
			out, err = framework.KubectlApplyLiteral(kubeContext,
				framework.ClusterAuthzRoleYAML(denyRoleName, mcpAuthzLabelKey, labelValue,
					[]string{"namespace:create"}))
			Expect(err).NotTo(HaveOccurred(), "create deny role: %s", out)
			out, err = framework.KubectlApplyLiteral(kubeContext,
				framework.ClusterAuthzRoleBindingYAML(denyBindingName, mcpAuthzLabelKey, labelValue,
					denyRoleName, subjectClientID, "deny"))
			Expect(err).NotTo(HaveOccurred(), "create deny binding: %s", out)

			// Denial surface for deny-override: because the allow binding is
			// present, the MCP filter middleware sees namespace:create as permitted and lets the call
			// THROUGH; the deny is then enforced at the SERVICE layer, which returns
			// "insufficient permissions to perform this action" (services/errors.go:8) — the same
			// backstop T6 exercises. It does NOT surface as the middleware's `missing permission`
			// message (that is the deny-by-default path, T2). The deny still wins; it just lands one
			// layer deeper than a no-binding denial.
			denyOverrideMsg := "insufficient permissions to perform this action"

			By("waiting until list_namespaces succeeds (allow landed) AND create_namespace is denied (deny landed)")
			Eventually(func(g Gomega) {
				_, listErr := framework.CallMCPTool(subjectSession, "list_namespaces", nil)
				g.Expect(listErr).NotTo(HaveOccurred(), "namespace:view should be allowed")
				denyWaitNs := "e2e-mcp-deny-wait-" + mcpRunID
				_, createErr := framework.CallMCPTool(subjectSession, "create_namespace",
					map[string]any{"name": denyWaitNs})
				if createErr == nil {
					// allow landed before the deny did; drop the namespace and keep waiting
					_, _ = framework.Kubectl(kubeContext, "delete", "namespace", denyWaitNs, "--ignore-not-found", "--wait=false")
				}
				g.Expect(createErr).To(HaveOccurred(), "deny binding should block namespace:create")
				g.Expect(createErr.Error()).To(ContainSubstring(denyOverrideMsg))
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())

			By("list_namespaces succeeds (not covered by the deny)")
			_, err = framework.CallMCPTool(subjectSession, "list_namespaces", nil)
			Expect(err).NotTo(HaveOccurred())

			By("create_namespace is denied despite the allow binding (service-layer deny-override)")
			framework.CallMCPToolExpectDenied(subjectSession, "create_namespace",
				map[string]any{"name": "e2e-mcp-deny-final-" + mcpRunID},
				denyOverrideMsg)

			By("revoking the deny binding first, then the allow binding")
			framework.DeleteClusterAuthzRoleBindingAndWaitForRevocation(kubeContext, denyBindingName, func() bool {
				probeNs := "e2e-mcp-deny-revoke-probe-" + mcpRunID
				_, probeErr := framework.CallMCPTool(subjectSession, "create_namespace",
					map[string]any{"name": probeNs})
				if probeErr == nil {
					_, _ = framework.Kubectl(kubeContext, "delete", "namespace", probeNs, "--ignore-not-found", "--wait=false")
					return true
				}
				return false
			})
			allowProbeNs := "e2e-mcp-allow-revoke-probe-" + mcpRunID
			framework.DeleteClusterAuthzRoleBindingAndWaitForRevocation(kubeContext, allowBindingName,
				framework.DeniedProbe(subjectSession, "create_namespace", map[string]any{"name": allowProbeNs},
					`missing permission "namespace:create"`,
					func() {
						_, _ = framework.Kubectl(kubeContext, "delete", "namespace", allowProbeNs, "--ignore-not-found", "--wait=false")
					}))
			_, _ = framework.Kubectl(kubeContext, "delete", "clusterauthzrole", allowRoleName, denyRoleName, "--ignore-not-found", "--wait=false")
		})

		It("T6: filterByAuthz=false — tool visible but the call is still denied by the service layer", func() {
			// T6: with ?filterByAuthz=false the tool is visible and passes the middleware, but the
			// SERVICE LAYER still denies ("insufficient permissions to perform this action").
			// Verifies defense-in-depth (filter.go:36-39): disabling the MCP filter does NOT bypass authz.
			// A real request through both layers proves the service layer still enforces when the filter is
			// off — exercised only when the MCP server, PDP, and service layer run together.
			off := false
			session, err := framework.NewMCPSession(context.Background(), framework.MCPClientConfig{
				Endpoint:      mcpEndpoint,
				Token:         subjectToken,
				FilterByAuthz: &off,
			})
			Expect(err).NotTo(HaveOccurred(), "filterByAuthz=false session should connect")
			defer session.Close()

			By("create_namespace is visible because the MCP filter is disabled")
			names, err := framework.ListMCPToolNames(session)
			Expect(err).NotTo(HaveOccurred())
			Expect(names).To(ContainElement("create_namespace"))

			By("but the call is still denied by the service layer")
			t6Ns := "e2e-mcp-t6-" + mcpRunID
			framework.CallMCPToolExpectDenied(session, "create_namespace",
				map[string]any{"name": t6Ns},
				"insufficient permissions to perform this action")

			By("verifying the namespace was not created")
			_, err = framework.Kubectl(kubeContext, "get", "namespace", t6Ns)
			Expect(err).To(HaveOccurred(), "service-layer-denied create_namespace must not create %s", t6Ns)
		})
	})
})

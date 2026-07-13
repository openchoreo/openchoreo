// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"net"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

// Multi-cluster (tier3) external endpoints. An empty opKubeContext means
// single-cluster, where these specs are skipped (see the BeforeAll gating
// below): the MCP specs depend on the external observer hostname which only the
// multi-cluster setup exposes.
const (
	mcObserverMCPEndpoint = "http://observer.e2e-mc-op.local:31080/mcp"
	mcThunderTokenURL     = "http://thunder.e2e-mc-cp.local:38080/oauth2/token"

	// Admin identity (service_mcp_client) carries the bootstrap admin
	// ClusterAuthzRoleBinding (admin = "*"), so it can call every observer
	// query tool. Uses client_secret_basic.
	obsAdminClientID     = "service_mcp_client"
	obsAdminClientSecret = "service_mcp_client_secret" //nolint:gosec

	// Subject identity (mcp-e2e-subject-client) is seeded unbound by the Thunder
	// bootstrap (install/k3d/common/values-thunder.yaml `61-mcp-e2e-subject-app.sh`)
	// and uses client_secret_post. It is the same permission-less subject the
	// control-plane MCP suite owns; the observer's PDP is the same CP authz API,
	// so grants/denials on it are controlled by ClusterAuthzRole(Binding) CRs on
	// the CP cluster.
	obsSubjectClientID     = "mcp-e2e-subject-client"
	obsSubjectClientSecret = "mcp-e2e-subject-secret" //nolint:gosec

	// obsMCPAuthzLabelKey labels the ClusterAuthzRoleBinding created by O6 so it
	// is swept on cleanup.
	obsMCPAuthzLabelKey = "e2e-obsmcp/run"
)

// allObserverTools is the exact set of 11 tools the observer MCP server
// registers (internal/observer/mcp/server.go). Pinned here so O3 catches an
// accidental add/remove.
var allObserverTools = []string{
	"query_component_logs",
	"query_workflow_logs",
	"query_component_events",
	"query_workflow_events",
	"query_resource_metrics",
	"query_http_metrics",
	"query_traces",
	"query_trace_spans",
	"get_span_details",
	"query_alerts",
	"query_incidents",
}

var _ = Describe("Observer MCP", Ordered, Label("tier3"), func() {
	SetDefaultEventuallyTimeout(framework.DefaultTimeout)
	SetDefaultEventuallyPollingInterval(framework.DefaultPolling)

	var (
		adminToken     string
		subjectToken   string
		adminSession   *mcp.ClientSession
		subjectSession *mcp.ClientSession
	)

	BeforeAll(func() {
		if opKubeContext == "" {
			Skip("observer MCP specs require the multi-cluster setup (--e2e.op-kubecontext)")
		}

		// These specs (and only these) drive host-side MCP sessions against the OP
		// cluster's external observer hostname, which must resolve to 127.0.0.1 (the
		// OP loadbalancer publishes 31080:11080). Checked here rather than in the
		// suite's BeforeSuite so REST-only observability runs aren't blocked by it.
		By("verifying observer.e2e-mc-op.local resolves to 127.0.0.1")
		addrs, lookupErr := net.LookupHost("observer.e2e-mc-op.local")
		Expect(lookupErr).NotTo(HaveOccurred(),
			"observer.e2e-mc-op.local does not resolve — add '127.0.0.1 observer.e2e-mc-op.local' to /etc/hosts")
		Expect(addrs).To(ContainElement("127.0.0.1"),
			"observer.e2e-mc-op.local resolves to %v instead of 127.0.0.1 — check /etc/hosts", addrs)

		By("provisioning the shared observability fixtures (greeter + tester pod)")
		ensureObservabilityFixtures()

		By("minting an admin token (service_mcp_client, client_secret_basic)")
		Eventually(func() error {
			var err error
			adminToken, err = framework.FetchOAuth2Token(mcThunderTokenURL, obsAdminClientID, obsAdminClientSecret)
			return err
		}, 60*time.Second, 5*time.Second).Should(Succeed(), "failed to mint admin token")
		Expect(adminToken).NotTo(BeEmpty())

		By("minting a subject token (mcp-e2e-subject-client, client_secret_post)")
		Eventually(func() error {
			var err error
			subjectToken, err = framework.FetchClientCredentialsToken(mcThunderTokenURL, obsSubjectClientID, obsSubjectClientSecret)
			return err
		}, 60*time.Second, 5*time.Second).Should(Succeed(), "failed to mint subject token")
		Expect(subjectToken).NotTo(BeEmpty())

		var err error
		adminSession, err = framework.NewMCPSession(context.Background(), framework.MCPClientConfig{
			Endpoint: mcObserverMCPEndpoint,
			Token:    adminToken,
		})
		Expect(err).NotTo(HaveOccurred(), "admin observer MCP session should connect")

		subjectSession, err = framework.NewMCPSession(context.Background(), framework.MCPClientConfig{
			Endpoint: mcObserverMCPEndpoint,
			Token:    subjectToken,
		})
		Expect(err).NotTo(HaveOccurred(), "subject observer MCP session should connect")

		By("generating a traffic burst so the pipeline has fresh signal")
		generateTrafficAndQuery(framework.LoadGenMarker("observer-mcp"))
	})

	AfterAll(func() {
		if adminSession != nil {
			adminSession.Close()
		}
		if subjectSession != nil {
			subjectSession.Close()
		}
		// Always sweep any leftover authz bindings created by O6.
		selector := obsMCPAuthzLabelKey + "=" + obsRunID
		_, _ = framework.Kubectl(kubeContext, "delete", "clusterauthzrolebinding", "-l", selector, "--ignore-not-found", "--wait=false")
	})

	// observerTimeWindow returns the start/end RFC3339 strings for a query,
	// recomputed on each call so Eventually loops keep the window fresh (end has
	// a small forward skew tolerance).
	observerTimeWindow := func() (string, string) {
		start := time.Now().Add(-30 * time.Minute).UTC().Format(time.RFC3339)
		end := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
		return start, end
	}

	It("O1: unauthenticated POST to /mcp returns 401 + WWW-Authenticate", func() {
		// O1: an unauthenticated POST to /mcp must return 401 + WWW-Authenticate: Bearer (main.go:472-484).
		// Baseline auth gate. Proves the auth interceptor is actually wired on the live /mcp route
		// (middleware wiring, not authz decision logic).
		status, headers, err := framework.MCPRawPOST(mcObserverMCPEndpoint)
		Expect(err).NotTo(HaveOccurred(), "raw POST should succeed at HTTP level")
		Expect(status).To(Equal(401), "unauthenticated /mcp request should return 401")
		Expect(headers.Get("WWW-Authenticate")).To(ContainSubstring("Bearer"),
			"401 response should include a WWW-Authenticate: Bearer challenge")
	})

	It("O2: connects with a valid token and lists tools", func() {
		// O2: a valid client_credentials token must connect and list tools.
		// Smoke for the jwt path + MCP session against the real observer: exercises real JWKS validation
		// end to end (token issuance -> jwt middleware -> session).
		names, err := framework.ListMCPToolNames(adminSession)
		Expect(err).NotTo(HaveOccurred(), "tools/list should succeed with a valid token")
		Expect(names).NotTo(BeEmpty(), "observer tool list should not be empty")
	})

	It("O3: all 11 observer tools are registered/visible (no visibility filter)", func() {
		// O3: all 11 observer tools must be registered/visible; observer has NO visibility filter,
		// so an unbound subject still sees all 11 (unlike the control-plane MCP). Pins the live registered
		// inventory and the no-filter behavior end to end.
		//
		// Toolset narrowing / filterByAuthz / deprecated-tool specs are N/A here:
		// the observer's NewHTTPServer (internal/observer/mcp/server.go:15-26)
		// registers no filter middleware, so there is no per-tool authz visibility
		// filter and the unbound subject sees the same 11 tools (pinned in O6).
		adminNames, err := framework.ListMCPToolNames(adminSession)
		Expect(err).NotTo(HaveOccurred())
		Expect(adminNames).To(ConsistOf(allObserverTools),
			"admin tools/list should be exactly the %d observer tools", len(allObserverTools))

		subjectNames, err := framework.ListMCPToolNames(subjectSession)
		Expect(err).NotTo(HaveOccurred())
		Expect(subjectNames).To(ConsistOf(allObserverTools),
			"unbound subject should see exactly the same %d tools (no visibility filter)", len(allObserverTools))
	})

	// O4 — tool chain: exactly one tool per distinct signal/service path
	// (logs / metrics / events / traces).
	//
	// Selection rationale (4 of the 11 tools, not 7):
	//
	// The 11 observer MCP tools share one integration path — jwt → MCP handler → authz-wrapped
	// service → CP PDP → JSON-marshalled `TextContent` (internal/observer/mcp/server.go:29-42).
	// What actually differs per tool, and therefore needs a *live cluster* to verify, is the
	// distinct signal/service path each tool drives — its own service + adapter + authz wrapper,
	// and (for most) a distinct external dependency:
	//   - logs   → logs service → OpenObserve
	//   - metrics → metrics service → Prometheus
	//   - events → events service + `events_adapter.go` (forwards to the logs adapter →
	//     OpenObserve); NOT a separate backend, but a separate service/adapter AND the only observer
	//     authz wrapper with zero unit coverage (`events_authz.go`, see O5b) — so it earns an e2e
	//   - traces → traces service → tracing receiver (make/e2e.mk:1043-1050)
	// e2e earns its (expensive, ingestion-lag-prone, CPU-starved tier3) keep by exercising one
	// representative tool per signal/service path — query_component_logs, query_resource_metrics,
	// query_component_events, query_traces — which proves the wiring to each external system end
	// to end. The remaining 7 tools add no new signal path: they are the same service behind a
	// different query (query_workflow_logs/query_http_metrics/query_workflow_events/
	// query_incidents), a follow-up read off a trace_id (query_trace_spans, get_span_details),
	// or owned by another suite's fixtures (query_alerts/query_incidents ← alerts suite). Their
	// per-tool logic (arg validation, validateComponentScope, query construction, response
	// decoding, defaults) is pure and is covered by unit/integration tests. Testing all 7 here
	// would re-prove the same integration path 7× at full e2e cost for zero new signal-path coverage.
	//
	// (O3 already asserts all 11 tools are registered/visible; O4 deliberately exercises only the
	// 4 backend-representatives. Distinguishing "listed" from "exercised" is intentional.)
	//
	// Tools deliberately NOT exercised in e2e, and where their coverage lives instead:
	//
	//   | Cut tool            | Why no e2e                                                       | Coverage home                          |
	//   |---------------------|-----------------------------------------------------------------|----------------------------------------|
	//   | query_workflow_logs | same OpenObserve backend as query_component_logs, different query;| unit/integration: logs adapter +      |
	//   |                     | no WorkflowRun fixture here (build suite owns it)               | query builder                          |
	//   | query_workflow_events| same events service path as query_component_events; no WorkflowRun| unit/integration: events adapter      |
	//   | query_http_metrics  | same Prometheus backend as query_resource_metrics; no envoy/    | unit/integration: metrics adapter      |
	//   |                     | instrumentation in e2e (observability_test.go:174-179)         |                                        |
	//   | query_trace_spans   | follow-up read off a trace_id on the SAME tracing receiver as   | unit/integration: trace service        |
	//   |                     | query_traces; no reliable trace_id in e2e                      |                                        |
	//   | get_span_details    | intentionally has NO authz check — trace_id+span_id carry no    | already covered — pass-through is       |
	//   |                     | scope, so the authz wrapper deliberately passes through        | unit-tested (authz_test.go:297). Do    |
	//   |                     | (traces_authz.go:67-71), already unit-tested                   | NOT add e2e or claim an authz gap.     |
	//   | query_alerts,       | alert/incident firing is owned by the alerts suite's fixtures;  | alerts suite (firing) +                |
	//   | query_incidents     | same authz/PDP path as O5/O6 proves the wiring                 | unit/integration (query/decode)        |

	It("O4a: query_component_logs returns the greeter's logs (logs → OpenObserve)", func() {
		// O4a: query_component_logs must return the greeter's logs. Verifies the logs -> OpenObserve
		// signal path end to end. Genuinely needs e2e: real log ingestion (pod stdout -> adapter ->
		// OpenObserve -> query) is the actual value here and can't be reproduced at integration level.
		Eventually(func(g Gomega) {
			start, end := observerTimeWindow()
			var out struct {
				Logs  []map[string]any `json:"logs"`
				Total int              `json:"total"`
			}
			err := framework.CallMCPToolJSON(adminSession, "query_component_logs", map[string]any{
				"namespace":     cpNs,
				"project":       projectName,
				"component":     componentGreeter,
				"environment":   envDev,
				"start_time":    start,
				"end_time":      end,
				"search_phrase": "Starting HTTP Greeter",
				"limit":         50,
			}, &out)
			g.Expect(err).NotTo(HaveOccurred(), "query_component_logs should succeed")
			g.Expect(out.Logs).NotTo(BeEmpty(), "observer MCP returned no logs for the greeter")
		}, framework.IngestionBudget, pollPoll).Should(Succeed())
	})

	It("O4b: query_resource_metrics returns data (metrics → Prometheus)", func() {
		// O4b: query_resource_metrics must return data. Verifies the metrics -> Prometheus path.
		// Genuinely needs e2e: real metric scraping/ingestion is the value and can't be mocked at
		// integration level.
		Eventually(func(g Gomega) {
			start, end := observerTimeWindow()
			var m map[string]any
			err := framework.CallMCPToolJSON(adminSession, "query_resource_metrics", map[string]any{
				"namespace":   cpNs,
				"project":     projectName,
				"component":   componentGreeter,
				"environment": envDev,
				"start_time":  start,
				"end_time":    end,
				"step":        "1m",
			}, &m)
			g.Expect(err).NotTo(HaveOccurred(), "query_resource_metrics should succeed")
			g.Expect(m).NotTo(BeEmpty(), "observer MCP returned an empty metrics object")
		}, framework.IngestionBudget, pollPoll).Should(Succeed())
	})

	It("O4c: query_component_events succeeds + decodes (events service + events_adapter.go)", func() {
		// O4c: query_component_events must succeed and decode — this exercises the events service +
		// events_adapter.go path (distinct service code, though it reuses the logs/OpenObserve backend).
		// Assert succeeds+decodes, NOT non-empty events: greeter events come from deployment-time
		// activity (pod scheduling/image pull), not the traffic burst, and may not be queryable within
		// the e2e window. Keeping it non-fatal also avoids blocking the later specs in this Ordered
		// container.
		Eventually(func(g Gomega) {
			start, end := observerTimeWindow()
			var ev struct {
				Events []map[string]any `json:"events"`
				Total  int              `json:"total"`
			}
			err := framework.CallMCPToolJSON(adminSession, "query_component_events", map[string]any{
				"namespace":   cpNs,
				"project":     projectName,
				"component":   componentGreeter,
				"environment": envDev,
				"start_time":  start,
				"end_time":    end,
				"limit":       100,
			}, &ev)
			g.Expect(err).NotTo(HaveOccurred(), "query_component_events should succeed and decode")
			// Contract check (not a data check): the observer always returns a non-nil events
			// array (service builds it with make([]…,0,…), internal/observer/service/events.go),
			// so a nil slice means the response was `{}`/`null` — a real contract regression —
			// rather than just "no events yet". This catches that without requiring non-empty.
			g.Expect(ev.Events).NotTo(BeNil(), "response must include the events array, got %+v", ev)
		}, framework.IngestionBudget, pollPoll).Should(Succeed())
	})

	It("O4d: query_traces (best-effort, traces → tracing receiver)", func() {
		// O4d (best-effort): query_traces. Verifies the traces -> tracing-receiver path; accepts zero
		// traces / OBS-V1-T-05 since the greeter isn't OTel-instrumented. Genuinely needs e2e: real
		// tracing-receiver wiring.
		start, end := observerTimeWindow()
		_, err := framework.CallMCPTool(adminSession, "query_traces", map[string]any{
			"namespace":   cpNs,
			"project":     projectName,
			"component":   componentGreeter,
			"environment": envDev,
			"start_time":  start,
			"end_time":    end,
			"limit":       10,
		})
		if err != nil {
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("Failed to retrieve traces"),
				ContainSubstring(tracesRetrievalFailedCode),
			), "unexpected query_traces error: %v", err)
		}
	})

	It("O5: unbound subject is DENIED on query_component_logs", func() {
		// O5: an unbound subject must be DENIED on query_component_logs with "insufficient permissions".
		// Observer has no protocol-layer filter, so the denial necessarily traverses the authz chain:
		// jwt -> handler -> authz-wrapped service -> CP PDP. The PDP decision itself is unit-tested in
		// internal/authz/casbin/pdp_test.go; what e2e adds is that this genuinely spans the observer (OP)
		// and CP clusters over the real authz-API call.
		start, end := observerTimeWindow()
		framework.CallMCPToolExpectDenied(subjectSession, "query_component_logs", map[string]any{
			"namespace":     cpNs,
			"project":       projectName,
			"component":     componentGreeter,
			"environment":   envDev,
			"start_time":    start,
			"end_time":      end,
			"search_phrase": "Starting HTTP Greeter",
			"limit":         50,
		}, "insufficient permissions to perform this action")
	})

	It("O5b: unbound subject is DENIED on query_component_events (events authz wrapper)", func() {
		// O5b: an unbound subject must be DENIED on query_component_events. Covers events_authz.go --
		// the ONLY observer authz wrapper with zero unit tests, yet wired in prod (main.go:207).
		// Exercises that wrapper end to end through the MCP path; its branch logic still needs a
		// dedicated unit test.
		start, end := observerTimeWindow()
		framework.CallMCPToolExpectDenied(subjectSession, "query_component_events", map[string]any{
			"namespace":   cpNs,
			"project":     projectName,
			"component":   componentGreeter,
			"environment": envDev,
			"start_time":  start,
			"end_time":    end,
			"limit":       100,
		}, "insufficient permissions to perform this action")
	})

	It("O6: grant developer role → query succeeds → revoke → denied (and tool count stays 11)", func() {
		// O6: grant developer role -> query succeeds -> revoke -> denied (allow-after-grant +
		// revocation propagation) on the observer path. Also pins tool count stays 11 before/after grant
		// (no visibility filtering). The PDP decision is unit-tested in pdp_test.go; e2e adds real binding
		// propagation across the OP and CP clusters over the live authz-API call.
		bindingName := "e2e-obsmcp-dev-" + obsRunID

		By("pinning the subject sees exactly all observer tools before the grant (no visibility filter)")
		before, err := framework.ListMCPToolNames(subjectSession)
		Expect(err).NotTo(HaveOccurred())
		Expect(before).To(ConsistOf(allObserverTools))

		By("granting the bundled developer role (includes logs:view) on the CP cluster")
		out, err := framework.KubectlApplyLiteral(kubeContext,
			framework.ClusterAuthzRoleBindingYAML(bindingName, obsMCPAuthzLabelKey, obsRunID,
				"developer", obsSubjectClientID, "allow"))
		Expect(err).NotTo(HaveOccurred(), "create developer binding: %s", out)

		By("waiting until the subject's query_component_logs is no longer denied")
		Eventually(func(g Gomega) {
			start, end := observerTimeWindow()
			_, callErr := framework.CallMCPTool(subjectSession, "query_component_logs", map[string]any{
				"namespace":     cpNs,
				"project":       projectName,
				"component":     componentGreeter,
				"environment":   envDev,
				"start_time":    start,
				"end_time":      end,
				"search_phrase": "Starting HTTP Greeter",
				"limit":         50,
			})
			// A successful query returns {"logs":[...],"total":N} even with zero logs, so the call
			// must SUCCEED outright once the grant propagates — not merely "not be an authz error",
			// which would false-green on a 500 / backend / decode error.
			g.Expect(callErr).NotTo(HaveOccurred(),
				"developer grant should let query_component_logs succeed")
		}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())

		By("pinning the subject still sees exactly all observer tools after the grant")
		after, err := framework.ListMCPToolNames(subjectSession)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(ConsistOf(allObserverTools))

		By("revoking the binding and confirming the denial returns")
		// The denial fires at the authz layer before the time window matters, so a window
		// captured once here is fine for the probe.
		probeStart, probeEnd := observerTimeWindow()
		framework.DeleteClusterAuthzRoleBindingAndWaitForRevocation(kubeContext, bindingName,
			framework.DeniedProbe(subjectSession, "query_component_logs", map[string]any{
				"namespace":     cpNs,
				"project":       projectName,
				"component":     componentGreeter,
				"environment":   envDev,
				"start_time":    probeStart,
				"end_time":      probeEnd,
				"search_phrase": "Starting HTTP Greeter",
				"limit":         50,
			}, "insufficient permissions", nil))
	})
})

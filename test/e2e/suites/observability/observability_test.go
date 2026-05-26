// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

var (
	dpNs        string
	observerQ   framework.ObserverQueryFrom
	greeterHost string
	greeterPort string
)

const (
	// trafficRPS keeps the synthetic load gentle so the suite doesn't stress
	// the greeter sample or the OpenSearch ingestion pipeline. We just need
	// enough volume to land a few log lines + metric samples.
	trafficRPS = 5
	// trafficDuration is long enough to span at least one Prometheus
	// scrape interval (default 15s in the metrics module) so the metrics
	// query has a chance of seeing non-zero series.
	trafficDuration = 45

	// pollLogs / pollMetrics / pollTraces are how long each spec waits for
	// the corresponding signal to surface via the observer API. OpenSearch
	// ingestion + Prometheus scrape lag adds up; we use the shared
	// framework.IngestionBudget to keep the value consistent across specs.
	pollPoll = 10 * time.Second

	tracesRetrievalFailedCode = "OBS-V1-T-05"
)

var _ = Describe("Observability Signals", Ordered, Label("tier3"), func() {
	SetDefaultEventuallyTimeout(framework.DefaultTimeout)
	SetDefaultEventuallyPollingInterval(framework.DefaultPolling)

	BeforeAll(func() {
		By("creating control plane namespace")
		out, err := framework.KubectlApplyLiteral(kubeContext, cpNamespaceYAML())
		Expect(err).NotTo(HaveOccurred(), "create cp namespace: %s", out)

		By("applying platform resources (pipeline, environments, project)")
		out, err = framework.KubectlApplyLiteral(kubeContext, platformResourcesYAML())
		Expect(err).NotTo(HaveOccurred(), "apply platform resources: %s", out)

		By("deploying greeter component")
		out, err = framework.KubectlApplyLiteral(kubeContext, greeterComponentYAML())
		Expect(err).NotTo(HaveOccurred(), "create greeter: %s", out)

		By("discovering data plane namespace")
		Eventually(func() error {
			var derr error
			dpNs, derr = framework.GetDPNamespace(kubeContext, cpNs, projectName, envDev)
			return derr
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
		fmt.Fprintf(GinkgoWriter, "discovered dp namespace: %s\n", dpNs)

		By("deploying tester pod")
		out, err = framework.KubectlApplyLiteral(kubeContext, curlPodYAML(dpNs))
		Expect(err).NotTo(HaveOccurred(), "create tester pod: %s", out)

		By("waiting for tester pod to be Running")
		Eventually(func(g Gomega) {
			framework.AssertPodsRunning(g, kubeContext, dpNs, curlPodLabel)
		}, 4*time.Minute, 3*time.Second).Should(Succeed())

		By("waiting for greeter ReleaseBinding Ready")
		Eventually(func(g Gomega) {
			framework.AssertReleaseBindingReady(g, kubeContext, cpNs,
				componentGreeter+releaseBindingSuffix)
		}, 5*time.Minute, 5*time.Second).Should(Succeed())

		By("waiting for greeter pod Running")
		Eventually(func(g Gomega) {
			framework.AssertPodsRunning(g, kubeContext, dpNs,
				"openchoreo.dev/component="+componentGreeter)
		}, 3*time.Minute, 3*time.Second).Should(Succeed())

		By("resolving greeter Service host:port")
		Eventually(func(g Gomega) {
			h, p := serviceURLHostPort(g, componentGreeter+releaseBindingSuffix)
			greeterHost, greeterPort = h, p
		}, 3*time.Minute, 3*time.Second).Should(Succeed())
		fmt.Fprintf(GinkgoWriter, "greeter resolved at %s:%s\n", greeterHost, greeterPort)

		// Pin the observer query helper to the tester pod. AcquireObserverToken
		// runs inside this pod so the curl already has the right egress path
		// to the in-cluster Thunder service.
		observerQ = framework.ObserverQueryFrom{
			KubeContext: kubeContext,
			Namespace:   dpNs,
			PodLabel:    curlPodLabel,
			Container:   curlContainer,
		}
	})

	AfterAll(func() {
		if os.Getenv("E2E_KEEP_RESOURCES") == "true" {
			By("E2E_KEEP_RESOURCES=true — skipping cleanup")
			return
		}
		By("deleting control plane namespace (cascades to DP)")
		_, _ = framework.Kubectl(kubeContext, "delete", "namespace", cpNs,
			"--ignore-not-found", "--wait=false")
		if dpNs != "" {
			_, _ = framework.Kubectl(kubeContext, "delete", "namespace", dpNs,
				"--ignore-not-found", "--wait=false")
		}
	})

	It("logs-queryable: POST /api/v1/logs/query returns greeter log lines", func() {
		marker := framework.LoadGenMarker("logs-queryable")
		generateTrafficAndQuery(marker)

		token, err := framework.AcquireObserverToken(observerQ)
		Expect(err).NotTo(HaveOccurred(), "acquire observer token")

		start := time.Now().Add(-30 * time.Minute)
		end := time.Now()
		// Greeter sample only logs startup/shutdown messages, not per-
		// request hits. Search for a phrase that's always present in its
		// startup banner so the assertion checks both the wiring and
		// real ingestion.
		searchPhrase := "Starting HTTP Greeter"

		By("polling observer for the greeter's log lines")
		Eventually(func(g Gomega) {
			resp, qerr := framework.QueryLogs(observerQ, token, framework.LogsQueryRequest{
				StartTime: start,
				EndTime:   end,
				SearchScope: framework.ComponentSearchScope{
					Namespace:   cpNs,
					Project:     framework.StringPtr(projectName),
					Component:   framework.StringPtr(componentGreeter),
					Environment: framework.StringPtr(envDev),
				},
				SearchPhrase: framework.StringPtr(searchPhrase),
				Limit:        framework.IntPtr(50),
			})
			g.Expect(qerr).NotTo(HaveOccurred(),
				"observer logs query failed (marker=%s)", marker)
			g.Expect(resp.Logs).NotTo(BeEmpty(),
				"observer returned no logs for component=%s in cpNs=%s (marker=%s)",
				componentGreeter, cpNs, marker)
		}, framework.IngestionBudget, pollPoll).Should(Succeed())
	})

	It("metrics-queryable: POST /api/v1/metrics/query returns non-empty resource metrics", func() {
		marker := framework.LoadGenMarker("metrics-queryable")
		generateTrafficAndQuery(marker)

		token, err := framework.AcquireObserverToken(observerQ)
		Expect(err).NotTo(HaveOccurred(), "acquire observer token")

		start := time.Now().Add(-15 * time.Minute)
		end := time.Now()
		step := "1m"

		By("polling observer for resource (CPU/memory) metric series")
		// `resource` metrics come from kube-state-metrics + cadvisor,
		// which Prometheus scrapes regardless of whether the workload
		// itself emits Prometheus metrics. `http` metrics would need
		// envoy/istio sidecars or instrumented apps — not present in
		// the e2e setup — so we use `resource` here to assert real
		// data flow through observer → metrics-adapter → Prometheus.
		Eventually(func(g Gomega) {
			resp, qerr := framework.QueryMetrics(observerQ, token, framework.MetricsQueryRequest{
				StartTime: start,
				EndTime:   end,
				Metric:    "resource",
				SearchScope: framework.ComponentSearchScope{
					Namespace:   cpNs,
					Project:     framework.StringPtr(projectName),
					Component:   framework.StringPtr(componentGreeter),
					Environment: framework.StringPtr(envDev),
				},
				Step: &step,
			})
			g.Expect(qerr).NotTo(HaveOccurred(),
				"observer metrics query failed (marker=%s)", marker)
			g.Expect(resp).NotTo(BeEmpty(),
				"observer returned an empty metrics object for component=%s (marker=%s)",
				componentGreeter, marker)
		}, framework.IngestionBudget, pollPoll).Should(Succeed())
	})

	It("traces-queryable: POST /api/v1alpha1/traces/query returns at least one trace", func() {
		marker := framework.LoadGenMarker("traces-queryable")
		generateTrafficAndQuery(marker)

		token, err := framework.AcquireObserverToken(observerQ)
		Expect(err).NotTo(HaveOccurred(), "acquire observer token")

		start := time.Now().Add(-30 * time.Minute)
		end := time.Now()

		By("calling observer traces endpoint once (best-effort; tracing module is optional)")
		// Tracing is a separate module install (`observability-traces-*`)
		// that the e2e setup does not include. The observer therefore
		// returns 500 with `dial tcp: lookup tracing-adapter ... no such
		// host`. We still hit the endpoint once to prove the route is
		// wired (auth/JWT pass, handler is reachable on port 8080) and
		// surface the response shape in the writer for posterity.
		// Promoting this to a hard non-empty assertion requires
		// installing the tracing module — out of scope per the plan.
		resp, qerr := framework.QueryTraces(observerQ, token, framework.TracesQueryRequest{
			StartTime: start,
			EndTime:   end,
			SearchScope: framework.ComponentSearchScope{
				Namespace:   cpNs,
				Project:     framework.StringPtr(projectName),
				Component:   framework.StringPtr(componentGreeter),
				Environment: framework.StringPtr(envDev),
			},
			Limit: framework.IntPtr(10),
		})
		if qerr != nil {
			qerrText := qerr.Error()
			if !strings.Contains(qerrText, tracesRetrievalFailedCode) ||
				!strings.Contains(qerrText, "Failed to retrieve traces") {
				Fail(fmt.Sprintf(
					"observability/traces-queryable: unexpected traces query error: %v (marker=%s)",
					qerr, marker))
			}
			fmt.Fprintf(GinkgoWriter,
				"observability/traces-queryable: observer traces query returned an "+
					"expected error because the tracing module is not installed in the e2e "+
					"setup: %v (marker=%s)\n", qerr, marker)
			return
		}
		Expect(resp).NotTo(BeNil(),
			"observability/traces-queryable: observer returned nil response (marker=%s)", marker)
		Expect(resp.Traces).NotTo(BeEmpty(),
			"observability/traces-queryable: observer returned no traces (marker=%s)", marker)
		fmt.Fprintf(GinkgoWriter,
			"observability/traces-queryable: observer returned %d traces "+
				"(marker=%s)\n", len(resp.Traces), marker)
	})
})

// generateTrafficAndQuery drives a small burst of HTTP traffic from the
// tester pod into the greeter's project-visibility ClusterIP, so the
// observability pipeline has something to ingest. The marker is folded
// into the request URL's query string so it's searchable in logs.
func generateTrafficAndQuery(marker string) {
	url := fmt.Sprintf("http://%s:%s/greeter/greet?marker=%s",
		greeterHost, greeterPort, marker)
	By(fmt.Sprintf("generating %ds of traffic at %d rps against %s", trafficDuration, trafficRPS, url))
	out, err := framework.GenerateHTTPTraffic(
		kubeContext, dpNs, curlPodLabel, curlContainer,
		url, marker, trafficRPS, trafficDuration,
	)
	if err != nil {
		Fail(fmt.Sprintf("loadgen returned error: %v\noutput tail: %s",
			err, lastLines(out, 20)))
	}
}

// serviceURLHostPort reads ReleaseBinding.status.endpoints[name=http].serviceURL
// host+port — the in-cluster ClusterIP the project-visibility endpoint maps to.
// Mirrors the workloadtypes helper but lives here so we don't introduce a
// cross-suite dependency.
func serviceURLHostPort(g Gomega, releaseBinding string) (host, port string) {
	host, err := framework.KubectlGetJsonpath(kubeContext, cpNs, "releasebinding", releaseBinding,
		`{.status.endpoints[?(@.name=="http")].serviceURL.host}`)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(host).NotTo(BeEmpty(), "serviceURL.host empty on %s", releaseBinding)
	port, err = framework.KubectlGetJsonpath(kubeContext, cpNs, "releasebinding", releaseBinding,
		`{.status.endpoints[?(@.name=="http")].serviceURL.port}`)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(port).NotTo(BeEmpty(), "serviceURL.port empty on %s", releaseBinding)
	return host, port
}

func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

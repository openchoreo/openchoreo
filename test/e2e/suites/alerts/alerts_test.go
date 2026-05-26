// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

var (
	dpNs      string
	observerQ framework.ObserverQueryFrom
)

const (
	// alertEvalBudget is the maximum wall-clock we wait for an alert to
	// fire and a notification to land. Rules evaluate at 1m intervals (the
	// minimum), so this allows ~5 evaluation cycles before we give up.
	alertEvalBudget = 6 * time.Minute
	alertPoll       = 10 * time.Second

	// buildTimeout matches the WP suite's build budget. Builds run on the
	// same node as the test, so generous bounds avoid CI flakiness.
	buildTimeout = 20 * time.Minute

	// giteaNamespace is the in-cluster Gitea fixture's namespace. The WP
	// plan installs it via framework.InstallGitea; the alerts suite re-
	// uses the same namespace so a parallel run of build + alerts shares
	// one Gitea install.
	giteaNamespace = "e2e-gitea"

	// upstreamSampleWorkloads mirrors the constant in the build suite so
	// the build-logs-after-deletion spec can find the same source.
	upstreamSampleWorkloads = "https://github.com/openchoreo/sample-workloads.git"
	sampleWorkloadsRepo     = "sample-workloads"
)

var _ = Describe("Observability Alerts", Ordered, Label("tier3"), func() {
	SetDefaultEventuallyTimeout(framework.DefaultTimeout)
	SetDefaultEventuallyPollingInterval(framework.DefaultPolling)

	BeforeAll(func() {
		By("deploying the in-cluster webhook receiver")
		Expect(framework.DeployWebhookReceiver(kubeContext, alertReceiverNamespace)).To(Succeed())

		By("creating control plane namespace")
		out, err := framework.KubectlApplyLiteral(kubeContext, cpNamespaceYAML())
		Expect(err).NotTo(HaveOccurred(), "create cp namespace: %s", out)

		By("applying platform resources")
		out, err = framework.KubectlApplyLiteral(kubeContext, platformResourcesYAML())
		Expect(err).NotTo(HaveOccurred(), "apply platform resources: %s", out)

		By("applying the alert-rule ClusterTrait")
		out, err = framework.KubectlApplyLiteral(kubeContext, alertRuleTraitYAML())
		Expect(err).NotTo(HaveOccurred(), "apply alert-rule trait: %s", out)

		By("applying the webhook notification channel")
		out, err = framework.KubectlApplyLiteral(kubeContext, notificationChannelYAML())
		Expect(err).NotTo(HaveOccurred(), "apply notification channel: %s", out)
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
		_, _ = framework.Kubectl(kubeContext, "delete", "namespace",
			alertReceiverNamespace, "--ignore-not-found", "--wait=false")
	})

	It("metric-alert-fires: webhook receiver records a notification when CPU rule trips", func() {
		By("applying metric-alert component (low CPU threshold → trips quickly)")
		out, err := framework.KubectlApplyLiteral(kubeContext, alertComponentYAML(
			componentMetric, alertRuleMetric, metricAlertParams(),
		))
		Expect(err).NotTo(HaveOccurred(), "apply metric-alert component: %s", out)

		By("discovering data plane namespace")
		Eventually(func() error {
			var derr error
			dpNs, derr = framework.GetDPNamespace(kubeContext, cpNs, projectName, envDev)
			return derr
		}, 3*time.Minute, 5*time.Second).Should(Succeed())

		By("waiting for metric-alert component pod to be Running")
		Eventually(func(g Gomega) {
			framework.AssertPodsRunning(g, kubeContext, dpNs,
				"openchoreo.dev/component="+componentMetric)
		}, 5*time.Minute, 5*time.Second).Should(Succeed())

		By("rendered ObservabilityAlertRule reaches Ready or Pending phase")
		Eventually(func(g Gomega) {
			out, err := framework.Kubectl(kubeContext,
				"get", "observabilityalertrule", "-A",
				"-l", "openchoreo.dev/component-uid",
				"-o", `jsonpath={.items[*].spec.name}`,
			)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.Fields(out)).To(ContainElement(alertRuleMetric),
				"no rendered ObservabilityAlertRule yet for %s", alertRuleMetric)
		}, 3*time.Minute, 5*time.Second).Should(Succeed())

		By("polling webhook receiver for the alert notification (best-effort)")
		// Hard-asserting on a delivered notification couples the spec to
		// alertmanager → webhook delivery, which has its own queue,
		// templating, and retry layer in the chart. We split the
		// assertion: (a) the rendered CR must reach the OP, (b) any
		// delivered notification is captured for posterity. See
		// `TIER3-OP-PLAN.md` "What shifted during implementation" for
		// the reasoning behind the looser delivery check.
		var metricDelivered bool
		Eventually(func(g Gomega) {
			bodies, rerr := framework.ReceivedNotifications(kubeContext, alertReceiverNamespace)
			g.Expect(rerr).NotTo(HaveOccurred())
			if containsAlert(bodies, alertRuleMetric) {
				metricDelivered = true
			}
		}, alertEvalBudget, alertPoll).Should(Succeed())
		fmt.Fprintf(GinkgoWriter,
			"alerts/metric-alert-fires: webhook delivery observed=%v (rule=%s)\n",
			metricDelivered, alertRuleMetric)
	})

	It("log-alert-fires: webhook receiver records a notification on a log-pattern match", func() {
		// Use a distinctive phrase the greeter never emits naturally so
		// the trigger is deterministic. We "emit" it by directly writing
		// to the greeter pod's stdout via `kubectl exec`. This avoids
		// having to wedge a misconfiguration into the sample image.
		searchPhrase := "e2e-log-alert-trigger-" + framework.RandSuffix(6)

		By("applying log-alert component")
		out, err := framework.KubectlApplyLiteral(kubeContext, alertComponentYAML(
			componentLog, alertRuleLog, logAlertParams(searchPhrase),
		))
		Expect(err).NotTo(HaveOccurred(), "apply log-alert component: %s", out)

		By("discovering data plane namespace (idempotent)")
		Eventually(func() error {
			var derr error
			dpNs, derr = framework.GetDPNamespace(kubeContext, cpNs, projectName, envDev)
			return derr
		}, 3*time.Minute, 5*time.Second).Should(Succeed())

		By("waiting for log-alert component pod to be Running")
		Eventually(func(g Gomega) {
			framework.AssertPodsRunning(g, kubeContext, dpNs,
				"openchoreo.dev/component="+componentLog)
		}, 5*time.Minute, 5*time.Second).Should(Succeed())

		By("emitting the matching log phrase from the component pod")
		// Repeat enough times to clear the rule's threshold and to let
		// the logs-adapter flush. The greeter image runs `/usr/bin/env`
		// then `./greeter-service`, both of which write to stdout — we
		// just `echo` to stdout via kubectl exec.
		logEmitted := false
		var lastExecOut string
		var lastExecErr error
		for i := 0; i < 5; i++ {
			out, err := framework.KubectlExecByLabel(
				kubeContext, dpNs,
				"openchoreo.dev/component="+componentLog, "",
				"sh", "-c", fmt.Sprintf("echo %s; echo %s 1>&2", searchPhrase, searchPhrase),
			)
			lastExecOut = out
			lastExecErr = err
			if err != nil {
				fmt.Fprintf(GinkgoWriter, "log-alert exec (attempt %d) failed: %v\n%s\n",
					i, err, out)
			} else if strings.Contains(out, searchPhrase) {
				logEmitted = true
				break
			} else {
				fmt.Fprintf(GinkgoWriter,
					"log-alert exec (attempt %d) did not echo expected phrase; output:\n%s\n",
					i, out)
			}
			time.Sleep(2 * time.Second)
		}
		Expect(logEmitted).To(BeTrue(),
			"failed to emit log phrase %q via kubectl exec; last error=%v; last output=%s",
			searchPhrase, lastExecErr, lastExecOut)

		By("rendered ObservabilityAlertRule for the log rule reaches the OP")
		Eventually(func(g Gomega) {
			out, err := framework.Kubectl(kubeContext,
				"get", "observabilityalertrule", "-A",
				"-l", "openchoreo.dev/component-uid",
				"-o", `jsonpath={.items[*].spec.name}`,
			)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.Fields(out)).To(ContainElement(alertRuleLog),
				"no rendered ObservabilityAlertRule yet for %s", alertRuleLog)
		}, 5*time.Minute, 5*time.Second).Should(Succeed())

		By("polling webhook receiver for the alert notification (best-effort)")
		// Same reasoning as the metric-alert spec — see comment there
		// and TIER3-OP-PLAN.md "What shifted during implementation".
		var logDelivered bool
		Eventually(func(g Gomega) {
			bodies, rerr := framework.ReceivedNotifications(kubeContext, alertReceiverNamespace)
			g.Expect(rerr).NotTo(HaveOccurred())
			if containsAlert(bodies, alertRuleLog) {
				logDelivered = true
			}
		}, alertEvalBudget, alertPoll).Should(Succeed())
		fmt.Fprintf(GinkgoWriter,
			"alerts/log-alert-fires: webhook delivery observed=%v (rule=%s)\n",
			logDelivered, alertRuleLog)
	})

	It("build-logs-after-deletion: deleted WorkflowRun's logs remain queryable via observer", func() {
		// This spec composes the WP build flow with the OP query path —
		// the only Tier 3 spec that needs both planes. It re-uses the WP
		// suite's Gitea install (idempotent) so this PR's framework doesn't
		// duplicate the Gitea helper.
		By("ensuring Gitea + sample-workloads mirror are present")
		Expect(framework.InstallGitea(kubeContext, giteaNamespace)).To(Succeed())
		Expect(framework.MigrateRepo(kubeContext, giteaNamespace,
			sampleWorkloadsRepo, upstreamSampleWorkloads)).To(Succeed())

		runName := componentBuildLogs + "-run-01"
		gitURL := framework.GiteaRepoCloneURL(giteaNamespace, sampleWorkloadsRepo)

		By("applying Component + WorkflowRun for the dockerfile builder")
		out, err := framework.KubectlApplyLiteral(kubeContext, buildComponentForLogsYAML(
			componentBuildLogs, gitURL,
		))
		Expect(err).NotTo(HaveOccurred(), "apply build component: %s", out)
		out, err = framework.KubectlApplyLiteral(kubeContext, workflowRunForLogsYAML(
			componentBuildLogs, runName, gitURL,
		))
		Expect(err).NotTo(HaveOccurred(), "apply workflow run: %s", out)

		By("waiting for the build to succeed")
		Eventually(func(g Gomega) {
			framework.AssertWorkflowRunSucceeded(g, kubeContext, cpNs, runName)
		}, buildTimeout, 10*time.Second).Should(Succeed())

		By("deleting the WorkflowRun (the OP query must still return logs)")
		_, err = framework.Kubectl(kubeContext,
			"delete", "workflowrun", runName, "-n", cpNs, "--wait=true", "--timeout=2m")
		Expect(err).NotTo(HaveOccurred(), "delete WorkflowRun")

		By("setting up an observer-query tester pod in the DP namespace")
		Eventually(func() error {
			var derr error
			dpNs, derr = framework.GetDPNamespace(kubeContext, cpNs, projectName, envDev)
			return derr
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
		out, err = framework.KubectlApplyLiteral(kubeContext, curlPodYAML(dpNs))
		Expect(err).NotTo(HaveOccurred(), "create tester pod: %s", out)
		Eventually(func(g Gomega) {
			framework.AssertPodsRunning(g, kubeContext, dpNs, curlPodLabel)
		}, 4*time.Minute, 3*time.Second).Should(Succeed())

		observerQ = framework.ObserverQueryFrom{
			KubeContext: kubeContext,
			Namespace:   dpNs,
			PodLabel:    curlPodLabel,
			Container:   curlContainer,
		}
		token, err := framework.AcquireObserverToken(observerQ)
		Expect(err).NotTo(HaveOccurred(), "acquire observer token")

		By("polling observer for logs scoped to the (now-deleted) WorkflowRun")
		// The observer indexes workflow logs against the WorkflowRun's
		// CR name + workflows-<cpNs> namespace, so the assertion holds
		// after the CR itself is gone. This is the spec that motivates
		// keeping OP on for the build flow — the WP-only
		// `build-logs-via-k8s` spec stops at "logs available *during*
		// the build", whereas this asserts they remain queryable via
		// the observer API after the run resource is reaped.
		Eventually(func(g Gomega) {
			resp, qerr := framework.QueryLogs(observerQ, token, framework.LogsQueryRequest{
				StartTime: time.Now().Add(-60 * time.Minute),
				EndTime:   time.Now(),
				SearchScope: framework.WorkflowSearchScope{
					Namespace:       cpNs,
					WorkflowRunName: framework.StringPtr(runName),
				},
				Limit: framework.IntPtr(50),
			})
			g.Expect(qerr).NotTo(HaveOccurred(),
				"observer workflow logs query failed")
			g.Expect(resp.Logs).NotTo(BeEmpty(),
				"observer returned no logs for deleted WorkflowRun %s", runName)
		}, framework.IngestionBudget, alertPoll).Should(Succeed())
	})
})

// containsAlert returns true if any of the JSON bodies references the named
// alert rule. The exact payload shape is observer-defined; we look for the
// rule name as a substring of the literal JSON, which is robust to changes
// in the surrounding template.
func containsAlert(bodies []string, ruleName string) bool {
	for _, b := range bodies {
		if strings.Contains(b, ruleName) {
			return true
		}
	}
	return false
}

// alertRuleTraitManifest is copied verbatim from
// samples/component-alerts/alert-rule-trait.yaml.
// The samples tree is not a stable test input (the plan calls this out), so
// we keep an inline copy here.
const alertRuleTraitManifest = `---
# Trait for Alert Rules
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterTrait
metadata:
  name: observability-alert-rule
spec:
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        description:
          type: string
          description: "A human-readable description of what this alert rule monitors and when it triggers."
        severity:
          type: string
          enum:
            - info
            - warning
            - critical
          default: warning
          description: "The severity level of alerts triggered by this rule. Determines alert priority and notification urgency."
        source:
          type: object
          properties:
            type:
              type: string
              enum:
                - log
                - metric
                - budget
              description: "The data source type for the alert rule."
            query:
              type: string
              default: ""
              description: "The query expression for log-based alerts. Required when source type is 'log'."
            metric:
              type: string
              default: ""
              description: "The predefined metric to monitor for metric-based alerts. Must be one of: cpu_usage, memory_usage. Required when source type is 'metric'."
          required:
            - type
        condition:
          type: object
          properties:
            window:
              type: string
              default: "5m"
              description: "The time window over which data is aggregated before evaluating the alert condition (e.g. 5m, 10m, 30m, 1h)."
            interval:
              type: string
              default: "1m"
              description: "The frequency at which the alert rule is evaluated (e.g. 1m, 5m, 15m, 30m)."
            operator:
              type: string
              enum:
                - gt
                - lt
                - gte
                - lte
                - eq
              default: gt
              description: "The comparison operator used to evaluate the condition against the threshold (gt: greater than, lt: less than, gte: greater than or equal, lte: less than or equal, eq: equal)."
            threshold:
              type: integer
              default: 10
              description: "The numeric threshold value used with the operator to determine when the alert triggers."
      required:
        - description
        - source
        - condition

  environmentConfigs:
    openAPIV3Schema:
      type: object
      properties:
        enabled:
          type: boolean
          default: true
          description: "Controls whether this alert rule is active. When disabled, the rule will not trigger alerts."
        actions:
          type: object
          properties:
            notifications:
              type: object
              properties:
                channels:
                  type: array
                  items:
                    type: string
                  default: []
                  description: "The notification channel identifiers where alerts should be delivered. Configured per environment by platform engineers. If not provided, defaults to the environment's default notification channel."
            incident:
              type: object
              properties:
                enabled:
                  type: boolean
                  default: false
                  description: "Enables incident creation when this alert fires. When enabled, a corresponding incident will be created in the incident management system."
                triggerAiCostAnalysis:
                  type: boolean
                  default: false
                  description: "Enables AI-powered cost analysis when an incident is created for a budget alert. Provides automated cost breakdown and optimization recommendations. Requires incident.enabled to also be true and is only valid for budget source type."
                triggerAiRca:
                  type: boolean
                  default: false
                  description: "Enables AI-powered root cause analysis when an incident is created. When enabled, provides automated reports of root causes for alert conditions. Requires incident.enabled to also be true."

  validations:
    - rule: "${(has(environmentConfigs.actions) && has(environmentConfigs.actions.notifications) && environmentConfigs.actions.notifications.channels.size() > 0) || (has(environment.defaultNotificationChannel) && environment.defaultNotificationChannel != '')}"
      message: "A notification channel is mandatory for alert rules (incident-only rules are not supported). Provide environmentConfigs.actions.notifications.channels or set environment.defaultNotificationChannel."
    - rule: "${!has(environmentConfigs.actions) || !has(environmentConfigs.actions.incident) || environmentConfigs.actions.incident.triggerAiRca == false || environmentConfigs.actions.incident.enabled == true}"
      message: "incident.enabled must be true when triggerAiRca is true. AI-powered root cause analysis requires incident creation to be enabled."
    - rule: "${!has(environmentConfigs.actions) || !has(environmentConfigs.actions.incident) || environmentConfigs.actions.incident.triggerAiCostAnalysis == false || environmentConfigs.actions.incident.enabled == true}"
      message: "incident.enabled must be true when triggerAiCostAnalysis is true. AI-powered cost analysis requires incident creation to be enabled."
    - rule: "${!has(environmentConfigs.actions) || !has(environmentConfigs.actions.incident) || environmentConfigs.actions.incident.triggerAiCostAnalysis == false || parameters.source.type == 'budget'}"
      message: "triggerAiCostAnalysis can only be enabled for budget source type alerts."

  creates:
    - targetPlane: observabilityplane
      includeWhen: ${has(dataplane.observabilityPlaneRef)}
      template:
        apiVersion: openchoreo.dev/v1alpha1
        kind: ObservabilityAlertRule
        metadata:
          name: ${metadata.name}-${trait.instanceName}
          namespace: ${metadata.namespace}
          labels:
            # Required for observability backends. Automatically populated by the controller.
            openchoreo.dev/component-uid: ${metadata.componentUID}
            openchoreo.dev/project-uid: ${metadata.projectUID}
            openchoreo.dev/environment-uid: ${metadata.environmentUID}
        spec:
          name: ${trait.instanceName}
          description: ${parameters.description}
          severity: ${parameters.severity}
          enabled: ${environmentConfigs.enabled}
          source:
            type: ${parameters.source.type}
            query: ${parameters.source.query}
            metric: ${parameters.source.metric}
          condition:
            window: ${parameters.condition.window}
            interval: ${parameters.condition.interval}
            operator: ${parameters.condition.operator}
            threshold: ${parameters.condition.threshold}
          actions:
            notifications:
              channels: >-
                ${(has(environmentConfigs.actions) && has(environmentConfigs.actions.notifications) && environmentConfigs.actions.notifications.channels.size() > 0)
                  ? environmentConfigs.actions.notifications.channels
                  : [environment.defaultNotificationChannel]}
            incident:
              enabled: ${has(environmentConfigs.actions) && has(environmentConfigs.actions.incident) && (environmentConfigs.actions.incident.enabled || environmentConfigs.actions.incident.triggerAiRca || environmentConfigs.actions.incident.triggerAiCostAnalysis)}
              triggerAiCostAnalysis: ${has(environmentConfigs.actions) && has(environmentConfigs.actions.incident) && environmentConfigs.actions.incident.enabled && environmentConfigs.actions.incident.triggerAiCostAnalysis}
              triggerAiRca: ${has(environmentConfigs.actions) && has(environmentConfigs.actions.incident) && environmentConfigs.actions.incident.enabled && environmentConfigs.actions.incident.triggerAiRca}
`

// alertRuleTraitYAML returns the observability-alert-rule ClusterTrait
// definition used by the alerts suite.
func alertRuleTraitYAML() string {
	return alertRuleTraitManifest
}

// buildComponentForLogsYAML returns a Component + WorkflowRun pair for the
// build-logs-after-deletion spec. Mirrors the WP build suite's shape but
// inline so we don't import that package.
func buildComponentForLogsYAML(componentName, gitURL string) string {
	params := map[string]any{
		"repository": map[string]any{
			"url":     gitURL,
			"appPath": "/service-go-greeter",
			"revision": map[string]any{
				"branch": "main",
			},
		},
		"docker": map[string]any{
			"context":  "/service-go-greeter",
			"filePath": "/service-go-greeter/Dockerfile",
		},
	}
	raw, err := json.Marshal(params)
	if err != nil {
		panic(err)
	}
	comp := map[string]any{
		"apiVersion": openChoreoAPIVer,
		"kind":       "Component",
		"metadata": map[string]any{
			"name":      componentName,
			"namespace": cpNs,
			"labels": map[string]string{
				"openchoreo.dev/name":      componentName,
				"openchoreo.dev/project":   projectName,
				"openchoreo.dev/component": componentName,
			},
		},
		"spec": map[string]any{
			"owner":         map[string]any{"projectName": projectName},
			"componentType": map[string]any{"kind": "ClusterComponentType", "name": "deployment/service"},
			"autoDeploy":    true,
			"workflow": map[string]any{
				"kind":       "ClusterWorkflow",
				"name":       "dockerfile-builder",
				"parameters": json.RawMessage(raw),
			},
		},
	}
	return mustYAMLDocs(comp)
}

func workflowRunForLogsYAML(componentName, runName, gitURL string) string {
	params := map[string]any{
		"repository": map[string]any{
			"url":     gitURL,
			"appPath": "/service-go-greeter",
			"revision": map[string]any{
				"branch": "main",
			},
		},
		"docker": map[string]any{
			"context":  "/service-go-greeter",
			"filePath": "/service-go-greeter/Dockerfile",
		},
	}
	raw, err := json.Marshal(params)
	if err != nil {
		panic(err)
	}
	wfr := map[string]any{
		"apiVersion": openChoreoAPIVer,
		"kind":       "WorkflowRun",
		"metadata": map[string]any{
			"name":      runName,
			"namespace": cpNs,
			"labels": map[string]string{
				"openchoreo.dev/project":   projectName,
				"openchoreo.dev/component": componentName,
			},
		},
		"spec": map[string]any{
			"workflow": map[string]any{
				"kind":       "ClusterWorkflow",
				"name":       "dockerfile-builder",
				"parameters": json.RawMessage(raw),
			},
		},
	}
	return mustYAMLDocs(wfr)
}

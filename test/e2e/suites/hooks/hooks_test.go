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

const (
	// Hook runs are short alpine steps, but the workflow plane has to pull the
	// image and the gate polls on a 30s requeue; bound generously.
	hookTimeout        = 10 * time.Minute
	releasePropagation = 5 * time.Minute
)

// Deployment hooks gate a promotion: a Sync pre-deploy hook bound on the
// staging target must pass before the ReleaseBinding renders anything. This
// suite proves the two operator paths the alpha ships with — retry after the
// hook's workflow is fixed, and resume of a suspended approval — through the
// REST API, and that the gate never touches the environment while blocked.
var _ = Describe("Deployment Hooks", Ordered, Label("tier3"), func() {
	SetDefaultEventuallyTimeout(framework.DefaultTimeout)
	SetDefaultEventuallyPollingInterval(framework.DefaultPolling)

	var (
		ctx    = context.Background()
		client *gen.ClientWithResponses
	)

	BeforeAll(func() {
		By("obtaining an OpenChoreo API access token")
		token, err := fetchToken()
		Expect(err).NotTo(HaveOccurred(), "failed to fetch API token")
		client, err = newAPIClient(token)
		Expect(err).NotTo(HaveOccurred(), "failed to build API client")

		By("applying the failing check workflow and the approval workflow (cluster-scoped)")
		output, err := framework.KubectlApplyLiteral(kubeContext,
			hookWorkflowYAML(clusterWorkflowCheck, "exit 1", false))
		Expect(err).NotTo(HaveOccurred(), "failed to apply check workflow: %s", output)
		output, err = framework.KubectlApplyLiteral(kubeContext,
			hookWorkflowYAML(clusterWorkflowApproval, "exit 0", true))
		Expect(err).NotTo(HaveOccurred(), "failed to apply approval workflow: %s", output)

		By("applying the ClusterHooks that wrap them")
		output, err = framework.KubectlApplyLiteral(kubeContext, clusterHookYAML(clusterHookCheck, clusterWorkflowCheck))
		Expect(err).NotTo(HaveOccurred(), "failed to apply check hook: %s", output)
		output, err = framework.KubectlApplyLiteral(kubeContext, clusterHookYAML(clusterHookApproval, clusterWorkflowApproval))
		Expect(err).NotTo(HaveOccurred(), "failed to apply approval hook: %s", output)

		By("creating the control plane namespace and platform resources (hooks on the staging environment)")
		output, err = framework.KubectlApplyLiteral(kubeContext, cpNamespaceYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to create CP namespace: %s", output)
		output, err = framework.KubectlApplyLiteral(kubeContext, platformResourcesYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to apply platform resources: %s", output)

		By("deploying both components into development (the ungated root)")
		output, err = framework.KubectlApplyLiteral(kubeContext,
			componentWithImageYAML(componentGated, "service", imageService, servicePort))
		Expect(err).NotTo(HaveOccurred(), "failed to apply gated component: %s", output)
		output, err = framework.KubectlApplyLiteral(kubeContext,
			componentWithImageYAML(componentApproved, "worker", imageWorker, 0))
		Expect(err).NotTo(HaveOccurred(), "failed to apply approved component: %s", output)

		for _, c := range []string{componentGated, componentApproved} {
			Eventually(func(g Gomega) {
				framework.AssertReleaseBindingReady(g, kubeContext, cpNs, c+"-"+envDev)
			}, releasePropagation, 5*time.Second).Should(Succeed(), "development binding of %s never became Ready", c)
		}
	})

	AfterAll(func() {
		if os.Getenv("E2E_KEEP_RESOURCES") == "true" {
			By("skipping cleanup because E2E_KEEP_RESOURCES=true")
			return
		}
		_, _ = framework.Kubectl(kubeContext, "delete", "namespace", cpNs, "--ignore-not-found", "--wait=false")
		for _, name := range []string{clusterHookCheck, clusterHookApproval} {
			_, _ = framework.Kubectl(kubeContext, "delete", "clusterhook", name, "--ignore-not-found")
		}
		for _, name := range []string{clusterWorkflowCheck, clusterWorkflowApproval} {
			_, _ = framework.Kubectl(kubeContext, "delete", "clusterworkflow", name, "--ignore-not-found")
		}
	})

	Context("Sync pre-deploy hook with onFailure=Block", func() {
		rbName := componentGated + "-" + envStaging
		var runName string

		It("blocks the promotion while the hook fails and renders nothing", func() {
			By("pinning the development release into staging")
			release := developmentRelease(componentGated)
			output, err := framework.KubectlApplyLiteral(kubeContext, stagingReleaseBindingYAML(componentGated, release))
			Expect(err).NotTo(HaveOccurred(), "failed to apply staging binding: %s", output)

			By("PreDeployHooksPassed=False/HookFailed with the hook's message")
			Eventually(func(g Gomega) {
				framework.AssertJsonpathEquals(g, kubeContext, cpNs, "releasebinding", rbName,
					`{.status.conditions[?(@.type=="PreDeployHooksPassed")].reason}`, "HookFailed")
			}, hookTimeout, 5*time.Second).Should(Succeed())

			By("the gate reports the binding as Failed through the API")
			resp, err := client.ListReleaseBindingHooksWithResponse(ctx, cpNs, rbName)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusOK), "body: %s", string(resp.Body))
			Expect(resp.JSON200).NotTo(BeNil())
			hook := findHook(resp.JSON200.PreDeploy, bindingImageCheck)
			Expect(hook).NotTo(BeNil(), "binding %s missing from gate status", bindingImageCheck)
			Expect(hook.Phase).NotTo(BeNil())
			Expect(*hook.Phase).To(Equal("Failed"))
			Expect(hook.WorkflowRunRef).NotTo(BeNil())
			runName = *hook.WorkflowRunRef

			By("the hook WorkflowRun is labelled as a deployment hook and received the release image")
			out, err := framework.KubectlGetJsonpath(kubeContext, cpNs, "workflowrun", runName,
				`{.metadata.labels.openchoreo\.dev/workflow-purpose}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(Equal("deployment-hook"))
			out, err = framework.KubectlGetJsonpath(kubeContext, cpNs, "workflowrun", runName,
				`{.spec.workflow.parameters.image}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(Equal(imageService), "hook parameter `image` was not resolved from the release workload")

			By("no RenderedRelease exists for the staging binding and Ready is False")
			Consistently(func(g Gomega) {
				out, err := framework.Kubectl(kubeContext, "get", "renderedrelease", "-n", cpNs,
					"-l", "openchoreo.dev/environment="+envStaging+",openchoreo.dev/component="+componentGated,
					"-o", "jsonpath={.items[*].metadata.name}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).To(BeEmpty(), "a RenderedRelease was written while the gate was blocked")
			}, 30*time.Second, 5*time.Second).Should(Succeed())
			framework.AssertJsonpathEquals(Default, kubeContext, cpNs, "releasebinding", rbName,
				`{.status.conditions[?(@.type=="Ready")].status}`, "False")
		})

		It("passes after the workflow is fixed and the hook is retried through the API", func() {
			By("replacing the check workflow with a passing one (hook spec, and so the gate key, unchanged)")
			output, err := framework.KubectlApplyLiteral(kubeContext,
				hookWorkflowYAML(clusterWorkflowCheck, "exit 0", false))
			Expect(err).NotTo(HaveOccurred(), "failed to update check workflow: %s", output)

			By("retrying the hook")
			resp, err := client.RetryReleaseBindingHookWithResponse(ctx, cpNs, rbName, bindingImageCheck,
				gen.HookRetryRequest{Phase: gen.HookRetryRequestPhase("preDeploy")})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(http.StatusOK), "body: %s", string(resp.Body))

			By("the gate passes: passedKey == key")
			Eventually(func(g Gomega) {
				key, err := framework.KubectlGetJsonpath(kubeContext, cpNs, "releasebinding", rbName, `{.status.gate.key}`)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(key).NotTo(BeEmpty())
				passed, err := framework.KubectlGetJsonpath(kubeContext, cpNs, "releasebinding", rbName, `{.status.gate.passedKey}`)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(passed).To(Equal(key))
			}, hookTimeout, 5*time.Second).Should(Succeed())

			By("the release renders and the binding becomes Ready")
			Eventually(func(g Gomega) {
				framework.AssertReleaseBindingReady(g, kubeContext, cpNs, rbName)
			}, releasePropagation, 5*time.Second).Should(Succeed())
			Eventually(func(g Gomega) {
				out, err := framework.Kubectl(kubeContext, "get", "renderedrelease", "-n", cpNs,
					"-l", "openchoreo.dev/environment="+envStaging+",openchoreo.dev/component="+componentGated,
					"-o", "jsonpath={.items[*].metadata.name}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).NotTo(BeEmpty())
			}, releasePropagation, 5*time.Second).Should(Succeed())
		})
	})

	Context("Sync pre-deploy hook with a suspended approval step", func() {
		rbName := componentApproved + "-" + envStaging

		It("waits on the suspend step until resumed through the API, then passes", func() {
			By("pinning the development release into staging")
			release := developmentRelease(componentApproved)
			output, err := framework.KubectlApplyLiteral(kubeContext, stagingReleaseBindingYAML(componentApproved, release))
			Expect(err).NotTo(HaveOccurred(), "failed to apply staging binding: %s", output)

			By("the hook is Running with the gate held open")
			var runName string
			Eventually(func(g Gomega) {
				resp, err := client.ListReleaseBindingHooksWithResponse(ctx, cpNs, rbName)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(Equal(http.StatusOK), "body: %s", string(resp.Body))
				g.Expect(resp.JSON200).NotTo(BeNil())
				hook := findHook(resp.JSON200.PreDeploy, bindingApproval)
				g.Expect(hook).NotTo(BeNil())
				g.Expect(hook.Phase).NotTo(BeNil())
				g.Expect(*hook.Phase).To(Equal("Running"))
				g.Expect(hook.WorkflowRunRef).NotTo(BeNil())
				runName = *hook.WorkflowRunRef
			}, hookTimeout, 5*time.Second).Should(Succeed())
			framework.AssertJsonpathEquals(Default, kubeContext, cpNs, "releasebinding", rbName,
				`{.status.conditions[?(@.type=="PreDeployHooksPassed")].reason}`, "HooksRunning")

			By("resuming the suspended WorkflowRun through the API (409 until Argo reaches the suspend node)")
			Eventually(func(g Gomega) {
				resp, err := client.ResumeWorkflowRunWithResponse(ctx, cpNs, runName)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(Equal(http.StatusOK), "body: %s", string(resp.Body))
			}, hookTimeout, 5*time.Second).Should(Succeed())

			By("the hook succeeds and the release renders")
			Eventually(func(g Gomega) {
				framework.AssertWorkflowRunSucceeded(g, kubeContext, cpNs, runName)
			}, hookTimeout, 5*time.Second).Should(Succeed())
			Eventually(func(g Gomega) {
				framework.AssertReleaseBindingReady(g, kubeContext, cpNs, rbName)
			}, releasePropagation, 5*time.Second).Should(Succeed())
		})
	})
})

// developmentRelease returns the ComponentRelease the auto-deployed development
// binding of `component` is pinned to — the release the suite then promotes.
func developmentRelease(component string) string {
	var release string
	Eventually(func(g Gomega) {
		out, err := framework.KubectlGetJsonpath(kubeContext, cpNs, "releasebinding",
			component+"-"+envDev, `{.spec.releaseName}`)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(out).NotTo(BeEmpty(), "development binding of %s has no releaseName yet", component)
		release = out
	}, releasePropagation, 5*time.Second).Should(Succeed())
	fmt.Fprintf(GinkgoWriter, "promoting %s release %s to %s\n", component, release, envStaging)
	return release
}

func findHook(hooks *[]gen.DeploymentHookStatus, name string) *gen.DeploymentHookStatus {
	if hooks == nil {
		return nil
	}
	for i := range *hooks {
		if (*hooks)[i].Name == name {
			return &(*hooks)[i]
		}
	}
	return nil
}

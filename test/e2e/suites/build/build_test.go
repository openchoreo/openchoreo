// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

const (
	// Builds take longer than image-pull deploys; bound generously to avoid
	// flakes when the CI runner is under load.
	buildTimeout       = 20 * time.Minute
	releasePropagation = 5 * time.Minute
)

var dpNs string

// Matrix specs are declared up front so BeforeAll can trigger every build in
// one pass — the workflow plane runs them concurrently and each spec below
// only waits on its own run. Wall-clock is then bounded by the slowest single
// build instead of the sum of all of them.
var (
	specDockerfileService = buildSpec{
		component:    componentDockerfile,
		componentTyp: "deployment/service",
		workflow:     "dockerfile-builder",
		repoName:     sampleWorkloadsRepo,
		appPath:      "/service-go-greeter",
		dockerfile:   "/service-go-greeter/Dockerfile",
		endpoint:     "greeter-api",
		assertReach:  true,
		assertLogs:   true,
	}
	specDockerfileReact = buildSpec{
		component:    componentDockerfileReact,
		componentTyp: "deployment/web-application",
		workflow:     "dockerfile-builder",
		repoName:     sampleWorkloadsRepo,
		appPath:      "/webapp-react-nginx",
		dockerfile:   "/webapp-react-nginx/Dockerfile",
		endpoint:     "webapp-endpoint",
		assertReach:  true,
	}
	specGCPBuildpacks = buildSpec{
		component:    componentGCP,
		componentTyp: "deployment/service",
		workflow:     "gcp-buildpacks-builder",
		repoName:     sampleWorkloadsRepo,
		appPath:      "/service-go-reading-list",
		endpoint:     "reading-list-api",
		assertReach:  false, // upstream sample's port surface varies; deploy + ComponentRelease are the meaningful signal
	}
	specPaketoBuildpacks = buildSpec{
		component:    componentPaketo,
		componentTyp: "deployment/service",
		workflow:     "paketo-buildpacks-builder",
		repoName:     paketoNodeRepo,
		appPath:      "/",
		endpoint:     "http",
		assertReach:  true,
	}
	specBallerinaBuildpack = buildSpec{
		component:    componentBallerina,
		componentTyp: "deployment/service",
		workflow:     "ballerina-buildpack-builder",
		repoName:     sampleWorkloadsRepo,
		appPath:      "/service-ballerina-patient-management",
		endpoint:     "patient-management-api",
		assertReach:  false,
	}

	matrixSpecs = []buildSpec{
		specDockerfileService,
		specDockerfileReact,
		specGCPBuildpacks,
		specPaketoBuildpacks,
		specBallerinaBuildpack,
	}
)

var _ = Describe("Build From Source Matrix", Ordered, Label("tier3"), func() {
	SetDefaultEventuallyTimeout(framework.DefaultTimeout)
	SetDefaultEventuallyPollingInterval(framework.DefaultPolling)

	BeforeAll(func() {
		By("installing in-cluster Gitea")
		// Gitea must be in the WP cluster so Argo Workflow pods can clone via
		// in-cluster DNS. In single-cluster mode wpCtx() == kubeContext.
		Expect(framework.InstallGitea(wpCtx(), giteaNamespace)).To(Succeed())

		By("mirroring openchoreo/sample-workloads into Gitea")
		Expect(framework.MigrateRepo(wpCtx(), giteaNamespace,
			sampleWorkloadsRepo, upstreamSampleWorkloads)).To(Succeed())

		By("seeding the no-workload fixture into Gitea")
		Expect(framework.EnsureGiteaRepo(wpCtx(), giteaNamespace, noWorkloadRepo)).To(Succeed())
		repoRoot, err := framework.RepoRoot()
		Expect(err).NotTo(HaveOccurred())
		Expect(framework.PushTree(wpCtx(), giteaNamespace, noWorkloadRepo, "main",
			filepath.Join(repoRoot, "test/e2e/fixtures/build/no-workload"))).To(Succeed())

		By("seeding the paketo-node fixture into Gitea")
		Expect(framework.EnsureGiteaRepo(wpCtx(), giteaNamespace, paketoNodeRepo)).To(Succeed())
		Expect(framework.PushTree(wpCtx(), giteaNamespace, paketoNodeRepo, "main",
			filepath.Join(repoRoot, "test/e2e/fixtures/build/paketo-node"))).To(Succeed())

		By("creating control plane namespace")
		output, err := framework.KubectlApplyLiteral(kubeContext, cpNamespaceYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to create CP namespace: %s", output)

		By("applying platform resources (pipeline, environments, project)")
		output, err = framework.KubectlApplyLiteral(kubeContext, platformResourcesYAML())
		Expect(err).NotTo(HaveOccurred(), "failed to apply platform resources: %s", output)

	})

	AfterAll(func() {
		if os.Getenv("E2E_KEEP_RESOURCES") == "true" {
			By("skipping cleanup because E2E_KEEP_RESOURCES=true")
			return
		}
		By("cleaning up control plane namespace (cascades to DP and WP)")
		_, _ = framework.Kubectl(kubeContext, "delete", "namespace", cpNs,
			"--ignore-not-found", "--wait=false")
		if dpNs != "" {
			_, _ = framework.Kubectl(dpCtx(), "delete", "namespace", dpNs,
				"--ignore-not-found", "--wait=false")
		}
		_, _ = framework.Kubectl(wpCtx(), "delete", "namespace", giteaNamespace,
			"--ignore-not-found", "--wait=false")
	})

	Context("builder matrix", func() {
		// Builds are triggered once for this matrix context; each spec only
		// waits on its own WorkflowRun and asserts the post-build chain.
		BeforeAll(func() {
			By("triggering all matrix builds up front so they run concurrently")
			for _, spec := range matrixSpecs {
				triggerDeployableBuildSpec(spec)
			}
		})

		It("dockerfile-builder: builds, deploys, and is reachable (service)", func() {
			assertDeployableBuildSpec(specDockerfileService)
		})

		It("dockerfile-builder: react web-application builds and is reachable", func() {
			assertDeployableBuildSpec(specDockerfileReact)
		})

		It("gcp-buildpacks-builder: builds and deploys reading-list", func() {
			assertDeployableBuildSpec(specGCPBuildpacks)
		})

		It("paketo-buildpacks-builder: builds and deploys the in-tree node fixture", func() {
			assertDeployableBuildSpec(specPaketoBuildpacks)
		})

		It("ballerina-buildpack-builder: builds and deploys patient-management", func() {
			assertDeployableBuildSpec(specBallerinaBuildpack)
		})
	})

	Context("solo specs", func() {
		It("workload-auto-generation: build without a workload.yaml lands a Workload+ComponentRelease", func() {
			// Worker (not service) because `occ workload create` without a
			// descriptor produces a Workload with zero endpoints, and the
			// service CCT's CEL validation requires `size(workload.endpoints) > 0`
			// — it would reject the rendered release. worker requires == 0,
			// which matches the auto-generation output. Reachability is N/A
			// for workers (no endpoints), so assertReach is false.
			runDeployableBuildSpec(buildSpec{
				component:    componentNoWorkload,
				componentTyp: "deployment/worker",
				workflow:     "dockerfile-builder",
				repoName:     noWorkloadRepo,
				appPath:      "/",
				dockerfile:   "/Dockerfile",
				assertReach:  false,
			})

			By("Workload CR was created by the pipeline (auto-generated)")
			Eventually(func(g Gomega) {
				out, err := framework.Kubectl(kubeContext,
					"get", "workload",
					"-n", cpNs,
					"-o", "jsonpath={.items[?(@.spec.owner.componentName==\""+componentNoWorkload+"\")].metadata.name}",
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).NotTo(BeEmpty(),
					"expected an auto-generated Workload for component %s", componentNoWorkload)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())
		})

		It("private-repo: clones a private GitHub repo with a PAT, builds, and the WorkflowRun succeeds", func() {
			pat := os.Getenv(privateRepoPATEnv)
			if pat == "" {
				Skip(fmt.Sprintf("%s not set; skipping private repository build scenario", privateRepoPATEnv))
			}

			By("creating the git basic-auth secret via the OpenChoreo Secret API")
			Expect(createGitBasicAuthSecret(cpNs, privateRepoSecretRef, privateRepoGitUser, pat)).To(Succeed(),
				"failed to create git secret via the OpenChoreo API")

			By("applying the Component bound to the dockerfile-builder workflow")
			output, err := framework.KubectlApplyLiteral(kubeContext,
				privateRepoComponentYAML(componentPrivate, privateRepoSecretRef))
			Expect(err).NotTo(HaveOccurred(), "failed to apply component: %s", output)

			runName := componentPrivate + "-run-01"
			By(fmt.Sprintf("applying WorkflowRun %s to trigger the private build", runName))
			output, err = framework.KubectlApplyLiteral(kubeContext,
				privateRepoWorkflowRunYAML(componentPrivate, runName, privateRepoSecretRef))
			Expect(err).NotTo(HaveOccurred(), "failed to apply workflow run: %s", output)

			By("build pod exposes streamable logs")
			assertWorkflowRunLogs(runName)

			By("verifying the checkout-source task succeeded (private repo cloned with the PAT)")
			Eventually(func(g Gomega) {
				framework.AssertWorkflowTaskSucceeded(g, kubeContext, cpNs, runName, "checkout-source")
			}, buildTimeout, 10*time.Second).Should(Succeed())

			By("waiting for the private-repo WorkflowRun to succeed")
			Eventually(func(g Gomega) {
				framework.AssertWorkflowRunSucceeded(g, kubeContext, cpNs, runName)
			}, buildTimeout, 10*time.Second).Should(Succeed())
		})

		It("externalrefs-in-cel: SecretReference spec surfaces in the rendered Argo Workflow", func() {
			By("applying SecretReference + Workflow + WorkflowRun")
			output, err := framework.KubectlApplyLiteral(kubeContext, externalRefsFixtureYAML())
			Expect(err).NotTo(HaveOccurred(), "failed to apply externalrefs fixture: %s", output)

			runName := componentExternalRefs + "-run-01"
			By("waiting for WorkflowRun.status.runReference to populate")
			var kind, refName, refNs string
			Eventually(func(g Gomega) {
				k, n, ns, e := framework.WorkflowRunReference(kubeContext, cpNs, runName)
				g.Expect(e).NotTo(HaveOccurred())
				g.Expect(k).NotTo(BeEmpty(), "runReference.kind not populated yet")
				g.Expect(n).NotTo(BeEmpty(), "runReference.name not populated yet")
				kind, refName, refNs = k, n, ns
			}, 3*time.Minute, 3*time.Second).Should(Succeed())
			fmt.Fprintf(GinkgoWriter, "rendered workflow ref: %s %s/%s\n", kind, refNs, refName)

			By("rendered Argo Workflow carries the resolved externalRef value as a label")
			// The Argo Workflow lives in the WP cluster in multi-cluster mode.
			Eventually(func(g Gomega) {
				out, err := framework.KubectlGetJsonpath(wpCtx(), refNs,
					"workflow.argoproj.io", refName,
					`{.metadata.labels['openchoreo\.dev/e2e-cel-probe']}`,
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).To(Equal("7m42s"),
					"expected label to equal SecretReference.spec.refreshInterval, got %q", out)
			}, 3*time.Minute, 5*time.Second).Should(Succeed())
		})

		It("build-logs-api: OpenChoreo API serves live workflow-plane logs and reflects workflow deletion", func() {
			// Reuse the dockerfile-service matrix build. The Describe is Ordered, so
			// the builder-matrix Context has already built and asserted this run by
			// the time the solo specs run; its Argo Workflow lingers (ttlAfterCompletion
			// 1d) and no later spec reads it, so deleting it here to drive the
			// live-logs lifecycle is safe.
			runName := componentDockerfile + "-run-01"

			By("obtaining an OpenChoreo API access token")
			token, err := fetchToken()
			Expect(err).NotTo(HaveOccurred(), "failed to fetch API token")
			client, err := newAPIClient(token)
			Expect(err).NotTo(HaveOccurred(), "failed to build API client")
			ctx := context.Background()

			By("status reports hasLiveObservability=true while the Argo Workflow exists")
			Eventually(func(g Gomega) {
				resp, err := client.GetWorkflowRunStatusWithResponse(ctx, cpNs, runName)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(Equal(http.StatusOK), "body: %s", string(resp.Body))
				g.Expect(resp.JSON200).NotTo(BeNil())
				g.Expect(resp.JSON200.HasLiveObservability).To(BeTrue(),
					"expected live observability while the workflow is present")
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("logs endpoint serves live build logs from the workflow plane")
			Eventually(func(g Gomega) {
				resp, err := client.GetWorkflowRunLogsWithResponse(ctx, cpNs, runName, &gen.GetWorkflowRunLogsParams{})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(Equal(http.StatusOK), "body: %s", string(resp.Body))
				g.Expect(resp.JSON200).NotTo(BeNil())
				g.Expect(*resp.JSON200).NotTo(BeEmpty(), "expected non-empty live build logs")
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("deleting the Argo Workflow from the workflow plane")
			_, err = framework.Kubectl(wpCtx(), "delete", "workflow.argoproj.io", runName,
				"-n", "workflows-"+cpNs, "--ignore-not-found")
			Expect(err).NotTo(HaveOccurred(), "failed to delete Argo Workflow")

			By("status reports hasLiveObservability=false once the workflow is gone")
			Eventually(func(g Gomega) {
				resp, err := client.GetWorkflowRunStatusWithResponse(ctx, cpNs, runName)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(Equal(http.StatusOK), "body: %s", string(resp.Body))
				g.Expect(resp.JSON200).NotTo(BeNil())
				g.Expect(resp.JSON200.HasLiveObservability).To(BeFalse(),
					"expected no live observability after the workflow is deleted")
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("logs endpoint no longer serves live logs (this endpoint has no archived fallback)")
			// With the Argo Workflow gone the WorkflowRun CR still resolves, so the
			// service reaches the workflow plane and surfaces the missing workflow as
			// an error (currently HTTP 500) with no log body — clients fall back to the
			// observability plane via hasLiveObservability, not this endpoint.
			Eventually(func(g Gomega) {
				resp, err := client.GetWorkflowRunLogsWithResponse(ctx, cpNs, runName, &gen.GetWorkflowRunLogsParams{})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode()).To(BeNumerically(">=", http.StatusBadRequest),
					"expected an error status once the workflow is gone; body: %s", string(resp.Body))
				g.Expect(resp.JSON200).To(BeNil(), "expected no live logs after the workflow is deleted")
			}, 2*time.Minute, 5*time.Second).Should(Succeed())
		})

		It("build-logs-via-k8s: live logs reachable from a running WorkflowRun pod", func() {
			component := componentLogs
			runName := component + "-run-01"

			By("waiting for controller-manager webhook to be ready")
			Eventually(func() error {
				out, err := framework.KubectlGetJsonpath(kubeContext, "openchoreo-control-plane",
					"endpoints", "controller-manager-webhook-service",
					`{.subsets[0].addresses[0].ip}`)
				if err != nil || strings.TrimSpace(out) == "" {
					return fmt.Errorf("webhook endpoint not ready yet")
				}
				return nil
			}, 3*time.Minute, 5*time.Second).Should(Succeed(),
				"controller-manager webhook endpoint never became ready")

			By("applying component + workflowrun")
			gitURL := framework.GiteaRepoCloneURL(giteaNamespace, sampleWorkloadsRepo)
			output, err := framework.KubectlApplyLiteral(kubeContext, buildComponentYAML(
				component, "deployment/service", "dockerfile-builder",
				gitURL, "/service-go-greeter", "/service-go-greeter/Dockerfile",
			))
			Expect(err).NotTo(HaveOccurred(), "failed to apply component: %s", output)
			output, err = framework.KubectlApplyLiteral(kubeContext, workflowRunYAML(
				component, runName, "dockerfile-builder",
				gitURL, "/service-go-greeter", "/service-go-greeter/Dockerfile",
			))
			Expect(err).NotTo(HaveOccurred(), "failed to apply workflow run: %s", output)

			By("waiting for the build Argo Workflow pod to appear")
			// Argo Workflow pods run in the WP cluster; wpCtx() is the WP
			// kubecontext in multi-cluster mode and falls back to kubeContext.
			wfNs := "workflows-" + cpNs
			Eventually(func(g Gomega) {
				out, err := framework.Kubectl(wpCtx(),
					"get", "pods", "-n", wfNs,
					"-l", "workflows.argoproj.io/workflow="+runName,
					"-o", "jsonpath={.items[*].metadata.name}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.TrimSpace(out)).NotTo(BeEmpty(),
					"no pod found yet for WorkflowRun %s in %s", runName, wfNs)
			}, 5*time.Minute, 5*time.Second).Should(Succeed())

			By("kubectl logs against the build pod returns non-empty content")
			Eventually(func(g Gomega) {
				out, err := framework.KubectlLogs(wpCtx(), wfNs,
					"workflows.argoproj.io/workflow="+runName, 50)
				g.Expect(err).NotTo(HaveOccurred(), "kubectl logs failed: %s", out)
				g.Expect(strings.TrimSpace(out)).NotTo(BeEmpty(),
					"expected non-empty build pod logs while WorkflowRun is running")
			}, 5*time.Minute, 5*time.Second).Should(Succeed())
		})
	})
})

type buildSpec struct {
	component    string
	componentTyp string
	workflow     string
	repoName     string
	appPath      string
	dockerfile   string // optional — leave empty for buildpacks
	endpoint     string // endpoint name as declared in the workload.yaml the build checks out
	assertReach  bool   // whether to assert in-cluster HTTP reachability of the rendered Service
	assertLogs   bool   // whether to assert the live build pod exposes non-empty logs
}

// runDeployableBuildSpec drives a Component + WorkflowRun through the build
// pipeline end to end: trigger, then wait and assert. Used by solo specs that
// are not part of the concurrently-triggered builder matrix.
func runDeployableBuildSpec(spec buildSpec) {
	triggerDeployableBuildSpec(spec)
	assertDeployableBuildSpec(spec)
}

// triggerDeployableBuildSpec applies the Component + WorkflowRun for a build
// spec without waiting on the run. The matrix specs are all triggered from
// BeforeAll so their builds execute concurrently on the workflow plane.
func triggerDeployableBuildSpec(spec buildSpec) {
	runName := spec.component + "-run-01"
	gitURL := framework.GiteaRepoCloneURL(giteaNamespace, spec.repoName)

	// The CP controller-manager webhook may be temporarily unavailable if the
	// controller restarted due to leader election loss during a long test run.
	// Wait for the webhook endpoint to be ready before applying any resources.
	By("waiting for controller-manager webhook to be ready")
	Eventually(func() error {
		out, err := framework.KubectlGetJsonpath(kubeContext, "openchoreo-control-plane",
			"endpoints", "controller-manager-webhook-service",
			`{.subsets[0].addresses[0].ip}`)
		if err != nil || strings.TrimSpace(out) == "" {
			return fmt.Errorf("webhook endpoint not ready yet")
		}
		return nil
	}, 3*time.Minute, 5*time.Second).Should(Succeed(),
		"controller-manager webhook endpoint never became ready")

	By(fmt.Sprintf("applying Component %s with workflow %s", spec.component, spec.workflow))
	output, err := framework.KubectlApplyLiteral(kubeContext, buildComponentYAML(
		spec.component, spec.componentTyp, spec.workflow, gitURL, spec.appPath, spec.dockerfile,
	))
	Expect(err).NotTo(HaveOccurred(), "failed to apply component: %s", output)

	By(fmt.Sprintf("applying WorkflowRun %s", runName))
	output, err = framework.KubectlApplyLiteral(kubeContext, workflowRunYAML(
		spec.component, runName, spec.workflow, gitURL, spec.appPath, spec.dockerfile,
	))
	Expect(err).NotTo(HaveOccurred(), "failed to apply workflow run: %s", output)
}

// assertDeployableBuildSpec waits for an already-triggered WorkflowRun and
// asserts the post-build artifacts (ComponentRelease, ComponentReleaseBinding, pod
// Running). Optionally probes the rendered Service for TCP reachability.
// Shared by every spec in the builder matrix so the assertions stay in lockstep.
func assertDeployableBuildSpec(spec buildSpec) {
	runName := spec.component + "-run-01"

	if spec.assertLogs {
		assertWorkflowRunLogs(runName)
	}

	By("waiting for WorkflowRun to succeed")
	Eventually(func(g Gomega) {
		// Emit runReference on each poll so CI logs show whether the CP
		// controller ever created the Argo Workflow in the WP cluster.
		kind, name, ns, _ := framework.WorkflowRunReference(kubeContext, cpNs, runName)
		fmt.Fprintf(GinkgoWriter, "WorkflowRun %s runReference: kind=%s name=%s ns=%s\n",
			runName, kind, name, ns)
		framework.AssertWorkflowRunSucceeded(g, kubeContext, cpNs, runName)
	}, buildTimeout, 10*time.Second).Should(Succeed())

	By("ComponentRelease appears for the component")
	Eventually(func(g Gomega) {
		framework.AssertComponentReleasePresent(g, kubeContext, cpNs, spec.component)
	}, releasePropagation, 5*time.Second).Should(Succeed())

	By("ComponentReleaseBinding reaches Ready")
	Eventually(func(g Gomega) {
		framework.AssertComponentReleaseBindingReady(g, kubeContext, cpNs, spec.component+componentReleaseBindingSuffix)
	}, 5*time.Minute, 5*time.Second).Should(Succeed())

	By("discovering the data plane namespace")
	Eventually(func() error {
		var discoverErr error
		dpNs, discoverErr = framework.GetDPNamespace(dpCtx(), cpNs, projectName, envDev)
		return discoverErr
	}, 3*time.Minute, 5*time.Second).Should(Succeed())

	By("workload pod is Running")
	Eventually(func(g Gomega) {
		framework.AssertPodsRunning(g, dpCtx(), dpNs,
			"openchoreo.dev/component="+spec.component)
	}, 3*time.Minute, 5*time.Second).Should(Succeed())

	if !spec.assertReach {
		return
	}

	By("ensuring a tester pod is available in the DP namespace")
	output, err := framework.KubectlApplyLiteral(dpCtx(), testerPodYAML(dpNs))
	Expect(err).NotTo(HaveOccurred(), "failed to apply tester pod: %s", output)
	Eventually(func(g Gomega) {
		framework.AssertPodsRunning(g, dpCtx(), dpNs, testerLabel)
	}, 3*time.Minute, 3*time.Second).Should(Succeed())

	By("rendered Service is TCP-reachable from the tester pod")
	host, port := endpointHostPort(spec.component, spec.endpoint)
	Eventually(func() error {
		_, err := framework.CheckTCPReachableFromPodByLabel(
			dpCtx(), dpNs, testerLabel, testerContainer, host, port, 5,
		)
		return err
	}, 2*time.Minute, 5*time.Second).Should(Succeed(),
		"%s:%s should be TCP-reachable for component %s", host, port, spec.component)
}

func assertWorkflowRunLogs(runName string) {
	By("waiting for the build Argo Workflow pod to appear")
	wfNs := "workflows-" + cpNs
	Eventually(func(g Gomega) {
		out, err := framework.Kubectl(wpCtx(),
			"get", "pods", "-n", wfNs,
			"-l", "workflows.argoproj.io/workflow="+runName,
			"-o", "jsonpath={.items[*].metadata.name}")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(strings.TrimSpace(out)).NotTo(BeEmpty(),
			"no pod found yet for WorkflowRun %s in %s", runName, wfNs)
	}, 5*time.Minute, 5*time.Second).Should(Succeed())

	By("kubectl logs against the build pod returns non-empty content")
	Eventually(func(g Gomega) {
		out, err := framework.KubectlLogs(wpCtx(), wfNs,
			"workflows.argoproj.io/workflow="+runName, 50)
		g.Expect(err).NotTo(HaveOccurred(), "kubectl logs failed: %s", out)
		g.Expect(strings.TrimSpace(out)).NotTo(BeEmpty(),
			"expected non-empty build pod logs while WorkflowRun is running")
	}, 5*time.Minute, 5*time.Second).Should(Succeed())
}

// endpointHostPort reads the rendered Service URL host+port for a named endpoint
// off the ComponentReleaseBinding status. Same shape as the workloadtypes suite — kept
// inline so the build suite doesn't pull a dependency on that test package.
func endpointHostPort(component, endpoint string) (host, port string) {
	rbName := component + componentReleaseBindingSuffix
	var hostOut, portOut string
	Eventually(func(g Gomega) {
		var err error
		hostOut, err = framework.KubectlGetJsonpath(
			kubeContext, cpNs, "componentreleasebinding", rbName,
			fmt.Sprintf(`{.status.endpoints[?(@.name=="%s")].serviceURL.host}`, endpoint),
		)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(hostOut).NotTo(BeEmpty(), "serviceURL.host empty on %s endpoint %s", rbName, endpoint)

		portOut, err = framework.KubectlGetJsonpath(
			kubeContext, cpNs, "componentreleasebinding", rbName,
			fmt.Sprintf(`{.status.endpoints[?(@.name=="%s")].serviceURL.port}`, endpoint),
		)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(portOut).NotTo(BeEmpty(), "serviceURL.port empty on %s endpoint %s", rbName, endpoint)
	}, 3*time.Minute, 2*time.Second).Should(Succeed())
	return hostOut, portOut
}

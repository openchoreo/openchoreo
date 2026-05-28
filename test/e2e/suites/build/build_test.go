// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

const (
	// Builds take longer than image-pull deploys; bound generously to avoid
	// flakes when the CI runner is under load.
	buildTimeout       = 20 * time.Minute
	releasePropagation = 5 * time.Minute
)

var dpNs string

var _ = Describe("Build From Source Matrix", Ordered, Label("tier3"), func() {
	SetDefaultEventuallyTimeout(framework.DefaultTimeout)
	SetDefaultEventuallyPollingInterval(framework.DefaultPolling)

	BeforeAll(func() {
		By("installing in-cluster Gitea")
		Expect(framework.InstallGitea(kubeContext, giteaNamespace)).To(Succeed())

		By("mirroring openchoreo/sample-workloads into Gitea")
		Expect(framework.MigrateRepo(kubeContext, giteaNamespace,
			sampleWorkloadsRepo, upstreamSampleWorkloads)).To(Succeed())

		By("seeding the no-workload fixture into Gitea")
		Expect(framework.EnsureGiteaRepo(kubeContext, giteaNamespace, noWorkloadRepo)).To(Succeed())
		repoRoot, err := framework.RepoRoot()
		Expect(err).NotTo(HaveOccurred())
		Expect(framework.PushTree(kubeContext, giteaNamespace, noWorkloadRepo, "main",
			filepath.Join(repoRoot, "test/e2e/fixtures/build/no-workload"))).To(Succeed())

		By("seeding the paketo-node fixture into Gitea")
		Expect(framework.EnsureGiteaRepo(kubeContext, giteaNamespace, paketoNodeRepo)).To(Succeed())
		Expect(framework.PushTree(kubeContext, giteaNamespace, paketoNodeRepo, "main",
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
			_, _ = framework.Kubectl(kubeContext, "delete", "namespace", dpNs,
				"--ignore-not-found", "--wait=false")
		}
		_, _ = framework.Kubectl(kubeContext, "delete", "namespace", giteaNamespace,
			"--ignore-not-found", "--wait=false")
	})

	Context("builder matrix", func() {
		It("dockerfile-builder: builds, deploys, and is reachable (service)", func() {
			runDeployableBuildSpec(buildSpec{
				component:    componentDockerfile,
				componentTyp: "deployment/service",
				workflow:     "dockerfile-builder",
				repoName:     sampleWorkloadsRepo,
				appPath:      "/service-go-greeter",
				dockerfile:   "/service-go-greeter/Dockerfile",
				endpoint:     "greeter-api",
				assertReach:  true,
			})
		})

		It("dockerfile-builder: react web-application builds and is reachable", func() {
			runDeployableBuildSpec(buildSpec{
				component:    componentDockerfileReact,
				componentTyp: "deployment/web-application",
				workflow:     "dockerfile-builder",
				repoName:     sampleWorkloadsRepo,
				appPath:      "/webapp-react-nginx",
				dockerfile:   "/webapp-react-nginx/Dockerfile",
				endpoint:     "webapp-endpoint",
				assertReach:  true,
			})
		})

		It("gcp-buildpacks-builder: builds and deploys reading-list", func() {
			runDeployableBuildSpec(buildSpec{
				component:    componentGCP,
				componentTyp: "deployment/service",
				workflow:     "gcp-buildpacks-builder",
				repoName:     sampleWorkloadsRepo,
				appPath:      "/service-go-reading-list",
				endpoint:     "reading-list-api",
				assertReach:  false, // upstream sample's port surface varies; deploy + ComponentRelease are the meaningful signal
			})
		})

		It("paketo-buildpacks-builder: builds and deploys the in-tree node fixture", func() {
			runDeployableBuildSpec(buildSpec{
				component:    componentPaketo,
				componentTyp: "deployment/service",
				workflow:     "paketo-buildpacks-builder",
				repoName:     paketoNodeRepo,
				appPath:      "/",
				endpoint:     "http",
				assertReach:  true,
			})
		})

		It("ballerina-buildpack-builder: builds and deploys patient-management", func() {
			runDeployableBuildSpec(buildSpec{
				component:    componentBallerina,
				componentTyp: "deployment/service",
				workflow:     "ballerina-buildpack-builder",
				repoName:     sampleWorkloadsRepo,
				appPath:      "/service-ballerina-patient-management",
				endpoint:     "patient-management-api",
				assertReach:  false,
			})
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
			Eventually(func(g Gomega) {
				out, err := framework.KubectlGetJsonpath(kubeContext, refNs,
					"workflow.argoproj.io", refName,
					`{.metadata.labels['openchoreo\.dev/e2e-cel-probe']}`,
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).To(Equal("7m42s"),
					"expected label to equal SecretReference.spec.refreshInterval, got %q", out)
			}, 3*time.Minute, 5*time.Second).Should(Succeed())
		})

		It("build-logs-via-k8s: live logs reachable from a running WorkflowRun pod", func() {
			component := componentLogs
			runName := component + "-run-01"

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
			wfNs := "workflows-" + cpNs
			Eventually(func(g Gomega) {
				out, err := framework.Kubectl(kubeContext,
					"get", "pods", "-n", wfNs,
					"-l", "workflows.argoproj.io/workflow="+runName,
					"-o", "jsonpath={.items[*].metadata.name}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.TrimSpace(out)).NotTo(BeEmpty(),
					"no pod found yet for WorkflowRun %s in %s", runName, wfNs)
			}, 5*time.Minute, 5*time.Second).Should(Succeed())

			By("kubectl logs against the build pod returns non-empty content")
			Eventually(func(g Gomega) {
				out, err := framework.KubectlLogs(kubeContext, wfNs,
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
}

// runDeployableBuildSpec drives a Component + WorkflowRun through the build
// pipeline and asserts the post-build artifacts (ComponentRelease, ReleaseBinding,
// pod Running). Optionally probes the rendered Service for TCP reachability.
// Shared by every spec in the builder matrix so the assertions stay in lockstep.
func runDeployableBuildSpec(spec buildSpec) {
	runName := spec.component + "-run-01"
	gitURL := framework.GiteaRepoCloneURL(giteaNamespace, spec.repoName)

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

	By("waiting for WorkflowRun to succeed")
	Eventually(func(g Gomega) {
		framework.AssertWorkflowRunSucceeded(g, kubeContext, cpNs, runName)
	}, buildTimeout, 10*time.Second).Should(Succeed())

	By("ComponentRelease appears for the component")
	Eventually(func(g Gomega) {
		framework.AssertComponentReleasePresent(g, kubeContext, cpNs, spec.component)
	}, releasePropagation, 5*time.Second).Should(Succeed())

	By("ReleaseBinding reaches Ready")
	Eventually(func(g Gomega) {
		framework.AssertReleaseBindingReady(g, kubeContext, cpNs, spec.component+releaseBindingSuffix)
	}, 5*time.Minute, 5*time.Second).Should(Succeed())

	By("discovering the data plane namespace")
	Eventually(func() error {
		var discoverErr error
		dpNs, discoverErr = framework.GetDPNamespace(kubeContext, cpNs, projectName, envDev)
		return discoverErr
	}, 3*time.Minute, 5*time.Second).Should(Succeed())

	By("workload pod is Running")
	Eventually(func(g Gomega) {
		framework.AssertPodsRunning(g, kubeContext, dpNs,
			"openchoreo.dev/component="+spec.component)
	}, 3*time.Minute, 5*time.Second).Should(Succeed())

	if !spec.assertReach {
		return
	}

	By("ensuring a tester pod is available in the DP namespace")
	output, err = framework.KubectlApplyLiteral(kubeContext, testerPodYAML(dpNs))
	Expect(err).NotTo(HaveOccurred(), "failed to apply tester pod: %s", output)
	Eventually(func(g Gomega) {
		framework.AssertPodsRunning(g, kubeContext, dpNs, testerLabel)
	}, 3*time.Minute, 3*time.Second).Should(Succeed())

	By("rendered Service is TCP-reachable from the tester pod")
	host, port := endpointHostPort(spec.component, spec.endpoint)
	Eventually(func() error {
		_, err := framework.CheckTCPReachableFromPodByLabel(
			kubeContext, dpNs, testerLabel, testerContainer, host, port, 5,
		)
		return err
	}, 2*time.Minute, 5*time.Second).Should(Succeed(),
		"%s:%s should be TCP-reachable for component %s", host, port, spec.component)
}

// endpointHostPort reads the rendered Service URL host+port for a named endpoint
// off the ReleaseBinding status. Same shape as the workloadtypes suite — kept
// inline so the build suite doesn't pull a dependency on that test package.
func endpointHostPort(component, endpoint string) (host, port string) {
	rbName := component + releaseBindingSuffix
	var hostOut, portOut string
	Eventually(func(g Gomega) {
		var err error
		hostOut, err = framework.KubectlGetJsonpath(
			kubeContext, cpNs, "releasebinding", rbName,
			fmt.Sprintf(`{.status.endpoints[?(@.name=="%s")].serviceURL.host}`, endpoint),
		)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(hostOut).NotTo(BeEmpty(), "serviceURL.host empty on %s endpoint %s", rbName, endpoint)

		portOut, err = framework.KubectlGetJsonpath(
			kubeContext, cpNs, "releasebinding", rbName,
			fmt.Sprintf(`{.status.endpoints[?(@.name=="%s")].serviceURL.port}`, endpoint),
		)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(portOut).NotTo(BeEmpty(), "serviceURL.port empty on %s endpoint %s", rbName, endpoint)
	}, 3*time.Minute, 2*time.Second).Should(Succeed())
	return hostOut, portOut
}

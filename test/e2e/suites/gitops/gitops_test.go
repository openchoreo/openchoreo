// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

var _ = Describe("GitOps with Flux", Ordered, Label("tier3"), func() {
	SetDefaultEventuallyTimeout(framework.DefaultTimeout)
	SetDefaultEventuallyPollingInterval(framework.DefaultPolling)

	var dpNs string

	BeforeAll(func() {
		By("installing Flux")
		Expect(framework.InstallFlux(kubeContext)).To(Succeed())

		By("installing in-cluster Gitea")
		Expect(framework.InstallGitea(kubeContext, giteaNamespace)).To(Succeed())

		By("creating the gitops platform repo and seeding the CP namespace + platform doc")
		Expect(framework.EnsureGiteaRepo(kubeContext, giteaNamespace, gitopsRepo)).To(Succeed())
		Expect(framework.PushFile(kubeContext, giteaNamespace, gitopsRepo, "main",
			"platform/namespace.yaml", []byte(cpNamespaceYAML()), "")).To(Succeed())
		Expect(framework.PushFile(kubeContext, giteaNamespace, gitopsRepo, "main",
			"platform/platform.yaml", []byte(platformResourcesYAML()), "")).To(Succeed())

		By("applying the GitRepository pointing Flux at the in-cluster Gitea")
		Expect(framework.ApplyGitRepository(kubeContext, fluxNs, gitopsRepo,
			framework.GiteaRepoCloneURL(giteaNamespace, gitopsRepo), "main")).To(Succeed())

		By("applying the platform Kustomization")
		Expect(framework.ApplyKustomization(kubeContext, fluxNs, "platform",
			gitopsRepo, "platform", "")).To(Succeed())
		Expect(framework.WaitForKustomizationReady(kubeContext, fluxNs, "platform", 3*time.Minute)).To(Succeed())
	})

	AfterAll(func() {
		if os.Getenv("E2E_KEEP_RESOURCES") == "true" {
			By("skipping cleanup because E2E_KEEP_RESOURCES=true")
			return
		}
		_, _ = framework.Kubectl(kubeContext, "delete", "kustomization", "platform",
			"-n", fluxNs, "--ignore-not-found", "--wait=false")
		_, _ = framework.Kubectl(kubeContext, "delete", "kustomization", "echo-app",
			"-n", fluxNs, "--ignore-not-found", "--wait=false")
		_, _ = framework.Kubectl(kubeContext, "delete", "kustomization", "bulk",
			"-n", fluxNs, "--ignore-not-found", "--wait=false")
		_, _ = framework.Kubectl(kubeContext, "delete", "gitrepository", gitopsRepo,
			"-n", fluxNs, "--ignore-not-found")
		_, _ = framework.Kubectl(kubeContext, "delete", "namespace", cpNs,
			"--ignore-not-found", "--wait=false")
		if dpNs != "" {
			_, _ = framework.Kubectl(dpCtx(), "delete", "namespace", dpNs,
				"--ignore-not-found", "--wait=false")
		}
		_, _ = framework.Kubectl(kubeContext, "delete", "namespace", giteaNamespace,
			"--ignore-not-found", "--wait=false")
	})

	Context("single component lifecycle", func() {
		It("flux-reconcile: push a Component + Workload, Flux reconciles it onto the cluster", func() {
			By("pushing the echo-svc Component + Workload doc to Gitea")
			Expect(framework.PushFile(kubeContext, giteaNamespace, gitopsRepo, "main",
				"apps/echo-svc.yaml",
				[]byte(componentWithImageYAML(componentSingle, imageInitial, "echo-initial")),
				"")).To(Succeed())

			By("applying the echo-app Kustomization")
			Expect(framework.ApplyKustomization(kubeContext, fluxNs, "echo-app",
				gitopsRepo, "apps", "")).To(Succeed())
			Expect(framework.WaitForKustomizationReady(kubeContext, fluxNs, "echo-app", 3*time.Minute)).
				To(Succeed())

			By("Component appears in the CP namespace")
			Eventually(func(g Gomega) {
				framework.AssertResourceExists(g, kubeContext, cpNs, "component", componentSingle)
			}, 2*time.Minute, 3*time.Second).Should(Succeed())

			By("ComponentReleaseBinding becomes Ready")
			Eventually(func(g Gomega) {
				framework.AssertComponentReleaseBindingReady(g, kubeContext, cpNs, componentSingle+componentReleaseBindingSuffix)
			}, 5*time.Minute, 5*time.Second).Should(Succeed())

			By("discovering the data plane namespace")
			Eventually(func() error {
				var err error
				dpNs, err = framework.GetDPNamespace(dpCtx(), cpNs, projectName, envDev)
				return err
			}, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("pod is Running with the initial image")
			Eventually(func(g Gomega) {
				framework.AssertPodsRunning(g, dpCtx(), dpNs,
					"openchoreo.dev/component="+componentSingle)
			}, 3*time.Minute, 5*time.Second).Should(Succeed())
		})

		It("flux-update: push an image bump, Flux rolls the Deployment", func() {
			Expect(dpNs).NotTo(BeEmpty(),
				"flux-reconcile must run first to populate dpNs")

			By("pushing an updated Workload (different image tag) to Gitea")
			Expect(framework.PushFile(kubeContext, giteaNamespace, gitopsRepo, "main",
				"apps/echo-svc.yaml",
				[]byte(componentWithImageYAML(componentSingle, imageUpdated, "echo-updated")),
				"e2e: bump echo-svc image")).To(Succeed())

			By("rendered Deployment image updates to the new tag")
			Eventually(func(g Gomega) {
				out, err := framework.Kubectl(dpCtx(),
					"get", "deployment",
					"-n", dpNs,
					"-l", "openchoreo.dev/component="+componentSingle,
					"-o", "jsonpath={.items[0].spec.template.spec.containers[0].image}",
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).To(Equal(imageUpdated),
					"deployment image should equal the new tag, got %q", out)
			}, 5*time.Minute, 5*time.Second).Should(Succeed())

			By("pod is Running on the new image")
			Eventually(func(g Gomega) {
				framework.AssertPodsRunning(g, dpCtx(), dpNs,
					"openchoreo.dev/component="+componentSingle)
			}, 3*time.Minute, 5*time.Second).Should(Succeed())
		})
	})

	Context("bulk promote", func() {
		It("bulk-promote: a single commit promotes multiple components into the env", func() {
			By("pushing 3 bulk component docs to Gitea under apps/bulk/")
			components := []string{componentBulkA, componentBulkB, componentBulkC}
			for _, c := range components {
				Expect(framework.PushFile(kubeContext, giteaNamespace, gitopsRepo, "main",
					fmt.Sprintf("apps-bulk/%s.yaml", c),
					[]byte(componentWithImageYAML(c, imageInitial, c)),
					"e2e: add bulk components")).To(Succeed())
			}

			By("applying the bulk Kustomization")
			Expect(framework.ApplyKustomization(kubeContext, fluxNs, "bulk",
				gitopsRepo, "apps-bulk", "")).To(Succeed())
			Expect(framework.WaitForKustomizationReady(kubeContext, fluxNs, "bulk", 3*time.Minute)).
				To(Succeed())

			By("all 3 ComponentReleaseBindings reach Ready")
			for _, c := range components {
				rb := c + componentReleaseBindingSuffix
				Eventually(func(g Gomega) {
					framework.AssertComponentReleaseBindingReady(g, kubeContext, cpNs, rb)
				}, 5*time.Minute, 5*time.Second).Should(Succeed(),
					"ComponentReleaseBinding %s did not reach Ready", rb)
			}

			By("all 3 component pods are Running")
			for _, c := range components {
				Eventually(func(g Gomega) {
					framework.AssertPodsRunning(g, dpCtx(), dpNs,
						"openchoreo.dev/component="+c)
				}, 5*time.Minute, 5*time.Second).Should(Succeed(),
					"pods for component %s never reached Running", c)
			}
		})
	})
})

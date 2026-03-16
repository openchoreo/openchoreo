// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

var _ = Describe("DeploymentPipeline Deletion", Ordered, func() {
	SetDefaultEventuallyTimeout(framework.DefaultTimeout)
	SetDefaultEventuallyPollingInterval(framework.DefaultPolling)

	// -------------------------------------------------------------------------
	// Scenario 1: Pipeline with a single referencing project
	// -------------------------------------------------------------------------
	Context("when a DeploymentPipeline referenced by a single Project is deleted", Ordered, func() {
		var (
			pipelineName string
			projectName  string
		)

		BeforeAll(func() {
			pipelineName = uniqueName("dp-del-single")
			projectName = uniqueName("proj-single")

			By("creating the DeploymentPipeline")
			_, err := framework.KubectlApplyLiteral(kubeContext, deploymentPipelineYAML(pipelineName, "development"))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for pipeline to become available")
			Eventually(func(g Gomega) {
				status, err := framework.KubectlGetJsonpath(
					kubeContext, testNS, "deploymentpipeline", pipelineName,
					`{.status.conditions[?(@.type=="Available")].status}`,
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(status).To(Equal("True"))
			}, 1*time.Minute, 2*time.Second).Should(Succeed())

			By("creating a Project referencing the pipeline")
			_, err = framework.KubectlApplyLiteral(kubeContext, projectYAML(projectName, pipelineName))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for project to be ready")
			Eventually(func(g Gomega) {
				_, err := framework.KubectlGetJsonpath(
					kubeContext, testNS, "project", projectName,
					`{.metadata.name}`,
				)
				g.Expect(err).NotTo(HaveOccurred())
			}, 1*time.Minute, 2*time.Second).Should(Succeed())
		})

		AfterAll(func() {
			// Cleanup: delete project (ignore errors if already gone)
			_, _ = framework.Kubectl(kubeContext, "delete", "project", projectName, "-n", testNS, "--ignore-not-found")
			_, _ = framework.Kubectl(kubeContext, "delete", "deploymentpipeline", pipelineName, "-n", testNS, "--ignore-not-found")
		})

		It("should clear the project's deploymentPipelineRef and delete the pipeline", func() {
			By("deleting the DeploymentPipeline")
			_, err := framework.Kubectl(kubeContext, "delete", "deploymentpipeline", pipelineName, "-n", testNS)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the pipeline is fully deleted (not stuck in Terminating)")
			Eventually(func(g Gomega) {
				output, err := framework.KubectlGetJsonpath(
					kubeContext, testNS, "deploymentpipeline", pipelineName,
					`{.metadata.name}`,
				)
				g.Expect(err).To(HaveOccurred(), "pipeline should be deleted")
				g.Expect(strings.Contains(output, "NotFound") || strings.Contains(err.Error(), "NotFound")).
					To(BeTrue(), "expected NotFound error, got: %v", err)
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying the project's deploymentPipelineRef is cleared")
			ref, err := framework.KubectlGetJsonpath(
				kubeContext, testNS, "project", projectName,
				`{.spec.deploymentPipelineRef}`,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(ref).To(BeEmpty(), "deploymentPipelineRef should be nil/absent on the project")
		})
	})

	// -------------------------------------------------------------------------
	// Scenario 2: Pipeline with multiple referencing projects
	// -------------------------------------------------------------------------
	Context("when a DeploymentPipeline referenced by multiple Projects is deleted", Ordered, func() {
		var (
			pipelineName string
			project1Name string
			project2Name string
		)

		BeforeAll(func() {
			pipelineName = uniqueName("dp-del-multi")
			project1Name = uniqueName("proj-multi-1")
			project2Name = uniqueName("proj-multi-2")

			By("creating the DeploymentPipeline")
			_, err := framework.KubectlApplyLiteral(kubeContext, deploymentPipelineYAML(pipelineName, "development"))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for pipeline to become available")
			Eventually(func(g Gomega) {
				status, err := framework.KubectlGetJsonpath(
					kubeContext, testNS, "deploymentpipeline", pipelineName,
					`{.status.conditions[?(@.type=="Available")].status}`,
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(status).To(Equal("True"))
			}, 1*time.Minute, 2*time.Second).Should(Succeed())

			By("creating two Projects referencing the pipeline")
			for _, name := range []string{project1Name, project2Name} {
				_, err := framework.KubectlApplyLiteral(kubeContext, projectYAML(name, pipelineName))
				Expect(err).NotTo(HaveOccurred())
			}

			By("waiting for both projects to be ready")
			for _, name := range []string{project1Name, project2Name} {
				projName := name
				Eventually(func(g Gomega) {
					_, err := framework.KubectlGetJsonpath(
						kubeContext, testNS, "project", projName,
						`{.metadata.name}`,
					)
					g.Expect(err).NotTo(HaveOccurred())
				}, 1*time.Minute, 2*time.Second).Should(Succeed())
			}
		})

		AfterAll(func() {
			for _, name := range []string{project1Name, project2Name} {
				_, _ = framework.Kubectl(kubeContext, "delete", "project", name, "-n", testNS, "--ignore-not-found")
			}
			_, _ = framework.Kubectl(kubeContext, "delete", "deploymentpipeline", pipelineName, "-n", testNS, "--ignore-not-found")
		})

		It("should clear refs from all projects and delete the pipeline", func() {
			By("deleting the DeploymentPipeline")
			_, err := framework.Kubectl(kubeContext, "delete", "deploymentpipeline", pipelineName, "-n", testNS)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the pipeline is fully deleted")
			Eventually(func(g Gomega) {
				output, err := framework.KubectlGetJsonpath(
					kubeContext, testNS, "deploymentpipeline", pipelineName,
					`{.metadata.name}`,
				)
				g.Expect(err).To(HaveOccurred(), "pipeline should be deleted")
				g.Expect(strings.Contains(output, "NotFound") || strings.Contains(err.Error(), "NotFound")).
					To(BeTrue(), "expected NotFound error, got: %v", err)
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying both projects have cleared deploymentPipelineRef")
			for _, name := range []string{project1Name, project2Name} {
				ref, err := framework.KubectlGetJsonpath(
					kubeContext, testNS, "project", name,
					`{.spec.deploymentPipelineRef}`,
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(ref).To(BeEmpty(),
					fmt.Sprintf("deploymentPipelineRef should be nil/absent on project %s", name))
			}
		})
	})

	// -------------------------------------------------------------------------
	// Scenario 3: Pipeline with no referencing projects
	// -------------------------------------------------------------------------
	Context("when a DeploymentPipeline with no referencing Projects is deleted", Ordered, func() {
		var pipelineName string

		BeforeAll(func() {
			pipelineName = uniqueName("dp-del-noref")

			By("creating the DeploymentPipeline")
			_, err := framework.KubectlApplyLiteral(kubeContext, deploymentPipelineYAML(pipelineName, "development"))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for pipeline to become available")
			Eventually(func(g Gomega) {
				status, err := framework.KubectlGetJsonpath(
					kubeContext, testNS, "deploymentpipeline", pipelineName,
					`{.status.conditions[?(@.type=="Available")].status}`,
				)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(status).To(Equal("True"))
			}, 1*time.Minute, 2*time.Second).Should(Succeed())
		})

		AfterAll(func() {
			_, _ = framework.Kubectl(kubeContext, "delete", "deploymentpipeline", pipelineName, "-n", testNS, "--ignore-not-found")
		})

		It("should delete the pipeline immediately without errors", func() {
			By("deleting the DeploymentPipeline")
			_, err := framework.Kubectl(kubeContext, "delete", "deploymentpipeline", pipelineName, "-n", testNS)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the pipeline is fully deleted")
			Eventually(func(g Gomega) {
				output, err := framework.KubectlGetJsonpath(
					kubeContext, testNS, "deploymentpipeline", pipelineName,
					`{.metadata.name}`,
				)
				g.Expect(err).To(HaveOccurred(), "pipeline should be deleted")
				g.Expect(strings.Contains(output, "NotFound") || strings.Contains(err.Error(), "NotFound")).
					To(BeTrue(), "expected NotFound error, got: %v", err)
			}, 1*time.Minute, 2*time.Second).Should(Succeed())
		})
	})
})

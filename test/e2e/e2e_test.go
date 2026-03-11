// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

var _ = Describe("Platform Health", Ordered, func() {
	const (
		cpNamespace = "openchoreo-control-plane"
		dpNamespace = "openchoreo-data-plane"
		defaultNS   = "default"
	)

	SetDefaultEventuallyTimeout(framework.DefaultTimeout)
	SetDefaultEventuallyPollingInterval(framework.DefaultPolling)

	Context("Control Plane", func() {
		It("should have all pods running", func() {
			Eventually(func(g Gomega) {
				framework.AssertAllPodsRunning(g, kubeContext, cpNamespace)
			}).Should(Succeed())
		})
	})

	Context("Data Plane", func() {
		It("should have all pods running", func() {
			Eventually(func(g Gomega) {
				framework.AssertAllPodsRunning(g, kubeContext, dpNamespace)
			}).Should(Succeed())
		})
	})

	Context("CRDs", func() {
		crds := []string{
			"projects.openchoreo.dev",
			"components.openchoreo.dev",
			"componenttypes.openchoreo.dev",
			"clustercomponenttypes.openchoreo.dev",
			"traits.openchoreo.dev",
			"clustertraits.openchoreo.dev",
			"environments.openchoreo.dev",
			"dataplanes.openchoreo.dev",
			"clusterdataplanes.openchoreo.dev",
			"deploymentpipelines.openchoreo.dev",
			"componentreleases.openchoreo.dev",
			"releasebindings.openchoreo.dev",
			"renderedreleases.openchoreo.dev",
			"workloads.openchoreo.dev",
			"workflows.openchoreo.dev",
			"clusterworkflows.openchoreo.dev",
		}

		for _, crd := range crds {
			It("should have CRD "+crd, func() {
				_, err := framework.Kubectl(kubeContext, "get", "crd", crd)
				Expect(err).NotTo(HaveOccurred(), "CRD %s should be registered", crd)
			})
		}
	})

	Context("Default Resources", func() {
		It("should have Project 'default'", func() {
			framework.AssertResourceExists(Default, kubeContext, defaultNS, "project", "default")
		})

		It("should have DeploymentPipeline 'default'", func() {
			framework.AssertResourceExists(Default, kubeContext, defaultNS, "deploymentpipeline", "default")
		})

		environments := []string{"development", "staging", "production"}
		for _, env := range environments {
			It("should have Environment '"+env+"'", func() {
				framework.AssertResourceExists(Default, kubeContext, defaultNS, "environment", env)
			})
		}

		clusterComponentTypes := []string{"worker", "service", "web-application", "scheduled-task"}
		for _, ct := range clusterComponentTypes {
			It("should have ClusterComponentType '"+ct+"'", func() {
				framework.AssertClusterResourceExists(Default, kubeContext, "clustercomponenttype", ct)
			})
		}

		clusterTraits := []string{"api-configuration", "observability-alert-rule"}
		for _, trait := range clusterTraits {
			It("should have ClusterTrait '"+trait+"'", func() {
				framework.AssertClusterResourceExists(Default, kubeContext, "clustertrait", trait)
			})
		}

		clusterWorkflows := []string{"dockerfile-builder", "ballerina-buildpack-builder", "gcp-buildpacks-builder", "paketo-buildpacks-builder"}
		for _, wf := range clusterWorkflows {
			It("should have ClusterWorkflow '"+wf+"'", func() {
				framework.AssertClusterResourceExists(Default, kubeContext, "clusterworkflow", wf)
			})
		}
	})

	Context("DataPlane Connectivity", func() {
		It("should have ClusterDataPlane 'default' with agent connected", func() {
			Eventually(func(g Gomega) {
				output, err := framework.Kubectl(kubeContext, "get", "clusterdataplane", "default",
					"-o", "jsonpath={.status.agentConnection.connected}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, 5*time.Minute).Should(Succeed())
		})

		It("should have cluster-agent logs showing connection", func() {
			// The "connected" log is only printed once at startup. If the pod
			// has been running for a while the line may have rotated out of
			// the tail window. Check the current logs first; if the substring
			// is missing, rollout-restart the agent so a fresh pod prints it.
			checkLogs := func(g Gomega) {
				output, err := framework.KubectlLogs(kubeContext, dpNamespace, "app=cluster-agent", 50)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("connected"),
					"cluster-agent logs should indicate connection to control plane")
			}

			output, err := framework.KubectlLogs(kubeContext, dpNamespace, "app=cluster-agent", 50)
			if err != nil || !strings.Contains(output, "connected") {
				By("'connected' log not found, restarting cluster-agent to get a fresh log")
				Expect(framework.KubectlRolloutRestart(kubeContext, dpNamespace, "deployment/cluster-agent-dataplane")).To(Succeed())
			}

			Eventually(checkLogs, 2*time.Minute).Should(Succeed())
		})
	})
})

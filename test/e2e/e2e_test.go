// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
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
			"traits.openchoreo.dev",
			"environments.openchoreo.dev",
			"dataplanes.openchoreo.dev",
			"deploymentpipelines.openchoreo.dev",
			"componentreleases.openchoreo.dev",
			"releasebindings.openchoreo.dev",
			"releases.openchoreo.dev",
			"workloads.openchoreo.dev",
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

		componentTypes := []string{"worker", "service", "web-application", "scheduled-task"}
		for _, ct := range componentTypes {
			It("should have ComponentType '"+ct+"'", func() {
				framework.AssertResourceExists(Default, kubeContext, defaultNS, "componenttype", ct)
			})
		}

		traits := []string{"api-configuration", "observability-alert-rule"}
		for _, trait := range traits {
			It("should have Trait '"+trait+"'", func() {
				framework.AssertResourceExists(Default, kubeContext, defaultNS, "trait", trait)
			})
		}
	})

	Context("DataPlane Connectivity", func() {
		It("should have DataPlane 'default' with agent connected", func() {
			Eventually(func(g Gomega) {
				framework.AssertJsonpathEquals(g, kubeContext, defaultNS,
					"dataplane", "default",
					"{.status.agentConnection.connected}", "true")
			}, 5*time.Minute).Should(Succeed())
		})

		It("should have cluster-agent logs showing connection", func() {
			Eventually(func(g Gomega) {
				output, err := framework.KubectlLogs(kubeContext, dpNamespace, "app=cluster-agent", 50)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("connected"),
					"cluster-agent logs should indicate connection to control plane")
			}).Should(Succeed())
		})
	})
})

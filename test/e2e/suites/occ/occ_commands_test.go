// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func describeResourceCommands() {
	Context("resource list and get", func() {
		It("lists projects in namespace", func() {
			stdout, _, err := occ.Run("project", "list", "-n", cpNs)
			Expect(err).NotTo(HaveOccurred(), "occ project list failed")
			Expect(stdout).To(ContainSubstring(projectName),
				"expected project %q in list output", projectName)
		})

		It("gets specific project", func() {
			stdout, _, err := occ.Run("project", "get", projectName, "-n", cpNs)
			Expect(err).NotTo(HaveOccurred(), "occ project get failed")
			Expect(stdout).To(ContainSubstring(projectName),
				"expected project name in get output")
		})

		It("lists components in namespace", func() {
			stdout, _, err := occ.Run("component", "list", "-n", cpNs)
			Expect(err).NotTo(HaveOccurred(), "occ component list failed")
			Expect(stdout).To(ContainSubstring(componentName),
				"expected component %q in list output", componentName)
		})

		It("gets specific component", func() {
			stdout, _, err := occ.Run("component", "get", componentName, "-n", cpNs)
			Expect(err).NotTo(HaveOccurred(), "occ component get failed")
			Expect(stdout).To(ContainSubstring(componentName),
				"expected component name in get output")
		})

		It("lists release bindings in namespace", func() {
			Eventually(func(g Gomega) {
				stdout, _, err := occ.Run("releasebinding", "list", "-n", cpNs)
				g.Expect(err).NotTo(HaveOccurred(), "occ releasebinding list failed")
				g.Expect(stdout).To(ContainSubstring(componentName),
					"expected component name in releasebinding list output")
			}, 3*time.Minute, 2*time.Second).Should(Succeed())
		})

		DescribeTable("cluster-scoped list and get",
			func(resource, getName string) {
				By(fmt.Sprintf("listing %s", resource))
				_, _, err := occ.Run(resource, "list")
				Expect(err).NotTo(HaveOccurred(), "occ %s list failed", resource)

				By(fmt.Sprintf("getting %s/%s", resource, getName))
				stdout, _, err := occ.Run(resource, "get", getName)
				Expect(err).NotTo(HaveOccurred(), "occ %s get %s failed", resource, getName)
				Expect(stdout).To(ContainSubstring(getName))
			},
			Entry("namespace", "namespace", cpNs),
			Entry("clustercomponenttype", "clustercomponenttype", "service"),
			Entry("clustertrait", "clustertrait", "observability-alert-rule"),
			Entry("clusterdataplane", "clusterdataplane", "default"),
			Entry("clusterauthzrole", "clusterauthzrole", clusterAuthzRoleName),
			Entry("clusterauthzrolebinding", "clusterauthzrolebinding", clusterAuthzBindingName),
			Entry("clusterworkflow", "clusterworkflow", "dockerfile-builder"),
		)

		DescribeTable("namespace-scoped list and get",
			func(resource, ns, getName string) {
				By(fmt.Sprintf("listing %s in %s", resource, ns))
				_, _, err := occ.Run(resource, "list", "-n", ns)
				Expect(err).NotTo(HaveOccurred(), "occ %s list failed", resource)

				By(fmt.Sprintf("getting %s/%s in %s", resource, getName, ns))
				stdout, _, err := occ.Run(resource, "get", getName, "-n", ns)
				Expect(err).NotTo(HaveOccurred(), "occ %s get %s failed", resource, getName)
				Expect(stdout).To(ContainSubstring(getName))
			},
			Entry("project (default ns)", "project", "default", "default"),
			Entry("deploymentpipeline (default ns)", "deploymentpipeline", "default", "default"),
			Entry("environment (default ns)", "environment", "default", "development"),
			Entry("component", "component", cpNs, componentName),
			Entry("workload", "workload", cpNs, componentName),
			Entry("authzrole", "authzrole", cpNs, authzRoleName),
			Entry("authzrolebinding", "authzrolebinding", cpNs, authzRoleBindingName),
			Entry("secretreference", "secretreference", cpNs, secretRefName),
		)

		DescribeTable("list-only (empty or no instances)",
			func(resource string, args ...string) {
				allArgs := append([]string{resource, "list"}, args...)
				_, _, err := occ.Run(allArgs...)
				Expect(err).NotTo(HaveOccurred(),
					"occ %s list should succeed even with no resources", resource)
			},
			Entry("clusterworkflowplane", "clusterworkflowplane"),
			Entry("clusterobservabilityplane", "clusterobservabilityplane"),
			Entry("clusterresourcetype", "clusterresourcetype"),
			Entry("componenttype", "componenttype", "-n", cpNs),
			Entry("trait", "trait", "-n", cpNs),
			Entry("dataplane", "dataplane", "-n", cpNs),
			Entry("workflowplane", "workflowplane", "-n", cpNs),
			Entry("observabilityplane", "observabilityplane", "-n", cpNs),
			Entry("workflow", "workflow", "-n", cpNs),
			Entry("workflowrun", "workflowrun", "-n", cpNs),
			Entry("secret", "secret", "-n", cpNs),
			Entry("resource", "resource", "-n", cpNs),
			Entry("resourcerelease", "resourcerelease", "-n", cpNs),
			Entry("resourcereleasebinding", "resourcereleasebinding", "-n", cpNs),
			Entry("resourcetype", "resourcetype", "-n", cpNs),
			Entry("observabilityalertsnotificationchannel",
				"observabilityalertsnotificationchannel", "-n", cpNs),
		)
	})
}

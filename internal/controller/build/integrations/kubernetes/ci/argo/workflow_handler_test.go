/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package argo

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/controller/build/integrations"
	argo "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
)

var _ = Describe("Argo Workflow", func() {
	var (
		buildCtx *integrations.BuildContext
	)
	BeforeEach(func() {
		buildCtx = newTestBuildContext()
	})

	Context("Get workflow status", func() {
		It("should return correct status for argo workflow status", func() {
			Expect(GetStepPhase(argo.NodeRunning)).To(Equal(integrations.Running))
			Expect(GetStepPhase(argo.NodePending)).To(Equal(integrations.Running))

			Expect(GetStepPhase(argo.NodeFailed)).To(Equal(integrations.Failed))
			Expect(GetStepPhase(argo.NodeError)).To(Equal(integrations.Failed))
			Expect(GetStepPhase(argo.NodeSkipped)).To(Equal(integrations.Failed))

			Expect(GetStepPhase(argo.NodeSucceeded)).To(Equal(integrations.Succeeded))
		})

		It("should return correct status for argo workflow status", func() {
			nodes := argo.Nodes{
				string(integrations.CloneStep): argo.NodeStatus{
					Name:         string(integrations.CloneStep),
					TemplateName: string(integrations.CloneStep),
					Phase:        argo.NodeSucceeded,
				},
				string(integrations.BuildStep): argo.NodeStatus{
					Name:         string(integrations.BuildStep),
					TemplateName: string(integrations.BuildStep),
					Phase:        argo.NodeRunning,
				},
				string(integrations.PushStep): argo.NodeStatus{
					Name:         string(integrations.PushStep),
					TemplateName: string(integrations.PushStep),
				},
			}
			node, found := GetStepByTemplateName(nodes, integrations.CloneStep)
			Expect(found).To(BeTrue())
			Expect(node.TemplateName).To(Equal(string(integrations.CloneStep)))
			Expect(node.Phase).To(Equal(argo.NodeSucceeded))
		})
	})

	DescribeTable("Get image name from workflow",
		func(output argo.Outputs, expectedImage string) {
			image := GetImageNameFromWorkflow(output)
			Expect(image).To(Equal(expectedImage))
		},
		Entry("should return image if it exists", argo.Outputs{
			Parameters: []argo.Parameter{{Name: "image", Value: ptr.To("registry.io/repo/image:tag")}},
		}, "registry.io/repo/image:tag"),
		Entry("should return empty string if it doesn't exist", argo.Outputs{}, ""),
	)

	Context("Make workflow name", func() {
		When("build name is longer than 63 characters", func() {
			BeforeEach(func() {
				longName := strings.Repeat("a", 100)
				buildCtx.Build.ObjectMeta.Name = longName
			})
			It("should return the workflow name of 63 characters", func() {
				name := makeWorkflowName(buildCtx)
				Expect(name).To(HaveLen(63))
				Expect(name).To(Equal("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-28165978"))
			})
		})

		When("build name is smaller than 63 characters", func() {
			BeforeEach(func() {
				smallName := strings.Repeat("a", 40)
				buildCtx.Build.ObjectMeta.Name = smallName
			})
			It("should return the workflow name with less than 63 characters", func() {
				name := makeWorkflowName(buildCtx)
				Expect(len(name)).To(BeNumerically("<", 63))
				Expect(name).To(Equal("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-e33cdf9c"))
			})
		})
	})
})

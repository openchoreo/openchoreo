// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package build

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

var _ = Describe("Build Conditions", func() {
	buildResource := newBuildpackBasedBuild()

	DescribeTable("Set initial build conditions",
		func(build openchoreov1alpha1.Build, conditionType controller.ConditionType, expectedReason controller.ConditionReason, expectedMessage string) {
			setInitialBuildConditions(&build)
			cond := meta.FindStatusCondition(build.Status.Conditions, string(conditionType))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(expectedReason)))
			Expect(cond.Message).To(Equal(expectedMessage))
		},
		Entry("should set clone step as queued", *buildResource, ConditionCloneStepSucceeded, ReasonStepQueued, "Clone source code step is queued for execution."),
		Entry("should set build step as queued", *buildResource, ConditionBuildStepSucceeded, ReasonStepQueued, "Image build step is queued for execution."),
		Entry("should set push step as queued", *buildResource, ConditionPushStepSucceeded, ReasonStepQueued, "Image push step is queued for execution."),
		Entry("should mark build process as in progress", *buildResource, ConditionCompleted, ReasonBuildInProgress, "Build process is in progress."),
	)

	DescribeTable("Mark step as in progress",
		func(build openchoreov1alpha1.Build, conditionType controller.ConditionType, expectedMessage string) {
			markStepInProgress(&build, conditionType)
			cond := meta.FindStatusCondition(build.Status.Conditions, string(conditionType))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Reason).To(Equal(string(ReasonStepInProgress)))
			Expect(cond.Message).To(Equal(expectedMessage))
		},
		Entry("should mark clone step as in progress", *buildResource, ConditionCloneStepSucceeded, "Clone source code step is executing."),
		Entry("should mark build step as in progress", *buildResource, ConditionBuildStepSucceeded, "Image build step is executing."),
		Entry("should mark push step as in progress", *buildResource, ConditionPushStepSucceeded, "Image push step is executing."),
	)

	DescribeTable("Mark step as succeeded",
		func(build openchoreov1alpha1.Build, conditionType controller.ConditionType, expectedReason controller.ConditionReason, expectedMessage string) {
			markStepSucceeded(&build, conditionType)
			cond := meta.FindStatusCondition(build.Status.Conditions, string(conditionType))
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(expectedReason)))
			Expect(cond.Message).To(Equal(expectedMessage))
		},
		Entry("should mark the condition clone step succeeded correctly", *buildResource, ConditionCloneStepSucceeded, ReasonStepSucceeded, "Source code clone step completed successfully."),
		Entry("should mark the condition build step succeeded correctly", *buildResource, ConditionBuildStepSucceeded, ReasonStepSucceeded, "Image build step completed successfully."),
		Entry("should mark the condition push step succeeded correctly", *buildResource, ConditionPushStepSucceeded, ReasonStepSucceeded, "Image push step completed successfully."),
	)

	DescribeTable("Mark step as failed",
		func(build openchoreov1alpha1.Build, conditionType controller.ConditionType, expectedStepReason controller.ConditionReason, expectedStepMessage string) {
			markStepFailed(&build, conditionType)
			stepCond := meta.FindStatusCondition(build.Status.Conditions, string(conditionType))
			Expect(stepCond).NotTo(BeNil())
			Expect(stepCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(stepCond.Reason).To(Equal(string(expectedStepReason)))
			Expect(stepCond.Message).To(Equal(expectedStepMessage))
		},
		Entry("should mark the condition clone step failed correctly", *buildResource, ConditionCloneStepSucceeded, ReasonStepFailed, "Source code cloning failed."),
		Entry("should mark the condition build step failed correctly", *buildResource, ConditionBuildStepSucceeded, ReasonStepFailed, "Building the image from the source code failed."),
		Entry("should mark the condition push step failed correctly", *buildResource, ConditionPushStepSucceeded, ReasonStepFailed, "Pushing the built image to the registry failed."),
	)
})

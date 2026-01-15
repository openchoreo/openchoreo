// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("Condition Functions", func() {
	var workflowRun *openchoreodevv1alpha1.WorkflowRun

	BeforeEach(func() {
		workflowRun = &openchoreodevv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-run",
				Namespace:  "default",
				Generation: 1,
			},
			Status: openchoreodevv1alpha1.WorkflowRunStatus{
				Conditions: []metav1.Condition{},
			},
		}
	})

	Describe("setWorkflowPendingCondition", func() {
		It("should set workflow pending condition", func() {
			setWorkflowPendingCondition(workflowRun)

			Expect(workflowRun.Status.Conditions).To(HaveLen(1))
			condition := workflowRun.Status.Conditions[0]
			Expect(condition.Type).To(Equal(string(ConditionWorkflowCompleted)))
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(string(ReasonWorkflowPending)))
			Expect(condition.Message).To(ContainSubstring("has not completed yet"))
			Expect(condition.ObservedGeneration).To(Equal(int64(1)))
		})
	})

	Describe("setWorkflowRunningCondition", func() {
		It("should set workflow running condition", func() {
			setWorkflowRunningCondition(workflowRun)

			Expect(workflowRun.Status.Conditions).To(HaveLen(1))
			condition := workflowRun.Status.Conditions[0]
			Expect(condition.Type).To(Equal(string(ConditionWorkflowRunning)))
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal(string(ReasonWorkflowRunning)))
			Expect(condition.Message).To(ContainSubstring("running"))
		})
	})

	Describe("setWorkflowSucceededCondition", func() {
		It("should set workflow succeeded conditions", func() {
			setWorkflowSucceededCondition(workflowRun)

			Expect(workflowRun.Status.Conditions).To(HaveLen(3))

			runningCondition := findCondition(workflowRun.Status.Conditions, string(ConditionWorkflowRunning))
			Expect(runningCondition).NotTo(BeNil())
			Expect(runningCondition.Status).To(Equal(metav1.ConditionFalse))

			succeededCondition := findCondition(workflowRun.Status.Conditions, string(ConditionWorkflowSucceeded))
			Expect(succeededCondition).NotTo(BeNil())
			Expect(succeededCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(succeededCondition.Reason).To(Equal(string(ReasonWorkflowSucceeded)))

			completedCondition := findCondition(workflowRun.Status.Conditions, string(ConditionWorkflowCompleted))
			Expect(completedCondition).NotTo(BeNil())
			Expect(completedCondition.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Describe("setWorkflowFailedCondition", func() {
		It("should set workflow failed conditions", func() {
			setWorkflowFailedCondition(workflowRun)

			Expect(workflowRun.Status.Conditions).To(HaveLen(3))

			runningCondition := findCondition(workflowRun.Status.Conditions, string(ConditionWorkflowRunning))
			Expect(runningCondition).NotTo(BeNil())
			Expect(runningCondition.Status).To(Equal(metav1.ConditionFalse))

			failedCondition := findCondition(workflowRun.Status.Conditions, string(ConditionWorkflowFailed))
			Expect(failedCondition).NotTo(BeNil())
			Expect(failedCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(failedCondition.Reason).To(Equal(string(ReasonWorkflowFailed)))

			completedCondition := findCondition(workflowRun.Status.Conditions, string(ConditionWorkflowCompleted))
			Expect(completedCondition).NotTo(BeNil())
			Expect(completedCondition.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Describe("setWorkflowNotFoundCondition", func() {
		It("should set workflow not found conditions", func() {
			setWorkflowNotFoundCondition(workflowRun)

			Expect(workflowRun.Status.Conditions).To(HaveLen(2))

			runningCondition := findCondition(workflowRun.Status.Conditions, string(ConditionWorkflowRunning))
			Expect(runningCondition).NotTo(BeNil())
			Expect(runningCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(runningCondition.Message).To(ContainSubstring("not found"))

			completedCondition := findCondition(workflowRun.Status.Conditions, string(ConditionWorkflowCompleted))
			Expect(completedCondition).NotTo(BeNil())
			Expect(completedCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(completedCondition.Message).To(ContainSubstring("deleted"))
		})
	})

	Describe("isWorkflowInitiated", func() {
		It("should return false when no conditions are set", func() {
			result := isWorkflowInitiated(workflowRun)
			Expect(result).To(BeFalse())
		})

		It("should return true when workflow completed condition exists", func() {
			setWorkflowPendingCondition(workflowRun)
			result := isWorkflowInitiated(workflowRun)
			Expect(result).To(BeTrue())
		})
	})

	Describe("isWorkflowCompleted", func() {
		It("should return false when workflow is not completed", func() {
			setWorkflowPendingCondition(workflowRun)
			result := isWorkflowCompleted(workflowRun)
			Expect(result).To(BeFalse())
		})

		It("should return true when workflow succeeded", func() {
			setWorkflowSucceededCondition(workflowRun)
			result := isWorkflowCompleted(workflowRun)
			Expect(result).To(BeTrue())
		})

		It("should return true when workflow failed", func() {
			setWorkflowFailedCondition(workflowRun)
			result := isWorkflowCompleted(workflowRun)
			Expect(result).To(BeTrue())
		})
	})

	Describe("isWorkflowSucceeded", func() {
		It("should return false when workflow has not succeeded", func() {
			setWorkflowRunningCondition(workflowRun)
			result := isWorkflowSucceeded(workflowRun)
			Expect(result).To(BeFalse())
		})

		It("should return true when workflow succeeded", func() {
			setWorkflowSucceededCondition(workflowRun)
			result := isWorkflowSucceeded(workflowRun)
			Expect(result).To(BeTrue())
		})

		It("should return false when workflow failed", func() {
			setWorkflowFailedCondition(workflowRun)
			result := isWorkflowSucceeded(workflowRun)
			Expect(result).To(BeFalse())
		})
	})

})

// Helper function to find a condition by type
func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

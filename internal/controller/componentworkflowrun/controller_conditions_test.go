// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentworkflowrun

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("Condition Functions", func() {
	var componentWorkflowRun *openchoreodevv1alpha1.ComponentWorkflowRun

	BeforeEach(func() {
		componentWorkflowRun = &openchoreodevv1alpha1.ComponentWorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-run",
				Namespace:  "default",
				Generation: 1,
			},
			Status: openchoreodevv1alpha1.ComponentWorkflowRunStatus{
				Conditions: []metav1.Condition{},
			},
		}
	})

	Describe("setWorkflowPendingCondition", func() {
		It("should set workflow pending condition", func() {
			setWorkflowPendingCondition(componentWorkflowRun)

			Expect(componentWorkflowRun.Status.Conditions).To(HaveLen(1))
			condition := componentWorkflowRun.Status.Conditions[0]
			Expect(condition.Type).To(Equal(string(ConditionWorkflowCompleted)))
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(string(ReasonWorkflowPending)))
			Expect(condition.Message).To(ContainSubstring("has not completed yet"))
			Expect(condition.ObservedGeneration).To(Equal(int64(1)))
		})
	})

	Describe("setWorkflowRunningCondition", func() {
		It("should set workflow running condition", func() {
			setWorkflowRunningCondition(componentWorkflowRun)

			Expect(componentWorkflowRun.Status.Conditions).To(HaveLen(1))
			condition := componentWorkflowRun.Status.Conditions[0]
			Expect(condition.Type).To(Equal(string(ConditionWorkflowRunning)))
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal(string(ReasonWorkflowRunning)))
			Expect(condition.Message).To(ContainSubstring("running"))
		})
	})

	Describe("setWorkflowSucceededCondition", func() {
		It("should set workflow succeeded conditions", func() {
			setWorkflowSucceededCondition(componentWorkflowRun)

			Expect(componentWorkflowRun.Status.Conditions).To(HaveLen(3))

			runningCondition := findCondition(componentWorkflowRun.Status.Conditions, string(ConditionWorkflowRunning))
			Expect(runningCondition).NotTo(BeNil())
			Expect(runningCondition.Status).To(Equal(metav1.ConditionFalse))

			succeededCondition := findCondition(componentWorkflowRun.Status.Conditions, string(ConditionWorkflowSucceeded))
			Expect(succeededCondition).NotTo(BeNil())
			Expect(succeededCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(succeededCondition.Reason).To(Equal(string(ReasonWorkflowSucceeded)))

			completedCondition := findCondition(componentWorkflowRun.Status.Conditions, string(ConditionWorkflowCompleted))
			Expect(completedCondition).NotTo(BeNil())
			Expect(completedCondition.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Describe("setWorkflowFailedCondition", func() {
		It("should set workflow failed conditions", func() {
			setWorkflowFailedCondition(componentWorkflowRun)

			Expect(componentWorkflowRun.Status.Conditions).To(HaveLen(3))

			runningCondition := findCondition(componentWorkflowRun.Status.Conditions, string(ConditionWorkflowRunning))
			Expect(runningCondition).NotTo(BeNil())
			Expect(runningCondition.Status).To(Equal(metav1.ConditionFalse))

			failedCondition := findCondition(componentWorkflowRun.Status.Conditions, string(ConditionWorkflowFailed))
			Expect(failedCondition).NotTo(BeNil())
			Expect(failedCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(failedCondition.Reason).To(Equal(string(ReasonWorkflowFailed)))

			completedCondition := findCondition(componentWorkflowRun.Status.Conditions, string(ConditionWorkflowCompleted))
			Expect(completedCondition).NotTo(BeNil())
			Expect(completedCondition.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Describe("setWorkflowNotFoundCondition", func() {
		It("should set workflow not found conditions", func() {
			setWorkflowNotFoundCondition(componentWorkflowRun)

			Expect(componentWorkflowRun.Status.Conditions).To(HaveLen(2))

			runningCondition := findCondition(componentWorkflowRun.Status.Conditions, string(ConditionWorkflowRunning))
			Expect(runningCondition).NotTo(BeNil())
			Expect(runningCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(runningCondition.Message).To(ContainSubstring("not found"))

			completedCondition := findCondition(componentWorkflowRun.Status.Conditions, string(ConditionWorkflowCompleted))
			Expect(completedCondition).NotTo(BeNil())
			Expect(completedCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(completedCondition.Message).To(ContainSubstring("deleted"))
		})
	})

	Describe("setWorkloadUpdatedCondition", func() {
		It("should set workload updated condition", func() {
			setWorkloadUpdatedCondition(componentWorkflowRun)

			Expect(componentWorkflowRun.Status.Conditions).To(HaveLen(1))
			condition := componentWorkflowRun.Status.Conditions[0]
			Expect(condition.Type).To(Equal(string(ConditionWorkloadUpdated)))
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal(string(ReasonWorkloadUpdated)))
			Expect(condition.Message).To(ContainSubstring("successfully"))
		})
	})

	Describe("setWorkloadUpdateFailedCondition", func() {
		It("should set workload update failed condition", func() {
			setWorkloadUpdateFailedCondition(componentWorkflowRun)

			Expect(componentWorkflowRun.Status.Conditions).To(HaveLen(1))
			condition := componentWorkflowRun.Status.Conditions[0]
			Expect(condition.Type).To(Equal(string(ConditionWorkloadUpdated)))
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(string(ReasonWorkloadUpdateFailed)))
			Expect(condition.Message).To(ContainSubstring("Failed"))
		})
	})

	Describe("isWorkflowInitiated", func() {
		It("should return false when no conditions are set", func() {
			result := isWorkflowInitiated(componentWorkflowRun)
			Expect(result).To(BeFalse())
		})

		It("should return true when workflow completed condition exists", func() {
			setWorkflowPendingCondition(componentWorkflowRun)
			result := isWorkflowInitiated(componentWorkflowRun)
			Expect(result).To(BeTrue())
		})
	})

	Describe("isWorkflowCompleted", func() {
		It("should return false when workflow is not completed", func() {
			setWorkflowPendingCondition(componentWorkflowRun)
			result := isWorkflowCompleted(componentWorkflowRun)
			Expect(result).To(BeFalse())
		})

		It("should return true when workflow succeeded", func() {
			setWorkflowSucceededCondition(componentWorkflowRun)
			result := isWorkflowCompleted(componentWorkflowRun)
			Expect(result).To(BeTrue())
		})

		It("should return true when workflow failed", func() {
			setWorkflowFailedCondition(componentWorkflowRun)
			result := isWorkflowCompleted(componentWorkflowRun)
			Expect(result).To(BeTrue())
		})
	})

	Describe("isWorkflowSucceeded", func() {
		It("should return false when workflow has not succeeded", func() {
			setWorkflowRunningCondition(componentWorkflowRun)
			result := isWorkflowSucceeded(componentWorkflowRun)
			Expect(result).To(BeFalse())
		})

		It("should return true when workflow succeeded", func() {
			setWorkflowSucceededCondition(componentWorkflowRun)
			result := isWorkflowSucceeded(componentWorkflowRun)
			Expect(result).To(BeTrue())
		})

		It("should return false when workflow failed", func() {
			setWorkflowFailedCondition(componentWorkflowRun)
			result := isWorkflowSucceeded(componentWorkflowRun)
			Expect(result).To(BeFalse())
		})
	})

	Describe("isWorkloadUpdated", func() {
		It("should return false when workload is not updated", func() {
			result := isWorkloadUpdated(componentWorkflowRun)
			Expect(result).To(BeFalse())
		})

		It("should return true when workload is updated", func() {
			setWorkloadUpdatedCondition(componentWorkflowRun)
			result := isWorkloadUpdated(componentWorkflowRun)
			Expect(result).To(BeTrue())
		})

		It("should return false when workload update failed", func() {
			setWorkloadUpdateFailedCondition(componentWorkflowRun)
			result := isWorkloadUpdated(componentWorkflowRun)
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

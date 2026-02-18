// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Unit tests for condition helper functions that don't require k8s test environment

func newTestBuild(generation int64) *openchoreov1alpha1.Build {
	b := &openchoreov1alpha1.Build{}
	b.Generation = generation
	return b
}

// ---- NewBuildInitiatedCondition ----

func TestNewBuildInitiatedCondition(t *testing.T) {
	cond := NewBuildInitiatedCondition(5)
	if cond.Type != string(ConditionBuildInitiated) {
		t.Errorf("Type = %q, want %q", cond.Type, ConditionBuildInitiated)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %q, want True", cond.Status)
	}
	if cond.Reason != string(ReasonBuildInitiated) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonBuildInitiated)
	}
	if cond.ObservedGeneration != 5 {
		t.Errorf("ObservedGeneration = %d, want 5", cond.ObservedGeneration)
	}
}

// ---- NewBuildTriggeredCondition ----

func TestNewBuildTriggeredCondition(t *testing.T) {
	cond := NewBuildTriggeredCondition(3)
	if cond.Type != string(ConditionBuildTriggered) {
		t.Errorf("Type = %q, want %q", cond.Type, ConditionBuildTriggered)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %q, want True", cond.Status)
	}
	if cond.Reason != string(ReasonBuildTriggered) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonBuildTriggered)
	}
	if cond.ObservedGeneration != 3 {
		t.Errorf("ObservedGeneration = %d, want 3", cond.ObservedGeneration)
	}
}

// ---- NewBuildCompletedCondition ----

func TestNewBuildCompletedCondition(t *testing.T) {
	cond := NewBuildCompletedCondition(2)
	if cond.Type != string(ConditionBuildCompleted) {
		t.Errorf("Type = %q, want %q", cond.Type, ConditionBuildCompleted)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %q, want True", cond.Status)
	}
	if cond.Reason != string(ReasonBuildCompleted) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonBuildCompleted)
	}
}

// ---- NewBuildFailedCondition ----

func TestNewBuildFailedCondition(t *testing.T) {
	cond := NewBuildFailedCondition(1)
	if cond.Type != string(ConditionBuildCompleted) {
		t.Errorf("Type = %q, want %q", cond.Type, ConditionBuildCompleted)
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("Status = %q, want False", cond.Status)
	}
	if cond.Reason != string(ReasonBuildFailed) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonBuildFailed)
	}
}

// ---- NewBuildInProgressCondition ----

func TestNewBuildInProgressCondition(t *testing.T) {
	cond := NewBuildInProgressCondition(4)
	if cond.Type != string(ConditionBuildCompleted) {
		t.Errorf("Type = %q, want %q", cond.Type, ConditionBuildCompleted)
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("Status = %q, want False", cond.Status)
	}
	if cond.Reason != string(ReasonBuildInProgress) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonBuildInProgress)
	}
}

// ---- NewWorkloadUpdatedCondition ----

func TestNewWorkloadUpdatedCondition(t *testing.T) {
	cond := NewWorkloadUpdatedCondition(7)
	if cond.Type != string(ConditionWorkloadUpdated) {
		t.Errorf("Type = %q, want %q", cond.Type, ConditionWorkloadUpdated)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %q, want True", cond.Status)
	}
	if cond.Reason != string(ReasonWorkloadUpdated) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonWorkloadUpdated)
	}
	if cond.ObservedGeneration != 7 {
		t.Errorf("ObservedGeneration = %d, want 7", cond.ObservedGeneration)
	}
}

// ---- NewWorkloadUpdateFailedCondition ----

func TestNewWorkloadUpdateFailedCondition(t *testing.T) {
	cond := NewWorkloadUpdateFailedCondition(9)
	if cond.Type != string(ConditionWorkloadUpdated) {
		t.Errorf("Type = %q, want %q", cond.Type, ConditionWorkloadUpdated)
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("Status = %q, want False", cond.Status)
	}
	if cond.Reason != string(ReasonWorkloadUpdateFailed) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonWorkloadUpdateFailed)
	}
}

// ---- setBuildInitiatedCondition ----

func TestSetBuildInitiatedCondition(t *testing.T) {
	build := newTestBuild(1)
	setBuildInitiatedCondition(build)
	if !isBuildInitiated(build) {
		t.Error("isBuildInitiated should be true after setBuildInitiatedCondition")
	}
}

// ---- setBuildTriggeredCondition ----

func TestSetBuildTriggeredCondition(t *testing.T) {
	build := newTestBuild(2)
	setBuildTriggeredCondition(build)
	// Verify the condition was set
	found := false
	for _, c := range build.Status.Conditions {
		if c.Type == string(ConditionBuildTriggered) && c.Status == metav1.ConditionTrue {
			found = true
			break
		}
	}
	if !found {
		t.Error("setBuildTriggeredCondition did not set BuildTriggered=True")
	}
}

// ---- setBuildCompletedCondition ----

func TestSetBuildCompletedCondition_WithMessage(t *testing.T) {
	build := newTestBuild(3)
	setBuildCompletedCondition(build, "custom message")
	found := false
	for _, c := range build.Status.Conditions {
		if c.Type == string(ConditionBuildCompleted) && c.Status == metav1.ConditionTrue {
			if c.Message != "custom message" {
				t.Errorf("Message = %q, want %q", c.Message, "custom message")
			}
			found = true
		}
	}
	if !found {
		t.Error("setBuildCompletedCondition did not set BuildCompleted=True")
	}
}

func TestSetBuildCompletedCondition_EmptyMessage(t *testing.T) {
	build := newTestBuild(3)
	setBuildCompletedCondition(build, "")
	// With empty message, it should use the default message
	found := false
	for _, c := range build.Status.Conditions {
		if c.Type == string(ConditionBuildCompleted) {
			found = true
		}
	}
	if !found {
		t.Error("setBuildCompletedCondition with empty message did not set condition")
	}
}

// ---- setBuildFailedCondition ----

func TestSetBuildFailedCondition(t *testing.T) {
	build := newTestBuild(4)
	setBuildFailedCondition(build, ReasonBuildFailed, "build failed message")
	found := false
	for _, c := range build.Status.Conditions {
		if c.Type == string(ConditionBuildCompleted) && c.Status == metav1.ConditionFalse {
			if c.Reason != string(ReasonBuildFailed) {
				t.Errorf("Reason = %q, want %q", c.Reason, ReasonBuildFailed)
			}
			found = true
		}
	}
	if !found {
		t.Error("setBuildFailedCondition did not set condition")
	}
}

func TestSetBuildFailedCondition_EmptyReasonAndMessage(t *testing.T) {
	build := newTestBuild(4)
	setBuildFailedCondition(build, "", "")
	found := false
	for _, c := range build.Status.Conditions {
		if c.Type == string(ConditionBuildCompleted) && c.Status == metav1.ConditionFalse {
			found = true
		}
	}
	if !found {
		t.Error("setBuildFailedCondition with empty params did not set condition")
	}
}

// ---- setBuildInProgressCondition ----

func TestSetBuildInProgressCondition(t *testing.T) {
	build := newTestBuild(5)
	setBuildInProgressCondition(build)
	found := false
	for _, c := range build.Status.Conditions {
		if c.Type == string(ConditionBuildCompleted) && c.Reason == string(ReasonBuildInProgress) {
			found = true
		}
	}
	if !found {
		t.Error("setBuildInProgressCondition did not set BuildInProgress condition")
	}
}

// ---- isBuildInitiated ----

func TestIsBuildInitiated_False(t *testing.T) {
	build := newTestBuild(1)
	if isBuildInitiated(build) {
		t.Error("isBuildInitiated should return false when no conditions set")
	}
}

func TestIsBuildInitiated_True(t *testing.T) {
	build := newTestBuild(1)
	setBuildInitiatedCondition(build)
	if !isBuildInitiated(build) {
		t.Error("isBuildInitiated should return true after condition set")
	}
}

// ---- isBuildCompleted ----

func TestIsBuildCompleted_False_NoConditions(t *testing.T) {
	build := newTestBuild(1)
	if isBuildCompleted(build) {
		t.Error("isBuildCompleted should return false with no conditions")
	}
}

func TestIsBuildCompleted_True_AfterWorkloadUpdated(t *testing.T) {
	build := newTestBuild(1)
	// Set WorkloadUpdated = True with ReasonWorkloadUpdated to make isBuildCompleted return true
	build.Status.Conditions = []metav1.Condition{
		{
			Type:   string(ConditionWorkloadUpdated),
			Status: metav1.ConditionTrue,
			Reason: string(ReasonWorkloadUpdated),
		},
	}
	if !isBuildCompleted(build) {
		t.Error("isBuildCompleted should return true when WorkloadUpdated condition is True")
	}
}

func TestIsBuildCompleted_False_WorkloadUpdateFailed(t *testing.T) {
	build := newTestBuild(1)
	build.Status.Conditions = []metav1.Condition{
		{
			Type:   string(ConditionWorkloadUpdated),
			Status: metav1.ConditionFalse,
			Reason: string(ReasonWorkloadUpdateFailed),
		},
	}
	if isBuildCompleted(build) {
		t.Error("isBuildCompleted should return false when WorkloadUpdated=False")
	}
}

// ---- isBuildWorkflowSucceeded ----

func TestIsBuildWorkflowSucceeded_False_NoConditions(t *testing.T) {
	build := newTestBuild(1)
	if isBuildWorkflowSucceeded(build) {
		t.Error("isBuildWorkflowSucceeded should return false with no conditions")
	}
}

func TestIsBuildWorkflowSucceeded_True(t *testing.T) {
	build := newTestBuild(1)
	setBuildCompletedCondition(build, "success")
	if !isBuildWorkflowSucceeded(build) {
		t.Error("isBuildWorkflowSucceeded should return true after setBuildCompletedCondition")
	}
}

func TestIsBuildWorkflowSucceeded_False_BuildFailed(t *testing.T) {
	build := newTestBuild(1)
	setBuildFailedCondition(build, ReasonBuildFailed, "failed")
	if isBuildWorkflowSucceeded(build) {
		t.Error("isBuildWorkflowSucceeded should return false after setBuildFailedCondition")
	}
}

// ---- shouldIgnoreReconcile ----

func TestShouldIgnoreReconcile_False_NoConditions(t *testing.T) {
	build := newTestBuild(1)
	if shouldIgnoreReconcile(build) {
		t.Error("shouldIgnoreReconcile should return false with no conditions")
	}
}

func TestShouldIgnoreReconcile_True_WhenCompleted(t *testing.T) {
	build := newTestBuild(1)
	// Set up the WorkloadUpdated=True condition that makes isBuildCompleted return true
	build.Status.Conditions = []metav1.Condition{
		{
			Type:   string(ConditionWorkloadUpdated),
			Status: metav1.ConditionTrue,
			Reason: string(ReasonWorkloadUpdated),
		},
	}
	if !shouldIgnoreReconcile(build) {
		t.Error("shouldIgnoreReconcile should return true when build is completed")
	}
}

func TestShouldIgnoreReconcile_False_WhenInProgress(t *testing.T) {
	build := newTestBuild(1)
	setBuildInProgressCondition(build)
	if shouldIgnoreReconcile(build) {
		t.Error("shouldIgnoreReconcile should return false when build is in progress")
	}
}

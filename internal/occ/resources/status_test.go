// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// helper to create a condition
func makeCondition(condType, status, reason, message string, t time.Time) metav1.Condition {
	return metav1.Condition{
		Type:               condType,
		Status:             metav1.ConditionStatus(status),
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(t),
	}
}

// ---- GetResourceStatus ----

func TestGetResourceStatus_NoConditions(t *testing.T) {
	got := GetResourceStatus(nil, []string{"Ready"}, testStatusPending, "Ready", "NotReady")
	if got != testStatusPending {
		t.Errorf("GetResourceStatus(no conditions) = %q, want Pending", got)
	}
}

func TestGetResourceStatus_PriorityConditionTrue(t *testing.T) {
	conditions := []metav1.Condition{
		makeCondition("Ready", "True", "AllGood", "", time.Now()),
	}
	got := GetResourceStatus(conditions, []string{"Ready"}, testStatusPending, "Ready", "NotReady")
	if got != testStatusReadyAllGood {
		t.Errorf("GetResourceStatus(Ready=True) = %q, want Ready (AllGood)", got)
	}
}

func TestGetResourceStatus_PriorityConditionFalse(t *testing.T) {
	conditions := []metav1.Condition{
		makeCondition("Ready", "False", "Error", "something failed", time.Now()),
	}
	got := GetResourceStatus(conditions, []string{"Ready"}, testStatusPending, "Ready", "NotReady")
	if got != "NotReady (Error: something failed)" {
		t.Errorf("GetResourceStatus(Ready=False) = %q, want NotReady (Error: something failed)", got)
	}
}

func TestGetResourceStatus_NoPriorityConditionMatch_FallsBackToLatest(t *testing.T) {
	now := time.Now()
	past := now.Add(-1 * time.Hour)
	conditions := []metav1.Condition{
		makeCondition("OldCond", "False", "OldReason", "old msg", past),
		makeCondition("NewCond", "True", "NewReason", "new msg", now),
	}
	// "Ready" is not in the conditions, so it should fall back to latest
	got := GetResourceStatus(conditions, []string{"Ready"}, testStatusPending, "Ready", "NotReady")
	// Latest condition is NewCond=True
	if got != "NewCond: NewReason" {
		t.Errorf("GetResourceStatus(fallback) = %q, want NewCond: NewReason", got)
	}
}

func TestGetResourceStatus_NoPriorityConditionMatch_FalseLatest(t *testing.T) {
	now := time.Now()
	conditions := []metav1.Condition{
		makeCondition("OldCond", "True", "OldReason", "old msg", now.Add(-1*time.Hour)),
		makeCondition("NewCond", "False", "NewReason", "new msg", now),
	}
	got := GetResourceStatus(conditions, []string{"Ready"}, testStatusPending, "Ready", "NotReady")
	if got != "NewCond: False - new msg" {
		t.Errorf("GetResourceStatus(fallback false) = %q, want NewCond: False - new msg", got)
	}
}

// ---- GetStatusForConditionGetter ----

type mockConditionGetter struct {
	conditions []metav1.Condition
}

func (m *mockConditionGetter) GetConditions() []metav1.Condition {
	return m.conditions
}

func TestGetStatusForConditionGetter_NoConditions(t *testing.T) {
	getter := &mockConditionGetter{conditions: nil}
	got := GetStatusForConditionGetter(getter, []string{"Ready"}, testStatusPending, "Ready", "NotReady")
	if got != testStatusPending {
		t.Errorf("GetStatusForConditionGetter(no conditions) = %q, want Pending", got)
	}
}

func TestGetStatusForConditionGetter_ReadyTrue(t *testing.T) {
	conditions := []metav1.Condition{
		makeCondition("Ready", "True", "AllSet", "", time.Now()),
	}
	getter := &mockConditionGetter{conditions: conditions}
	got := GetStatusForConditionGetter(getter, []string{"Ready"}, testStatusPending, "Running", "Failed")
	if got != "Running (AllSet)" {
		t.Errorf("GetStatusForConditionGetter(Ready=True) = %q, want Running (AllSet)", got)
	}
}

// ---- GetReadyStatus ----

func TestGetReadyStatus_NoConditions(t *testing.T) {
	got := GetReadyStatus(nil, testStatusPending, "Ready", "NotReady")
	if got != testStatusPending {
		t.Errorf("GetReadyStatus(no conditions) = %q, want Pending", got)
	}
}

func TestGetReadyStatus_ReadyTrue(t *testing.T) {
	conditions := []metav1.Condition{
		makeCondition("Ready", "True", "AllGood", "", time.Now()),
	}
	got := GetReadyStatus(conditions, testStatusPending, "Ready", "NotReady")
	if got != testStatusReadyAllGood {
		t.Errorf("GetReadyStatus(Ready=True) = %q, want Ready (AllGood)", got)
	}
}

func TestGetReadyStatus_ReadyFalse(t *testing.T) {
	conditions := []metav1.Condition{
		makeCondition("Ready", "False", "Error", "something went wrong", time.Now()),
	}
	got := GetReadyStatus(conditions, testStatusPending, "Ready", "NotReady")
	if got != "NotReady (Error: something went wrong)" {
		t.Errorf("GetReadyStatus(Ready=False) = %q, want NotReady (Error: something went wrong)", got)
	}
}

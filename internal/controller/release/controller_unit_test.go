// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package release

import (
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Unit tests for release condition helper functions

// ---- NewReleaseFinalizingCondition ----

func TestNewReleaseFinalizingCondition(t *testing.T) {
	cond := NewReleaseFinalizingCondition(3)
	if cond.Type != string(ConditionFinalizing) {
		t.Errorf("Type = %q, want %q", cond.Type, ConditionFinalizing)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %q, want True", cond.Status)
	}
	if cond.Reason != string(ReasonCleanupInProgress) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonCleanupInProgress)
	}
	if cond.ObservedGeneration != 3 {
		t.Errorf("ObservedGeneration = %d, want 3", cond.ObservedGeneration)
	}
}

// ---- NewReleaseCleanupFailedCondition ----

func TestNewReleaseCleanupFailedCondition(t *testing.T) {
	err := errors.New("cleanup failed: network timeout")
	cond := NewReleaseCleanupFailedCondition(7, err)
	if cond.Type != string(ConditionFinalizing) {
		t.Errorf("Type = %q, want %q", cond.Type, ConditionFinalizing)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %q, want True", cond.Status)
	}
	if cond.Reason != string(ReasonCleanupFailed) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonCleanupFailed)
	}
	if cond.Message != err.Error() {
		t.Errorf("Message = %q, want %q", cond.Message, err.Error())
	}
	if cond.ObservedGeneration != 7 {
		t.Errorf("ObservedGeneration = %d, want 7", cond.ObservedGeneration)
	}
}

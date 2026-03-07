// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewEnvironmentReadyCondition(t *testing.T) {
	tests := []struct {
		name       string
		generation int64
	}{
		{name: "generation 0", generation: 0},
		{name: "generation 1", generation: 1},
		{name: "large generation", generation: 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewEnvironmentReadyCondition(tt.generation)
			if cond.Type != ConditionReady.String() {
				t.Errorf("expected type %q, got %q", ConditionReady.String(), cond.Type)
			}
			if cond.Status != metav1.ConditionTrue {
				t.Errorf("expected status %q, got %q", metav1.ConditionTrue, cond.Status)
			}
			if cond.Reason != "EnvironmentReady" {
				t.Errorf("expected reason %q, got %q", "EnvironmentReady", cond.Reason)
			}
			if cond.Message != "Environment is ready" {
				t.Errorf("expected message %q, got %q", "Environment is ready", cond.Message)
			}
			if cond.ObservedGeneration != tt.generation {
				t.Errorf("expected observedGeneration %d, got %d", tt.generation, cond.ObservedGeneration)
			}
			if cond.LastTransitionTime.IsZero() {
				t.Error("expected LastTransitionTime to be set")
			}
		})
	}
}

func TestNewEnvironmentFinalizingCondition(t *testing.T) {
	tests := []struct {
		name       string
		generation int64
	}{
		{name: "generation 0", generation: 0},
		{name: "generation 5", generation: 5},
		{name: "generation 99", generation: 99},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewEnvironmentFinalizingCondition(tt.generation)
			if cond.Type != ConditionReady.String() {
				t.Errorf("expected type %q, got %q", ConditionReady.String(), cond.Type)
			}
			if cond.Status != metav1.ConditionFalse {
				t.Errorf("expected status %q, got %q", metav1.ConditionFalse, cond.Status)
			}
			if cond.Reason != "EnvironmentFinalizing" {
				t.Errorf("expected reason %q, got %q", "EnvironmentFinalizing", cond.Reason)
			}
			if cond.Message != "Environment is finalizing" {
				t.Errorf("expected message %q, got %q", "Environment is finalizing", cond.Message)
			}
			if cond.ObservedGeneration != tt.generation {
				t.Errorf("expected observedGeneration %d, got %d", tt.generation, cond.ObservedGeneration)
			}
			if cond.LastTransitionTime.IsZero() {
				t.Error("expected LastTransitionTime to be set")
			}
		})
	}
}

func TestConditionTypeConstants(t *testing.T) {
	const wantReady = "Ready"
	if ConditionReady.String() != wantReady {
		t.Errorf("expected ConditionReady = %q, got %q", wantReady, ConditionReady.String())
	}
}

func TestConditionReasonConstants(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   string
	}{
		{name: "ReasonDeploymentReady", reason: string(ReasonDeploymentReady), want: "EnvironmentReady"},
		{name: "ReasonEnvironmentFinalizing", reason: string(ReasonEnvironmentFinalizing), want: "EnvironmentFinalizing"},
		{name: "ReasonDeletionBlocked", reason: string(ReasonDeletionBlocked), want: "DeletionBlocked"},
		{name: "ReasonReleaseBindingsPending", reason: string(ReasonReleaseBindingsPending), want: "ReleaseBindingsPending"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.reason != tt.want {
				t.Errorf("expected %q, got %q", tt.want, tt.reason)
			}
		})
	}
}

func TestNewDeletionBlockedCondition(t *testing.T) {
	tests := []struct {
		name       string
		generation int64
		message    string
	}{
		{name: "generation 1", generation: 1, message: "blocked by pipeline foo"},
		{name: "generation 5", generation: 5, message: "blocked by pipeline bar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewDeletionBlockedCondition(tt.generation, tt.message)
			if cond.Type != ConditionReady.String() {
				t.Errorf("expected type %q, got %q", ConditionReady.String(), cond.Type)
			}
			if cond.Status != metav1.ConditionFalse {
				t.Errorf("expected status %q, got %q", metav1.ConditionFalse, cond.Status)
			}
			if cond.Reason != string(ReasonDeletionBlocked) {
				t.Errorf("expected reason %q, got %q", ReasonDeletionBlocked, cond.Reason)
			}
			if cond.Message != tt.message {
				t.Errorf("expected message %q, got %q", tt.message, cond.Message)
			}
			if cond.ObservedGeneration != tt.generation {
				t.Errorf("expected observedGeneration %d, got %d", tt.generation, cond.ObservedGeneration)
			}
			if cond.LastTransitionTime.IsZero() {
				t.Error("expected LastTransitionTime to be set")
			}
		})
	}
}

func TestNewReleaseBindingsPendingCondition(t *testing.T) {
	tests := []struct {
		name       string
		generation int64
		message    string
	}{
		{name: "generation 1", generation: 1, message: "Waiting for release bindings to be removed"},
		{name: "generation 3", generation: 3, message: "2 release bindings still exist"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewReleaseBindingsPendingCondition(tt.generation, tt.message)
			if cond.Type != ConditionReady.String() {
				t.Errorf("expected type %q, got %q", ConditionReady.String(), cond.Type)
			}
			if cond.Status != metav1.ConditionFalse {
				t.Errorf("expected status %q, got %q", metav1.ConditionFalse, cond.Status)
			}
			if cond.Reason != string(ReasonReleaseBindingsPending) {
				t.Errorf("expected reason %q, got %q", ReasonReleaseBindingsPending, cond.Reason)
			}
			if cond.Message != tt.message {
				t.Errorf("expected message %q, got %q", tt.message, cond.Message)
			}
			if cond.ObservedGeneration != tt.generation {
				t.Errorf("expected observedGeneration %d, got %d", tt.generation, cond.ObservedGeneration)
			}
			if cond.LastTransitionTime.IsZero() {
				t.Error("expected LastTransitionTime to be set")
			}
		})
	}
}

func TestReadyAndFinalizingConditionsDiffer(t *testing.T) {
	ready := NewEnvironmentReadyCondition(1)
	finalizing := NewEnvironmentFinalizingCondition(1)

	if ready.Status == finalizing.Status {
		t.Errorf("Ready and Finalizing conditions should have different statuses, both are %q", ready.Status)
	}
	if ready.Reason == finalizing.Reason {
		t.Errorf("Ready and Finalizing conditions should have different reasons, both are %q", ready.Reason)
	}
	if ready.Type != finalizing.Type {
		t.Errorf("Ready and Finalizing conditions should share the same type, got %q and %q", ready.Type, finalizing.Type)
	}
}

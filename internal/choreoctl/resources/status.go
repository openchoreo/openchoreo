// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConditionGetter is an interface for objects that can get conditions
type ConditionGetter interface {
	GetConditions() []metav1.Condition
}

// GetResourceStatus returns a human-readable status string based on the resource's conditions
// priorityConditions are checked in order, other conditions are considered if none of those match
// defaultStatus is returned if no conditions exist
func GetResourceStatus(
	conditions []metav1.Condition,
	priorityConditions []string,
	defaultStatus string,
	readyStatus string,
	notReadyStatus string,
) string {
	if len(conditions) == 0 {
		return defaultStatus
	}

	// Check priority conditions in order
	for _, condType := range priorityConditions {
		for _, condition := range conditions {
			if condition.Type == condType {
				if condition.Status == "True" {
					return fmt.Sprintf("%s (%s)", readyStatus, condition.Reason)
				}
				return fmt.Sprintf("%s (%s: %s)", notReadyStatus, condition.Reason, condition.Message)
			}
		}
	}

	// If no priority conditions match, find the most recent condition
	latest := conditions[0]
	for _, condition := range conditions[1:] {
		if condition.LastTransitionTime.After(latest.LastTransitionTime.Time) {
			latest = condition
		}
	}

	if latest.Status == "True" {
		return fmt.Sprintf("%s: %s", latest.Type, latest.Reason)
	}
	return fmt.Sprintf("%s: %s - %s", latest.Type, latest.Status, latest.Message)
}

// GetStatusForConditionGetter is a convenience wrapper for resources that implement ConditionGetter
func GetStatusForConditionGetter(
	resource ConditionGetter,
	priorityConditions []string,
	defaultStatus string,
	readyStatus string,
	notReadyStatus string,
) string {
	return GetResourceStatus(
		resource.GetConditions(),
		priorityConditions,
		defaultStatus,
		readyStatus,
		notReadyStatus,
	)
}

// GetReadyStatus is a specialized helper that focuses on the "Ready" condition,
// common across many Choreo resources
func GetReadyStatus(conditions []metav1.Condition, defaultStatus, readyStatus, notReadyStatus string) string {
	return GetResourceStatus(
		conditions,
		[]string{"Ready"},
		defaultStatus,
		readyStatus,
		notReadyStatus,
	)
}

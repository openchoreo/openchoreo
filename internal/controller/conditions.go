// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// This file contains the types and functions to manage the conditions in the Kubernetes objects.

// ConditionType represents the type of condition describing a specific state of the resource.
// Use CamelCase format (e.g., Ready, Available).
type ConditionType string

// String returns the string representation of the condition type.
func (c ConditionType) String() string {
	return string(c)
}

// ConditionReason represents the machine-readable reason for a condition's status.
// Use CamelCase format (e.g., MinimumReplicasUnavailable, MinimumReplicasMet).
type ConditionReason string

// ConditionedObject describes a Kubernetes resource that has a mutable
// Conditions field in its Status
type ConditionedObject interface {
	client.Object

	GetConditions() []metav1.Condition
	SetConditions(conditions []metav1.Condition)
}

// NewCondition creates a new condition with the last transition time set to the current time.
func NewCondition(conditionType ConditionType, status metav1.ConditionStatus, reason ConditionReason,
	message string, observedGeneration int64) metav1.Condition {
	return metav1.Condition{
		Type:               string(conditionType),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: observedGeneration,
	}
}

func MarkTrueCondition(obj ConditionedObject, ct ConditionType, reason ConditionReason, message string) (changed bool) {
	cond := NewCondition(ct, metav1.ConditionTrue, reason, message, obj.GetGeneration())
	conditions := obj.GetConditions()
	return meta.SetStatusCondition(&conditions, cond)
}

func MarkFalseCondition(obj ConditionedObject, ct ConditionType, reason ConditionReason, message string) (changed bool) {
	cond := NewCondition(ct, metav1.ConditionFalse, reason, message, obj.GetGeneration())
	conditions := obj.GetConditions()
	return meta.SetStatusCondition(&conditions, cond)
}

func MarkUnknownCondition(obj ConditionedObject, ct ConditionType, reason ConditionReason, message string) (changed bool) {
	cond := NewCondition(ct, metav1.ConditionUnknown, reason, message, obj.GetGeneration())
	conditions := obj.GetConditions()
	return meta.SetStatusCondition(&conditions, cond)
}

// NeedConditionUpdate checks if the conditions need to be updated based on the current and updated conditions.
func NeedConditionUpdate(currentConditions, updatedConditions []metav1.Condition) bool {
	// If the number of conditions is different, an update is needed
	if len(currentConditions) != len(updatedConditions) {
		return true
	}

	// Track seen conditions
	seenConditions := make(map[string]bool)

	// Check for changes in existing conditions and new conditions
	for _, updated := range updatedConditions {
		current := meta.FindStatusCondition(currentConditions, updated.Type)
		if current == nil {
			// New condition added
			return true
		}

		if current.Status != updated.Status ||
			current.Reason != updated.Reason ||
			current.Message != updated.Message ||
			current.ObservedGeneration != updated.ObservedGeneration {
			return true
		}

		seenConditions[updated.Type] = true
	}

	// Check for removed conditions
	for _, current := range currentConditions {
		if _, ok := seenConditions[current.Type]; !ok {
			// Condition removed
			return true
		}
	}

	return false
}

// UpdateStatusConditions will compare the current and updated conditions and update the status conditions if needed.
func UpdateStatusConditions[T ConditionedObject](
	ctx context.Context,
	c client.Client,
	current, updated T,
) error {
	// Update the conditions if needed
	if NeedConditionUpdate(current.GetConditions(), updated.GetConditions()) {
		// Create a copy of the object to avoid modifying the original object to avoid updating
		// other status fields that might have been updated in the updated object.
		newObj := current.DeepCopyObject().(ConditionedObject)
		newObj.SetConditions(updated.GetConditions())
		return c.Status().Update(ctx, newObj)
	}
	return nil
}

// UpdateStatusConditionsAndRequeue updates status conditions and requests a requeue.
// This indicates that the controller should requeue the request for further processing.
// It returns an error if the status update fails.
func UpdateStatusConditionsAndRequeue[T ConditionedObject](
	ctx context.Context, c client.Client, current, updated T,
) (ctrl.Result, error) {
	if err := UpdateStatusConditions(ctx, c, current, updated); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

// UpdateStatusConditionsAndReturn updates status conditions without requeuing.
// It returns an error if the update fails.
func UpdateStatusConditionsAndReturn[T ConditionedObject](
	ctx context.Context, c client.Client, current, updated T,
) (ctrl.Result, error) {
	if err := UpdateStatusConditions(ctx, c, current, updated); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// UpdateStatusConditionsAndReturnError updates status conditions and returns the given error.
// It returns an update error if the conditions update fails.
// It prioritizes the status update error over the provided error, if any.
func UpdateStatusConditionsAndReturnError[T ConditionedObject](
	ctx context.Context, c client.Client, current, updated T, err error,
) (ctrl.Result, error) {
	if updateErr := UpdateStatusConditions(ctx, c, current, updated); updateErr != nil {
		return ctrl.Result{}, updateErr
	}
	return ctrl.Result{}, err
}

// UpdateStatusConditionsAndRequeueAfter updates status conditions and requeues after the specified duration.
// It returns an error if the status update fails.
func UpdateStatusConditionsAndRequeueAfter[T ConditionedObject](
	ctx context.Context, c client.Client, current, updated T, duration time.Duration,
) (ctrl.Result, error) {
	if err := UpdateStatusConditions(ctx, c, current, updated); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: duration}, nil
}

// UpdateStatusConditionsWithPatch updates status conditions using a patch operation with retry on conflicts.
// This is more robust for handling concurrent updates to the same resource.
func UpdateStatusConditionsWithPatch[T ConditionedObject](
	ctx context.Context,
	c client.Client,
	current, updated T,
) error {
	// Only update if there are actually changes
	if !NeedConditionUpdate(current.GetConditions(), updated.GetConditions()) {
		return nil
	}

	// Maximum number of retries for conflict resolution
	maxRetries := 5

	for i := 0; i < maxRetries; i++ {
		// Get a fresh copy of the object to ensure we have the latest version
		latestObj := current.DeepCopyObject().(T)
		key := client.ObjectKeyFromObject(current)

		if err := c.Get(ctx, key, latestObj); err != nil {
			if apierrors.IsNotFound(err) {
				// Object no longer exists, nothing to update
				return nil
			}
			return fmt.Errorf("failed to get latest object: %w", err)
		}

		// Create a patch from the latest version
		patch := client.MergeFrom(latestObj.DeepCopyObject().(T))

		// Only update the conditions, preserving other status fields
		latestObj.SetConditions(updated.GetConditions())

		// Apply the patch to the status subresource
		if err := c.Status().Patch(ctx, latestObj, patch); err != nil {
			if apierrors.IsConflict(err) {
				// On conflict, wait with exponential backoff and retry
				backoffTime := time.Millisecond * 100 * time.Duration(1<<uint(i))
				time.Sleep(backoffTime)
				continue
			}
			return fmt.Errorf("failed to patch status: %w", err)
		}

		// Success
		return nil
	}

	return fmt.Errorf("exceeded maximum retries (%d) resolving conflicts when updating status", maxRetries)
}

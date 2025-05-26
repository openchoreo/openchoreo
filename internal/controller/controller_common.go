// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// States for conditions
const (
	TypeAccepted    = "Accepted"
	TypeProgressing = "Progressing"
	TypeAvailable   = "Available"
	TypeCreated     = "Created"
	TypeReady       = "Ready"
	TypeTerminating = "Terminating"
)

// UpdateCondition updates or adds a condition to any resource that has a Status with Conditions
func UpdateCondition(
	ctx context.Context,
	c client.StatusWriter,
	resource client.Object,
	conditions *[]metav1.Condition,
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	logger := log.FromContext(ctx)

	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: resource.GetGeneration(),
	}

	changed := meta.SetStatusCondition(conditions, condition)
	if changed {
		logger.Info("Updating Resource status",
			"Resource.Kind", resource.GetObjectKind().GroupVersionKind().Kind,
			"Resource.Name", resource.GetName())

		if err := c.Update(ctx, resource); err != nil {
			logger.Error(err, "Failed to update resource status",
				"Resource.Kind", resource.GetObjectKind().GroupVersionKind().Kind,
				"Resource.Name", resource.GetName())
			return err
		}

		logger.Info("Updated Resource status",
			"Resource.Kind", resource.GetObjectKind().GroupVersionKind().Kind,
			"Resource.Name", resource.GetName())
	}
	return nil
}

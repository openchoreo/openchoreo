/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

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

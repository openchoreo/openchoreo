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

package project

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// ConditionCreated represents whether the project is created
	ConditionCreated controller.ConditionType = "Created"
	// ConditionFinalizing represents whether the project is being finalized
	ConditionFinalizing controller.ConditionType = "Finalizing"
)

const (
	// ReasonProjectCreated is the reason used when a project is created/ready
	ReasonProjectCreated controller.ConditionReason = "ProjectCreated"

	// ReasonProjectFinalizing is the reason used when a projects's dependents are being deleted'
	ReasonProjectFinalizing controller.ConditionReason = "ProjectFinalizing"
)

// NewProjectCreatedCondition creates a condition to indicate the project is created/ready
func NewProjectCreatedCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionCreated,
		metav1.ConditionTrue,
		ReasonProjectCreated,
		"Project is created",
		generation,
	)
}

func NewProjectFinalizingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonProjectFinalizing,
		"Project is finalizing",
		generation,
	)
}

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

package deployableartifact

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// ConditionAvailable represents whether the deployableArtifact is created
	ConditionAvailable controller.ConditionType = "Available"
	// ConditionFinalizing represents whether the deployableArtifact is being finalized
	ConditionFinalizing controller.ConditionType = "Finalizing"
)

const (
	// ReasonDeployableArtifactAvailable is the reason used when a deployableArtifact is available
	ReasonDeployableArtifactAvailable controller.ConditionReason = "DeployableArtifactAvailable"

	// ReasonDeployableArtifactFinalizing is the reason used when a deployableArtifact's dependents are being deleted'
	ReasonDeployableArtifactFinalizing controller.ConditionReason = "DeployableArtifactFinalizing"
)

// NewDeployableArtifactAvailableCondition creates a condition to indicate the deployableArtifact is available
func NewDeployableArtifactAvailableCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionAvailable,
		metav1.ConditionTrue,
		ReasonDeployableArtifactAvailable,
		"DeployableArtifact is available",
		generation,
	)
}

// NewDeployableArtifactFinalizingCondition creates a condition to indicate the deployableArtifact is being finalized
func NewDeployableArtifactFinalizingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonDeployableArtifactFinalizing,
		"DeployableArtifact is finalizing",
		generation,
	)
}

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

package deployment

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

// Constants for condition types

const (
	// ConditionArtifactResolved represents whether the deployable artifact has been resolved
	ConditionArtifactResolved controller.ConditionType = "ArtifactResolved"
	// ConditionReady represents whether the deployment is ready
	ConditionReady controller.ConditionType = "Ready"
)

// Constants for condition reasons

const (
	// Reasons for ArtifactResolved condition type

	// ReasonArtifactResolvedSuccessfully the deployable artifact has been resolved successfully for deployment
	ReasonArtifactResolvedSuccessfully controller.ConditionReason = "ArtifactResolvedSuccessfully"
	// ReasonArtifactNotFound the referenced deployable artifact resource was not found in the deployment track
	ReasonArtifactNotFound controller.ConditionReason = "ArtifactNotFound"
	// ReasonArtifactBuildNotFound the build resource referenced by the deployable artifact was not found in the deployment track
	ReasonArtifactBuildNotFound controller.ConditionReason = "ArtifactBuildNotFound"

	// Reasons for Ready condition type

	// ReasonDeploymentReady the deployment is ready
	ReasonDeploymentReady       controller.ConditionReason = "DeploymentReady"
	ReasonDeploymentProgressing controller.ConditionReason = "DeploymentProgressing"
	ReasonDeploymentFinalizing  controller.ConditionReason = "DeploymentFinalizing"
)

func NewArtifactResolvedCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionArtifactResolved,
		metav1.ConditionTrue,
		ReasonArtifactResolvedSuccessfully,
		"Artifact resolved successfully",
		generation,
	)
}

func NewArtifactNotFoundCondition(artifactRef string, generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionArtifactResolved,
		metav1.ConditionFalse,
		ReasonArtifactNotFound,
		fmt.Sprintf("Artifact %q not found", artifactRef),
		generation,
	)
}

func NewArtifactBuildNotFoundCondition(artifactRef, buildRef string, generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionArtifactResolved,
		metav1.ConditionFalse,
		ReasonArtifactBuildNotFound,
		fmt.Sprintf("Build %q not found for the referenced artifact %q", buildRef, artifactRef),
		generation,
	)
}

func NewDeploymentReadyCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionReady,
		metav1.ConditionTrue,
		ReasonDeploymentReady,
		"Deployment is ready",
		generation,
	)
}

func NewDeploymentProgressingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionReady,
		metav1.ConditionFalse,
		ReasonDeploymentProgressing,
		"Deployment is progressing",
		generation,
	)
}

func NewDeploymentFinalizingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionReady,
		metav1.ConditionFalse,
		ReasonDeploymentFinalizing,
		"Deployment is finalizing",
		generation,
	)
}

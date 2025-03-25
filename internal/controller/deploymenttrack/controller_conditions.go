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

package deploymenttrack

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/choreo-idp/choreo/internal/controller"
)

const (
	// ConditionAvailable represents whether the deploymentTrack is created
	ConditionAvailable controller.ConditionType = "Available"
	// ConditionFinalizing represents whether the deploymentTrack is being finalized
	ConditionFinalizing controller.ConditionType = "Finalizing"
)

const (
	// ReasonDeploymentTrackAvailable is the reason used when a deploymentTrack is available
	ReasonDeploymentTrackAvailable controller.ConditionReason = "DeploymentTrackAvailable"

	// ReasonDeploymentTrackFinalizingStarted is the reason used when a deploymentTrack's dependents are being deleted'
	ReasonDeploymentTrackFinalizingStarted controller.ConditionReason = "DeploymentTrackFinalizingStarted"

	// ReasonDeploymentTrackDeletingBuilds is the reason used when a deploymentTrack's builds are being deleted'
	ReasonDeploymentTrackDeletingBuilds controller.ConditionReason = "DeploymentTrackDeletingBuilds"

	// ReasonDeploymentTrackDeletingDeployableArtifacts is the reason used when a deploymentTrack's deployable artifacts are being deleted'
	ReasonDeploymentTrackDeletingDeployableArtifacts controller.ConditionReason = "DeploymentTrackDeletingDeployableArtifacts"

	// ReasonDeploymentTrackDeletingDeployments is the reason used when a deploymentTrack's deployments are being deleted'
	ReasonDeploymentTrackDeletingDeployments controller.ConditionReason = "DeploymentTrackDeletingDeployments"
)

// NewDeploymentTrackAvailableCondition creates a condition to indicate the deploymentTrack is available
func NewDeploymentTrackAvailableCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionAvailable,
		metav1.ConditionTrue,
		ReasonDeploymentTrackAvailable,
		"DeploymentTrack is available",
		generation,
	)
}

// NewDeploymentTrackStartFinalizeCondition creates a condition to indicate the deploymenttrack is being finalized
func NewDeploymentTrackStartFinalizeCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonDeploymentTrackFinalizingStarted,
		"DeploymentTrack is being finalized",
		generation,
	)
}

// NewDeploymentTrackCleanBuildsCondition creates a condition to indicate the deploymenttrack is being finalized
func NewDeploymentTrackCleanBuildsCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonDeploymentTrackDeletingBuilds,
		"DeploymentTracks's Builds are being cleaned",
		generation,
	)
}

// NewDeploymentTrackCleanDeployableArtifactsCondition creates a condition to indicate the deploymenttrack is being finalized
func NewDeploymentTrackCleanDeployableArtifactsCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonDeploymentTrackDeletingDeployableArtifacts,
		"DeploymentTracks's DeployableArtifacts are being cleaned",
		generation,
	)
}

// NewDeploymentTrackCleanDeploymentsCondition creates a condition to indicate the deploymenttrack is being finalized
func NewDeploymentTrackCleanDeploymentsCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonDeploymentTrackDeletingDeployments,
		"DeploymentTracks's Deployments are being cleaned",
		generation,
	)
}

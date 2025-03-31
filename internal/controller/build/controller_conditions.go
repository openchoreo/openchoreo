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

package build

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller"
)

// Constants for condition types

const (
	// ConditionCloneStepSucceeded represents whether the source code clone step is succeeded
	ConditionCloneStepSucceeded controller.ConditionType = "StepCloneSucceeded"
	// ConditionBuildStepSucceeded represents whether the build step is succeeded
	ConditionBuildStepSucceeded controller.ConditionType = "StepBuildSucceeded"
	// ConditionPushStepSucceeded represents whether the push step is succeeded
	ConditionPushStepSucceeded controller.ConditionType = "StepPushSucceeded"

	// ConditionDeployableArtifactCreated represents whether the deployable artifact is created after a successful build
	ConditionDeployableArtifactCreated controller.ConditionType = "DeployableArtifactCreated"
	// ConditionDeploymentApplied represents whether the deployment is created/updated when auto deploy is enabled
	ConditionDeploymentApplied controller.ConditionType = "DeploymentApplied"
	// ConditionCompleted represents whether the build is completed
	ConditionCompleted controller.ConditionType = "Completed"
	// ConditionBuildFinalizing represents the build resource is being deleted
	ConditionBuildFinalizing controller.ConditionType = "Finalizing"
	// ConditionDeployableArtifactReferencesRemaining indicates that the build deletion is blocked due to existing DeployableArtifact references
	ConditionDeployableArtifactReferencesRemaining controller.ConditionType = "DeployableArtifactReferencesRemaining"
)

// Constants for condition reasons

const (
	// Reasons for ci workflow/pipeline related steps

	ReasonStepQueued     controller.ConditionReason = "Queued"
	ReasonStepInProgress controller.ConditionReason = "Progressing"
	ReasonStepSucceeded  controller.ConditionReason = "Succeeded"
	ReasonStepFailed     controller.ConditionReason = "Failed"

	// ReasonWorkflowCreatedSuccessfully represents the workflow has been created successfully
	ReasonWorkflowCreatedSuccessfully controller.ConditionReason = "WorkflowCreated"

	// ReasonArtifactCreatedSuccessfully represents the reason for DeployableArtifactCreated condition type
	ReasonArtifactCreatedSuccessfully controller.ConditionReason = "ArtifactCreationSuccessful"

	// Reasons for auto deployment related conditions

	ReasonAutoDeploymentFailed  controller.ConditionReason = "DeploymentFailed"
	ReasonAutoDeploymentApplied controller.ConditionReason = "DeploymentAppliedSuccessfully"

	ReasonBuildInProgress controller.ConditionReason = "BuildProgressing"
	ReasonBuildFailed     controller.ConditionReason = "BuildFailed"
	ReasonBuildCompleted  controller.ConditionReason = "BuildCompleted"

	// Reasons for build finalizing

	ReasonBuildFinalizing                  controller.ConditionReason = "BuildCleanupOngoing"
	ReasonDeployableArtifactDeletionFailed controller.ConditionReason = "DeployableArtifactRemain"
)

func setInitialBuildConditions(build *choreov1.Build) {
	steps := []struct {
		conditionType controller.ConditionType
		reason        controller.ConditionReason
		message       string
	}{
		{ConditionCloneStepSucceeded, ReasonStepQueued, "Clone source code step is queued for execution."},
		{ConditionBuildStepSucceeded, ReasonStepQueued, "Image build step is queued for execution."},
		{ConditionPushStepSucceeded, ReasonStepQueued, "Image push step is queued for execution."},
		{ConditionCompleted, ReasonBuildInProgress, "Build process is in progress."},
	}

	for _, step := range steps {
		meta.SetStatusCondition(&build.Status.Conditions, controller.NewCondition(
			step.conditionType,
			metav1.ConditionFalse,
			step.reason,
			step.message,
			build.Generation,
		))
	}
}

func markStepInProgress(build *choreov1.Build, conditionType controller.ConditionType) {
	messageMap := map[controller.ConditionType]string{
		ConditionCloneStepSucceeded: "Clone source code step is executing.",
		ConditionBuildStepSucceeded: "Image build step is executing.",
		ConditionPushStepSucceeded:  "Image push step is executing.",
	}

	meta.SetStatusCondition(&build.Status.Conditions, controller.NewCondition(
		conditionType,
		metav1.ConditionFalse,
		ReasonStepInProgress,
		messageMap[conditionType],
		build.Generation,
	))
}

func markStepSucceeded(build *choreov1.Build, conditionType controller.ConditionType) {
	successMessages := map[controller.ConditionType]string{
		ConditionCloneStepSucceeded: "Source code clone step completed successfully.",
		ConditionBuildStepSucceeded: "Image build step completed successfully.",
		ConditionPushStepSucceeded:  "Image push step completed successfully.",
	}
	meta.SetStatusCondition(&build.Status.Conditions, controller.NewCondition(
		conditionType,
		metav1.ConditionTrue,
		ReasonStepSucceeded,
		successMessages[conditionType],
		build.Generation,
	))
}

func markStepFailed(build *choreov1.Build, conditionType controller.ConditionType) {
	failureMessages := map[controller.ConditionType]string{
		ConditionCloneStepSucceeded: "Source code cloning failed.",
		ConditionBuildStepSucceeded: "Building the image from the source code failed.",
		ConditionPushStepSucceeded:  "Pushing the built image to the registry failed.",
	}
	meta.SetStatusCondition(&build.Status.Conditions, controller.NewCondition(
		conditionType,
		metav1.ConditionFalse,
		ReasonStepFailed,
		failureMessages[conditionType],
		build.Generation,
	))
}

func NewDeployableArtifactCreatedCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionDeployableArtifactCreated,
		metav1.ConditionTrue,
		ReasonArtifactCreatedSuccessfully,
		"Successfully created a deployable artifact for the build.",
		generation,
	)
}

func NewBuildFailedCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionCompleted,
		metav1.ConditionFalse,
		ReasonBuildFailed,
		"Build completed with a failure status.",
		generation,
	)
}

func NewBuildCompletedCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionCompleted,
		metav1.ConditionTrue,
		ReasonBuildCompleted,
		"Build completed successfully.",
		generation,
	)
}

func NewImageMissingBuildFailedCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionCompleted,
		metav1.ConditionFalse,
		ReasonBuildFailed,
		"Image name is not found in the ci workflow.",
		generation,
	)
}

func NewAutoDeploymentFailedCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionDeploymentApplied,
		metav1.ConditionFalse,
		ReasonAutoDeploymentFailed,
		"Auto deployment failed.",
		generation,
	)
}

func NewAutoDeploymentSuccessfulCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionDeploymentApplied,
		metav1.ConditionTrue,
		ReasonAutoDeploymentApplied,
		"Successfully applied the deployment.",
		generation,
	)
}

func NewBuildFinalizingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionBuildFinalizing,
		metav1.ConditionTrue,
		ReasonBuildFinalizing,
		"Build resource is being finalized.",
		generation,
	)
}

func NewArtifactRemainingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionDeployableArtifactReferencesRemaining,
		metav1.ConditionTrue,
		ReasonDeployableArtifactDeletionFailed,
		"Deployable artifact resource is remaining.",
		generation,
	)
}

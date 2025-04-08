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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/labels"
)

// This file contains the helper functions to get the Choreo specific metadata from the Kubernetes objects.

// GetOrganizationName returns the organization name that the object belongs to.
func GetOrganizationName(obj client.Object) string {
	return getLabelValueOrEmpty(obj, labels.LabelKeyOrganizationName)
}

// GetProjectName returns the project name that the object belongs to.
func GetProjectName(obj client.Object) string {
	return getLabelValueOrEmpty(obj, labels.LabelKeyProjectName)
}

// GetComponentName returns the component name that the object belongs to.
func GetComponentName(obj client.Object) string {
	return getLabelValueOrEmpty(obj, labels.LabelKeyComponentName)
}

// GetDeploymentTrackName returns the deployment track name that the object belongs to.
func GetDeploymentTrackName(obj client.Object) string {
	return getLabelValueOrEmpty(obj, labels.LabelKeyDeploymentTrackName)
}

// GetBuildName returns the build name that the object belongs to.
func GetBuildName(obj client.Object) string {
	return getLabelValueOrEmpty(obj, labels.LabelKeyBuildName)
}

// GetDeploymentName returns the deployment name that the object belongs to.
func GetDeploymentName(obj client.Object) string {
	return getLabelValueOrEmpty(obj, labels.LabelKeyDeploymentName)
}

// GetDeployableArtifactName returns the deployable artifact name that the object belongs to.
func GetDeployableArtifactName(obj client.Object) string {
	return getLabelValueOrEmpty(obj, labels.LabelKeyDeployableArtifactName)
}

// GetEnvironmentName returns the environment name that the object belongs to.
func GetEnvironmentName(obj client.Object) string {
	return getLabelValueOrEmpty(obj, labels.LabelKeyEnvironmentName)
}

// GetName returns the name of the object. This is specific to the Choreo, and it is not the Kubernetes object name.
func GetName(obj client.Object) string {
	return getLabelValueOrEmpty(obj, labels.LabelKeyName)
}

// GetDisplayName returns the display name of the object.
func GetDisplayName(obj client.Object) string {
	return getAnnotationValueOrEmpty(obj, AnnotationKeyDisplayName)
}

// GetDescription returns the description of the object.
func GetDescription(obj client.Object) string {
	return getAnnotationValueOrEmpty(obj, AnnotationKeyDescription)
}

func getLabelValueOrEmpty(obj client.Object, labelKey string) string {
	if obj.GetLabels() == nil {
		return ""
	}
	return obj.GetLabels()[labelKey]
}

func getAnnotationValueOrEmpty(obj client.Object, annotationKey string) string {
	if obj.GetAnnotations() == nil {
		return ""
	}
	return obj.GetAnnotations()[annotationKey]
}

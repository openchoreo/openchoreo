// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/labels"
)

// This file contains the helper functions to get the Choreo specific metadata from the Kubernetes objects.

// GetNamespaceName returns the namespace name that the object belongs to.
func GetNamespaceName(obj client.Object) string {
	return getLabelValueOrEmpty(obj, labels.LabelKeyNamespaceName)
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

// GetEnvironmentName returns the environment name that the object belongs to.
func GetEnvironmentName(obj client.Object) string {
	return getLabelValueOrEmpty(obj, labels.LabelKeyEnvironmentName)
}

// GetDataPlaneName returns the data plane name that the object belongs to.
func GetDataPlaneName(obj client.Object) string {
	return getLabelValueOrEmpty(obj, labels.LabelKeyDataPlaneName)
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

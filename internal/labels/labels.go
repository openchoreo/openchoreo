// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package labels

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// This file contains the all the labels that are used to store Choreo specific the metadata in the Kubernetes objects.

const (
	LabelKeyOrganizationName    = "openchoreo.dev/organization"
	LabelKeyProjectName         = "openchoreo.dev/project"
	LabelKeyComponentName       = "openchoreo.dev/component"
	LabelKeyComponentType       = "openchoreo.dev/component-type"
	LabelKeyDeploymentTrackName = "openchoreo.dev/deployment-track"
	LabelKeyBuildName           = "openchoreo.dev/build"
	LabelKeyEnvironmentName     = "openchoreo.dev/environment"
	LabelKeyName                = "openchoreo.dev/name"
	LabelKeyDataPlaneName       = "openchoreo.dev/dataplane"
	LabelKeyBuildPlane          = "openchoreo.dev/build-plane"

	LabelKeyProjectUID     = "openchoreo.dev/project-uid"
	LabelKeyComponentUID   = "openchoreo.dev/component-uid"
	LabelKeyEnvironmentUID = "openchoreo.dev/environment-uid"

	// LabelKeyCreatedBy identifies which controller initially created a resource (audit trail).
	// Example: A namespace created by release-controller would have created-by=release-controller.
	// Note: For shared resources like namespaces, the creator and lifecycle manager may differ.
	LabelKeyCreatedBy = "openchoreo.dev/created-by"

	// LabelKeyManagedBy identifies which controller manages the lifecycle of a resource.
	// Example: Resources deployed by release-controller have managed-by=release-controller.
	LabelKeyManagedBy = "openchoreo.dev/managed-by"

	// LabelKeyReleaseResourceID identifies a specific resource within a release.
	LabelKeyReleaseResourceID = "openchoreo.dev/release-resource-id"

	// LabelKeyReleaseUID tracks which release UID owns/manages a resource.
	LabelKeyReleaseUID = "openchoreo.dev/release-uid"

	// LabelKeyReleaseName tracks the name of the release that manages a resource.
	LabelKeyReleaseName = "openchoreo.dev/release-name"

	// LabelKeyReleaseNamespace tracks the namespace of the release that manages a resource.
	LabelKeyReleaseNamespace = "openchoreo.dev/release-namespace"

	LabelValueManagedBy = "openchoreo-control-plane"
)

// SetLabels updates ObjectMeta labels with desired labels.
// Returns true if any labels were updated.
func SetLabels(meta *metav1.ObjectMeta, desired map[string]string) bool {
	if meta.Labels == nil {
		meta.Labels = make(map[string]string)
	}

	updated := false
	for key, value := range desired {
		if meta.Labels[key] != value {
			meta.Labels[key] = value
			updated = true
		}
	}
	return updated
}

// MakeComponentLabels creates labels for a Component.
func MakeComponentLabels(comp *openchoreov1alpha1.Component) map[string]string {
	return map[string]string{
		LabelKeyProjectName:   comp.Spec.Owner.ProjectName,
		LabelKeyComponentType: comp.Spec.ComponentType,
	}
}

// MakeComponentReleaseLabels creates labels for a ComponentRelease.
func MakeComponentReleaseLabels(cr *openchoreov1alpha1.ComponentRelease) map[string]string {
	return map[string]string{
		LabelKeyProjectName:   cr.Spec.Owner.ProjectName,
		LabelKeyComponentName: cr.Spec.Owner.ComponentName,
	}
}

// MakeReleaseBindingLabels creates labels for a ReleaseBinding.
func MakeReleaseBindingLabels(rb *openchoreov1alpha1.ReleaseBinding) map[string]string {
	return map[string]string{
		LabelKeyProjectName:     rb.Spec.Owner.ProjectName,
		LabelKeyComponentName:   rb.Spec.Owner.ComponentName,
		LabelKeyEnvironmentName: rb.Spec.Environment,
	}
}

// MakeWorkloadLabels creates labels for a Workload.
func MakeWorkloadLabels(wl *openchoreov1alpha1.Workload) map[string]string {
	return map[string]string{
		LabelKeyProjectName:   wl.Spec.Owner.ProjectName,
		LabelKeyComponentName: wl.Spec.Owner.ComponentName,
	}
}

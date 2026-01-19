// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package labels

// This file contains the all the labels that are used to store Choreo specific the metadata in the Kubernetes objects.

const (
	// TODO: chathurangas: check whether this should be openchoreo.dev/namespace or the corev1/namespace API
	LabelKeyNamespaceName       = "openchoreo.dev/namespace"
	LabelKeyProjectName         = "openchoreo.dev/project"
	LabelKeyComponentName       = "openchoreo.dev/component"
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

	// LabelKeyNotificationChannelName identifies a notification channel resource (ConfigMap/Secret)
	// created by the observabilityalertsnotificationchannel controller.
	LabelKeyNotificationChannelName = "openchoreo.dev/notification-channel-name"

	LabelValueManagedBy = "openchoreo-control-plane"
)

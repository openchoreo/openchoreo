// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package labels

// This file contains the all the labels that are used to store Choreo specific the metadata in the Kubernetes objects.

const (
	// LabelKeyNamespaceName identifies the OpenChoreo namespace for resources.
	LabelKeyNamespaceName   = "openchoreo.dev/namespace"
	LabelKeyProjectName     = "openchoreo.dev/project"
	LabelKeyComponentName   = "openchoreo.dev/component"
	LabelKeyEnvironmentName = "openchoreo.dev/environment"
	LabelKeyName            = "openchoreo.dev/name"
	LabelKeyDataPlaneName   = "openchoreo.dev/dataplane"
	LabelKeyWorkflowPlane   = "openchoreo.dev/workflow-plane"

	LabelKeyProjectUID     = "openchoreo.dev/project-uid"
	LabelKeyComponentUID   = "openchoreo.dev/component-uid"
	LabelKeyEnvironmentUID = "openchoreo.dev/environment-uid"

	// LabelKeyCreatedBy identifies which controller initially created a resource (audit trail).
	// Example: A namespace created by renderedrelease-controller would have created-by=renderedrelease-controller.
	// Note: For shared resources like namespaces, the creator and lifecycle manager may differ.
	LabelKeyCreatedBy = "openchoreo.dev/created-by"

	// LabelKeyManagedBy identifies which controller manages the lifecycle of a resource.
	// Example: Resources deployed by renderedrelease-controller have managed-by=renderedrelease-controller.
	LabelKeyManagedBy = "openchoreo.dev/managed-by"

	// LabelKeyRenderedReleaseResourceID identifies a specific resource within a rendered release.
	LabelKeyRenderedReleaseResourceID = "openchoreo.dev/rendered-release-resource-id"

	// LabelKeyRenderedReleaseUID tracks which rendered release UID owns/manages a resource.
	LabelKeyRenderedReleaseUID = "openchoreo.dev/rendered-release-uid"

	// LabelKeyRenderedReleaseName tracks the name of the rendered release that manages a resource.
	LabelKeyRenderedReleaseName = "openchoreo.dev/rendered-release-name"

	// LabelKeyRenderedReleaseNamespace tracks the namespace of the rendered release that manages a resource.
	LabelKeyRenderedReleaseNamespace = "openchoreo.dev/rendered-release-namespace"

	// LabelKeyNotificationChannelName identifies a notification channel resource (ConfigMap/Secret)
	// created by the observabilityalertsnotificationchannel controller.
	LabelKeyNotificationChannelName = "openchoreo.dev/notification-channel-name"

	// LabelKeyEndpointName identifies the workload endpoint name associated with a rendered gateway resource (e.g. HTTPRoute).
	LabelKeyEndpointName = "openchoreo.dev/endpoint-name"

	// LabelKeyEndpointVisibility identifies the visibility scope of the endpoint associated with a rendered gateway resource.
	// Valid values match EndpointVisibility: "project", "namespace", "internal", "external".
	LabelKeyEndpointVisibility = "openchoreo.dev/endpoint-visibility"

	// LabelKeyControlPlaneNamespace identifies a namespace as an OpenChoreo control plane namespace
	// that groups user resources (Projects, Components, Environments, etc.)
	// This label distinguishes control plane namespaces from:
	// - System namespaces (openchoreo-control-plane, openchoreo-data-plane, kube-system, etc.)
	// - User-created namespaces unrelated to OpenChoreo
	// - Data plane runtime namespaces (e.g., dp-*)
	LabelKeyControlPlaneNamespace = "openchoreo.dev/controlplane-namespace"

	// AnnotationKeyDPResourceHash contains a hash of all dataplane resources (excluding the main workload)
	// to trigger pod rollout when dependent ConfigMaps, Secrets, etc. change.
	AnnotationKeyDPResourceHash = "openchoreo.dev/dp-resource-hash"

	LabelValueManagedBy = "openchoreo-control-plane"
	// LabelValueTrue is the standard "true" value for boolean labels
	LabelValueTrue = "true"
)

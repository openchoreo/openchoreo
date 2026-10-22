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
	LabelKeyWorkflowType    = "openchoreo.dev/workflow-type"

	// LabelValueWorkflowTypeComponent marks a workflow as a component CI workflow,
	// used for component builds, webhooks, and auto-build integration.
	LabelValueWorkflowTypeComponent = "component"

	LabelKeyProjectUID     = "openchoreo.dev/project-uid"
	LabelKeyComponentUID   = "openchoreo.dev/component-uid"
	LabelKeyEnvironmentUID = "openchoreo.dev/environment-uid"

	// LabelKeyReleaseLine identifies the release line a rendered object belongs to.
	// Absent for the component's main line.
	LabelKeyReleaseLine = "openchoreo.dev/release-line"

	// LabelKeyReleaseLineUID holds the UID of the ReleaseLine a rendered object belongs to, which
	// observability backends filter on. Absent for the component's main line.
	LabelKeyReleaseLineUID = "openchoreo.dev/release-line-uid"

	// LabelKeyReleaseLineComponentUID holds the component UID on a release line's pods, which don't
	// carry LabelKeyComponentUID so that the main line's selectors never match them. Match either
	// label to find every pod of a component.
	LabelKeyReleaseLineComponentUID = "openchoreo.dev/release-line-component-uid"

	// LabelKeyResourceName identifies the Resource (managed-infrastructure
	// abstraction) that owns a rendered DP-side object.
	LabelKeyResourceName = "openchoreo.dev/resource"
	// LabelKeyResourceUID carries the owning Resource CR's UID for stable
	// identification across name reuse.
	LabelKeyResourceUID = "openchoreo.dev/resource-uid"

	// LabelKeyEndpointCRName identifies the Endpoint CR (traffic routing
	// abstraction) that owns a rendered DP-side object. Distinct from
	// LabelKeyEndpointName, which names a workload endpoint on a component's
	// gateway route.
	LabelKeyEndpointCRName = "openchoreo.dev/endpoint"
	// LabelKeyEndpointCRUID carries the owning Endpoint CR's UID for stable
	// identification across name reuse.
	LabelKeyEndpointCRUID = "openchoreo.dev/endpoint-uid"

	// LabelKeySourceComponent marks an Endpoint as created for one component's
	// workload endpoint (portal, occ or GitOps tooling at component creation).
	// While every backend of the Endpoint is that component, the Endpoint
	// controller makes the Component its owner so deleting the component deletes
	// the Endpoint; otherwise the Project owns it.
	LabelKeySourceComponent = "openchoreo.dev/source-component"
	// LabelKeySourceEndpoint names the workload endpoint of the source component
	// an Endpoint was created for.
	LabelKeySourceEndpoint = "openchoreo.dev/source-endpoint"

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

	// LabelKeyComponentReleaseName records the ComponentRelease a RenderedRelease was
	// rendered from. The renderedrelease controller uses it to resolve release-scoped
	// context (delivery events, commit provenance).
	LabelKeyComponentReleaseName = "openchoreo.dev/component-release-name"

	// LabelKeyComponentReleaseUID records the UID of that ComponentRelease. Because a
	// ComponentRelease is immutable and every release is a new object, this UID is the
	// per-rollout identity used by delivery lifecycle events.
	LabelKeyComponentReleaseUID = "openchoreo.dev/component-release-uid"

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
	LabelKeyControlPlaneNamespace = "openchoreo.dev/control-plane"

	// LabelKeyKVNamespace marks a plane namespace that holds the K8s Secrets and PushSecrets the
	// Secret API provisions for one control plane namespace, named by LabelKeyNamespaceName.
	LabelKeyKVNamespace = "openchoreo.dev/kv-namespace"

	// LabelKeyWorkflowNamespace marks a workflow plane namespace that runs the workflows and holds the
	// git secrets of one control plane namespace, named by LabelKeyNamespaceName.
	LabelKeyWorkflowNamespace = "openchoreo.dev/workflow-namespace"

	// LabelKeySecretType marks the kind of secret a SecretReference stands for, such as a git secret.
	LabelKeySecretType = "openchoreo.dev/secret-type"
	// LabelKeyWorkflowPlaneKind is the kind of the workflow plane a git secret was pushed through.
	LabelKeyWorkflowPlaneKind = "openchoreo.dev/workflow-plane-kind"
	// LabelKeyWorkflowPlaneName is the name of the workflow plane a git secret was pushed through.
	LabelKeyWorkflowPlaneName = "openchoreo.dev/workflow-plane-name"
	// LabelValueGitCredentials is the LabelKeySecretType value of a git secret.
	LabelValueGitCredentials = "git-credentials"

	// LabelKeySystemComponent identifies platform infrastructure pods (e.g., gateway) that need
	// network access to user workloads. Used in NetworkPolicy rules to allow ingress from system components.
	LabelKeySystemComponent = "openchoreo.dev/system-component"

	// LabelKeyPlane and LabelKeyPlaneID attribute a platform (system component) pod to the
	// OpenChoreo plane that owns it, for platform observability. Stamped by the plane Helm
	// charts on pod templates.
	// LabelKeyPlaneID is omitted on the control plane, which is a singleton.
	LabelKeyPlane   = "openchoreo.dev/plane"
	LabelKeyPlaneID = "openchoreo.dev/plane-id"

	// AnnotationKeyDPResourceHash contains a hash of other dataplane resources of the component, to
	// trigger pod rollout when they change. It covers every resource except the workload, or, when
	// referenced-config rollout is enabled, only the ConfigMaps, Secrets and ExternalSecrets the
	// workload's pod template references.
	AnnotationKeyDPResourceHash = "openchoreo.dev/dp-resource-hash"

	// AnnotationKeyRestartOnChange lets ComponentType and Trait authors decide whether a rendered
	// dataplane resource restarts the component's pods when it changes. "true" includes it in
	// AnnotationKeyDPResourceHash even when the pod template does not reference it, e.g. a
	// ConfigMap the application reads through the Kubernetes API. "false" leaves it out even
	// when the pod template references it, e.g. a config file the application reloads by itself.
	// Without the annotation, only the ConfigMaps, Secrets and ExternalSecrets the pod template
	// references are included. It applies to deployment and statefulset ComponentTypes, and not
	// on the workload itself. It is only used when referenced-config rollout is enabled.
	AnnotationKeyRestartOnChange = "openchoreo.dev/restart-on-change"

	// AnnotationKeyEndpointBasePath records the gateway-exposed base path of a rendered route
	// (e.g. HTTPRoute). The ReleaseBinding controller uses it to construct the endpoint invoke URL.
	// It is required for routes that render per-resource matches (e.g. one match per OpenAPI path),
	// where the first route match is a specific operation rather than the endpoint base. When absent,
	// the controller falls back to the first route match path (the prefix-routing convention).
	AnnotationKeyEndpointBasePath = "openchoreo.dev/endpoint-base-path"

	LabelValueManagedBy = "openchoreo-control-plane"
	// LabelValueOpenChoreoAPI marks resources created or managed by the openchoreo-api.
	LabelValueOpenChoreoAPI = "openchoreo-api"
	// LabelValueTrue is the standard "true" value for boolean labels
	LabelValueTrue = "true"
)

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package postrendervalidator

import "strings"

// fixForDiagnostic returns actionable guidance for correcting a finding. The
// location of the fix is carried separately on Finding so the caller can point
// the user at the exact field that should change.
func fixForDiagnostic(code, message string) string {
	switch code {
	case valueImageLatest:
		return imageTagGuidance
	case valueLimitsRequired:
		return "add resources.limits.cpu and resources.limits.memory to the container"
	case valueRequestsRequired:
		return "add resources.requests.cpu and resources.requests.memory to the container"
	case valueReplicasPositive:
		return "set replicas to at least 1 in the Component parameters or the ComponentType template"
	case valueNoPrivileged:
		return "set securityContext.privileged to false"
	case valueNoHostNamespaces:
		return "set hostNetwork, hostPID, and hostIPC to false"
	case valuePostRenderUnresolvedExpression:
		return "replace the unresolved expression with a value provided by the render context"
	case "post-render-missing-field":
		return "add the missing Kubernetes field to the ComponentType or Trait template"
	case "post-render-error":
		return fixForRenderError(message)
	default:
		return ""
	}
}

// fixForRenderError maps the preflight/render errors raised before rule
// evaluation to the configuration that most often needs correcting.
func fixForRenderError(message string) string {
	switch {
	case strings.Contains(message, "references ComponentType"),
		strings.Contains(message, "references ClusterComponentType"):
		return "add the missing component type or correct spec.componentType in the Component"
	case strings.Contains(message, "no Workload for Component"):
		return "add a Workload owned by this Component"
	case strings.Contains(message, "multiple Workloads"):
		return "keep only one Workload for this Component"
	case strings.Contains(message, "references Environment"):
		return "add the missing Environment or correct spec.environment in the ReleaseBinding"
	case strings.Contains(message, "multiple Environments"):
		return "keep only one Environment with this name"
	case strings.Contains(message, "references ClusterDataPlane"):
		return "add the missing ClusterDataPlane or correct spec.dataPlaneRef in the Environment"
	case strings.Contains(message, "multiple ClusterDataPlanes"):
		return "keep only one ClusterDataPlane with this name"
	case strings.Contains(message, "references DataPlane"):
		return "add the missing DataPlane or correct spec.dataPlaneRef in the Environment"
	case strings.Contains(message, "multiple DataPlanes"):
		return "keep only one DataPlane with this name"
	case strings.Contains(message, "unsupported data plane kind"):
		return "set spec.dataPlaneRef.kind to DataPlane or ClusterDataPlane"
	case strings.Contains(message, "referenced by the Component was not found"):
		return "add the missing trait or correct spec.traits in the Component"
	case strings.Contains(message, "multiple ReleaseBindings"):
		return "open the intended ReleaseBinding and run validation again"
	case strings.Contains(message, "active ReleaseBinding") &&
		strings.Contains(message, "does not match Component"):
		return "point spec.owner at the active Component or open the matching Component"
	case strings.Contains(message, "multiple Components"):
		return "open the intended Component file so only one Component is validated"
	case strings.Contains(message, "no Component matches"):
		return "open a Component file or one of its related resources"
	case strings.Contains(message, "cannot be resolved to a Component"):
		return "open a Component or one of its related resources"
	case strings.Contains(message, "does not declare an OpenChoreo apiVersion"):
		return "add apiVersion openchoreo.dev/v1alpha1 to the active file"
	case strings.Contains(message, "contains no OpenChoreo documents"):
		return "add an OpenChoreo resource document to the active file"
	case strings.Contains(message, "parse active file"):
		return "fix the YAML syntax error at the reported location"
	case strings.Contains(message, "decode "):
		return "fix the YAML fields reported by the decoder"
	default:
		return "fix the reported configuration error and run validation again"
	}
}

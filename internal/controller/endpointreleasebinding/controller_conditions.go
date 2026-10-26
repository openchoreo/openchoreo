// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

// Condition types.

const (
	// ConditionBackendsResolved indicates every backend that should receive
	// traffic resolved to a Service in the binding's environment. Per-backend
	// detail lives in status.backends.
	ConditionBackendsResolved controller.ConditionType = "BackendsResolved"

	// ConditionSynced indicates the binding has resolved its pinned
	// EndpointRelease and emitted a RenderedRelease for the current spec.
	ConditionSynced controller.ConditionType = "Synced"

	// ConditionResourcesReady indicates every entry on the owned
	// RenderedRelease's status.resources passes its per-Kind health check.
	ConditionResourcesReady controller.ConditionType = "ResourcesReady"

	// ConditionURLsResolved indicates every URL the EndpointType declares for a
	// visibility the Endpoint requests resolved for the binding's environment.
	// The values live in status.urls.
	ConditionURLsResolved controller.ConditionType = "URLsResolved"

	// ConditionReady aggregates BackendsResolved, Synced, ResourcesReady, and
	// URLsResolved.
	ConditionReady controller.ConditionType = "Ready"

	// ConditionFinalizing indicates the binding is being finalized (deleted).
	ConditionFinalizing controller.ConditionType = "Finalizing"
)

// Condition reasons.

const (
	// ReasonEndpointReleaseNotSet indicates the binding has no
	// spec.endpointRelease pin yet.
	ReasonEndpointReleaseNotSet controller.ConditionReason = "EndpointReleaseNotSet"

	// ReasonEndpointReleaseNotFound indicates the pinned EndpointRelease does
	// not exist in the binding's namespace.
	ReasonEndpointReleaseNotFound controller.ConditionReason = "EndpointReleaseNotFound"

	// ReasonInvalidReleaseConfiguration indicates the pinned EndpointRelease
	// belongs to a different Endpoint than the binding.
	ReasonInvalidReleaseConfiguration controller.ConditionReason = "InvalidReleaseConfiguration"

	// ReasonInvalidEnvironmentConfigs indicates
	// spec.endpointTypeEnvironmentConfigs does not satisfy the type's
	// environmentConfigs schema.
	ReasonInvalidEnvironmentConfigs controller.ConditionReason = "InvalidEnvironmentConfigs"

	// ReasonEnvironmentNotFound indicates the referenced Environment does not exist.
	ReasonEnvironmentNotFound controller.ConditionReason = "EnvironmentNotFound"

	// ReasonDataPlaneNotFound indicates the Environment's dataPlaneRef does not
	// resolve to an existing DataPlane or ClusterDataPlane.
	ReasonDataPlaneNotFound controller.ConditionReason = "DataPlaneNotFound"

	// ReasonEndpointNotFound indicates the owning Endpoint does not exist.
	ReasonEndpointNotFound controller.ConditionReason = "EndpointNotFound"

	// ReasonProjectNotFound indicates the owning Project does not exist.
	ReasonProjectNotFound controller.ConditionReason = "ProjectNotFound"

	// ReasonBackendsResolved indicates every backend resolved.
	ReasonBackendsResolved controller.ConditionReason = "BackendsResolved"

	// ReasonBackendsUnresolved indicates a backend that should receive traffic
	// could not be resolved, so the binding does not render. Also set on Synced,
	// since the RenderedRelease is left as it was rather than updated.
	ReasonBackendsUnresolved controller.ConditionReason = "BackendsUnresolved"

	// ReasonRenderingFailed indicates the endpoint pipeline failed to render
	// the type's resources (CEL evaluation error, failed validation rule,
	// malformed template).
	ReasonRenderingFailed controller.ConditionReason = "RenderingFailed"

	// ReasonReleaseCreated indicates the RenderedRelease was created or updated
	// by this reconcile.
	ReasonReleaseCreated controller.ConditionReason = "ReleaseCreated"

	// ReasonReleaseSynced indicates the RenderedRelease is up to date.
	ReasonReleaseSynced controller.ConditionReason = "ReleaseSynced"

	// ReasonReleaseUpdateFailed indicates a transient failure creating or
	// updating the RenderedRelease.
	ReasonReleaseUpdateFailed controller.ConditionReason = "ReleaseUpdateFailed"

	// ReasonReleaseOwnershipConflict indicates a RenderedRelease already exists
	// at the target name but is owned by something else.
	ReasonReleaseOwnershipConflict controller.ConditionReason = "ReleaseOwnershipConflict"

	// ReasonResourcesReady indicates every rendered resource is healthy.
	ReasonResourcesReady controller.ConditionReason = "ResourcesReady"

	// ReasonResourcesProgressing indicates a rendered resource is still being
	// created or has no observed status yet.
	ReasonResourcesProgressing controller.ConditionReason = "ResourcesProgressing"

	// ReasonResourcesDegraded indicates a rendered resource reported
	// HealthStatus=Degraded on the data plane.
	ReasonResourcesDegraded controller.ConditionReason = "ResourcesDegraded"

	// ReasonResourceApplyFailed indicates the RenderedRelease reported
	// ResourcesApplied=False for its current generation.
	ReasonResourceApplyFailed controller.ConditionReason = "ResourceApplyFailed"

	// ReasonURLsResolved indicates every applicable URL resolved.
	ReasonURLsResolved controller.ConditionReason = "URLsResolved"

	// ReasonURLResolutionFailed indicates at least one URL failed to resolve
	// (CEL error, or a value outside its allowed range). The URLs that did
	// resolve are still reported, and the RenderedRelease is unaffected.
	ReasonURLResolutionFailed controller.ConditionReason = "URLResolutionFailed"

	// ReasonReady indicates the aggregate Ready condition is True.
	ReasonReady controller.ConditionReason = "Ready"

	// ReasonSyncedNotReady is set on ResourcesReady while Synced is False, so a
	// ResourcesReady=True written by an earlier successful reconcile does not
	// linger after the binding stops being in sync.
	ReasonSyncedNotReady controller.ConditionReason = "SyncedNotReady"

	// ReasonFinalizing indicates the binding is being finalized.
	ReasonFinalizing controller.ConditionReason = "Finalizing"
)

// Backend resolution reasons, reported on status.backends[].reason. They are
// plain strings rather than condition reasons because they describe a backend,
// not the binding.
const (
	// BackendReasonNotDeployed indicates the backend component has no active
	// ReleaseBinding in the binding's environment.
	BackendReasonNotDeployed = "NotDeployed"

	// BackendReasonAmbiguous indicates more than one ReleaseBinding matches the
	// backend component in the environment.
	BackendReasonAmbiguous = "AmbiguousBackend"

	// BackendReasonEndpointNotAvailable indicates the component is deployed but
	// does not (yet) report an in-cluster address for the workload endpoint.
	BackendReasonEndpointNotAvailable = "EndpointNotAvailable"

	// BackendReasonUnsupportedProtocol indicates the workload endpoint speaks a
	// protocol no route kind can carry.
	BackendReasonUnsupportedProtocol = "UnsupportedProtocol"

	// BackendReasonUnsupportedKind indicates the backendRef's kind is not one
	// this controller can resolve.
	BackendReasonUnsupportedKind = "UnsupportedKind"

	// BackendReasonNamespaceMismatch indicates the backend Service lives
	// outside the data-plane namespace the routes render into, so a route could
	// not reference it without a ReferenceGrant.
	BackendReasonNamespaceMismatch = "NamespaceMismatch"
)

// NewFinalizingCondition returns a Finalizing=True condition observed at the
// given generation, used while the binding is being torn down.
func NewFinalizingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonFinalizing,
		"EndpointReleaseBinding is finalizing",
		generation,
	)
}

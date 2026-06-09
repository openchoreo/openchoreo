// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/controller"
)

// Constants for condition types

const (
	// ConditionSynced indicates that the owned RenderedRelease has been
	// created/updated for this binding. Mirrors ReleaseBinding's ReleaseSynced
	// (shortened since "Release" is implicit on a *ReleaseBinding resource).
	ConditionSynced controller.ConditionType = "Synced"

	// ConditionResourcesReady indicates every entry in
	// ResourceType.spec.resources[] passes its readiness check (per-entry
	// readyWhen if set, else RenderedRelease per-Kind health inference).
	ConditionResourcesReady controller.ConditionType = "ResourcesReady"

	// ConditionOutputsResolved indicates status.outputs has been populated
	// from RenderedRelease.status.resources[]; CEL evaluation succeeded for
	// every declared output.
	ConditionOutputsResolved controller.ConditionType = "OutputsResolved"

	// ConditionReady aggregates Synced, ResourcesReady, and OutputsResolved.
	ConditionReady controller.ConditionType = "Ready"

	// ConditionFinalizing indicates the binding is being finalized (deleted).
	ConditionFinalizing controller.ConditionType = "Finalizing"
)

// Constants for condition reasons

const (
	// ReasonResourceReleaseNotSet indicates the binding has no
	// spec.resourceRelease pin yet. The pin is advanced externally via
	// `occ resource promote` or `kubectl edit`.
	ReasonResourceReleaseNotSet controller.ConditionReason = "ResourceReleaseNotSet"

	// ReasonResourceReleaseNotFound indicates the referenced ResourceRelease
	// does not exist.
	ReasonResourceReleaseNotFound controller.ConditionReason = "ResourceReleaseNotFound"

	// ReasonInvalidReleaseConfiguration indicates the ResourceRelease snapshot
	// disagrees with the binding's owner.
	ReasonInvalidReleaseConfiguration controller.ConditionReason = "InvalidReleaseConfiguration"

	// ReasonEnvironmentNotFound indicates the referenced Environment does not
	// exist.
	ReasonEnvironmentNotFound controller.ConditionReason = "EnvironmentNotFound"

	// ReasonDataPlaneNotFound indicates the Environment's dataPlaneRef does
	// not resolve to an existing DataPlane or ClusterDataPlane.
	ReasonDataPlaneNotFound controller.ConditionReason = "DataPlaneNotFound"

	// ReasonResourceNotFound indicates the owning Resource named by
	// spec.owner.resourceName does not exist in the binding's namespace.
	// Normally the Resource finalizer holds it open while bindings exist;
	// this reason surfaces the anomalous case where the Resource was
	// deleted directly.
	ReasonResourceNotFound controller.ConditionReason = "ResourceNotFound"

	// ReasonProjectNotFound indicates the owning Project named by
	// spec.owner.projectName does not exist in the binding's namespace.
	ReasonProjectNotFound controller.ConditionReason = "ProjectNotFound"

	// ReasonReleaseCreated indicates the underlying RenderedRelease was
	// created by this reconcile.
	ReasonReleaseCreated controller.ConditionReason = "ReleaseCreated"

	// ReasonReleaseSynced indicates the underlying RenderedRelease is up to
	// date.
	ReasonReleaseSynced controller.ConditionReason = "ReleaseSynced"

	// ReasonRenderingFailed indicates the pipeline failed to render the
	// snapshot's templates (CEL evaluation error, malformed template).
	ReasonRenderingFailed controller.ConditionReason = "RenderingFailed"

	// ReasonReleaseOwnershipConflict indicates a RenderedRelease already
	// exists at the target name but is owned by a different controller.
	ReasonReleaseOwnershipConflict controller.ConditionReason = "ReleaseOwnershipConflict"

	// ReasonReleaseUpdateFailed indicates a transient failure creating or
	// updating the underlying RenderedRelease.
	ReasonReleaseUpdateFailed controller.ConditionReason = "ReleaseUpdateFailed"

	// ReasonResourcesReady indicates every resources[] entry passes its
	// readiness check (per-entry readyWhen if set, else RenderedRelease
	// per-Kind health inference).
	ReasonResourcesReady controller.ConditionReason = "ResourcesReady"

	// ReasonResourcesProgressing indicates one or more resources[] entries
	// are still being created/updated, missing observed status, or have a
	// readyWhen expression that returned false.
	ReasonResourcesProgressing controller.ConditionReason = "ResourcesProgressing"

	// ReasonResourcesDegraded indicates one or more resources[] entries
	// reported HealthStatus=Degraded on the data plane.
	ReasonResourcesDegraded controller.ConditionReason = "ResourcesDegraded"

	// ReasonResourceApplyFailed indicates the underlying RenderedRelease
	// reported ResourcesApplied=False for the current generation.
	ReasonResourceApplyFailed controller.ConditionReason = "ResourceApplyFailed"

	// ReasonOutputsResolved indicates status.outputs has been populated and
	// every declared output's CEL evaluated successfully.
	ReasonOutputsResolved controller.ConditionReason = "OutputsResolved"

	// ReasonOutputResolutionFailed indicates one or more output CEL
	// expressions failed to evaluate against the observed applied status.
	// Successfully-resolved outputs are still written to status.outputs.
	ReasonOutputResolutionFailed controller.ConditionReason = "OutputResolutionFailed"

	// ReasonReady indicates the binding is fully ready: Synced, ResourcesReady,
	// and OutputsResolved are all True.
	ReasonReady controller.ConditionReason = "Ready"

	// ReasonSyncedNotReady is set on ResourcesReady and OutputsResolved when
	// Synced=False, signaling that those axes cannot be evaluated until
	// the binding can sync against the snapshot again.
	ReasonSyncedNotReady controller.ConditionReason = "SyncedNotReady"

	// ReasonFinalizing indicates the binding is being finalized.
	ReasonFinalizing controller.ConditionReason = "Finalizing"

	// ReasonRetainHold indicates the finalizer is being held because the
	// effective retainPolicy is Retain. PE clears it manually after
	// reclaiming any DP-side state.
	ReasonRetainHold controller.ConditionReason = "RetainHold"
)

// NewFinalizingCondition returns a Finalizing=True condition observed at the
// given generation, used while the binding is being torn down.
func NewFinalizingCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		ConditionFinalizing,
		metav1.ConditionTrue,
		ReasonFinalizing,
		"ResourceReleaseBinding is finalizing",
		generation,
	)
}

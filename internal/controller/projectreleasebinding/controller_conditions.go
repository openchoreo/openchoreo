// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Condition types.

const (
	// ConditionSynced indicates the binding has resolved its pinned
	// ProjectRelease, the inlined (Cluster)ProjectType passes the
	// cell-namespace mandate, and a RenderedRelease has been emitted for
	// the inlined resources.
	ConditionSynced controller.ConditionType = "Synced"

	// ConditionNamespaceReady indicates the project's data-plane namespace
	// (status.namespace) has been observed as healthy on the owned
	// RenderedRelease. Located by Group="" + Kind="Namespace" + Name
	// matching binding.Status.Namespace; additional Namespace objects
	// the PE chooses to render flow through ResourcesReady instead.
	ConditionNamespaceReady controller.ConditionType = "NamespaceReady"

	// ConditionResourcesReady indicates every non-namespace entry on the
	// owned RenderedRelease's status.resources passes its per-Kind health
	// check.
	ConditionResourcesReady controller.ConditionType = "ResourcesReady"

	// ConditionReady aggregates Synced, NamespaceReady, and ResourcesReady.
	ConditionReady controller.ConditionType = "Ready"
)

// Condition reasons.

const (
	// ReasonProjectReleaseNotSet indicates the binding has no
	// spec.projectRelease pin yet. The pin is advanced externally via a
	// promote workflow or kubectl edit.
	ReasonProjectReleaseNotSet controller.ConditionReason = "ProjectReleaseNotSet"

	// ReasonProjectReleaseNotFound indicates the referenced ProjectRelease
	// does not exist in the binding's namespace.
	ReasonProjectReleaseNotFound controller.ConditionReason = "ProjectReleaseNotFound"

	// ReasonInvalidReleaseConfiguration indicates the ProjectRelease snapshot
	// disagrees with the binding's owner.
	ReasonInvalidReleaseConfiguration controller.ConditionReason = "InvalidReleaseConfiguration"

	// ReasonNamespaceMissing indicates the rendered
	// (Cluster)ProjectType resources contain no v1/Namespace whose
	// metadata.name resolves to the project's data-plane namespace
	// (${metadata.namespace}). Checked on the rendered output, so an
	// includeWhen that suppresses the namespace entry also surfaces here.
	ReasonNamespaceMissing controller.ConditionReason = "NamespaceMissing"

	// ReasonEnvironmentNotFound indicates the referenced Environment does not
	// exist.
	ReasonEnvironmentNotFound controller.ConditionReason = "EnvironmentNotFound"

	// ReasonDataPlaneNotFound indicates the Environment's dataPlaneRef does
	// not resolve to an existing DataPlane or ClusterDataPlane.
	ReasonDataPlaneNotFound controller.ConditionReason = "DataPlaneNotFound"

	// ReasonProjectNotFound indicates the owning Project named by
	// spec.owner.projectName does not exist in the binding's namespace.
	ReasonProjectNotFound controller.ConditionReason = "ProjectNotFound"

	// ReasonRenderingFailed indicates the project pipeline failed to render
	// the inlined (Cluster)ProjectType resources (CEL evaluation error,
	// malformed template).
	ReasonRenderingFailed controller.ConditionReason = "RenderingFailed"

	// ReasonReleaseCreated indicates the underlying RenderedRelease was
	// created or updated by this reconcile.
	ReasonReleaseCreated controller.ConditionReason = "ReleaseCreated"

	// ReasonReleaseSynced indicates the underlying RenderedRelease is up to
	// date.
	ReasonReleaseSynced controller.ConditionReason = "ReleaseSynced"

	// ReasonReleaseUpdateFailed indicates a transient failure creating or
	// updating the underlying RenderedRelease.
	ReasonReleaseUpdateFailed controller.ConditionReason = "ReleaseUpdateFailed"

	// ReasonReleaseOwnershipConflict indicates a RenderedRelease already
	// exists at the target name but is owned by a different controller.
	ReasonReleaseOwnershipConflict controller.ConditionReason = "ReleaseOwnershipConflict"

	// ReasonNamespaceReady indicates the cell namespace is observed
	// healthy on the owned RenderedRelease.
	ReasonNamespaceReady controller.ConditionReason = "NamespaceReady"

	// ReasonNamespaceProgressing indicates the cell namespace is still
	// being created or has no observed status yet.
	ReasonNamespaceProgressing controller.ConditionReason = "NamespaceProgressing"

	// ReasonNamespaceDegraded indicates the cell namespace reported
	// HealthStatus=Degraded on the data plane.
	ReasonNamespaceDegraded controller.ConditionReason = "NamespaceDegraded"

	// ReasonResourcesReady indicates every non-namespace entry on the
	// owned RenderedRelease is healthy.
	ReasonResourcesReady controller.ConditionReason = "ResourcesReady"

	// ReasonResourcesProgressing indicates one or more non-namespace
	// entries are still being created/updated or have no observed status
	// yet.
	ReasonResourcesProgressing controller.ConditionReason = "ResourcesProgressing"

	// ReasonResourcesDegraded indicates one or more non-namespace entries
	// reported HealthStatus=Degraded on the data plane.
	ReasonResourcesDegraded controller.ConditionReason = "ResourcesDegraded"

	// ReasonResourceApplyFailed indicates the underlying RenderedRelease
	// reported ResourcesApplied=False for the current generation. Surfaces
	// on both NamespaceReady and ResourcesReady since the apply failure is
	// at the RR level.
	ReasonResourceApplyFailed controller.ConditionReason = "ResourceApplyFailed"

	// ReasonReady indicates the binding's aggregate Ready condition is True.
	ReasonReady controller.ConditionReason = "Ready"

	// ReasonSyncedNotReady is set on Ready, NamespaceReady, and
	// ResourcesReady while Synced is False; per-axis sub-conditions written
	// by a previous successful reconcile would otherwise stay True after
	// upstream validation breaks (snapshot deleted, environment removed,
	// render now failing), giving a misleading status.
	ReasonSyncedNotReady controller.ConditionReason = "SyncedNotReady"
)

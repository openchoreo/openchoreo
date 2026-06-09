// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Condition types.

const (
	// ConditionSynced indicates the binding has resolved its pinned
	// ProjectRelease and the inlined (Cluster)ProjectType passes the
	// cell-namespace mandate. Downstream rendering, namespace readiness, and
	// resource readiness conditions will be added in later phases.
	ConditionSynced controller.ConditionType = "Synced"

	// ConditionReady aggregates downstream sub-conditions. Until rendering
	// and per-resource readiness land, Ready tracks Synced.
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

	// ReasonCellNamespaceMissing indicates the inlined
	// (Cluster)ProjectType.spec.resources has no v1/Namespace entry whose
	// metadata.name is the cell-namespace placeholder. Duplicate-resource
	// detection is out of scope for this check and is surfaced separately
	// (e.g. at render time when two manifests collide on metadata.name).
	ReasonCellNamespaceMissing controller.ConditionReason = "CellNamespaceMissing"

	// ReasonSyncedReady indicates the binding has resolved its release
	// snapshot and the inlined ProjectType passes the cell-namespace mandate.
	// Rendering and resource readiness happen in later phases.
	ReasonSyncedReady controller.ConditionReason = "SyncedReady"

	// ReasonReady indicates the binding's aggregate Ready condition is True.
	ReasonReady controller.ConditionReason = "Ready"

	// ReasonSyncedNotReady is set on Ready while Synced has not yet been
	// evaluated.
	ReasonSyncedNotReady controller.ConditionReason = "SyncedNotReady"
)

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// ConditionReady indicates the line was reconciled. With autoDeploy, its latest release is bound
	// in the first environment of the pipeline.
	ConditionReady controller.ConditionType = "Ready"
)

const (
	// ReasonReconciled indicates the line was validated; autoDeploy is off for the line.
	ReasonReconciled controller.ConditionReason = "Reconciled"
	// ReasonComponentReleaseReady indicates the line's latest release is bound in the first environment.
	ReasonComponentReleaseReady controller.ConditionReason = "ComponentReleaseReady"

	// ReasonComponentNotFound indicates the owning Component does not exist.
	ReasonComponentNotFound controller.ConditionReason = "ComponentNotFound"
	// ReasonReleaseLinesNotEnabled indicates the owning Component does not have releaseLinesEnabled.
	ReasonReleaseLinesNotEnabled controller.ConditionReason = "ReleaseLinesNotEnabled"
	// ReasonInvalidConfiguration indicates the line or its effective Component is invalid.
	ReasonInvalidConfiguration controller.ConditionReason = "InvalidConfiguration"
	// ReasonWorkloadNotFound indicates the line has no Workload yet.
	ReasonWorkloadNotFound controller.ConditionReason = "WorkloadNotFound"
	// ReasonTraitNotFound indicates a trait the line uses does not exist.
	ReasonTraitNotFound controller.ConditionReason = "TraitNotFound"
	// ReasonAutoDeployFailed indicates the line's release or binding could not be created or updated.
	ReasonAutoDeployFailed controller.ConditionReason = "AutoDeployFailed"
)

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package revision

import (
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller/renderedrelease"
)

// Condition types and reasons read from a binding's status. Both binding
// controllers use these names; they are repeated here rather than imported
// because the controllers import this package.
const (
	conditionResourcesReady = "ResourcesReady"

	reasonResourcesProgressing = "ResourcesProgressing"
	reasonResourcesDegraded    = "ResourcesDegraded"
	reasonResourcesNotReady    = "ResourcesNotReady"
	reasonResourceApplyFailed  = "ResourceApplyFailed"
	reasonJobRunning           = "JobRunning"
	reasonJobFailed            = "JobFailed"

	reasonRenderingFailed          = "RenderingFailed"
	reasonReleaseOwnershipConflict = "ReleaseOwnershipConflict"
	reasonReleaseUpdateFailed      = "ReleaseUpdateFailed"
)

// Observation is what a binding controller knows, at the end of a reconcile,
// about how the binding's current spec is doing on the data plane.
type Observation struct {
	// Generation is the binding's metadata.generation. Conditions computed for
	// an earlier generation describe an earlier spec and are not used.
	Generation int64
	// Conditions are the binding's status conditions as this reconcile left them.
	Conditions []metav1.Condition
	// SyncedCondition is the condition type reporting whether the
	// RenderedRelease could be produced: "ReleaseSynced" for a ReleaseBinding,
	// "Synced" for a ResourceReleaseBinding.
	SyncedCondition string
	// Undeploy is true when the binding asks for its resources to be removed.
	Undeploy bool
	// RenderedRelease is the binding's data-plane RenderedRelease, or nil when
	// it does not exist.
	RenderedRelease *openchoreov1alpha1.RenderedRelease
}

// Health maps an observation to the health of the revision recording the
// binding's current spec:
//
//   - Undeploy: Progressing while the RenderedRelease still exists, Healthy once
//     it is gone (removal is what the revision asked for).
//   - Failed when the spec could not be rendered or written (Synced False with a
//     rendering or release-write reason), when the RenderedRelease reports an
//     apply failure for its current generation, or when ResourcesReady reports
//     an apply failure, a failed Job, or not-ready resources.
//   - Progressing while the RenderedRelease does not exist yet, while its
//     resource health still describes an earlier generation (old pods can be
//     serving and healthy), and while ResourcesReady has not been evaluated for
//     this spec -- or reports progressing resources or a running Job.
//   - Healthy when ResourcesReady is True for this spec and the RenderedRelease
//     health is current.
//   - Degraded when ResourcesReady reports degraded resources.
//   - Unknown when ResourcesReady is missing or reports unknown resources.
func (o Observation) Health() openchoreov1alpha1.RevisionHealth {
	if o.Undeploy {
		if o.RenderedRelease == nil {
			return openchoreov1alpha1.RevisionHealthHealthy
		}
		return openchoreov1alpha1.RevisionHealthProgressing
	}

	if synced := o.current(o.SyncedCondition); synced != nil && synced.Status == metav1.ConditionFalse {
		switch synced.Reason {
		case reasonRenderingFailed, reasonReleaseOwnershipConflict, reasonReleaseUpdateFailed:
			return openchoreov1alpha1.RevisionHealthFailed
		}
	}

	if o.RenderedRelease == nil {
		return openchoreov1alpha1.RevisionHealthProgressing
	}
	applied := apimeta.FindStatusCondition(o.RenderedRelease.Status.Conditions, renderedrelease.ConditionResourcesApplied)
	if applied == nil || applied.ObservedGeneration != o.RenderedRelease.Generation {
		// The renderedrelease controller has not reported on this generation yet,
		// so the resource health beside it still describes the previous spec.
		return openchoreov1alpha1.RevisionHealthProgressing
	}
	if applied.Status == metav1.ConditionFalse {
		return openchoreov1alpha1.RevisionHealthFailed
	}

	ready := apimeta.FindStatusCondition(o.Conditions, conditionResourcesReady)
	if ready == nil {
		return openchoreov1alpha1.RevisionHealthUnknown
	}
	if ready.ObservedGeneration != o.Generation {
		// Not yet evaluated for this spec (a guard returned early, or the
		// RenderedRelease was just written).
		return openchoreov1alpha1.RevisionHealthProgressing
	}
	if ready.Status == metav1.ConditionTrue {
		return openchoreov1alpha1.RevisionHealthHealthy
	}
	switch ready.Reason {
	case reasonResourcesProgressing, reasonJobRunning:
		return openchoreov1alpha1.RevisionHealthProgressing
	case reasonResourcesDegraded:
		return openchoreov1alpha1.RevisionHealthDegraded
	case reasonResourceApplyFailed, reasonJobFailed, reasonResourcesNotReady:
		return openchoreov1alpha1.RevisionHealthFailed
	default:
		return openchoreov1alpha1.RevisionHealthUnknown
	}
}

// current returns the condition of the given type when it was computed for the
// observed generation, and nil otherwise.
func (o Observation) current(conditionType string) *metav1.Condition {
	c := apimeta.FindStatusCondition(o.Conditions, conditionType)
	if c == nil || c.ObservedGeneration != o.Generation {
		return nil
	}
	return c
}

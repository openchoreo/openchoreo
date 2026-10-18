// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/renderedrelease"
)

// evaluateResourcesReady derives ResourcesReady from the owned RenderedRelease's
// observed status. An apply-level failure for the current generation wins over
// per-entry health. Mirrors the ProjectReleaseBinding's readiness evaluation.
func evaluateResourcesReady(binding *openchoreov1alpha1.EndpointReleaseBinding, rr *openchoreov1alpha1.RenderedRelease) {
	if cond := meta.FindStatusCondition(rr.Status.Conditions, renderedrelease.ConditionResourcesApplied); cond != nil &&
		cond.Status == metav1.ConditionFalse && cond.ObservedGeneration == rr.Generation {
		controller.MarkFalseCondition(binding, ConditionResourcesReady, ReasonResourceApplyFailed, cond.Message)
		return
	}

	if len(rr.Status.Resources) < len(rr.Spec.Resources) {
		controller.MarkFalseCondition(binding, ConditionResourcesReady, ReasonResourcesProgressing,
			fmt.Sprintf("%d of %d resource(s) observed on the data plane",
				len(rr.Status.Resources), len(rr.Spec.Resources)))
		return
	}

	for i := range rr.Status.Resources {
		st := &rr.Status.Resources[i]
		switch st.HealthStatus {
		case openchoreov1alpha1.HealthStatusHealthy, openchoreov1alpha1.HealthStatusSuspended:
			// passes
		case openchoreov1alpha1.HealthStatusDegraded:
			controller.MarkFalseCondition(binding, ConditionResourcesReady, ReasonResourcesDegraded,
				fmt.Sprintf("Resource %q (%s) is degraded", st.ID, st.Kind))
			return
		default:
			controller.MarkFalseCondition(binding, ConditionResourcesReady, ReasonResourcesProgressing,
				fmt.Sprintf("Resource %q (%s) is %s", st.ID, st.Kind, st.HealthStatus))
			return
		}
	}
	controller.MarkTrueCondition(binding, ConditionResourcesReady, ReasonResourcesReady,
		fmt.Sprintf("All %d resource(s) ready", len(rr.Status.Resources)))
}

// markSyncedFalse marks Synced=False and forces ResourcesReady to Unknown, so a
// ResourcesReady=True from an earlier successful reconcile does not linger
// once the binding stops being in sync.
func markSyncedFalse(binding *openchoreov1alpha1.EndpointReleaseBinding,
	reason controller.ConditionReason, message string) {
	controller.MarkFalseCondition(binding, ConditionSynced, reason, message)
	controller.MarkUnknownCondition(binding, ConditionResourcesReady, ReasonSyncedNotReady,
		"ResourcesReady cannot be evaluated until Synced=True")
}

// setReadyCondition aggregates BackendsResolved, Synced, and ResourcesReady
// into Ready. Ready=True only when all three are True; otherwise Ready=False
// inherits the first failing sub-condition's reason and message, with
// BackendsResolved first because an unresolved backend is the most actionable
// explanation.
func setReadyCondition(binding *openchoreov1alpha1.EndpointReleaseBinding) {
	backends := meta.FindStatusCondition(binding.Status.Conditions, string(ConditionBackendsResolved))
	synced := meta.FindStatusCondition(binding.Status.Conditions, string(ConditionSynced))
	resources := meta.FindStatusCondition(binding.Status.Conditions, string(ConditionResourcesReady))

	if isTrue(backends) && isTrue(synced) && isTrue(resources) {
		controller.MarkTrueCondition(binding, ConditionReady, ReasonReady, "EndpointReleaseBinding is ready")
		return
	}

	for _, c := range []*metav1.Condition{backends, synced, resources} {
		if c == nil || c.Status == metav1.ConditionTrue {
			continue
		}
		controller.MarkFalseCondition(binding, ConditionReady, controller.ConditionReason(c.Reason), c.Message)
		return
	}

	controller.MarkFalseCondition(binding, ConditionReady, ReasonSyncedNotReady, "Awaiting sub-condition evaluation")
}

func isTrue(c *metav1.Condition) bool {
	return c != nil && c.Status == metav1.ConditionTrue
}

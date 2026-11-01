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
// per-entry health, and per-entry health counts only once the current generation
// has been applied: until then it describes the previous routes.
func evaluateResourcesReady(binding *openchoreov1alpha1.EndpointReleaseBinding, rr *openchoreov1alpha1.RenderedRelease) {
	applied := meta.FindStatusCondition(rr.Status.Conditions, renderedrelease.ConditionResourcesApplied)
	if applied != nil && applied.Status == metav1.ConditionFalse && applied.ObservedGeneration == rr.Generation {
		controller.MarkFalseCondition(binding, ConditionResourcesReady, ReasonResourceApplyFailed, applied.Message)
		return
	}
	if applied == nil || applied.Status != metav1.ConditionTrue || applied.ObservedGeneration != rr.Generation {
		controller.MarkFalseCondition(binding, ConditionResourcesReady, ReasonResourcesProgressing,
			fmt.Sprintf("RenderedRelease %q has not applied its current resources yet", rr.Name))
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

// setReadyCondition aggregates BackendsResolved, Synced, ResourcesReady, and
// URLsResolved into Ready. Ready=True only when all four are True for the
// binding's current generation; otherwise Ready=False inherits the first failing
// sub-condition's reason and message, with BackendsResolved first because an
// unresolved backend is the most actionable explanation. A sub-condition left
// over from an earlier generation, such as when a reconcile of a repinned
// binding fails before reaching it, describes the previous spec and counts as
// not yet evaluated.
func setReadyCondition(binding *openchoreov1alpha1.EndpointReleaseBinding) {
	current := func(ct controller.ConditionType) *metav1.Condition {
		c := meta.FindStatusCondition(binding.Status.Conditions, string(ct))
		if c == nil || c.ObservedGeneration != binding.Generation {
			return nil
		}
		return c
	}
	backends := current(ConditionBackendsResolved)
	synced := current(ConditionSynced)
	resources := current(ConditionResourcesReady)
	urls := current(ConditionURLsResolved)

	if isTrue(backends) && isTrue(synced) && isTrue(resources) && isTrue(urls) {
		controller.MarkTrueCondition(binding, ConditionReady, ReasonReady, "EndpointReleaseBinding is ready")
		return
	}

	for _, c := range []*metav1.Condition{backends, synced, resources, urls} {
		if c == nil || c.Status == metav1.ConditionTrue {
			continue
		}
		controller.MarkFalseCondition(binding, ConditionReady, controller.ConditionReason(c.Reason), c.Message)
		return
	}

	controller.MarkFalseCondition(binding, ConditionReady, ReasonSyncedNotReady, "Awaiting sub-condition evaluation")
}

// trackSpecUpdate stamps status.lastSpecUpdateTime when this reconcile is the
// first to see the binding's current generation. Ready is rewritten with the
// current generation on every reconcile, so its observedGeneration lagging
// behind means the spec changed since. A binding that already has Ready for
// its generation (one reconciled before this field existed) is left alone
// rather than stamped with the time of a controller upgrade.
func trackSpecUpdate(binding *openchoreov1alpha1.EndpointReleaseBinding) {
	ready := meta.FindStatusCondition(binding.Status.Conditions, string(ConditionReady))
	if ready != nil && ready.ObservedGeneration == binding.Generation {
		return
	}
	now := metav1.Now()
	binding.Status.LastSpecUpdateTime = &now
}

func isTrue(c *metav1.Condition) bool {
	return c != nil && c.Status == metav1.ConditionTrue
}

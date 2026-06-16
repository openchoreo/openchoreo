// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/renderedrelease"
)

// evaluateReadiness derives NamespaceReady and ResourcesReady from the owned
// RenderedRelease's observed status. The aggregate Ready condition is
// computed separately by setReadyCondition.
//
// Apply-level failure on the RenderedRelease (rr.Status.Conditions[ResourcesApplied]
// = False for the current generation) wins over per-entry health and surfaces
// on both sub-conditions with Reason=ResourceApplyFailed.
func (r *Reconciler) evaluateReadiness(
	binding *openchoreov1alpha1.ProjectReleaseBinding,
	rr *openchoreov1alpha1.RenderedRelease,
) {
	if applyFailed, msg := rrApplyFailed(rr); applyFailed {
		controller.MarkFalseCondition(binding, ConditionNamespaceReady, ReasonResourceApplyFailed, msg)
		controller.MarkFalseCondition(binding, ConditionResourcesReady, ReasonResourceApplyFailed, msg)
		return
	}

	evaluateNamespaceReady(binding, rr)
	evaluateResourcesReady(binding, rr)
}

// rrApplyFailed reports whether the RenderedRelease's ResourcesApplied
// condition is False for the current generation. Stale-generation conditions
// are ignored.
func rrApplyFailed(rr *openchoreov1alpha1.RenderedRelease) (bool, string) {
	cond := meta.FindStatusCondition(rr.Status.Conditions, renderedrelease.ConditionResourcesApplied)
	if cond == nil {
		return false, ""
	}
	if cond.Status != metav1.ConditionFalse {
		return false, ""
	}
	if cond.ObservedGeneration != rr.Generation {
		return false, ""
	}
	return true, cond.Message
}

// evaluateNamespaceReady locates the project's data-plane namespace entry in
// rr.Status.Resources[] by Group="" + Kind="Namespace" + Name matching
// binding.Status.Namespace, then maps its HealthStatus to a NamespaceReady
// condition. Other Namespace objects the PE chose to render are not
// considered here — those flow through ResourcesReady.
func evaluateNamespaceReady(
	binding *openchoreov1alpha1.ProjectReleaseBinding,
	rr *openchoreov1alpha1.RenderedRelease,
) {
	entry := findNamespaceEntry(rr.Status.Resources, binding.Status.Namespace)
	if entry == nil {
		controller.MarkFalseCondition(binding, ConditionNamespaceReady, ReasonNamespaceProgressing,
			fmt.Sprintf("Namespace %q has no observed status yet", binding.Status.Namespace))
		return
	}
	switch entry.HealthStatus {
	case openchoreov1alpha1.HealthStatusHealthy, openchoreov1alpha1.HealthStatusSuspended:
		controller.MarkTrueCondition(binding, ConditionNamespaceReady, ReasonNamespaceReady,
			fmt.Sprintf("Namespace %q is ready", binding.Status.Namespace))
	case openchoreov1alpha1.HealthStatusDegraded:
		controller.MarkFalseCondition(binding, ConditionNamespaceReady, ReasonNamespaceDegraded,
			fmt.Sprintf("Namespace %q is degraded", binding.Status.Namespace))
	default:
		controller.MarkFalseCondition(binding, ConditionNamespaceReady, ReasonNamespaceProgressing,
			fmt.Sprintf("Namespace %q is %s", binding.Status.Namespace, entry.HealthStatus))
	}
}

// evaluateResourcesReady aggregates HealthStatus over every entry in
// rr.Status.Resources[] except the project namespace. Any Degraded entry
// flips the condition to False with Reason=ResourcesDegraded; any non-Healthy
// non-Degraded entry flips it to Progressing.
func evaluateResourcesReady(
	binding *openchoreov1alpha1.ProjectReleaseBinding,
	rr *openchoreov1alpha1.RenderedRelease,
) {
	ns := binding.Status.Namespace
	considered := 0
	for i := range rr.Status.Resources {
		st := &rr.Status.Resources[i]
		if isNamespaceEntry(st, ns) {
			continue
		}
		considered++
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
		fmt.Sprintf("All %d resource(s) ready", considered))
}

// findNamespaceEntry returns the rendered status entry that corresponds to
// the project's data-plane namespace, or nil if not yet observed. Matched by
// Group="" + Kind="Namespace" + Name == ns.
func findNamespaceEntry(
	statuses []openchoreov1alpha1.RenderedManifestStatus,
	ns string,
) *openchoreov1alpha1.RenderedManifestStatus {
	for i := range statuses {
		if isNamespaceEntry(&statuses[i], ns) {
			return &statuses[i]
		}
	}
	return nil
}

// isNamespaceEntry reports whether the given status entry is the mandated
// project namespace. ns is the resolved dp-{ns}-{project}-{env}-{hash} name
// (binding.Status.Namespace).
func isNamespaceEntry(st *openchoreov1alpha1.RenderedManifestStatus, ns string) bool {
	return st.Group == "" && st.Kind == "Namespace" && st.Name == ns
}

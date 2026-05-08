// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/renderedrelease"
	resourcepipeline "github.com/openchoreo/openchoreo/internal/pipeline/resource"
)

// evaluateReadiness reads the live RenderedRelease status, resolves declared
// outputs, and aggregates per-entry readiness. Writes ResourcesReady,
// OutputsResolved, and the binding's status.outputs. The aggregate Ready
// condition is computed separately by setReadyCondition.
//
// ResourcesReady takes precedence from the RenderedRelease ResourcesApplied
// condition: if applying failed for the current generation, readiness is
// false with Reason=ResourceApplyFailed regardless of per-entry health.
// Stale-generation conditions are ignored.
func (r *Reconciler) evaluateReadiness(
	ctx context.Context,
	binding *openchoreov1alpha1.ResourceReleaseBinding,
	release *openchoreov1alpha1.ResourceRelease,
	environment *openchoreov1alpha1.Environment,
	dataPlane *openchoreov1alpha1.DataPlane,
	resource *openchoreov1alpha1.Resource,
	project *openchoreov1alpha1.Project,
	rr *openchoreov1alpha1.RenderedRelease,
) {
	logger := log.FromContext(ctx)

	observed := observedStatusByID(rr.Status.Resources, logger)
	r.evaluateOutputs(binding, release, environment, dataPlane, resource, project, observed, logger)
	r.evaluateResourcesReady(binding, release, environment, dataPlane, resource, project, rr, observed, logger)
}

// observedStatusByID decodes RenderedRelease.status.resources[].status from
// RawExtension into a {id → status-fields} map suitable for the pipeline's
// applied.<id> CEL surface. Entries with no status decode into an empty map
// rather than being dropped, so the pipeline still sees applied.<id>.
// Decode errors are logged so a corrupt status surfaces as a diagnostic
// rather than masquerading as missing fields downstream.
func observedStatusByID(resources []openchoreov1alpha1.RenderedManifestStatus, logger logr.Logger) map[string]map[string]any {
	observed := make(map[string]map[string]any, len(resources))
	for i := range resources {
		entry := &resources[i]
		status := map[string]any{}
		if entry.Status != nil && len(entry.Status.Raw) > 0 {
			if err := json.Unmarshal(entry.Status.Raw, &status); err != nil {
				logger.Error(err, "decode RenderedRelease entry status", "id", entry.ID)
			}
		}
		observed[entry.ID] = status
	}
	return observed
}

// evaluateOutputs runs ResolveOutputs and writes the result into
// status.outputs plus the OutputsResolved condition. Per-output errors leave
// successfully-resolved entries in place; the condition turns False with
// Reason=OutputResolutionFailed and the joined error message. Top-level
// failures (resolved is nil) preserve the previous status.outputs rather
// than clobbering it — wiping a successful prior result on a transient
// pipeline failure is misleading.
func (r *Reconciler) evaluateOutputs(
	binding *openchoreov1alpha1.ResourceReleaseBinding,
	release *openchoreov1alpha1.ResourceRelease,
	environment *openchoreov1alpha1.Environment,
	dataPlane *openchoreov1alpha1.DataPlane,
	resource *openchoreov1alpha1.Resource,
	project *openchoreov1alpha1.Project,
	observed map[string]map[string]any,
	logger logr.Logger,
) {
	input := buildPipelineInput(binding, release, environment, dataPlane, resource, project)

	resolved, err := r.Pipeline.ResolveOutputs(input, observed)
	if err == nil {
		binding.Status.Outputs = mapResolvedOutputs(resolved)
		controller.MarkTrueCondition(binding, ConditionOutputsResolved, ReasonOutputsResolved,
			fmt.Sprintf("Resolved %d output(s)", len(resolved)))
		return
	}

	// Partial failure: pipeline returns the successful subset alongside the
	// joined err; reflect that subset in status.outputs. Top-level failure
	// (resolved is nil): keep the previous outputs untouched so a transient
	// pipeline error doesn't erase a still-valid view.
	if len(resolved) > 0 {
		binding.Status.Outputs = mapResolvedOutputs(resolved)
	}
	controller.MarkFalseCondition(binding, ConditionOutputsResolved, ReasonOutputResolutionFailed,
		fmt.Sprintf("Failed to resolve %d output(s): %v", countOutputErrors(err), err))
	logger.Info("Output resolution failed", "error", err)
}

// countOutputErrors returns the number of joined errors when the pipeline
// reports per-output failures, otherwise 1.
func countOutputErrors(err error) int {
	type unwrapper interface{ Unwrap() []error }
	if u, ok := err.(unwrapper); ok {
		return len(u.Unwrap())
	}
	return 1
}

// mapResolvedOutputs converts pipeline-resolved outputs into the binding's
// API shape (list of named subobjects keyed by name).
func mapResolvedOutputs(resolved []resourcepipeline.ResolvedOutput) []openchoreov1alpha1.ResolvedResourceOutput {
	if len(resolved) == 0 {
		return nil
	}
	out := make([]openchoreov1alpha1.ResolvedResourceOutput, 0, len(resolved))
	for i := range resolved {
		entry := &resolved[i]
		out = append(out, openchoreov1alpha1.ResolvedResourceOutput{
			Name:            entry.Name,
			Value:           entry.Value,
			SecretKeyRef:    entry.SecretKeyRef,
			ConfigMapKeyRef: entry.ConfigMapKeyRef,
		})
	}
	return out
}

// evaluateResourcesReady computes ResourcesReady. The RenderedRelease's
// ResourcesApplied condition (when current-generation) overrides any
// per-entry signal. Otherwise iterate snapshot.Resources: per-entry
// readyWhen takes precedence; the fallback uses the per-Kind health
// inference written into RenderedRelease.status.resources[].healthStatus.
func (r *Reconciler) evaluateResourcesReady(
	binding *openchoreov1alpha1.ResourceReleaseBinding,
	release *openchoreov1alpha1.ResourceRelease,
	environment *openchoreov1alpha1.Environment,
	dataPlane *openchoreov1alpha1.DataPlane,
	resource *openchoreov1alpha1.Resource,
	project *openchoreov1alpha1.Project,
	rr *openchoreov1alpha1.RenderedRelease,
	observed map[string]map[string]any,
	logger logr.Logger,
) {
	if applyCond := meta.FindStatusCondition(rr.Status.Conditions, renderedrelease.ConditionResourcesApplied); applyCond != nil &&
		applyCond.Status == metav1.ConditionFalse &&
		applyCond.ObservedGeneration == rr.Generation {
		controller.MarkFalseCondition(binding, ConditionResourcesReady, ReasonResourceApplyFailed, applyCond.Message)
		return
	}

	statusByID := make(map[string]*openchoreov1alpha1.RenderedManifestStatus, len(rr.Status.Resources))
	for i := range rr.Status.Resources {
		statusByID[rr.Status.Resources[i].ID] = &rr.Status.Resources[i]
	}

	// Iterate only entries that survived IncludeWhen and were actually
	// rendered. Snapshot entries filtered out by IncludeWhen never produce
	// a manifest and never get observed status — counting them would gate
	// ResourcesReady on data that will never arrive.
	renderedIDs := make(map[string]bool, len(rr.Spec.Resources))
	for i := range rr.Spec.Resources {
		renderedIDs[rr.Spec.Resources[i].ID] = true
	}

	input := buildPipelineInput(binding, release, environment, dataPlane, resource, project)
	entries := release.Spec.ResourceType.Spec.Resources

	rendered := 0
	for i := range entries {
		entry := &entries[i]
		if !renderedIDs[entry.ID] {
			continue
		}
		rendered++

		if entry.ReadyWhen != "" {
			ready, err := r.Pipeline.EvaluateReadyWhen(input, observed, entry.ReadyWhen)
			if err != nil {
				controller.MarkFalseCondition(binding, ConditionResourcesReady, ReasonResourcesProgressing,
					fmt.Sprintf("readyWhen evaluation failed for %q: %v", entry.ID, err))
				logger.Info("readyWhen evaluation failed", "id", entry.ID, "error", err)
				return
			}
			if !ready {
				controller.MarkFalseCondition(binding, ConditionResourcesReady, ReasonResourcesProgressing,
					fmt.Sprintf("Resource %q readyWhen returned false", entry.ID))
				return
			}
			continue
		}

		st, found := statusByID[entry.ID]
		if !found {
			controller.MarkFalseCondition(binding, ConditionResourcesReady, ReasonResourcesProgressing,
				fmt.Sprintf("Resource %q has no observed status yet", entry.ID))
			return
		}
		switch st.HealthStatus {
		case openchoreov1alpha1.HealthStatusHealthy, openchoreov1alpha1.HealthStatusSuspended:
			// passes
		case openchoreov1alpha1.HealthStatusDegraded:
			controller.MarkFalseCondition(binding, ConditionResourcesReady, ReasonResourcesDegraded,
				fmt.Sprintf("Resource %q (%s) is degraded", entry.ID, st.Kind))
			return
		default:
			controller.MarkFalseCondition(binding, ConditionResourcesReady, ReasonResourcesProgressing,
				fmt.Sprintf("Resource %q (%s) is %s", entry.ID, st.Kind, st.HealthStatus))
			return
		}
	}

	controller.MarkTrueCondition(binding, ConditionResourcesReady, ReasonResourcesReady,
		fmt.Sprintf("All %d resource(s) ready", rendered))
}

// setReadyCondition aggregates Synced, ResourcesReady, and OutputsResolved
// into the top-level Ready. Ready=True only when all three sub-conditions
// are True; otherwise Ready=False inherits the failing sub-condition's
// Reason and Message.
func (r *Reconciler) setReadyCondition(binding *openchoreov1alpha1.ResourceReleaseBinding) {
	synced := meta.FindStatusCondition(binding.Status.Conditions, string(ConditionSynced))
	resReady := meta.FindStatusCondition(binding.Status.Conditions, string(ConditionResourcesReady))
	outputs := meta.FindStatusCondition(binding.Status.Conditions, string(ConditionOutputsResolved))

	if isTrue(synced) && isTrue(resReady) && isTrue(outputs) {
		controller.MarkTrueCondition(binding, ConditionReady, ReasonReady, "ResourceReleaseBinding is ready")
		return
	}

	for _, c := range []*metav1.Condition{synced, resReady, outputs} {
		if c == nil || c.Status == metav1.ConditionTrue {
			continue
		}
		controller.MarkFalseCondition(binding, ConditionReady,
			controller.ConditionReason(c.Reason), c.Message)
		return
	}

	controller.MarkFalseCondition(binding, ConditionReady, ReasonResourcesProgressing,
		"Awaiting sub-condition evaluation")
}

func isTrue(c *metav1.Condition) bool {
	return c != nil && c.Status == metav1.ConditionTrue
}

// markSyncedFalse marks Synced=False and forces ResourcesReady and
// OutputsResolved to Unknown. Per-axis sub-conditions written by a previous
// successful reconcile would otherwise stay True after upstream validation
// breaks (snapshot deleted, environment removed, render now failing), giving
// a misleading status. Unknown signals "cannot evaluate" until Synced
// returns to True.
func markSyncedFalse(binding *openchoreov1alpha1.ResourceReleaseBinding,
	reason controller.ConditionReason, message string) {
	controller.MarkFalseCondition(binding, ConditionSynced, reason, message)
	controller.MarkUnknownCondition(binding, ConditionResourcesReady, ReasonSyncedNotReady,
		"ResourcesReady cannot be evaluated until Synced=True")
	controller.MarkUnknownCondition(binding, ConditionOutputsResolved, ReasonSyncedNotReady,
		"OutputsResolved cannot be evaluated until Synced=True")
}

// buildPipelineInput is shared by output resolution and readyWhen
// evaluation; both surface the same CEL context.
func buildPipelineInput(
	binding *openchoreov1alpha1.ResourceReleaseBinding,
	release *openchoreov1alpha1.ResourceRelease,
	environment *openchoreov1alpha1.Environment,
	dataPlane *openchoreov1alpha1.DataPlane,
	resource *openchoreov1alpha1.Resource,
	project *openchoreov1alpha1.Project,
) *resourcepipeline.RenderInput {
	return &resourcepipeline.RenderInput{
		ResourceType:           buildResourceTypeFromRelease(release),
		Resource:               buildResourceFromRelease(release),
		ResourceReleaseBinding: binding,
		Metadata:               buildMetadataContext(binding, environment, dataPlane, resource, project),
		DataPlane:              buildDataPlaneContext(dataPlane),
	}
}

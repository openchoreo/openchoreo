// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"fmt"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

func filterWorkloadsByLine(items []openchoreov1alpha1.Workload, line string) []openchoreov1alpha1.Workload {
	out := items[:0:0]
	for i := range items {
		if items[i].Spec.Owner.ReleaseLine == line {
			out = append(out, items[i])
		}
	}
	return out
}

// reconcileReleaseLineComponent validates a component with releaseLinesEnabled. Such a component
// deploys only through its ReleaseLines, which the ReleaseLine controller reconciles.
func (r *Reconciler) reconcileReleaseLineComponent(ctx context.Context, comp *openchoreov1alpha1.Component) error {
	firstEnv, err := r.validateAndFetchDeploymentPipeline(ctx, comp)
	if err != nil || firstEnv == "" {
		return err
	}

	workloads, err := NewSteps(r.Client, r.Scheme, r.Recorder).Workloads(ctx, comp)
	if err != nil {
		return err
	}
	var unlined []string
	for _, w := range filterWorkloadsByLine(workloads, "") {
		unlined = append(unlined, w.Name)
	}
	if len(unlined) > 0 {
		controller.MarkFalseCondition(comp, ConditionReady, ReasonInvalidConfiguration,
			fmt.Sprintf("Component has release lines enabled; Workloads %v must set spec.owner.releaseLine", unlined))
		return nil
	}

	controller.MarkTrueCondition(comp, ConditionReady, ReasonReleaseLinesReconciled,
		"Component deploys through its release lines; see each ReleaseLine for its status")
	return nil
}

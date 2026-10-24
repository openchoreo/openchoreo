// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

// listReleaseLines returns the ReleaseLines owned by the component.
func (r *Reconciler) listReleaseLines(ctx context.Context, comp *openchoreov1alpha1.Component) ([]openchoreov1alpha1.ReleaseLine, error) {
	var lines openchoreov1alpha1.ReleaseLineList
	if err := r.List(ctx, &lines,
		client.InNamespace(comp.Namespace),
		client.MatchingFields{controller.IndexKeyReleaseLineOwner: controller.MakeReleaseLineOwnerKey(comp.Spec.Owner.ProjectName, comp.Name)}); err != nil {
		return nil, fmt.Errorf("failed to list release lines: %w", err)
	}
	return lines.Items, nil
}

// findComponentForReleaseLine maps a ReleaseLine to its owner Component.
func (r *Reconciler) findComponentForReleaseLine(_ context.Context, obj client.Object) []ctrl.Request {
	line := obj.(*openchoreov1alpha1.ReleaseLine)
	return []ctrl.Request{{NamespacedName: types.NamespacedName{Namespace: line.Namespace, Name: line.Spec.Owner.ComponentName}}}
}

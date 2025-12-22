// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// findProjectForComponent maps a Component to its owner Project
func (r *Reconciler) findProjectForComponent(ctx context.Context, obj client.Object) []ctrl.Request {
	component := obj.(*openchoreov1alpha1.Component)
	if component.Spec.Owner.ProjectName == "" {
		return nil
	}
	return []ctrl.Request{{
		NamespacedName: client.ObjectKey{
			Name:      component.Spec.Owner.ProjectName,
			Namespace: component.Namespace,
		},
	}}
}

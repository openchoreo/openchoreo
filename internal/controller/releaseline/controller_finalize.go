// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"context"
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// ReleaseLineFinalizer makes deleting a ReleaseLine remove the line's ReleaseBindings,
// ComponentReleases and Workload first.
const ReleaseLineFinalizer = "openchoreo.dev/releaseline-cleanup"

func (r *Reconciler) finalize(ctx context.Context, line *openchoreov1alpha1.ReleaseLine) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("releaseLine", line.Name)

	if !controllerutil.ContainsFinalizer(line, ReleaseLineFinalizer) {
		return ctrl.Result{}, nil
	}

	remaining, err := r.deleteOwned(ctx, line)
	if err != nil {
		return ctrl.Result{}, err
	}
	if remaining > 0 {
		logger.Info("Waiting for the release line's resources to be deleted", "remaining", remaining)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if controllerutil.RemoveFinalizer(line, ReleaseLineFinalizer) {
		if err := r.Update(ctx, line); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}
	logger.Info("Successfully finalized ReleaseLine")
	return ctrl.Result{}, nil
}

// deleteOwned deletes the line's ReleaseBindings, ComponentReleases and Workloads, and returns how many still exist.
func (r *Reconciler) deleteOwned(ctx context.Context, line *openchoreov1alpha1.ReleaseLine) (int, error) {
	owner := line.Spec.Owner
	owned := func(project, comp, rl string) bool {
		return project == owner.ProjectName && comp == owner.ComponentName && rl == line.Name
	}

	var objs []client.Object

	var bindings openchoreov1alpha1.ReleaseBindingList
	if err := r.List(ctx, &bindings, client.InNamespace(line.Namespace),
		client.MatchingFields{controller.IndexKeyReleaseBindingOwnerComponentName: owner.ComponentName}); err != nil {
		return 0, fmt.Errorf("failed to list ReleaseBindings: %w", err)
	}
	for i := range bindings.Items {
		o := bindings.Items[i].Spec.Owner
		if owned(o.ProjectName, o.ComponentName, o.ReleaseLine) {
			objs = append(objs, &bindings.Items[i])
		}
	}

	var releases openchoreov1alpha1.ComponentReleaseList
	if err := r.List(ctx, &releases, client.InNamespace(line.Namespace)); err != nil {
		return 0, fmt.Errorf("failed to list ComponentReleases: %w", err)
	}
	for i := range releases.Items {
		o := releases.Items[i].Spec.Owner
		if owned(o.ProjectName, o.ComponentName, o.ReleaseLine) {
			objs = append(objs, &releases.Items[i])
		}
	}

	var workloads openchoreov1alpha1.WorkloadList
	if err := r.List(ctx, &workloads, client.InNamespace(line.Namespace)); err != nil {
		return 0, fmt.Errorf("failed to list Workloads: %w", err)
	}
	for i := range workloads.Items {
		o := workloads.Items[i].Spec.Owner
		if owned(o.ProjectName, o.ComponentName, o.ReleaseLine) {
			objs = append(objs, &workloads.Items[i])
		}
	}

	for _, obj := range objs {
		if !obj.GetDeletionTimestamp().IsZero() {
			continue
		}
		if err := client.IgnoreNotFound(r.Delete(ctx, obj)); err != nil {
			return 0, fmt.Errorf("failed to delete %T %q: %w", obj, obj.GetName(), err)
		}
	}
	return len(objs), nil
}

// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"fmt"
	"sort"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// cleanupListTimeout is the timeout for List calls during cleanup.
	cleanupListTimeout = 30 * time.Second
	// cleanupDeleteTimeout is the timeout for each Delete call during cleanup.
	cleanupDeleteTimeout = 10 * time.Second
)

// cleanupComponentReleases deletes the oldest ComponentReleases that exceed the
// RevisionHistoryLimit for the given component, while protecting releases that
// are still in use (referenced by any ReleaseBinding or marked as LatestRelease).
func (r *Reconciler) cleanupComponentReleases(ctx context.Context, comp *openchoreov1alpha1.Component) error {
	logger := log.FromContext(ctx).WithValues("component", comp.Name)

	// List all ComponentReleases owned by this component
	listCtx, listCancel := context.WithTimeout(ctx, cleanupListTimeout)
	defer listCancel()

	releaseList := &openchoreov1alpha1.ComponentReleaseList{}
	if err := r.List(listCtx, releaseList,
		client.InNamespace(comp.Namespace),
		client.MatchingFields{"spec.owner.componentName": comp.Name}); err != nil {
		return fmt.Errorf("failed to list component releases: %w", err)
	}

	// Nothing to clean if within limit
	if len(releaseList.Items) <= r.RevisionHistoryLimit {
		return nil
	}

	// Build the in-use set from ReleaseBindings
	inUse := make(map[string]bool)

	bindingCtx, bindingCancel := context.WithTimeout(ctx, cleanupListTimeout)
	defer bindingCancel()

	bindingList := &openchoreov1alpha1.ReleaseBindingList{}
	if err := r.List(bindingCtx, bindingList,
		client.InNamespace(comp.Namespace),
		client.MatchingFields{controller.IndexKeyReleaseBindingOwnerComponentName: comp.Name}); err != nil {
		return fmt.Errorf("failed to list release bindings: %w", err)
	}
	for i := range bindingList.Items {
		if bindingList.Items[i].Spec.ReleaseName != "" {
			inUse[bindingList.Items[i].Spec.ReleaseName] = true
		}
	}

	// Also protect the LatestRelease
	if comp.Status.LatestRelease != nil && comp.Status.LatestRelease.Name != "" {
		inUse[comp.Status.LatestRelease.Name] = true
	}

	// Sort releases by creation timestamp ascending (oldest first)
	releases := releaseList.Items
	sort.Slice(releases, func(i, j int) bool {
		return releases[i].CreationTimestamp.Before(&releases[j].CreationTimestamp)
	})

	// Calculate how many need to be deleted
	excess := len(releases) - r.RevisionHistoryLimit
	var errs []error

	for i := range releases {
		if excess <= 0 {
			break
		}
		release := &releases[i]
		if inUse[release.Name] {
			continue
		}
		logger.Info("Deleting old ComponentRelease", "release", release.Name)

		deleteCtx, deleteCancel := context.WithTimeout(ctx, cleanupDeleteTimeout)
		if err := client.IgnoreNotFound(r.Delete(deleteCtx, release)); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete ComponentRelease %s: %w", release.Name, err))
			deleteCancel()
			continue
		}
		deleteCancel()
		excess--
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during ComponentRelease cleanup: %v", errs)
	}
	return nil
}

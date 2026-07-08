// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	controllerpkg "github.com/openchoreo/openchoreo/internal/controller"
)

// seedBindingPins fills spec.projectRelease on every ProjectReleaseBinding of
// the project whose pin is empty, using status.latestRelease. An empty pin
// means "seed once with the project's latest release": clients (console, occ,
// API) create bindings without a pin because the first ProjectRelease name is
// not knowable at creation time (it embeds a controller-computed hash). A
// non-empty pin is never touched — advancing it (promotion) stays external.
func (r *Reconciler) seedBindingPins(ctx context.Context, project *openchoreov1alpha1.Project) error {
	if project.Status.LatestRelease == nil {
		// No release cut yet; nothing to seed. The next reconcile after the
		// release lands picks these bindings up.
		return nil
	}

	bindings := &openchoreov1alpha1.ProjectReleaseBindingList{}
	if err := r.List(ctx, bindings,
		client.InNamespace(project.Namespace),
		client.MatchingFields{controllerpkg.IndexKeyProjectReleaseBindingOwner: project.Name}); err != nil {
		return fmt.Errorf("list ProjectReleaseBindings for project %q: %w", project.Name, err)
	}

	releaseName := project.Status.LatestRelease.Name
	for i := range bindings.Items {
		binding := &bindings.Items[i]
		if binding.Spec.ProjectRelease != "" {
			continue
		}
		binding.Spec.ProjectRelease = releaseName
		if err := r.Update(ctx, binding); err != nil {
			// Conflicts (concurrent writer, slightly stale informer cache) surface
			// as reconcile errors; controller-runtime re-enqueues with backoff and
			// the next pass reads a fresh copy.
			return fmt.Errorf("seed ProjectRelease pin on ProjectReleaseBinding %q: %w", binding.Name, err)
		}
		log.FromContext(ctx).Info("Seeded ProjectReleaseBinding pin",
			"name", binding.Name, "environment", binding.Spec.Environment, "projectRelease", releaseName)
	}
	return nil
}

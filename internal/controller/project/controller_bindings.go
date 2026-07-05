// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	controllerpkg "github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
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

// reconcileBindings creates an initial ProjectReleaseBinding for every
// environment declared in the Project's DeploymentPipeline if one does
// not already exist. Created bindings are pinned to the latest
// ProjectRelease at creation time, carry empty environmentConfigs, and
// are Project-owned (OwnerReference) so K8s GC cascades on Project
// delete.
//
// The controller is **create-only**: existing bindings (whether owned
// by this Project or authored externally) are left untouched. Advancing
// spec.projectRelease after the initial creation is the responsibility
// of whoever drives promotion (occ, GitOps, manual kubectl edit).
func (r *Reconciler) reconcileBindings(ctx context.Context, project *openchoreov1alpha1.Project) error {
	if project.Status.LatestRelease == nil {
		// No release cut yet; nothing to bind. reconcileProjectRelease will
		// drive the next iteration once the release lands.
		return nil
	}

	pipeline, err := r.findDeploymentPipeline(ctx, project)
	if err != nil {
		return err
	}
	if pipeline == nil {
		// DeploymentPipeline missing; the existing reconcile already logs
		// this in findDeploymentPipeline. The DP watch re-enqueues us when
		// it lands.
		return nil
	}

	envNames := r.findEnvironmentNamesFromDeploymentPipeline(pipeline)
	for _, envName := range envNames {
		if err := r.ensureProjectReleaseBinding(ctx, project, envName, project.Status.LatestRelease.Name); err != nil {
			return err
		}
	}
	return nil
}

// ensureProjectReleaseBinding creates a ProjectReleaseBinding for the
// (project, env) tuple if none exists. Existence is determined by spec
// identity (owner.projectName + environment) via the shared
// IndexKeyProjectReleaseBindingOwnerEnv field index. The controller never
// updates an existing binding.
func (r *Reconciler) ensureProjectReleaseBinding(
	ctx context.Context,
	project *openchoreov1alpha1.Project,
	envName, releaseName string,
) error {
	key := controllerpkg.MakeProjectReleaseBindingOwnerEnvKey(project.Name, envName)
	bindings := &openchoreov1alpha1.ProjectReleaseBindingList{}
	if err := r.List(ctx, bindings,
		client.InNamespace(project.Namespace),
		client.MatchingFields{controllerpkg.IndexKeyProjectReleaseBindingOwnerEnv: key}); err != nil {
		return fmt.Errorf("list ProjectReleaseBindings for (project=%q, env=%q): %w", project.Name, envName, err)
	}
	if len(bindings.Items) > 0 {
		return nil
	}
	return r.createProjectReleaseBinding(ctx, project, projectReleaseBindingName(project.Name, envName), envName, releaseName)
}

// createProjectReleaseBinding creates a fresh Project-owned binding with
// the given env and release pin. environmentConfigs is left unset; the
// inlined (Cluster)ProjectType.spec.environmentConfigs defaults apply at
// render time.
func (r *Reconciler) createProjectReleaseBinding(
	ctx context.Context,
	project *openchoreov1alpha1.Project,
	name, envName, releaseName string,
) error {
	binding := &openchoreov1alpha1.ProjectReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: project.Namespace,
			Labels: map[string]string{
				labels.LabelKeyProjectName:     project.Name,
				labels.LabelKeyEnvironmentName: envName,
			},
		},
		Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
			Owner: openchoreov1alpha1.ProjectReleaseBindingOwner{
				ProjectName: project.Name,
			},
			Environment:    envName,
			ProjectRelease: releaseName,
		},
	}
	if err := controllerutil.SetControllerReference(project, binding, r.Scheme); err != nil {
		return fmt.Errorf("set owner ref on ProjectReleaseBinding %q: %w", name, err)
	}
	if err := r.Create(ctx, binding); err != nil {
		return fmt.Errorf("create ProjectReleaseBinding %q: %w", name, err)
	}
	log.FromContext(ctx).Info("Created ProjectReleaseBinding",
		"name", name, "environment", envName, "projectRelease", releaseName)
	return nil
}

// projectReleaseBindingName returns the deterministic name used for an
// auto-created ProjectReleaseBinding. There is exactly one binding per
// (project, environment) tuple; no hash suffix is needed because the
// tuple itself is unique.
func projectReleaseBindingName(projectName, envName string) string {
	return fmt.Sprintf("%s-%s", projectName, envName)
}

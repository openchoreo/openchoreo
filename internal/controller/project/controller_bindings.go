// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

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

// ensureProjectReleaseBinding creates a ProjectReleaseBinding named
// "<project>-<env>" if no binding exists at that name; otherwise leaves
// the existing binding untouched. The controller never advances
// spec.projectRelease on an existing binding — promotions are driven
// externally.
func (r *Reconciler) ensureProjectReleaseBinding(
	ctx context.Context,
	project *openchoreov1alpha1.Project,
	envName, releaseName string,
) error {
	name := projectReleaseBindingName(project.Name, envName)

	binding := &openchoreov1alpha1.ProjectReleaseBinding{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: project.Namespace}, binding)
	if err == nil {
		// Binding already exists at the target name — do nothing.
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get ProjectReleaseBinding %q: %w", name, err)
	}
	return r.createProjectReleaseBinding(ctx, project, name, envName, releaseName)
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

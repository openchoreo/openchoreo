// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// firstEnvironment resolves the root environment of the Endpoint's project
// deployment pipeline. A configuration problem (missing project or pipeline, a
// pipeline without a root) is returned as a message with no error, so the
// caller can surface it on status without retrying.
func (r *Reconciler) firstEnvironment(ctx context.Context, ep *openchoreov1alpha1.Endpoint) (string, string, error) {
	project := &openchoreov1alpha1.Project{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: ep.Namespace, Name: ep.Spec.Owner.ProjectName}, project); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Sprintf("Project %q not found", ep.Spec.Owner.ProjectName), nil
		}
		return "", "", fmt.Errorf("get Project %q: %w", ep.Spec.Owner.ProjectName, err)
	}
	if project.Spec.DeploymentPipelineRef.Name == "" {
		return "", fmt.Sprintf("Project %q has empty deploymentPipelineRef", project.Name), nil
	}

	pipeline := &openchoreov1alpha1.DeploymentPipeline{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: project.Namespace,
		Name:      project.Spec.DeploymentPipelineRef.Name,
	}, pipeline); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Sprintf("DeploymentPipeline %q not found", project.Spec.DeploymentPipelineRef.Name), nil
		}
		return "", "", fmt.Errorf("get DeploymentPipeline %q: %w", project.Spec.DeploymentPipelineRef.Name, err)
	}

	env, err := controller.FindRootEnvironment(pipeline)
	if err != nil {
		return "", fmt.Sprintf("Invalid deployment pipeline configuration: %v", err), nil
	}
	return env, "", nil
}

// ensureAutoDeployBinding points the Endpoint's binding for env at releaseName,
// creating it as {endpoint}-{env} when the Endpoint has none for that
// environment. Mirrors the Component controller's ensureReleaseBinding: only the
// first environment is managed, so promotions to later environments stay
// explicit.
func (r *Reconciler) ensureAutoDeployBinding(
	ctx context.Context, ep *openchoreov1alpha1.Endpoint, env, releaseName string,
) error {
	logger := log.FromContext(ctx)

	var bindings openchoreov1alpha1.EndpointReleaseBindingList
	if err := r.List(ctx, &bindings, client.InNamespace(ep.Namespace),
		client.MatchingFields{controller.IndexKeyEndpointReleaseBindingOwnerEndpointName: ep.Name}); err != nil {
		return fmt.Errorf("list EndpointReleaseBindings: %w", err)
	}

	var existing []*openchoreov1alpha1.EndpointReleaseBinding
	for i := range bindings.Items {
		b := &bindings.Items[i]
		if b.Spec.Owner.ProjectName == ep.Spec.Owner.ProjectName && b.Spec.Environment == env {
			existing = append(existing, b)
		}
	}

	switch len(existing) {
	case 0:
		binding := &openchoreov1alpha1.EndpointReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", ep.Name, env),
				Namespace: ep.Namespace,
			},
			Spec: openchoreov1alpha1.EndpointReleaseBindingSpec{
				Owner: openchoreov1alpha1.EndpointReleaseBindingOwner{
					ProjectName:  ep.Spec.Owner.ProjectName,
					EndpointName: ep.Name,
				},
				Environment:     env,
				EndpointRelease: releaseName,
			},
		}
		if err := r.Create(ctx, binding); err != nil {
			return fmt.Errorf("create EndpointReleaseBinding %q: %w", binding.Name, err)
		}
		logger.Info("Created EndpointReleaseBinding", "binding", binding.Name, "release", releaseName, "environment", env)
		return nil
	case 1:
	default:
		return fmt.Errorf("found multiple EndpointReleaseBindings for environment %q", env)
	}

	binding := existing[0]
	if binding.Spec.EndpointRelease == releaseName {
		return nil
	}
	binding.Spec.EndpointRelease = releaseName
	if err := r.Update(ctx, binding); err != nil {
		return fmt.Errorf("update EndpointReleaseBinding %q: %w", binding.Name, err)
	}
	logger.Info("Updated EndpointReleaseBinding with new release",
		"binding", binding.Name, "release", releaseName, "environment", env)
	return nil
}

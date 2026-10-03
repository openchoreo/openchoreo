// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// promotionSources returns the environments that promote into env. It is empty when env is
// not a promotion target, such as the root environment or one the pipeline does not mention.
func promotionSources(pipeline *openchoreov1alpha1.DeploymentPipeline, env string) []string {
	var sources []string
	for _, path := range pipeline.Spec.PromotionPaths {
		for _, target := range path.TargetEnvironmentRefs {
			if target.Name == env {
				sources = append(sources, path.SourceEnvironmentRef.Name)
				break
			}
		}
	}
	return sources
}

// isPromoted reports whether the binding's release is bound in one of the source environments,
// or is the release this binding has already rendered. The second case keeps a running release
// in place after the source environment moves on to a newer one.
func (r *Reconciler) isPromoted(ctx context.Context, rb *openchoreov1alpha1.ReleaseBinding,
	componentRelease *openchoreov1alpha1.ComponentRelease, sources []string) (bool, error) {
	for _, source := range sources {
		var bindings openchoreov1alpha1.ReleaseBindingList
		if err := r.List(ctx, &bindings,
			client.InNamespace(rb.Namespace),
			client.MatchingFields{controller.IndexKeyReleaseBindingOwnerEnv: controller.MakeReleaseBindingOwnerEnvKey(
				rb.Spec.Owner.ProjectName, rb.Spec.Owner.ComponentName, source)},
		); err != nil {
			return false, fmt.Errorf("failed to list ReleaseBindings for environment %q: %w", source, err)
		}
		for _, binding := range bindings.Items {
			if binding.Spec.ReleaseName == rb.Spec.ReleaseName {
				return true, nil
			}
		}
	}

	rendered := &openchoreov1alpha1.RenderedRelease{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      makeDataPlaneReleaseName(componentRelease, rb),
		Namespace: rb.Namespace,
	}, rendered)
	if err != nil {
		return false, client.IgnoreNotFound(err)
	}
	return metav1.IsControlledBy(rendered, rb) &&
		rendered.Labels[labels.LabelKeyComponentReleaseName] == rb.Spec.ReleaseName, nil
}

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"fmt"
	"slices"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

func (s *releaseBindingService) validatePromotionPath(ctx context.Context, namespaceName string,
	rb, existing *openchoreov1alpha1.ReleaseBinding) error {
	// Bindings without a release wait for auto-deploy. Undeploy never needs a promotion.
	if rb.Spec.ReleaseName == "" || rb.Spec.State == openchoreov1alpha1.ReleaseStateUndeploy {
		return nil
	}

	project := &openchoreov1alpha1.Project{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: rb.Spec.Owner.ProjectName}, project); err != nil {
		if apierrors.IsNotFound(err) {
			return &services.ValidationError{Msg: fmt.Sprintf("Project %q not found", rb.Spec.Owner.ProjectName)}
		}
		return fmt.Errorf("failed to get project for promotion: %w", err)
	}
	pipeline := &openchoreov1alpha1.DeploymentPipeline{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: project.Spec.DeploymentPipelineRef.Name}, pipeline); err != nil {
		if apierrors.IsNotFound(err) {
			return &services.ValidationError{Msg: fmt.Sprintf("DeploymentPipeline %q not found", project.Spec.DeploymentPipelineRef.Name)}
		}
		return fmt.Errorf("failed to get deployment pipeline for promotion: %w", err)
	}

	var sources []string
	for _, path := range pipeline.Spec.PromotionPaths {
		for _, target := range path.TargetEnvironmentRefs {
			if target.Name == rb.Spec.Environment {
				sources = append(sources, path.SourceEnvironmentRef.Name)
				break
			}
		}
	}
	if len(sources) == 0 {
		return nil
	}

	var bindings openchoreov1alpha1.ReleaseBindingList
	if err := s.k8sClient.List(ctx, &bindings, client.InNamespace(namespaceName)); err != nil {
		return fmt.Errorf("failed to list release bindings for promotion: %w", err)
	}
	for _, binding := range bindings.Items {
		if binding.Spec.Owner == rb.Spec.Owner && binding.Spec.ReleaseName == rb.Spec.ReleaseName &&
			slices.Contains(sources, binding.Spec.Environment) {
			return nil
		}
	}

	// Only persisted ownership proves that this binding already rendered the release.
	// Request UIDs and status must not let a new or blocked binding bypass promotion.
	if existing != nil && existing.Spec.Owner == rb.Spec.Owner && existing.Spec.Environment == rb.Spec.Environment {
		rendered := &openchoreov1alpha1.RenderedRelease{}
		key := client.ObjectKey{Namespace: namespaceName, Name: fmt.Sprintf("%s-%s", rb.Spec.Owner.ComponentName, rb.Spec.Environment)}
		if err := s.k8sClient.Get(ctx, key, rendered); err != nil {
			if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to get rendered release for promotion: %w", err)
			}
		} else if metav1.IsControlledBy(rendered, existing) && rendered.Labels[labels.LabelKeyComponentReleaseName] == rb.Spec.ReleaseName {
			return nil
		}
	}

	return &services.ValidationError{Msg: fmt.Sprintf("ComponentRelease %q must be referenced by a ReleaseBinding in %s before it can be promoted to %s",
		rb.Spec.ReleaseName, strings.Join(sources, " or "), rb.Spec.Environment)}
}

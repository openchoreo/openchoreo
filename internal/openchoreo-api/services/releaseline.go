// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ValidateReleaseLineOwner checks the release line a resource is created for against its
// component. An empty line is the component's default line; any other must be one of the component's
// own lines. A component that doesn't exist is left to the caller.
func ValidateReleaseLineOwner(ctx context.Context, c client.Reader, namespace, projectName, componentName, releaseLine string) error {
	if componentName == "" || releaseLine == "" {
		return nil
	}
	component := &openchoreov1alpha1.Component{}
	if err := c.Get(ctx, client.ObjectKey{Name: componentName, Namespace: namespace}, component); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get component %s: %w", componentName, err)
	}

	line := &openchoreov1alpha1.ReleaseLine{}
	if err := c.Get(ctx, client.ObjectKey{Name: releaseLine, Namespace: namespace}, line); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get release line %s: %w", releaseLine, err)
		}
		line = nil
	}
	if line == nil || line.Spec.Owner.ComponentName != componentName ||
		(projectName != "" && line.Spec.Owner.ProjectName != projectName) {
		return &ValidationError{Msg: fmt.Sprintf("release line %q not found for component %q", releaseLine, componentName)}
	}
	return nil
}

// ValidateReleaseLinesAllowed checks that a component type allows release lines.
// A type that doesn't exist is left to the controller, which reports it on the component.
func ValidateReleaseLinesAllowed(ctx context.Context, c client.Reader, namespace string, ref openchoreov1alpha1.ComponentTypeRef) error {
	name := ref.Name
	if _, n, ok := strings.Cut(ref.Name, "/"); ok {
		name = n
	}

	var policy openchoreov1alpha1.ReleaseLinePolicy
	if ref.Kind == openchoreov1alpha1.ComponentTypeRefKindClusterComponentType {
		cct := &openchoreov1alpha1.ClusterComponentType{}
		if err := c.Get(ctx, client.ObjectKey{Name: name}, cct); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to get ClusterComponentType %s: %w", name, err)
		}
		policy = cct.Spec.ReleaseLines
	} else {
		ct := &openchoreov1alpha1.ComponentType{}
		if err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, ct); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to get ComponentType %s: %w", name, err)
		}
		policy = ct.Spec.ReleaseLines
	}

	if !policy.AllowsReleaseLines() {
		return ReleaseLinesNotAllowedError(ref.Name)
	}
	return nil
}

// ReleaseLinesNotAllowedError reports a component type that does not allow release lines.
func ReleaseLinesNotAllowedError(componentTypeName string) error {
	return &ValidationError{Msg: fmt.Sprintf(
		"component type %q does not allow release lines; its spec.releaseLines must be Allowed", componentTypeName)}
}

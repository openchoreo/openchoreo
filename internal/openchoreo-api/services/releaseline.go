// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ValidateReleaseLineOwner checks the release line a resource is created for against its
// component. A component with release lines enabled requires one of its own lines, and a
// component without them accepts none. A component that doesn't exist is left to the caller.
func ValidateReleaseLineOwner(ctx context.Context, c client.Reader, namespace, projectName, componentName, releaseLine string) error {
	if componentName == "" {
		return nil
	}
	component := &openchoreov1alpha1.Component{}
	if err := c.Get(ctx, client.ObjectKey{Name: componentName, Namespace: namespace}, component); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get component %s: %w", componentName, err)
	}

	switch {
	case !component.Spec.ReleaseLinesEnabled && releaseLine == "":
		return nil
	case !component.Spec.ReleaseLinesEnabled:
		return &ValidationError{Msg: fmt.Sprintf("component %q does not have release lines enabled", componentName)}
	case releaseLine == "":
		return &ValidationError{Msg: fmt.Sprintf("component %q has release lines enabled; a release line is required", componentName)}
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

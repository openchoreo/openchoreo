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
// component. An empty line is the component's main line; any other must be one of the component's
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

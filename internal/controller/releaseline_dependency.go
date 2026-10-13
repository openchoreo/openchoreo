// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ReleaseLineDependencyProblem explains why a dependency on a component's release line matches
// no ReleaseBinding in the environment. It returns "" when the cause is not about release lines,
// such as an unknown component or a component without lines that is not deployed.
func ReleaseLineDependencyProblem(ctx context.Context, c client.Reader,
	namespace, project, component, releaseLine, environment string) (string, error) {
	comp := &openchoreov1alpha1.Component{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: component}, comp); err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get component %s/%s: %w", project, component, err)
	}
	if comp.Spec.Owner.ProjectName != project {
		return "", nil
	}

	switch {
	case comp.Spec.ReleaseLinesEnabled && releaseLine == "":
		return fmt.Sprintf("component %s/%s uses release lines; set releaseLine on the dependency", project, component), nil
	case !comp.Spec.ReleaseLinesEnabled && releaseLine != "":
		return fmt.Sprintf("component %s/%s does not have release lines enabled; remove releaseLine from the dependency",
			project, component), nil
	case releaseLine == "":
		return "", nil
	}

	line := &openchoreov1alpha1.ReleaseLine{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: releaseLine}, line); err != nil {
		if !apierrors.IsNotFound(err) {
			return "", fmt.Errorf("failed to get release line %q: %w", releaseLine, err)
		}
		line = nil
	}
	if line == nil || line.Spec.Owner.ComponentName != component || line.Spec.Owner.ProjectName != project {
		return fmt.Sprintf("release line %q not found for component %s/%s", releaseLine, project, component), nil
	}
	return fmt.Sprintf("release line %q of component %s/%s is not deployed in environment %s",
		releaseLine, project, component, environment), nil
}

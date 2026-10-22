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
// no ReleaseBinding in the environment. A dependency without a line targets the default line. It returns
// "" when the cause is not about release lines, such as an unknown component or a component without
// lines that is not deployed.
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

	if releaseLine == "" {
		var lines openchoreov1alpha1.ReleaseLineList
		if err := c.List(ctx, &lines, client.InNamespace(namespace)); err != nil {
			return "", fmt.Errorf("failed to list release lines: %w", err)
		}
		for i := range lines.Items {
			if lines.Items[i].Spec.Owner.ComponentName == component && lines.Items[i].Spec.Owner.ProjectName == project {
				return fmt.Sprintf("the default line of component %s/%s is not deployed in environment %s; "+
					"set releaseLine to depend on one of its release lines", project, component, environment), nil
			}
		}
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

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/releaseline"
)

// componentForLine returns the component with the release line's overrides applied, or the component
// itself when line is empty. It enforces that a line is given exactly when the component uses lines.
func (s *componentService) componentForLine(ctx context.Context, comp *openchoreov1alpha1.Component, line string) (*openchoreov1alpha1.Component, error) {
	switch {
	case comp.Spec.ReleaseLinesEnabled && line == "":
		return nil, ErrReleaseLineRequired
	case !comp.Spec.ReleaseLinesEnabled && line != "":
		return nil, ErrReleaseLinesNotEnabled
	case line == "":
		return comp, nil
	}

	rl, err := s.componentReleaseLine(ctx, comp, line)
	if err != nil {
		return nil, err
	}

	effective, err := releaseline.EffectiveComponent(comp, rl)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrValidation, err)
	}
	return effective, nil
}

// componentReleaseLine returns the named release line of comp, or ErrReleaseLineNotFound when the
// line does not exist or belongs to another component.
func (s *componentService) componentReleaseLine(ctx context.Context, comp *openchoreov1alpha1.Component, name string) (*openchoreov1alpha1.ReleaseLine, error) {
	rl := &openchoreov1alpha1.ReleaseLine{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: comp.Namespace, Name: name}, rl); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrReleaseLineNotFound
		}
		return nil, fmt.Errorf("failed to get release line: %w", err)
	}
	if rl.Spec.Owner.ProjectName != comp.Spec.Owner.ProjectName || rl.Spec.Owner.ComponentName != comp.Name {
		return nil, ErrReleaseLineNotFound
	}
	return rl, nil
}

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// This file holds what the bindings share to keep a release in Observe mode.

// ErrNotOwned is returned when a binding's RenderedRelease exists but another
// object controls it.
var ErrNotOwned = errors.New("release exists but is not owned by this binding")

// EmitObserved creates or updates owner's RenderedRelease at key in Observe
// mode, with labels and spec, and makes owner its controller. It returns
// ErrNotOwned when another object controls the release.
func EmitObserved(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	owner client.Object,
	key types.NamespacedName,
	labels map[string]string,
	spec openchoreov1alpha1.RenderedReleaseSpec,
) (*openchoreov1alpha1.RenderedRelease, controllerutil.OperationResult, error) {
	rr := &openchoreov1alpha1.RenderedRelease{
		ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace},
	}
	op, err := controllerutil.CreateOrUpdate(ctx, c, rr, func() error {
		if rr.UID != "" && !metav1.IsControlledBy(rr, owner) {
			return ErrNotOwned
		}
		rr.Labels = labels
		rr.Spec = spec
		rr.Spec.Mode = openchoreov1alpha1.RenderedReleaseModeObserve
		return controllerutil.SetControllerReference(owner, rr, scheme)
	})
	if err != nil {
		return nil, op, err
	}
	return rr, op, nil
}

// GetObserving returns the RenderedRelease at key if owner controls it, it is
// not being deleted, and it is in Observe mode, and nil otherwise.
func GetObserving(
	ctx context.Context,
	c client.Reader,
	owner metav1.Object,
	key types.NamespacedName,
) (*openchoreov1alpha1.RenderedRelease, error) {
	rr := &openchoreov1alpha1.RenderedRelease{}
	if err := c.Get(ctx, key, rr); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get RenderedRelease %q: %w", key.Name, err)
	}
	if !metav1.IsControlledBy(rr, owner) || !rr.DeletionTimestamp.IsZero() ||
		rr.Spec.Mode != openchoreov1alpha1.RenderedReleaseModeObserve {
		return nil, nil
	}
	return rr, nil
}

// DeleteObserving deletes a release read in Observe mode. The delete is
// conditioned on the release's UID and resourceVersion, so a release that has
// since been converted to Manage, and may have applied resources, is never
// deleted from a stale read.
func DeleteObserving(ctx context.Context, c client.Writer, rr *openchoreov1alpha1.RenderedRelease) error {
	err := c.Delete(ctx, rr, client.Preconditions{UID: &rr.UID, ResourceVersion: &rr.ResourceVersion})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete RenderedRelease %q: %w", rr.Name, err)
	}
	return nil
}

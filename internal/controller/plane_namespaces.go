// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/planenamespace"
)

const (
	// PlaneDeletionRetryInterval is how often a plane finalizer retries while its
	// deletion waits on secrets or on reaching the plane.
	PlaneDeletionRetryInterval = 30 * time.Second

	// ConditionPlaneFinalizing reports progress of a plane's deletion.
	ConditionPlaneFinalizing ConditionType = "Finalizing"

	// ReasonPlaneDeletionBlocked is used while a plane's deletion waits.
	ReasonPlaneDeletionBlocked ConditionReason = "DeletionBlocked"
)

// PlaneClientFunc returns a client for the cluster of a plane.
type PlaneClientFunc func() (client.Client, error)

// NewPlaneDeletionBlockedCondition reports why a plane's deletion is waiting.
func NewPlaneDeletionBlockedCondition(generation int64, msg string) metav1.Condition {
	return NewCondition(ConditionPlaneFinalizing, metav1.ConditionFalse, ReasonPlaneDeletionBlocked, msg, generation)
}

// FinalizeNamespacedPlane handles the plane namespaces of a namespaced plane's
// control plane namespace when the plane is deleted, and returns a message
// when the deletion must wait.
//
// Plane namespaces are shared by every plane that targets the same cluster, so
// they are deleted only when the control plane namespace itself is going away.
// Otherwise the plane waits until no secret targets it, as nothing would
// clean up its plane namespaces once the plane is gone.
func FinalizeNamespacedPlane(ctx context.Context, c client.Client, live client.Reader, plane client.Object, planeKind string, planeClient PlaneClientFunc) (string, error) {
	terminating, err := namespaceGoneOrTerminating(ctx, live, plane.GetNamespace())
	if err != nil {
		return "", err
	}
	if !terminating {
		count, err := CountPlaneSecretReferences(ctx, c, plane.GetNamespace(), planeKind, plane.GetName())
		if err != nil {
			return "", err
		}
		if count > 0 {
			return fmt.Sprintf("Deletion blocked: %d secret(s) still target this plane", count), nil
		}
		return "", nil
	}

	pc, err := planeClient()
	if err == nil {
		err = planenamespace.DeleteForNamespace(ctx, pc, plane.GetNamespace())
	}
	if err != nil {
		return fmt.Sprintf("Waiting to delete the plane namespaces of %s: %v", plane.GetNamespace(), err), nil
	}
	return "", nil
}

// FinalizeClusterPlane handles plane namespaces when a cluster-scoped plane is
// deleted, and returns a message when the deletion must wait. The plane waits
// until no secret targets it, then sweeps its cluster one last time.
func FinalizeClusterPlane(ctx context.Context, c client.Client, live client.Reader, planeKind, planeName string, planeClient PlaneClientFunc) (string, error) {
	count, err := CountPlaneSecretReferences(ctx, c, "", planeKind, planeName)
	if err != nil {
		return "", err
	}
	if count > 0 {
		return fmt.Sprintf("Deletion blocked: %d secret(s) still target this plane", count), nil
	}

	pc, err := planeClient()
	if err == nil {
		_, err = planenamespace.Sweep(ctx, pc, c, live)
	}
	if err != nil {
		return fmt.Sprintf("Waiting to delete stale plane namespaces: %v", err), nil
	}
	return "", nil
}

// SweepPlaneNamespaces deletes the plane namespaces in a cluster-scoped plane
// whose control plane namespace is gone or terminating. Failures are logged
// and retried on the next reconcile.
func SweepPlaneNamespaces(ctx context.Context, c, live client.Reader, planeClient PlaneClientFunc) {
	logger := log.FromContext(ctx)

	pc, err := planeClient()
	if err != nil {
		logger.Error(err, "Failed to get plane client for plane namespace sweep")
		return
	}
	deleted, err := planenamespace.Sweep(ctx, pc, c, live)
	if len(deleted) > 0 {
		logger.Info("Deleted plane namespaces of removed control plane namespaces", "namespaces", deleted)
	}
	if err != nil {
		logger.Error(err, "Failed to sweep plane namespaces")
	}
}

// CountPlaneSecretReferences counts the SecretReferences of Secret API secrets
// and git secrets that target a plane. An empty namespace counts across all
// namespaces.
func CountPlaneSecretReferences(ctx context.Context, c client.Reader, namespace, planeKind, planeName string) (int, error) {
	var opts []client.ListOption
	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}
	var list openchoreov1alpha1.SecretReferenceList
	if err := c.List(ctx, &list, opts...); err != nil {
		return 0, fmt.Errorf("failed to list secret references: %w", err)
	}

	count := 0
	for i := range list.Items {
		if secretReferenceTargets(&list.Items[i], planeKind, planeName) {
			count++
		}
	}
	return count, nil
}

func secretReferenceTargets(sr *openchoreov1alpha1.SecretReference, planeKind, planeName string) bool {
	if sr.Labels[labels.LabelKeyManagedBy] == labels.LabelValueOpenChoreoAPI {
		target := sr.Spec.TargetPlane
		return target != nil && target.Kind == planeKind && target.Name == planeName
	}
	// Git secrets record their workflow plane in labels instead of spec.targetPlane.
	return sr.Labels[labels.LabelKeySecretType] == labels.LabelValueGitCredentials &&
		sr.Labels[labels.LabelKeyWorkflowPlaneKind] == planeKind &&
		sr.Labels[labels.LabelKeyWorkflowPlaneName] == planeName
}

// ControlPlaneNamespaceRemovedPredicate passes control plane namespaces that
// start terminating or are deleted.
func ControlPlaneNamespaceRemovedPredicate() predicate.Predicate {
	isControlPlane := func(obj client.Object) bool {
		return obj.GetLabels()[labels.LabelKeyControlPlaneNamespace] == labels.LabelValueTrue
	}
	return predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			return isControlPlane(e.ObjectNew) &&
				e.ObjectOld.GetDeletionTimestamp().IsZero() && !e.ObjectNew.GetDeletionTimestamp().IsZero()
		},
		DeleteFunc:  func(e event.DeleteEvent) bool { return isControlPlane(e.Object) },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

func namespaceGoneOrTerminating(ctx context.Context, c client.Reader, name string) (bool, error) {
	ns := &corev1.Namespace{}
	if err := c.Get(ctx, client.ObjectKey{Name: name}, ns); err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, fmt.Errorf("failed to get namespace %s: %w", name, err)
	}
	return !ns.DeletionTimestamp.IsZero(), nil
}

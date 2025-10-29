// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/openchoreo/openchoreo/internal/security-scanner/db/backend"
	"github.com/openchoreo/openchoreo/internal/security-scanner/resolver"
)

type PodReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Queries backend.Querier
}

func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := slog.Default()

	pod := &corev1.Pod{}
	if err := r.Get(ctx, req.NamespacedName, pod); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	resolved, err := resolver.ResolveParentController(ctx, r.Client, pod)
	if err != nil {
		logger.Error("Failed to resolve parent controller",
			"namespace", pod.Namespace,
			"name", pod.Name,
			"error", err)
		return ctrl.Result{}, err
	}

	logger.Info("Resolved parent controller",
		"pod", fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
		"controllerType", resolved.Type,
		"controller", fmt.Sprintf("%s/%s", resolved.Namespace, resolved.Name),
		"resourceVersion", resolved.ResourceVersion)

	resourceID, err := r.Queries.UpsertResource(ctx,
		string(resolved.Type),
		resolved.Namespace,
		resolved.Name,
		resolved.UID,
		resolved.ResourceVersion)
	if err != nil {
		logger.Error("Failed to upsert resource",
			"type", resolved.Type,
			"namespace", resolved.Namespace,
			"name", resolved.Name,
			"error", err)
		return ctrl.Result{}, err
	}

	scannedResource, err := r.Queries.GetPostureScannedResource(ctx,
		string(resolved.Type),
		resolved.Namespace,
		resolved.Name)
	if err != nil && err != sql.ErrNoRows {
		logger.Error("Failed to get scanned resource",
			"type", resolved.Type,
			"namespace", resolved.Namespace,
			"name", resolved.Name,
			"error", err)
		return ctrl.Result{}, err
	}

	if err == nil && scannedResource.ResourceVersion == resolved.ResourceVersion {
		logger.Info("Resource already scanned at this version, skipping",
			"type", resolved.Type,
			"namespace", resolved.Namespace,
			"name", resolved.Name,
			"resourceVersion", resolved.ResourceVersion)
		return ctrl.Result{}, nil
	}

	if err := r.Queries.DeleteResourceLabels(ctx, resourceID); err != nil {
		logger.Error("Failed to delete old resource labels",
			"resourceID", resourceID,
			"error", err)
		return ctrl.Result{}, err
	}

	for key, value := range resolved.Labels {
		if err := r.Queries.InsertResourceLabel(ctx, resourceID, key, value); err != nil {
			logger.Error("Failed to insert resource label",
				"resourceID", resourceID,
				"key", key,
				"value", value,
				"error", err)
			return ctrl.Result{}, err
		}
	}

	logger.Info("Ready to scan resource",
		"type", resolved.Type,
		"namespace", resolved.Namespace,
		"name", resolved.Name,
		"resourceID", resourceID,
		"resourceVersion", resolved.ResourceVersion,
		"labelCount", len(resolved.Labels))

	if err := r.Queries.UpsertPostureScannedResource(ctx, resourceID, resolved.ResourceVersion, nil); err != nil {
		logger.Error("Failed to upsert posture scanned resource",
			"resourceID", resourceID,
			"resourceVersion", resolved.ResourceVersion,
			"error", err)
		return ctrl.Result{}, err
	}

	logger.Info("Marked resource as scanned",
		"type", resolved.Type,
		"namespace", resolved.Namespace,
		"name", resolved.Name,
		"resourceVersion", resolved.ResourceVersion)

	return ctrl.Result{}, nil
}

func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return false
			},
		}).
		Complete(r)
}

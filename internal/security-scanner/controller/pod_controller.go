// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/openchoreo/openchoreo/internal/security-scanner/db/backend"
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

	resourceID, err := r.Queries.UpsertResource(ctx,
		"Pod",
		pod.Namespace,
		pod.Name,
		string(pod.UID),
		pod.ResourceVersion)
	if err != nil {
		logger.Error("Failed to upsert pod resource",
			"namespace", pod.Namespace,
			"name", pod.Name,
			"error", err)
		return ctrl.Result{}, err
	}

	if err := r.Queries.DeleteResourceLabels(ctx, resourceID); err != nil {
		logger.Error("Failed to delete old resource labels",
			"resourceID", resourceID,
			"error", err)
		return ctrl.Result{}, err
	}

	for key, value := range pod.Labels {
		if err := r.Queries.InsertResourceLabel(ctx, resourceID, key, value); err != nil {
			logger.Error("Failed to insert resource label",
				"resourceID", resourceID,
				"key", key,
				"value", value,
				"error", err)
			return ctrl.Result{}, err
		}
	}

	logger.Info("Pod resource upserted with labels",
		"name", pod.Name,
		"namespace", pod.Namespace,
		"resourceID", resourceID,
		"labelCount", len(pod.Labels))

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

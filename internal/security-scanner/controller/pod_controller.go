// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"database/sql"
	"errors"
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

	podFullName := pod.Namespace + "/" + pod.Name

	podExists, err := r.Queries.GetScannedPodByName(ctx, podFullName)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		logger.Error("Failed to get pod from database",
			"name", podFullName,
			"error", err)
		return ctrl.Result{}, err
	}
	if podExists.ID != 0 {
		logger.Info("Pod already exists in database",
			"name", podFullName)
		return ctrl.Result{}, nil
	}

	if err := r.Queries.InsertScannedPod(ctx, podFullName); err != nil {
		logger.Error("Failed to insert pod into database",
			"name", podFullName,
			"error", err)
		return ctrl.Result{}, err
	}

	logger.Info("New pod created and stored",
		"name", pod.Name,
		"namespace", pod.Namespace,
		"phase", pod.Status.Phase)

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

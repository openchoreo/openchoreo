// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/security-scanner/checkov"
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
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
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

	scanStartTime := time.Now()

	manifest, err := r.generateManifest(resolved.Object)
	if err != nil {
		logger.Error("Failed to generate manifest",
			"type", resolved.Type,
			"namespace", resolved.Namespace,
			"name", resolved.Name,
			"error", err)
		return ctrl.Result{}, err
	}

	findings, err := checkov.RunCheckov(ctx, manifest)
	if err != nil {
		logger.Error("Failed to run Checkov scan",
			"type", resolved.Type,
			"namespace", resolved.Namespace,
			"name", resolved.Name,
			"error", err)
		return ctrl.Result{}, err
	}

	if err := r.Queries.DeletePostureFindingsByResourceID(ctx, resourceID); err != nil {
		logger.Error("Failed to delete old posture findings",
			"resourceID", resourceID,
			"error", err)
		return ctrl.Result{}, err
	}

	for _, finding := range findings {
		var category, description, remediation *string
		if finding.Category != "" {
			category = &finding.Category
		}
		if finding.Description != "" {
			description = &finding.Description
		}
		if finding.Remediation != "" {
			remediation = &finding.Remediation
		}

		if err := r.Queries.InsertPostureFinding(ctx, resourceID, finding.CheckID, finding.CheckName, string(finding.Severity), category, description, remediation, resolved.ResourceVersion); err != nil {
			logger.Error("Failed to insert posture finding",
				"resourceID", resourceID,
				"checkID", finding.CheckID,
				"checkName", finding.CheckName,
				"severity", finding.Severity,
				"error", err)
			return ctrl.Result{}, err
		}
	}

	scanDuration := time.Since(scanStartTime).Milliseconds()

	if err := r.Queries.UpsertPostureScannedResource(ctx, resourceID, resolved.ResourceVersion, &scanDuration); err != nil {
		logger.Error("Failed to upsert posture scanned resource",
			"resourceID", resourceID,
			"resourceVersion", resolved.ResourceVersion,
			"error", err)
		return ctrl.Result{}, err
	}

	logger.Info("Successfully scanned resource",
		"type", resolved.Type,
		"namespace", resolved.Namespace,
		"name", resolved.Name,
		"resourceVersion", resolved.ResourceVersion,
		"scanDurationMs", scanDuration,
		"findingsCount", len(findings))

	return ctrl.Result{}, nil
}

func (r *PodReconciler) generateManifest(obj runtime.Object) ([]byte, error) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata accessor: %w", err)
	}

	// Get GVK from the object
	gvk := obj.GetObjectKind().GroupVersionKind()
	typeMeta := metav1.TypeMeta{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
	}

	objectMeta := metav1.ObjectMeta{
		Name:              accessor.GetName(),
		Namespace:         accessor.GetNamespace(),
		Labels:            accessor.GetLabels(),
		Annotations:       accessor.GetAnnotations(),
		UID:               accessor.GetUID(),
		ResourceVersion:   accessor.GetResourceVersion(),
		Generation:        accessor.GetGeneration(),
		CreationTimestamp: accessor.GetCreationTimestamp(),
	}

	// For different resource types, we need to extract the spec
	switch obj := obj.(type) {
	case *appsv1.Deployment:
		deployment := &appsv1.Deployment{
			TypeMeta:   typeMeta,
			ObjectMeta: objectMeta,
			Spec:       obj.Spec,
		}
		return yaml.Marshal(deployment)
	case *appsv1.StatefulSet:
		sts := &appsv1.StatefulSet{
			TypeMeta:   typeMeta,
			ObjectMeta: objectMeta,
			Spec:       obj.Spec,
		}
		return yaml.Marshal(sts)
	case *appsv1.DaemonSet:
		ds := &appsv1.DaemonSet{
			TypeMeta:   typeMeta,
			ObjectMeta: objectMeta,
			Spec:       obj.Spec,
		}
		return yaml.Marshal(ds)
	case *batchv1.Job:
		job := &batchv1.Job{
			TypeMeta:   typeMeta,
			ObjectMeta: objectMeta,
			Spec:       obj.Spec,
		}
		return yaml.Marshal(job)
	case *batchv1.CronJob:
		cronjob := &batchv1.CronJob{
			TypeMeta:   typeMeta,
			ObjectMeta: objectMeta,
			Spec:       obj.Spec,
		}
		return yaml.Marshal(cronjob)
	case *corev1.Pod:
		pod := &corev1.Pod{
			TypeMeta:   typeMeta,
			ObjectMeta: objectMeta,
			Spec:       obj.Spec,
		}
		return yaml.Marshal(pod)
	default:
		return nil, fmt.Errorf("unsupported resource type: %T", obj)
	}
}

func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Only process updates if the resourceVersion changed
				oldPod := e.ObjectOld.(*corev1.Pod)
				newPod := e.ObjectNew.(*corev1.Pod)
				return oldPod.ResourceVersion != newPod.ResourceVersion
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

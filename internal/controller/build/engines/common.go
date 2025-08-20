// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package engines

import (
	"context"
	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller/build/names"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Common step names used across build engines
const (
	StepPush           = "push-step"
	StepWorkloadCreate = "workload-create-step"
)

// EnsureResource creates a resource if it doesn't exist
// This is a common utility function shared across all build engines
func EnsureResource(ctx context.Context, client client.Client, obj client.Object, resourceType string, logger logr.Logger) error {
	err := client.Create(ctx, obj)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			logger.V(1).Info("Resource already exists", "type", resourceType, "name", obj.GetName(), "namespace", obj.GetNamespace())
			return nil
		}
		logger.Error(err, "Failed to create resource", "type", resourceType, "name", obj.GetName(), "namespace", obj.GetNamespace())
		return err
	}
	logger.Info("Created resource", "type", resourceType, "name", obj.GetName(), "namespace", obj.GetNamespace())
	return nil
}

// MakeNamespace creates a namespace for the build
func MakeNamespace(build *openchoreov1alpha1.Build) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   names.MakeNamespaceName(build),
			Labels: names.MakeWorkflowLabels(build),
		},
	}
}

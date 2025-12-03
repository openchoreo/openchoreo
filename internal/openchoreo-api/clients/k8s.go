// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8s

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func NewK8sClient() (client.Client, error) {
	config, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes config: %w", err)
	}

	scheme := runtime.NewScheme()

	// Add core Kubernetes types (Secret, ConfigMap, etc.)
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add core v1 scheme: %w", err)
	}

	// Add OpenChoreo custom types
	if err := openchoreov1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add OpenChoreo scheme: %w", err)
	}

	return client.New(config, client.Options{Scheme: scheme})
}

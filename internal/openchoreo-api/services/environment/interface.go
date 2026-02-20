// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// ObserverURLResult holds the observer URL response for an environment.
type ObserverURLResult struct {
	ObserverURL string
	Message     string
}

// RCAAgentURLResult holds the RCA agent URL response for an environment.
type RCAAgentURLResult struct {
	RCAAgentURL string
	Message     string
}

// Service defines the environment service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design.
type Service interface {
	ListEnvironments(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Environment], error)
	GetEnvironment(ctx context.Context, namespaceName, envName string) (*openchoreov1alpha1.Environment, error)
	CreateEnvironment(ctx context.Context, namespaceName string, env *openchoreov1alpha1.Environment) (*openchoreov1alpha1.Environment, error)
	GetObserverURL(ctx context.Context, namespaceName, envName string) (*ObserverURLResult, error)
	GetRCAAgentURL(ctx context.Context, namespaceName, envName string) (*RCAAgentURLResult, error)
}

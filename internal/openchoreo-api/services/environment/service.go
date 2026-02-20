// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"
	"fmt"
	"log/slog"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// environmentService handles environment-related business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type environmentService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*environmentService)(nil)

// NewService creates a new environment service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &environmentService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *environmentService) ListEnvironments(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Environment], error) {
	s.logger.Debug("Listing environments", "namespace", namespaceName, "limit", opts.Limit, "cursor", opts.Cursor)

	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var envList openchoreov1alpha1.EnvironmentList
	if err := s.k8sClient.List(ctx, &envList, listOpts...); err != nil {
		s.logger.Error("Failed to list environments", "error", err)
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	result := &services.ListResult[openchoreov1alpha1.Environment]{
		Items:      envList.Items,
		NextCursor: envList.Continue,
	}
	if envList.RemainingItemCount != nil {
		remaining := *envList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed environments", "namespace", namespaceName, "count", len(envList.Items))
	return result, nil
}

func (s *environmentService) GetEnvironment(ctx context.Context, namespaceName, envName string) (*openchoreov1alpha1.Environment, error) {
	s.logger.Debug("Getting environment", "namespace", namespaceName, "env", envName)

	env := &openchoreov1alpha1.Environment{}
	key := client.ObjectKey{
		Name:      envName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, env); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Environment not found", "namespace", namespaceName, "env", envName)
			return nil, ErrEnvironmentNotFound
		}
		s.logger.Error("Failed to get environment", "error", err)
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	return env, nil
}

func (s *environmentService) CreateEnvironment(ctx context.Context, namespaceName string, env *openchoreov1alpha1.Environment) (*openchoreov1alpha1.Environment, error) {
	s.logger.Debug("Creating environment", "namespace", namespaceName, "env", env.Name)

	// Check if environment already exists
	existing := &openchoreov1alpha1.Environment{}
	key := client.ObjectKey{
		Name:      env.Name,
		Namespace: namespaceName,
	}
	if err := s.k8sClient.Get(ctx, key, existing); err == nil {
		return nil, ErrEnvironmentAlreadyExists
	} else if client.IgnoreNotFound(err) != nil {
		return nil, fmt.Errorf("failed to check environment existence: %w", err)
	}

	// Resolve DataPlaneRef default if not provided
	if env.Spec.DataPlaneRef == nil || env.Spec.DataPlaneRef.Name == "" {
		defaultDP := &openchoreov1alpha1.DataPlane{}
		dpKey := client.ObjectKey{
			Name:      controller.DefaultPlaneName,
			Namespace: namespaceName,
		}
		if err := s.k8sClient.Get(ctx, dpKey, defaultDP); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return nil, ErrDataPlaneNotFound
			}
			return nil, fmt.Errorf("failed to get default dataplane: %w", err)
		}
		env.Spec.DataPlaneRef = &openchoreov1alpha1.DataPlaneRef{
			Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
			Name: controller.DefaultPlaneName,
		}
	} else if env.Spec.DataPlaneRef.Kind == "" {
		env.Spec.DataPlaneRef.Kind = openchoreov1alpha1.DataPlaneRefKindDataPlane
	}

	env.Namespace = namespaceName

	if err := s.k8sClient.Create(ctx, env); err != nil {
		s.logger.Error("Failed to create environment CR", "error", err)
		return nil, fmt.Errorf("failed to create environment: %w", err)
	}

	s.logger.Debug("Environment created successfully", "namespace", namespaceName, "env", env.Name)
	return env, nil
}

func (s *environmentService) GetObserverURL(ctx context.Context, namespaceName, envName string) (*ObserverURLResult, error) {
	s.logger.Debug("Getting environment observer URL", "namespace", namespaceName, "env", envName)

	env, err := s.GetEnvironment(ctx, namespaceName, envName)
	if err != nil {
		return nil, err
	}

	if env.Spec.DataPlaneRef == nil || env.Spec.DataPlaneRef.Name == "" {
		return nil, ErrDataPlaneNotFound
	}

	if env.Spec.DataPlaneRef.Kind == openchoreov1alpha1.DataPlaneRefKindClusterDataPlane {
		return &ObserverURLResult{
			Message: "observability-logs for ClusterDataPlane not yet supported",
		}, nil
	}

	dp := &openchoreov1alpha1.DataPlane{}
	dpKey := client.ObjectKey{
		Name:      env.Spec.DataPlaneRef.Name,
		Namespace: namespaceName,
	}
	if err := s.k8sClient.Get(ctx, dpKey, dp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrDataPlaneNotFound
		}
		return nil, fmt.Errorf("failed to get dataplane: %w", err)
	}

	observabilityResult, err := controller.GetObservabilityPlaneOrClusterObservabilityPlaneOfDataPlane(ctx, s.k8sClient, dp)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return &ObserverURLResult{
				Message: "observability-logs have not been configured",
			}, nil
		}
		return nil, fmt.Errorf("failed to get observability plane: %w", err)
	}

	observerURL := observabilityResult.GetObserverURL()
	if observerURL == "" {
		return &ObserverURLResult{
			Message: "observability-logs have not been configured",
		}, nil
	}

	return &ObserverURLResult{
		ObserverURL: observerURL,
	}, nil
}

func (s *environmentService) GetRCAAgentURL(ctx context.Context, namespaceName, envName string) (*RCAAgentURLResult, error) {
	s.logger.Debug("Getting RCA agent URL", "namespace", namespaceName, "env", envName)

	env, err := s.GetEnvironment(ctx, namespaceName, envName)
	if err != nil {
		return nil, err
	}

	if env.Spec.DataPlaneRef == nil || env.Spec.DataPlaneRef.Name == "" {
		return nil, ErrDataPlaneNotFound
	}

	if env.Spec.DataPlaneRef.Kind == openchoreov1alpha1.DataPlaneRefKindClusterDataPlane {
		return &RCAAgentURLResult{
			Message: "RCA agent for ClusterDataPlane not yet supported",
		}, nil
	}

	dp := &openchoreov1alpha1.DataPlane{}
	dpKey := client.ObjectKey{
		Name:      env.Spec.DataPlaneRef.Name,
		Namespace: namespaceName,
	}
	if err := s.k8sClient.Get(ctx, dpKey, dp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrDataPlaneNotFound
		}
		return nil, fmt.Errorf("failed to get dataplane: %w", err)
	}

	observabilityResult, err := controller.GetObservabilityPlaneOrClusterObservabilityPlaneOfDataPlane(ctx, s.k8sClient, dp)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return &RCAAgentURLResult{
				Message: "ObservabilityPlaneRef has not been configured",
			}, nil
		}
		return nil, fmt.Errorf("failed to get observability plane: %w", err)
	}

	var rcaAgentURL string
	if observabilityResult.ObservabilityPlane != nil {
		rcaAgentURL = observabilityResult.ObservabilityPlane.Spec.RCAAgentURL
	} else if observabilityResult.ClusterObservabilityPlane != nil {
		rcaAgentURL = observabilityResult.ClusterObservabilityPlane.Spec.RCAAgentURL
	}

	if rcaAgentURL == "" {
		return &RCAAgentURLResult{
			Message: "RCAAgentURL has not been configured",
		}, nil
	}

	return &RCAAgentURLResult{
		RCAAgentURL: rcaAgentURL,
	}, nil
}

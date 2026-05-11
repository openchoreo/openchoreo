// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

var environmentTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "Environment",
}

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

	commonOpts, err := services.BuildListOptions(opts)
	if err != nil {
		return nil, err
	}
	listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

	var envList openchoreov1alpha1.EnvironmentList
	if err := s.k8sClient.List(ctx, &envList, listOpts...); err != nil {
		s.logger.Error("Failed to list environments", "error", err)
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	for i := range envList.Items {
		envList.Items[i].TypeMeta = environmentTypeMeta
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

	env.TypeMeta = environmentTypeMeta
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

	env.Status = openchoreov1alpha1.EnvironmentStatus{}
	env.Namespace = namespaceName

	if err := s.k8sClient.Create(ctx, env); err != nil {
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		s.logger.Error("Failed to create environment CR", "error", err)
		return nil, fmt.Errorf("failed to create environment: %w", err)
	}

	s.logger.Debug("Environment created successfully", "namespace", namespaceName, "env", env.Name)
	env.TypeMeta = environmentTypeMeta
	return env, nil
}

// UpdateEnvironment replaces an existing environment with the provided state.
func (s *environmentService) UpdateEnvironment(ctx context.Context, namespaceName string, env *openchoreov1alpha1.Environment) (*openchoreov1alpha1.Environment, error) {
	if env == nil {
		return nil, ErrEnvironmentNil
	}

	s.logger.Debug("Updating environment", "namespace", namespaceName, "env", env.Name)

	existing := &openchoreov1alpha1.Environment{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: env.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrEnvironmentNotFound
		}
		s.logger.Error("Failed to get environment", "error", err)
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	// Clear status from user input — status is server-managed
	env.Status = openchoreov1alpha1.EnvironmentStatus{}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	existing.Spec = env.Spec
	existing.Labels = env.Labels
	existing.Annotations = env.Annotations

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if vErr := services.ExtractValidationError(err); vErr != nil {
			s.logger.Error("Environment update rejected by validation", "error", err)
			return nil, vErr
		}
		s.logger.Error("Failed to update environment CR", "error", err)
		return nil, fmt.Errorf("failed to update environment: %w", err)
	}

	s.logger.Debug("Environment updated successfully", "namespace", namespaceName, "env", env.Name)
	existing.TypeMeta = environmentTypeMeta
	return existing, nil
}

// DeleteEnvironment removes an environment by name.
func (s *environmentService) DeleteEnvironment(ctx context.Context, namespaceName, envName string) error {
	s.logger.Debug("Deleting environment", "namespace", namespaceName, "env", envName)

	env := &openchoreov1alpha1.Environment{}
	env.Name = envName
	env.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, env); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrEnvironmentNotFound
		}
		s.logger.Error("Failed to delete environment CR", "error", err)
		return fmt.Errorf("failed to delete environment: %w", err)
	}

	s.logger.Debug("Environment deleted successfully", "namespace", namespaceName, "env", envName)
	return nil
}

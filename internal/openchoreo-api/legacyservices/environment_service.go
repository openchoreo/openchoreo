// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// EnvironmentService handles environment-related business logic
type EnvironmentService struct {
	k8sClient client.Client
	logger    *slog.Logger
	authzPDP  authz.PDP
}

// NewEnvironmentService creates a new environment service
func NewEnvironmentService(k8sClient client.Client, logger *slog.Logger, authzPDP authz.PDP) *EnvironmentService {
	return &EnvironmentService{
		k8sClient: k8sClient,
		logger:    logger,
		authzPDP:  authzPDP,
	}
}

// ListEnvironments lists all environments in the specified namespace
func (s *EnvironmentService) ListEnvironments(ctx context.Context, namespaceName string) ([]*models.EnvironmentResponse, error) {
	s.logger.Debug("Listing environments", "namespace", namespaceName)

	var envList openchoreov1alpha1.EnvironmentList
	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}

	if err := s.k8sClient.List(ctx, &envList, listOpts...); err != nil {
		s.logger.Error("Failed to list environments", "error", err, "namespace", namespaceName)
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	// Check authorization for each environment
	environments := make([]*models.EnvironmentResponse, 0, len(envList.Items))
	for i := range envList.Items {
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewEnvironment, ResourceTypeEnvironment, envList.Items[i].Name,
			authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
			if errors.Is(err, ErrForbidden) {
				s.logger.Debug("Skipping unauthorized environment", "namespace", namespaceName, "environment", envList.Items[i].Name)
				continue
			}
			// Return other errors
			return nil, err
		}
		environments = append(environments, s.toEnvironmentResponse(&envList.Items[i]))
	}

	s.logger.Debug("Listed environments", "count", len(environments), "namespace", namespaceName)
	return environments, nil
}

// getEnvironment is the internal helper without authorization (INTERNAL USE ONLY)
func (s *EnvironmentService) getEnvironment(ctx context.Context, namespaceName, envName string) (*models.EnvironmentResponse, error) {
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
		s.logger.Error("Failed to get environment", "error", err, "namespace", namespaceName, "env", envName)
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	return s.toEnvironmentResponse(env), nil
}

// GetEnvironment retrieves a specific environment
func (s *EnvironmentService) GetEnvironment(ctx context.Context, namespaceName, envName string) (*models.EnvironmentResponse, error) {
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewEnvironment, ResourceTypeEnvironment, envName,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

	return s.getEnvironment(ctx, namespaceName, envName)
}

// CreateEnvironment creates a new environment
func (s *EnvironmentService) CreateEnvironment(ctx context.Context, namespaceName string, req *models.CreateEnvironmentRequest) (*models.EnvironmentResponse, error) {
	s.logger.Debug("Creating environment", "namespace", namespaceName, "env", req.Name)

	// Sanitize input
	req.Sanitize()

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionCreateEnvironment, ResourceTypeEnvironment, req.Name,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

	// Check if environment already exists
	exists, err := s.environmentExists(ctx, namespaceName, req.Name)
	if err != nil {
		s.logger.Error("Failed to check environment existence", "error", err)
		return nil, fmt.Errorf("failed to check environment existence: %w", err)
	}
	if exists {
		s.logger.Warn("Environment already exists", "namespace", namespaceName, "env", req.Name)
		return nil, ErrEnvironmentAlreadyExists
	}

	// Resolve DataPlaneRef (default to "default" if not provided and it exists)
	if req.DataPlaneRef == nil || req.DataPlaneRef.Name == "" {
		defaultDataPlane := &openchoreov1alpha1.DataPlane{}
		key := client.ObjectKey{
			Name:      controller.DefaultPlaneName,
			Namespace: namespaceName,
		}
		if err := s.k8sClient.Get(ctx, key, defaultDataPlane); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return nil, ErrDataPlaneNotFound
			}
			return nil, fmt.Errorf("failed to get default dataplane: %w", err)
		}

		req.DataPlaneRef = &models.DataPlaneRef{
			Kind: string(openchoreov1alpha1.DataPlaneRefKindDataPlane),
			Name: controller.DefaultPlaneName,
		}
	} else if req.DataPlaneRef.Kind == "" {
		// Default kind for backward compatibility if omitted
		req.DataPlaneRef.Kind = string(openchoreov1alpha1.DataPlaneRefKindDataPlane)
	}

	// Create the environment CR
	environmentCR := s.buildEnvironmentCR(namespaceName, req)
	if err := s.k8sClient.Create(ctx, environmentCR); err != nil {
		s.logger.Error("Failed to create environment CR", "error", err)
		return nil, fmt.Errorf("failed to create environment: %w", err)
	}

	s.logger.Debug("Environment created successfully", "namespace", namespaceName, "env", req.Name)
	return s.toEnvironmentResponse(environmentCR), nil
}

// environmentExists checks if an environment exists in the given namespace
func (s *EnvironmentService) environmentExists(ctx context.Context, namespaceName, envName string) (bool, error) {
	env := &openchoreov1alpha1.Environment{}
	key := client.ObjectKey{
		Name:      envName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, env); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// buildEnvironmentCR builds an Environment CR from the request
func (s *EnvironmentService) buildEnvironmentCR(namespaceName string, req *models.CreateEnvironmentRequest) *openchoreov1alpha1.Environment {
	// Convert DataPlaneRef from request to CRD type
	var dataPlaneRef *openchoreov1alpha1.DataPlaneRef
	if req.DataPlaneRef != nil && req.DataPlaneRef.Name != "" {
		dataPlaneRef = &openchoreov1alpha1.DataPlaneRef{
			Kind: openchoreov1alpha1.DataPlaneRefKind(req.DataPlaneRef.Kind),
			Name: req.DataPlaneRef.Name,
		}
	}
	// If not provided, leave nil to use default resolution

	// Set default display name if not provided
	displayName := req.DisplayName
	if displayName == "" {
		displayName = req.Name
	}

	// Set default description if not provided
	description := req.Description
	if description == "" {
		description = fmt.Sprintf("Environment for %s", req.Name)
	}

	return &openchoreov1alpha1.Environment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Environment",
			APIVersion: "openchoreo.dev/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: namespaceName,
			Annotations: map[string]string{
				controller.AnnotationKeyDisplayName: displayName,
				controller.AnnotationKeyDescription: description,
			},
			Labels: map[string]string{
				labels.LabelKeyNamespaceName: namespaceName,
				labels.LabelKeyName:          req.Name,
			},
		},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: dataPlaneRef,
			IsProduction: req.IsProduction,
		},
	}
}

// toEnvironmentResponse converts an Environment CR to an EnvironmentResponse
func (s *EnvironmentService) toEnvironmentResponse(env *openchoreov1alpha1.Environment) *models.EnvironmentResponse {
	// Extract display name and description from annotations
	displayName := env.Annotations[controller.AnnotationKeyDisplayName]
	description := env.Annotations[controller.AnnotationKeyDescription]

	// Get status from conditions
	status := statusUnknown
	if len(env.Status.Conditions) > 0 {
		// Get the latest condition
		latestCondition := env.Status.Conditions[len(env.Status.Conditions)-1]
		if latestCondition.Status == metav1.ConditionTrue {
			status = statusReady
		} else {
			status = statusNotReady
		}
	}

	// Convert DataPlaneRef from CRD to response type
	var dataPlaneRef *models.DataPlaneRef
	if env.Spec.DataPlaneRef != nil {
		dataPlaneRef = &models.DataPlaneRef{
			Kind: string(env.Spec.DataPlaneRef.Kind),
			Name: env.Spec.DataPlaneRef.Name,
		}
	}

	return &models.EnvironmentResponse{
		UID:          string(env.UID),
		Name:         env.Name,
		Namespace:    env.Namespace,
		DisplayName:  displayName,
		Description:  description,
		DataPlaneRef: dataPlaneRef,
		IsProduction: env.Spec.IsProduction,
		CreatedAt:    env.CreationTimestamp.Time,
		Status:       status,
	}
}

// EnvironmentObserverResponse represents the response for observer URL requests
type EnvironmentObserverResponse struct {
	ObserverURL string `json:"observerUrl,omitempty"`
	Message     string `json:"message,omitempty"`
}

// GetEnvironmentObserverURL retrieves the observer URL for the environment
func (s *EnvironmentService) GetEnvironmentObserverURL(ctx context.Context, namespaceName, envName string) (*EnvironmentObserverResponse, error) {
	s.logger.Debug("Getting environment observer URL", "namespace", namespaceName, "env", envName)

	env, err := s.getEnvironment(ctx, namespaceName, envName)
	if err != nil {
		s.logger.Error("Failed to get environment", "error", err, "namespace", namespaceName, "env", envName)
		return nil, err
	}

	// Check if environment has a dataplane reference
	if env.DataPlaneRef == nil || env.DataPlaneRef.Name == "" {
		s.logger.Error("Environment has no dataplane reference", "environment", envName)
		return nil, ErrDataPlaneNotFound
	}

	var observabilityResult *controller.ObservabilityPlaneResult

	switch env.DataPlaneRef.Kind {
	case string(openchoreov1alpha1.DataPlaneRefKindClusterDataPlane):
		// Get the ClusterDataPlane configuration (cluster-scoped)
		cdp := &openchoreov1alpha1.ClusterDataPlane{}
		cdpKey := client.ObjectKey{Name: env.DataPlaneRef.Name}

		if err := s.k8sClient.Get(ctx, cdpKey, cdp); err != nil {
			if client.IgnoreNotFound(err) == nil {
				s.logger.Error("ClusterDataPlane not found", "clusterDataPlane", env.DataPlaneRef.Name)
				return nil, ErrDataPlaneNotFound
			}
			s.logger.Error("Failed to get ClusterDataPlane", "error", err, "clusterDataPlane", env.DataPlaneRef.Name)
			return nil, fmt.Errorf("failed to get ClusterDataPlane: %w", err)
		}

		cop, err := controller.GetClusterObservabilityPlaneOfClusterDataPlane(ctx, s.k8sClient, cdp)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				s.logger.Debug("ClusterObservabilityPlane not found", "error", err, "clusterDataPlane", cdp.Name)
				return &EnvironmentObserverResponse{
					Message: "observability-logs have not been configured",
				}, nil
			}
			s.logger.Error("Failed to get ClusterObservabilityPlane", "error", err, "clusterDataPlane", cdp.Name)
			return nil, fmt.Errorf("failed to get ClusterObservabilityPlane: %w", err)
		}
		observabilityResult = &controller.ObservabilityPlaneResult{ClusterObservabilityPlane: cop}

	case "", string(openchoreov1alpha1.DataPlaneRefKindDataPlane):
		// Get the DataPlane configuration for the environment
		dp := &openchoreov1alpha1.DataPlane{}
		dpKey := client.ObjectKey{
			Name:      env.DataPlaneRef.Name,
			Namespace: namespaceName,
		}

		if err := s.k8sClient.Get(ctx, dpKey, dp); err != nil {
			if client.IgnoreNotFound(err) == nil {
				s.logger.Error("DataPlane not found", "namespace", namespaceName, "dataplane", env.DataPlaneRef.Name)
				return nil, ErrDataPlaneNotFound
			}
			s.logger.Error("Failed to get dataplane", "error", err, "namespace", namespaceName, "dataplane", env.DataPlaneRef.Name)
			return nil, fmt.Errorf("failed to get dataplane: %w", err)
		}

		var resolveErr error
		observabilityResult, resolveErr = controller.GetObservabilityPlaneOrClusterObservabilityPlaneOfDataPlane(ctx, s.k8sClient, dp)
		if resolveErr != nil {
			if k8serrors.IsNotFound(resolveErr) {
				s.logger.Debug("Observability plane not found", "error", resolveErr, "dataplane", dp.Name)
				return &EnvironmentObserverResponse{
					Message: "observability-logs have not been configured",
				}, nil
			}
			s.logger.Error("Failed to get observability plane", "error", resolveErr, "dataplane", dp.Name)
			return nil, fmt.Errorf("failed to get observability plane: %w", resolveErr)
		}

	default:
		s.logger.Error("Unsupported DataPlaneRef kind", "kind", env.DataPlaneRef.Kind, "environment", envName)
		return nil, fmt.Errorf("unsupported DataPlaneRef.Kind: %q", env.DataPlaneRef.Kind)
	}

	observerURL := observabilityResult.GetObserverURL()
	if observerURL == "" {
		s.logger.Debug("ObserverURL not configured in observability plane", "observabilityPlane", observabilityResult.GetName())
		return &EnvironmentObserverResponse{
			Message: "observability-logs have not been configured",
		}, nil
	}

	return &EnvironmentObserverResponse{
		ObserverURL: observerURL,
	}, nil
}

// RCAAgentURLResponse represents the response for RCA agent URL requests
type RCAAgentURLResponse struct {
	RCAAgentURL string `json:"rcaAgentUrl,omitempty"`
	Message     string `json:"message,omitempty"`
}

// GetRCAAgentURL retrieves the RCA agent URL for the environment
func (s *EnvironmentService) GetRCAAgentURL(ctx context.Context, namespaceName, envName string) (*RCAAgentURLResponse, error) {
	s.logger.Debug("Getting RCA agent URL", "namespace", namespaceName, "env", envName)

	env, err := s.getEnvironment(ctx, namespaceName, envName)
	if err != nil {
		s.logger.Error("Failed to get environment", "error", err, "namespace", namespaceName, "env", envName)
		return nil, err
	}

	// Check if environment has a dataplane reference
	if env.DataPlaneRef == nil || env.DataPlaneRef.Name == "" {
		s.logger.Error("Environment has no dataplane reference", "environment", envName)
		return nil, ErrDataPlaneNotFound
	}

	var observabilityResult *controller.ObservabilityPlaneResult

	switch env.DataPlaneRef.Kind {
	case string(openchoreov1alpha1.DataPlaneRefKindClusterDataPlane):
		// Get the ClusterDataPlane configuration (cluster-scoped)
		cdp := &openchoreov1alpha1.ClusterDataPlane{}
		cdpKey := client.ObjectKey{Name: env.DataPlaneRef.Name}

		if err := s.k8sClient.Get(ctx, cdpKey, cdp); err != nil {
			if client.IgnoreNotFound(err) == nil {
				s.logger.Error("ClusterDataPlane not found", "clusterDataPlane", env.DataPlaneRef.Name)
				return nil, ErrDataPlaneNotFound
			}
			s.logger.Error("Failed to get ClusterDataPlane", "error", err, "clusterDataPlane", env.DataPlaneRef.Name)
			return nil, fmt.Errorf("failed to get ClusterDataPlane: %w", err)
		}

		cop, err := controller.GetClusterObservabilityPlaneOfClusterDataPlane(ctx, s.k8sClient, cdp)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				s.logger.Debug("ClusterObservabilityPlane not found", "error", err, "clusterDataPlane", cdp.Name)
				return &RCAAgentURLResponse{
					Message: "ObservabilityPlaneRef has not been configured",
				}, nil
			}
			s.logger.Error("Failed to get ClusterObservabilityPlane", "error", err, "clusterDataPlane", cdp.Name)
			return nil, fmt.Errorf("failed to get ClusterObservabilityPlane: %w", err)
		}
		observabilityResult = &controller.ObservabilityPlaneResult{ClusterObservabilityPlane: cop}

	case "", string(openchoreov1alpha1.DataPlaneRefKindDataPlane):
		// Get the DataPlane configuration for the environment
		dp := &openchoreov1alpha1.DataPlane{}
		dpKey := client.ObjectKey{
			Name:      env.DataPlaneRef.Name,
			Namespace: namespaceName,
		}

		if err := s.k8sClient.Get(ctx, dpKey, dp); err != nil {
			if client.IgnoreNotFound(err) == nil {
				s.logger.Error("DataPlane not found", "namespace", namespaceName, "dataplane", env.DataPlaneRef.Name)
				return nil, ErrDataPlaneNotFound
			}
			s.logger.Error("Failed to get dataplane", "error", err, "namespace", namespaceName, "dataplane", env.DataPlaneRef.Name)
			return nil, fmt.Errorf("failed to get dataplane: %w", err)
		}

		var resolveErr error
		observabilityResult, resolveErr = controller.GetObservabilityPlaneOrClusterObservabilityPlaneOfDataPlane(ctx, s.k8sClient, dp)
		if resolveErr != nil {
			if k8serrors.IsNotFound(resolveErr) {
				s.logger.Debug("Observability plane not found", "error", resolveErr, "dataplane", dp.Name)
				return &RCAAgentURLResponse{
					Message: "ObservabilityPlaneRef has not been configured",
				}, nil
			}
			s.logger.Error("Failed to get observability plane", "error", resolveErr, "dataplane", dp.Name)
			return nil, fmt.Errorf("failed to get observability plane: %w", resolveErr)
		}

	default:
		s.logger.Error("Unsupported DataPlaneRef kind", "kind", env.DataPlaneRef.Kind, "environment", envName)
		return nil, fmt.Errorf("unsupported DataPlaneRef.Kind: %q", env.DataPlaneRef.Kind)
	}

	// Get RCAAgentURL from ObservabilityPlane or ClusterObservabilityPlane
	var rcaAgentURL string
	if observabilityResult.ObservabilityPlane != nil {
		rcaAgentURL = observabilityResult.ObservabilityPlane.Spec.RCAAgentURL
	} else if observabilityResult.ClusterObservabilityPlane != nil {
		rcaAgentURL = observabilityResult.ClusterObservabilityPlane.Spec.RCAAgentURL
	}

	if rcaAgentURL == "" {
		s.logger.Debug("RCAAgentURL not configured in observability plane", "observabilityPlane", observabilityResult.GetName())
		return &RCAAgentURLResponse{
			Message: "RCAAgentURL has not been configured",
		}, nil
	}

	return &RCAAgentURLResponse{
		RCAAgentURL: rcaAgentURL,
	}, nil
}

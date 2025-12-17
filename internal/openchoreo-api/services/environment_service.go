// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

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

// ListEnvironments lists all environments in the specified organization
func (s *EnvironmentService) ListEnvironments(ctx context.Context, orgName string) ([]*models.EnvironmentResponse, error) {
	s.logger.Debug("Listing environments", "org", orgName)

	var envList openchoreov1alpha1.EnvironmentList
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
	}

	if err := s.k8sClient.List(ctx, &envList, listOpts...); err != nil {
		s.logger.Error("Failed to list environments", "error", err, "org", orgName)
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	// Check authorization for each environment
	environments := make([]*models.EnvironmentResponse, 0, len(envList.Items))
	for i := range envList.Items {
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewEnvironment, ResourceTypeEnvironment, envList.Items[i].Name,
			authz.ResourceHierarchy{Organization: orgName}); err != nil {
			if errors.Is(err, ErrForbidden) {
				s.logger.Debug("Skipping unauthorized environment", "org", orgName, "environment", envList.Items[i].Name)
				continue
			}
			// Return other errors
			return nil, err
		}
		environments = append(environments, s.toEnvironmentResponse(&envList.Items[i]))
	}

	s.logger.Debug("Listed environments", "count", len(environments), "org", orgName)
	return environments, nil
}

// getEnvironment is the internal helper without authorization (INTERNAL USE ONLY)
func (s *EnvironmentService) getEnvironment(ctx context.Context, orgName, envName string) (*models.EnvironmentResponse, error) {
	s.logger.Debug("Getting environment", "org", orgName, "env", envName)

	env := &openchoreov1alpha1.Environment{}
	key := client.ObjectKey{
		Name:      envName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, env); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Environment not found", "org", orgName, "env", envName)
			return nil, ErrEnvironmentNotFound
		}
		s.logger.Error("Failed to get environment", "error", err, "org", orgName, "env", envName)
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	return s.toEnvironmentResponse(env), nil
}

// GetEnvironment retrieves a specific environment
func (s *EnvironmentService) GetEnvironment(ctx context.Context, orgName, envName string) (*models.EnvironmentResponse, error) {
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewEnvironment, ResourceTypeEnvironment, envName,
		authz.ResourceHierarchy{Organization: orgName}); err != nil {
		return nil, err
	}

	return s.getEnvironment(ctx, orgName, envName)
}

// CreateEnvironment creates a new environment
func (s *EnvironmentService) CreateEnvironment(ctx context.Context, orgName string, req *models.CreateEnvironmentRequest) (*models.EnvironmentResponse, error) {
	s.logger.Debug("Creating environment", "org", orgName, "env", req.Name)

	// Sanitize input
	req.Sanitize()

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionCreateEnvironment, ResourceTypeEnvironment, req.Name,
		authz.ResourceHierarchy{Organization: orgName}); err != nil {
		return nil, err
	}

	// Check if environment already exists
	exists, err := s.environmentExists(ctx, orgName, req.Name)
	if err != nil {
		s.logger.Error("Failed to check environment existence", "error", err)
		return nil, fmt.Errorf("failed to check environment existence: %w", err)
	}
	if exists {
		s.logger.Warn("Environment already exists", "org", orgName, "env", req.Name)
		return nil, ErrEnvironmentAlreadyExists
	}

	// Create the environment CR
	environmentCR := s.buildEnvironmentCR(orgName, req)
	if err := s.k8sClient.Create(ctx, environmentCR); err != nil {
		s.logger.Error("Failed to create environment CR", "error", err)
		return nil, fmt.Errorf("failed to create environment: %w", err)
	}

	s.logger.Debug("Environment created successfully", "org", orgName, "env", req.Name)
	return s.toEnvironmentResponse(environmentCR), nil
}

// environmentExists checks if an environment exists in the given organization
func (s *EnvironmentService) environmentExists(ctx context.Context, orgName, envName string) (bool, error) {
	env := &openchoreov1alpha1.Environment{}
	key := client.ObjectKey{
		Name:      envName,
		Namespace: orgName,
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
func (s *EnvironmentService) buildEnvironmentCR(orgName string, req *models.CreateEnvironmentRequest) *openchoreov1alpha1.Environment {
	// Set default data plane if not provided
	dataPlaneRef := req.DataPlaneRef
	if dataPlaneRef == "" {
		dataPlaneRef = defaultPipeline
	}

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
			Namespace: orgName,
			Annotations: map[string]string{
				controller.AnnotationKeyDisplayName: displayName,
				controller.AnnotationKeyDescription: description,
			},
			Labels: map[string]string{
				labels.LabelKeyOrganizationName: orgName,
				labels.LabelKeyName:             req.Name,
			},
		},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: dataPlaneRef,
			IsProduction: req.IsProduction,
			Gateway: openchoreov1alpha1.GatewayConfig{
				DNSPrefix: req.DNSPrefix,
			},
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

	return &models.EnvironmentResponse{
		UID:          string(env.UID),
		Name:         env.Name,
		Namespace:    env.Namespace,
		DisplayName:  displayName,
		Description:  description,
		DataPlaneRef: env.Spec.DataPlaneRef,
		IsProduction: env.Spec.IsProduction,
		DNSPrefix:    env.Spec.Gateway.DNSPrefix,
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
func (s *EnvironmentService) GetEnvironmentObserverURL(ctx context.Context, orgName, envName string) (*EnvironmentObserverResponse, error) {
	s.logger.Debug("Getting environment observer URL", "org", orgName, "env", envName)

	env, err := s.getEnvironment(ctx, orgName, envName)
	if err != nil {
		s.logger.Error("Failed to get environment", "error", err, "org", orgName, "env", envName)
		return nil, err
	}

	// Check if environment has a dataplane reference
	if env.DataPlaneRef == "" {
		s.logger.Error("Environment has no dataplane reference", "environment", envName)
		return nil, ErrDataPlaneNotFound
	}

	// Get the DataPlane configuration for the environment
	dp := &openchoreov1alpha1.DataPlane{}
	dpKey := client.ObjectKey{
		Name:      env.DataPlaneRef,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, dpKey, dp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Error("DataPlane not found", "org", orgName, "dataplane", env.DataPlaneRef)
			return nil, ErrDataPlaneNotFound
		}
		s.logger.Error("Failed to get dataplane", "error", err, "org", orgName, "dataplane", env.DataPlaneRef)
		return nil, fmt.Errorf("failed to get dataplane: %w", err)
	}

	// Check if observer is configured via ObservabilityPlaneRef
	if dp.Spec.ObservabilityPlaneRef == "" {
		s.logger.Debug("ObservabilityPlaneRef not configured in dataplane", "dataplane", dp.Name)
		return &EnvironmentObserverResponse{
			Message: "observability-logs have not been configured",
		}, nil
	}

	// Fetch the ObservabilityPlane to get the ObserverURL
	observabilityPlane := &openchoreov1alpha1.ObservabilityPlane{}
	opKey := client.ObjectKey{
		Name:      dp.Spec.ObservabilityPlaneRef,
		Namespace: dp.Namespace,
	}
	if err := s.k8sClient.Get(ctx, opKey, observabilityPlane); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Debug("ObservabilityPlane not found", "observabilityPlane", dp.Spec.ObservabilityPlaneRef)
			return &EnvironmentObserverResponse{
				Message: "observability-logs have not been configured",
			}, nil
		}
		s.logger.Error("Failed to get observability plane", "error", err, "observabilityPlane", dp.Spec.ObservabilityPlaneRef)
		return nil, fmt.Errorf("failed to get observability plane: %w", err)
	}

	if observabilityPlane.Spec.ObserverURL == "" {
		s.logger.Debug("ObserverURL not configured in observability plane", "observabilityPlane", observabilityPlane.Name)
		return &EnvironmentObserverResponse{
			Message: "observability-logs have not been configured",
		}, nil
	}

	return &EnvironmentObserverResponse{
		ObserverURL: observabilityPlane.Spec.ObserverURL,
	}, nil
}

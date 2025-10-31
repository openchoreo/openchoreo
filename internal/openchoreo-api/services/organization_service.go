// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"fmt"

	"golang.org/x/exp/slog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// OrganizationService handles organization-related business logic
type OrganizationService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

// NewOrganizationService creates a new organization service
func NewOrganizationService(k8sClient client.Client, logger *slog.Logger) *OrganizationService {
	return &OrganizationService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

// ListOrganizations lists all organizations
func (s *OrganizationService) ListOrganizations(ctx context.Context) ([]*models.OrganizationResponse, error) {
	s.logger.Debug("Listing organizations")

	var orgList openchoreov1alpha1.OrganizationList
	if err := s.k8sClient.List(ctx, &orgList); err != nil {
		s.logger.Error("Failed to list organizations", "error", err)
		return nil, fmt.Errorf("failed to list organizations: %w", err)
	}

	// Use index to get stable pointer to slice element (not loop variable)
	// This prevents pointer aliasing issues where range copies the value
	organizations := make([]*models.OrganizationResponse, 0, len(orgList.Items))
	for i := range orgList.Items {
		organizations = append(organizations, s.toOrganizationResponse(&orgList.Items[i]))
	}

	s.logger.Debug("Listed organizations", "count", len(organizations))
	return organizations, nil
}

// ListOrganizationsWithCursor lists organizations with cursor-based pagination
// Note: continueToken is validated at the handler layer before reaching this service.
// The Kubernetes API server performs additional validation and will return appropriate
// errors if the token is malformed or expired, which are handled below.
func (s *OrganizationService) ListOrganizationsWithCursor(
	ctx context.Context,
	continueToken string,
	limit int64,
) ([]*models.OrganizationResponse, string, error) {
	s.logger.Debug("Listing organizations with cursor",
		"continue", continueToken,
		"limit", limit)

	var orgList openchoreov1alpha1.OrganizationList

	// Set up list options with K8s pagination
	listOpts := []client.ListOption{
		client.Limit(limit),
	}

	// Add continue token if provided
	if continueToken != "" {
		listOpts = append(listOpts, client.Continue(continueToken))
	}

	if err := s.k8sClient.List(ctx, &orgList, listOpts...); err != nil {
		// Check if continue token expired
		if isExpiredTokenError(err) {
			return nil, "", ErrContinueTokenExpired
		}
		if isInvalidCursorError(err) {
			return nil, "", ErrInvalidCursorFormat
		}
		if isServiceUnavailableError(err) {
			return nil, "", fmt.Errorf("service unavailable: %w", err)
		}
		return nil, "", fmt.Errorf("failed to list organizations: %w", err)
	}

	// Convert to response models
	organizations := make([]*models.OrganizationResponse, 0, len(orgList.Items))
	for i := range orgList.Items {
		organizations = append(organizations, s.toOrganizationResponse(&orgList.Items[i]))
	}

	// Get the next continue token from K8s response
	nextContinue := orgList.Continue

	s.logger.Debug("Listed organizations",
		"count", len(organizations),
		"nextContinue", nextContinue)

	return organizations, nextContinue, nil
}

// GetOrganization retrieves a specific organization
func (s *OrganizationService) GetOrganization(ctx context.Context, orgName string) (*models.OrganizationResponse, error) {
	s.logger.Debug("Getting organization", "org", orgName)

	org := &openchoreov1alpha1.Organization{}
	key := client.ObjectKey{
		Name: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, org); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Organization not found", "org", orgName)
			return nil, ErrOrganizationNotFound
		}
		s.logger.Error("Failed to get organization", "error", err)
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	return s.toOrganizationResponse(org), nil
}

// toOrganizationResponse converts an Organization CR to an OrganizationResponse
func (s *OrganizationService) toOrganizationResponse(org *openchoreov1alpha1.Organization) *models.OrganizationResponse {
	// Extract display name and description from annotations
	displayName := org.Annotations[controller.AnnotationKeyDisplayName]
	description := org.Annotations[controller.AnnotationKeyDescription]

	// handle empty conditions slice
	status := statusUnknown
	if len(org.Status.Conditions) > 0 {
		// Get the latest condition safely
		latestCondition := org.Status.Conditions[len(org.Status.Conditions)-1]
		if latestCondition.Status == metav1.ConditionTrue {
			status = statusReady
		} else {
			status = statusNotReady
		}
	} else {
		s.logger.Debug("Organization has no status conditions", "org", org.Name)
	}

	return &models.OrganizationResponse{
		Name:        org.Name,
		DisplayName: displayName,
		Description: description,
		Namespace:   org.Status.Namespace,
		CreatedAt:   org.CreationTimestamp.Time,
		Status:      status,
	}
}

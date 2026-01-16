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
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// OrganizationService handles organization-related business logic
type OrganizationService struct {
	k8sClient client.Client
	logger    *slog.Logger
	authzPDP  authz.PDP
}

// NewOrganizationService creates a new organization service
func NewOrganizationService(k8sClient client.Client, logger *slog.Logger, authzPDP authz.PDP) *OrganizationService {
	return &OrganizationService{
		k8sClient: k8sClient,
		logger:    logger,
		authzPDP:  authzPDP,
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

	organizations := make([]*models.OrganizationResponse, 0, len(orgList.Items))
	for _, item := range orgList.Items {
		// Authorization check for each organization
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewOrganization, ResourceTypeOrganization, item.Name,
			authz.ResourceHierarchy{Namespace: item.Name}); err != nil {
			if errors.Is(err, ErrForbidden) {
				// Skip unauthorized organizations
				s.logger.Debug("Skipping unauthorized organization", "org", item.Name)
				continue
			}
			// system failures, return the error
			return nil, err
		}
		organizations = append(organizations, s.toOrganizationResponse(&item))
	}

	s.logger.Debug("Listed organizations", "count", len(organizations))
	return organizations, nil
}

// GetOrganization retrieves a specific organization
func (s *OrganizationService) GetOrganization(ctx context.Context, orgName string) (*models.OrganizationResponse, error) {
	s.logger.Debug("Getting organization", "org", orgName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewOrganization, ResourceTypeOrganization, orgName,
		authz.ResourceHierarchy{Namespace: orgName}); err != nil {
		return nil, err
	}

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

	// Get status from conditions
	status := statusUnknown
	if len(org.Status.Conditions) > 0 {
		// Get the latest condition
		latestCondition := org.Status.Conditions[len(org.Status.Conditions)-1]
		if latestCondition.Status == metav1.ConditionTrue {
			status = statusReady
		} else {
			status = statusNotReady
		}
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

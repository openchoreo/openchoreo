// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacy_services

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

// DataPlaneService handles dataplane-related business logic
type DataPlaneService struct {
	k8sClient client.Client
	logger    *slog.Logger
	authzPDP  authz.PDP
}

// NewDataPlaneService creates a new dataplane service
func NewDataPlaneService(k8sClient client.Client, logger *slog.Logger, authzPDP authz.PDP) *DataPlaneService {
	return &DataPlaneService{
		k8sClient: k8sClient,
		logger:    logger,
		authzPDP:  authzPDP,
	}
}

// ListDataPlanes lists all dataplanes in the specified namespace
func (s *DataPlaneService) ListDataPlanes(ctx context.Context, namespaceName string) ([]*models.DataPlaneResponse, error) {
	s.logger.Debug("Listing dataplanes", "namespace", namespaceName)

	var dpList openchoreov1alpha1.DataPlaneList
	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}

	if err := s.k8sClient.List(ctx, &dpList, listOpts...); err != nil {
		s.logger.Error("Failed to list dataplanes", "error", err, "namespace", namespaceName)
		return nil, fmt.Errorf("failed to list dataplanes: %w", err)
	}

	dataplanes := make([]*models.DataPlaneResponse, 0, len(dpList.Items))
	for i := range dpList.Items {
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewDataPlane, ResourceTypeDataPlane, dpList.Items[i].Name,
			authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
			if errors.Is(err, ErrForbidden) {
				s.logger.Debug("Skipping unauthorized dataplane", "namespace", namespaceName, "dataplane", dpList.Items[i].Name)
				continue
			}
			return nil, err
		}
		dataplanes = append(dataplanes, s.toDataPlaneResponse(&dpList.Items[i]))
	}

	s.logger.Debug("Listed dataplanes", "count", len(dataplanes), "namespace", namespaceName)
	return dataplanes, nil
}

// GetDataPlane retrieves a specific dataplane
func (s *DataPlaneService) GetDataPlane(ctx context.Context, namespaceName, dpName string) (*models.DataPlaneResponse, error) {
	s.logger.Debug("Getting dataplane", "namespace", namespaceName, "dataplane", dpName)

	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewDataPlane, ResourceTypeDataPlane, dpName,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

	dp := &openchoreov1alpha1.DataPlane{}
	key := client.ObjectKey{
		Name:      dpName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, dp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("DataPlane not found", "namespace", namespaceName, "dataplane", dpName)
			return nil, ErrDataPlaneNotFound
		}
		s.logger.Error("Failed to get dataplane", "error", err, "namespace", namespaceName, "dataplane", dpName)
		return nil, fmt.Errorf("failed to get dataplane: %w", err)
	}

	return s.toDataPlaneResponse(dp), nil
}

// CreateDataPlane creates a new dataplane
func (s *DataPlaneService) CreateDataPlane(ctx context.Context, namespaceName string, req *models.CreateDataPlaneRequest) (*models.DataPlaneResponse, error) {
	s.logger.Debug("Creating dataplane", "namespace", namespaceName, "dataplane", req.Name)

	// Sanitize input
	req.Sanitize()

	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionCreateDataPlane, ResourceTypeDataPlane, req.Name,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

	// Check if dataplane already exists
	exists, err := s.dataPlaneExists(ctx, namespaceName, req.Name)
	if err != nil {
		s.logger.Error("Failed to check dataplane existence", "error", err)
		return nil, fmt.Errorf("failed to check dataplane existence: %w", err)
	}
	if exists {
		s.logger.Warn("DataPlane already exists", "namespace", namespaceName, "dataplane", req.Name)
		return nil, ErrDataPlaneAlreadyExists
	}

	// Create the dataplane CR
	dataplaneCR := s.buildDataPlaneCR(namespaceName, req)
	if err := s.k8sClient.Create(ctx, dataplaneCR); err != nil {
		s.logger.Error("Failed to create dataplane CR", "error", err)
		return nil, fmt.Errorf("failed to create dataplane: %w", err)
	}

	s.logger.Debug("DataPlane created successfully", "namespace", namespaceName, "dataplane", req.Name)
	return s.toDataPlaneResponse(dataplaneCR), nil
}

// dataPlaneExists checks if a dataplane exists in the given namespace
func (s *DataPlaneService) dataPlaneExists(ctx context.Context, namespaceName, dpName string) (bool, error) {
	dp := &openchoreov1alpha1.DataPlane{}
	key := client.ObjectKey{
		Name:      dpName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, dp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// buildDataPlaneCR builds a DataPlane CR from the request
func (s *DataPlaneService) buildDataPlaneCR(namespaceName string, req *models.CreateDataPlaneRequest) *openchoreov1alpha1.DataPlane {
	// Set default display name if not provided
	displayName := req.DisplayName
	if displayName == "" {
		displayName = req.Name
	}

	// Set default description if not provided
	description := req.Description
	if description == "" {
		description = fmt.Sprintf("DataPlane for %s", req.Name)
	}

	gatewaySpec := openchoreov1alpha1.GatewaySpec{
		PublicVirtualHost:       req.PublicVirtualHost,
		OrganizationVirtualHost: req.OrganizationVirtualHost,
	}

	// Set port values if provided, otherwise CRD defaults will be used
	if req.PublicHTTPPort != nil {
		gatewaySpec.PublicHTTPPort = *req.PublicHTTPPort
	}
	if req.PublicHTTPSPort != nil {
		gatewaySpec.PublicHTTPSPort = *req.PublicHTTPSPort
	}
	if req.OrganizationHTTPPort != nil {
		gatewaySpec.OrganizationHTTPPort = *req.OrganizationHTTPPort
	}
	if req.OrganizationHTTPSPort != nil {
		gatewaySpec.OrganizationHTTPSPort = *req.OrganizationHTTPSPort
	}

	spec := openchoreov1alpha1.DataPlaneSpec{
		ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
			ClientCA: openchoreov1alpha1.ValueFrom{
				Value: req.ClusterAgentClientCA,
			},
		},
		Gateway: gatewaySpec,
	}

	// Set observability plane reference if provided
	if req.ObservabilityPlaneRef != nil && req.ObservabilityPlaneRef.Name != "" {
		spec.ObservabilityPlaneRef = &openchoreov1alpha1.ObservabilityPlaneRef{
			Kind: openchoreov1alpha1.ObservabilityPlaneRefKind(req.ObservabilityPlaneRef.Kind),
			Name: req.ObservabilityPlaneRef.Name,
		}
	}

	return &openchoreov1alpha1.DataPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DataPlane",
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
		Spec: spec,
	}
}

// toDataPlaneResponse converts a DataPlane CR to a DataPlaneResponse
func (s *DataPlaneService) toDataPlaneResponse(dp *openchoreov1alpha1.DataPlane) *models.DataPlaneResponse {
	// Extract display name and description from annotations
	displayName := dp.Annotations[controller.AnnotationKeyDisplayName]
	description := dp.Annotations[controller.AnnotationKeyDescription]

	// Get status from conditions
	status := statusUnknown
	if len(dp.Status.Conditions) > 0 {
		// Get the latest condition
		latestCondition := dp.Status.Conditions[len(dp.Status.Conditions)-1]
		if latestCondition.Status == metav1.ConditionTrue {
			status = statusReady
		} else {
			status = statusNotReady
		}
	}

	// Extract secretStoreRef name if present
	var secretStoreRef string
	if dp.Spec.SecretStoreRef != nil {
		secretStoreRef = dp.Spec.SecretStoreRef.Name
	}

	response := &models.DataPlaneResponse{
		Name:                    dp.Name,
		Namespace:               dp.Namespace,
		DisplayName:             displayName,
		Description:             description,
		ImagePullSecretRefs:     dp.Spec.ImagePullSecretRefs,
		SecretStoreRef:          secretStoreRef,
		PublicVirtualHost:       dp.Spec.Gateway.PublicVirtualHost,
		OrganizationVirtualHost: dp.Spec.Gateway.OrganizationVirtualHost,
		PublicHTTPPort:          dp.Spec.Gateway.PublicHTTPPort,
		PublicHTTPSPort:         dp.Spec.Gateway.PublicHTTPSPort,
		OrganizationHTTPPort:    dp.Spec.Gateway.OrganizationHTTPPort,
		OrganizationHTTPSPort:   dp.Spec.Gateway.OrganizationHTTPSPort,
		CreatedAt:               dp.CreationTimestamp.Time,
		Status:                  status,
	}

	// Add observability plane reference if present
	if dp.Spec.ObservabilityPlaneRef != nil {
		response.ObservabilityPlaneRef = &models.ObservabilityPlaneRef{
			Kind: string(dp.Spec.ObservabilityPlaneRef.Kind),
			Name: dp.Spec.ObservabilityPlaneRef.Name,
		}
	}

	// Map agent connection status
	if dp.Status.AgentConnection != nil {
		response.AgentConnection = toAgentConnectionStatusResponse(dp.Status.AgentConnection)
	}

	return response
}

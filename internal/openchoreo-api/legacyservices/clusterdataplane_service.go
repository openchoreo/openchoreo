// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ClusterDataPlaneService handles cluster-scoped dataplane-related business logic
type ClusterDataPlaneService struct {
	k8sClient client.Client
	logger    *slog.Logger
	authzPDP  authz.PDP
}

// NewClusterDataPlaneService creates a new cluster dataplane service
func NewClusterDataPlaneService(k8sClient client.Client, logger *slog.Logger, authzPDP authz.PDP) *ClusterDataPlaneService {
	return &ClusterDataPlaneService{
		k8sClient: k8sClient,
		logger:    logger,
		authzPDP:  authzPDP,
	}
}

// ListClusterDataPlanes lists all cluster-scoped dataplanes
func (s *ClusterDataPlaneService) ListClusterDataPlanes(ctx context.Context) ([]*models.ClusterDataPlaneResponse, error) {
	result, err := s.ListClusterDataPlanesPaginated(ctx, 0, "")
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

// ListClusterDataPlanesPaginated lists cluster-scoped dataplanes with cursor-based pagination.
// A limit of 0 means no limit (return all items).
func (s *ClusterDataPlaneService) ListClusterDataPlanesPaginated(ctx context.Context, limit int, cursor string) (*models.ClusterDataPlaneListResult, error) {
	s.logger.Debug("Listing cluster dataplanes", "limit", limit, "cursor", cursor)

	listOpts := []client.ListOption{}
	if limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(limit)))
	}
	if cursor != "" {
		listOpts = append(listOpts, client.Continue(cursor))
	}

	var cdpList openchoreov1alpha1.ClusterDataPlaneList
	if err := s.k8sClient.List(ctx, &cdpList, listOpts...); err != nil {
		s.logger.Error("Failed to list cluster dataplanes", "error", err)
		return nil, fmt.Errorf("failed to list cluster dataplanes: %w", err)
	}

	dataplanes := make([]*models.ClusterDataPlaneResponse, 0, len(cdpList.Items))
	for i := range cdpList.Items {
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewClusterDataPlane, ResourceTypeClusterDataPlane, cdpList.Items[i].Name,
			authz.ResourceHierarchy{}); err != nil {
			if errors.Is(err, ErrForbidden) {
				s.logger.Debug("Skipping unauthorized cluster dataplane", "clusterDataPlane", cdpList.Items[i].Name)
				continue
			}
			return nil, fmt.Errorf("authorize %s %s for clusterDataPlane %q: %w",
				SystemActionViewClusterDataPlane, ResourceTypeClusterDataPlane, cdpList.Items[i].Name, err)
		}
		dataplanes = append(dataplanes, s.toClusterDataPlaneResponse(&cdpList.Items[i]))
	}

	s.logger.Debug("Listed cluster dataplanes", "count", len(dataplanes))
	return &models.ClusterDataPlaneListResult{
		Items:          dataplanes,
		NextCursor:     cdpList.GetContinue(),
		RemainingCount: cdpList.GetRemainingItemCount(),
	}, nil
}

// GetClusterDataPlane retrieves a specific cluster-scoped dataplane
func (s *ClusterDataPlaneService) GetClusterDataPlane(ctx context.Context, name string) (*models.ClusterDataPlaneResponse, error) {
	s.logger.Debug("Getting cluster dataplane", "clusterDataPlane", name)

	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewClusterDataPlane, ResourceTypeClusterDataPlane, name,
		authz.ResourceHierarchy{}); err != nil {
		if errors.Is(err, ErrForbidden) {
			return nil, err
		}
		return nil, fmt.Errorf("authorize %s %s for clusterDataPlane %q: %w",
			SystemActionViewClusterDataPlane, ResourceTypeClusterDataPlane, name, err)
	}

	cdp := &openchoreov1alpha1.ClusterDataPlane{}
	key := client.ObjectKey{Name: name}

	if err := s.k8sClient.Get(ctx, key, cdp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ClusterDataPlane not found", "clusterDataPlane", name)
			return nil, ErrClusterDataPlaneNotFound
		}
		s.logger.Error("Failed to get cluster dataplane", "error", err, "clusterDataPlane", name)
		return nil, fmt.Errorf("failed to get cluster dataplane: %w", err)
	}

	return s.toClusterDataPlaneResponse(cdp), nil
}

// CreateClusterDataPlane creates a new cluster-scoped dataplane
func (s *ClusterDataPlaneService) CreateClusterDataPlane(ctx context.Context, req *models.CreateClusterDataPlaneRequest) (*models.ClusterDataPlaneResponse, error) {
	s.logger.Debug("Creating cluster dataplane", "clusterDataPlane", req.Name)

	// Sanitize input
	req.Sanitize()

	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionCreateClusterDataPlane, ResourceTypeClusterDataPlane, req.Name,
		authz.ResourceHierarchy{}); err != nil {
		if errors.Is(err, ErrForbidden) {
			return nil, err
		}
		return nil, fmt.Errorf("authorize %s %s for clusterDataPlane %q: %w",
			SystemActionCreateClusterDataPlane, ResourceTypeClusterDataPlane, req.Name, err)
	}

	// Check if cluster dataplane already exists
	exists, err := s.clusterDataPlaneExists(ctx, req.Name)
	if err != nil {
		s.logger.Error("Failed to check cluster dataplane existence", "error", err)
		return nil, fmt.Errorf("failed to check cluster dataplane existence: %w", err)
	}
	if exists {
		s.logger.Warn("ClusterDataPlane already exists", "clusterDataPlane", req.Name)
		return nil, ErrClusterDataPlaneAlreadyExists
	}

	// Create the cluster dataplane CR
	cdpCR := s.buildClusterDataPlaneCR(req)
	if err := s.k8sClient.Create(ctx, cdpCR); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("ClusterDataPlane already exists", "clusterDataPlane", req.Name)
			return nil, ErrClusterDataPlaneAlreadyExists
		}
		s.logger.Error("Failed to create cluster dataplane CR", "error", err)
		return nil, fmt.Errorf("failed to create cluster dataplane: %w", err)
	}

	s.logger.Debug("ClusterDataPlane created successfully", "clusterDataPlane", req.Name)
	return s.toClusterDataPlaneResponse(cdpCR), nil
}

// clusterDataPlaneExists checks if a cluster dataplane exists
func (s *ClusterDataPlaneService) clusterDataPlaneExists(ctx context.Context, name string) (bool, error) {
	cdp := &openchoreov1alpha1.ClusterDataPlane{}
	key := client.ObjectKey{Name: name}

	if err := s.k8sClient.Get(ctx, key, cdp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// buildClusterDataPlaneCR builds a ClusterDataPlane CR from the request
func (s *ClusterDataPlaneService) buildClusterDataPlaneCR(req *models.CreateClusterDataPlaneRequest) *openchoreov1alpha1.ClusterDataPlane {
	displayName := req.DisplayName
	if displayName == "" {
		displayName = req.Name
	}

	description := req.Description
	if description == "" {
		description = fmt.Sprintf("ClusterDataPlane for %s", req.Name)
	}

	gatewaySpec := openchoreov1alpha1.GatewaySpec{
		PublicVirtualHost:       req.PublicVirtualHost,
		OrganizationVirtualHost: req.OrganizationVirtualHost,
	}

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

	spec := openchoreov1alpha1.ClusterDataPlaneSpec{
		PlaneID: req.PlaneID,
		ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
			ClientCA: openchoreov1alpha1.ValueFrom{
				Value: req.ClusterAgentClientCA,
			},
		},
		Gateway: gatewaySpec,
	}

	if req.ObservabilityPlaneRef != nil && req.ObservabilityPlaneRef.Name != "" {
		spec.ObservabilityPlaneRef = &openchoreov1alpha1.ClusterObservabilityPlaneRef{
			Kind: openchoreov1alpha1.ClusterObservabilityPlaneRefKind(req.ObservabilityPlaneRef.Kind),
			Name: req.ObservabilityPlaneRef.Name,
		}
	}

	return &openchoreov1alpha1.ClusterDataPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterDataPlane",
			APIVersion: "openchoreo.dev/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Name,
			Annotations: map[string]string{
				controller.AnnotationKeyDisplayName: displayName,
				controller.AnnotationKeyDescription: description,
			},
		},
		Spec: spec,
	}
}

// toClusterDataPlaneResponse converts a ClusterDataPlane CR to a ClusterDataPlaneResponse
func (s *ClusterDataPlaneService) toClusterDataPlaneResponse(cdp *openchoreov1alpha1.ClusterDataPlane) *models.ClusterDataPlaneResponse {
	displayName := cdp.Annotations[controller.AnnotationKeyDisplayName]
	description := cdp.Annotations[controller.AnnotationKeyDescription]

	status := statusUnknown
	if len(cdp.Status.Conditions) > 0 {
		latestCondition := cdp.Status.Conditions[len(cdp.Status.Conditions)-1]
		if latestCondition.Status == metav1.ConditionTrue {
			status = statusReady
		} else {
			status = statusNotReady
		}
	}

	var secretStoreRef string
	if cdp.Spec.SecretStoreRef != nil {
		secretStoreRef = cdp.Spec.SecretStoreRef.Name
	}

	response := &models.ClusterDataPlaneResponse{
		Name:                    cdp.Name,
		PlaneID:                 cdp.Spec.PlaneID,
		DisplayName:             displayName,
		Description:             description,
		ImagePullSecretRefs:     cdp.Spec.ImagePullSecretRefs,
		SecretStoreRef:          secretStoreRef,
		PublicVirtualHost:       cdp.Spec.Gateway.PublicVirtualHost,
		OrganizationVirtualHost: cdp.Spec.Gateway.OrganizationVirtualHost,
		PublicHTTPPort:          cdp.Spec.Gateway.PublicHTTPPort,
		PublicHTTPSPort:         cdp.Spec.Gateway.PublicHTTPSPort,
		OrganizationHTTPPort:    cdp.Spec.Gateway.OrganizationHTTPPort,
		OrganizationHTTPSPort:   cdp.Spec.Gateway.OrganizationHTTPSPort,
		CreatedAt:               cdp.CreationTimestamp.Time,
		Status:                  status,
	}

	if cdp.Spec.ObservabilityPlaneRef != nil {
		response.ObservabilityPlaneRef = &models.ObservabilityPlaneRef{
			Kind: string(cdp.Spec.ObservabilityPlaneRef.Kind),
			Name: cdp.Spec.ObservabilityPlaneRef.Name,
		}
	}

	if cdp.Status.AgentConnection != nil {
		response.AgentConnection = toAgentConnectionStatusResponse(cdp.Status.AgentConnection)
	}

	return response
}

package services

import (
	"context"
	"fmt"

	"golang.org/x/exp/slog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// DataPlaneService handles dataplane-related business logic
type DataPlaneService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

// NewDataPlaneService creates a new dataplane service
func NewDataPlaneService(k8sClient client.Client, logger *slog.Logger) *DataPlaneService {
	return &DataPlaneService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

// ListDataPlanes lists all dataplanes in the specified organization
func (s *DataPlaneService) ListDataPlanes(ctx context.Context, orgName string) ([]*models.DataPlaneResponse, error) {
	s.logger.Debug("Listing dataplanes", "org", orgName)

	var dpList openchoreov1alpha1.DataPlaneList
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
	}

	if err := s.k8sClient.List(ctx, &dpList, listOpts...); err != nil {
		s.logger.Error("Failed to list dataplanes", "error", err, "org", orgName)
		return nil, fmt.Errorf("failed to list dataplanes: %w", err)
	}

	var dataplanes []*models.DataPlaneResponse
	for _, item := range dpList.Items {
		dataplanes = append(dataplanes, s.toDataPlaneResponse(&item))
	}

	s.logger.Debug("Listed dataplanes", "count", len(dataplanes), "org", orgName)
	return dataplanes, nil
}

// GetDataPlane retrieves a specific dataplane
func (s *DataPlaneService) GetDataPlane(ctx context.Context, orgName, dpName string) (*models.DataPlaneResponse, error) {
	s.logger.Debug("Getting dataplane", "org", orgName, "dataplane", dpName)

	dp := &openchoreov1alpha1.DataPlane{}
	key := client.ObjectKey{
		Name:      dpName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, dp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("DataPlane not found", "org", orgName, "dataplane", dpName)
			return nil, ErrDataPlaneNotFound
		}
		s.logger.Error("Failed to get dataplane", "error", err, "org", orgName, "dataplane", dpName)
		return nil, fmt.Errorf("failed to get dataplane: %w", err)
	}

	return s.toDataPlaneResponse(dp), nil
}

// CreateDataPlane creates a new dataplane
func (s *DataPlaneService) CreateDataPlane(ctx context.Context, orgName string, req *models.CreateDataPlaneRequest) (*models.DataPlaneResponse, error) {
	s.logger.Debug("Creating dataplane", "org", orgName, "dataplane", req.Name)

	// Sanitize input
	req.Sanitize()

	// Check if dataplane already exists
	exists, err := s.dataPlaneExists(ctx, orgName, req.Name)
	if err != nil {
		s.logger.Error("Failed to check dataplane existence", "error", err)
		return nil, fmt.Errorf("failed to check dataplane existence: %w", err)
	}
	if exists {
		s.logger.Warn("DataPlane already exists", "org", orgName, "dataplane", req.Name)
		return nil, ErrDataPlaneAlreadyExists
	}

	// Create the dataplane CR
	dataplaneCR := s.buildDataPlaneCR(orgName, req)
	if err := s.k8sClient.Create(ctx, dataplaneCR); err != nil {
		s.logger.Error("Failed to create dataplane CR", "error", err)
		return nil, fmt.Errorf("failed to create dataplane: %w", err)
	}

	s.logger.Debug("DataPlane created successfully", "org", orgName, "dataplane", req.Name)
	return s.toDataPlaneResponse(dataplaneCR), nil
}

// dataPlaneExists checks if a dataplane exists in the given organization
func (s *DataPlaneService) dataPlaneExists(ctx context.Context, orgName, dpName string) (bool, error) {
	dp := &openchoreov1alpha1.DataPlane{}
	key := client.ObjectKey{
		Name:      dpName,
		Namespace: orgName,
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
func (s *DataPlaneService) buildDataPlaneCR(orgName string, req *models.CreateDataPlaneRequest) *openchoreov1alpha1.DataPlane {
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

	spec := openchoreov1alpha1.DataPlaneSpec{
		Registry: openchoreov1alpha1.Registry{
			Prefix:    req.RegistryPrefix,
			SecretRef: req.RegistrySecretRef,
		},
		KubernetesCluster: openchoreov1alpha1.KubernetesClusterSpec{
			Name: req.KubernetesClusterName,
			Credentials: openchoreov1alpha1.APIServerCredentials{
				APIServerURL: req.APIServerURL,
				CACert:       req.CACert,
				ClientCert:   req.ClientCert,
				ClientKey:    req.ClientKey,
			},
		},
		Gateway: openchoreov1alpha1.GatewaySpec{
			PublicVirtualHost:           req.PublicVirtualHost,
			OrganizationVirtualHost:     req.OrganizationVirtualHost,
		},
	}

	// Add observer configuration if provided
	if req.ObserverURL != "" {
		spec.Observer = openchoreov1alpha1.ObserverAPI{
			URL: req.ObserverURL,
			Authentication: openchoreov1alpha1.ObserverAuthentication{
				BasicAuth: openchoreov1alpha1.BasicAuthCredentials{
					Username: req.ObserverUsername,
					Password: req.ObserverPassword,
				},
			},
		}
	}

	return &openchoreov1alpha1.DataPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DataPlane",
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
		Spec: spec,
	}
}

// toDataPlaneResponse converts a DataPlane CR to a DataPlaneResponse
func (s *DataPlaneService) toDataPlaneResponse(dp *openchoreov1alpha1.DataPlane) *models.DataPlaneResponse {
	// Extract display name and description from annotations
	displayName := dp.Annotations[controller.AnnotationKeyDisplayName]
	description := dp.Annotations[controller.AnnotationKeyDescription]

	// Get status from conditions
	status := "Unknown"
	if len(dp.Status.Conditions) > 0 {
		// Get the latest condition
		latestCondition := dp.Status.Conditions[len(dp.Status.Conditions)-1]
		if latestCondition.Status == metav1.ConditionTrue {
			status = "Ready"
		} else {
			status = "NotReady"
		}
	}

	response := &models.DataPlaneResponse{
		Name:                        dp.Name,
		Namespace:                   dp.Namespace,
		DisplayName:                 displayName,
		Description:                 description,
		RegistryPrefix:              dp.Spec.Registry.Prefix,
		RegistrySecretRef:           dp.Spec.Registry.SecretRef,
		KubernetesClusterName:       dp.Spec.KubernetesCluster.Name,
		APIServerURL:                dp.Spec.KubernetesCluster.Credentials.APIServerURL,
		PublicVirtualHost:           dp.Spec.Gateway.PublicVirtualHost,
		OrganizationVirtualHost:     dp.Spec.Gateway.OrganizationVirtualHost,
		CreatedAt:                   dp.CreationTimestamp.Time,
		Status:                      status,
	}

	// Add observer configuration if present
	if dp.Spec.Observer.URL != "" {
		response.ObserverURL = dp.Spec.Observer.URL
		response.ObserverUsername = dp.Spec.Observer.Authentication.BasicAuth.Username
		// Password is excluded for security
	}

	return response
}
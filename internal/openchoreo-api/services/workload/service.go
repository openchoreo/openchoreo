// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"context"
	"fmt"
	"log/slog"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// workloadService handles workload business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type workloadService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*workloadService)(nil)

// NewService creates a new workload service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &workloadService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *workloadService) CreateWorkload(ctx context.Context, namespaceName string, w *openchoreov1alpha1.Workload) (*openchoreov1alpha1.Workload, error) {
	if w == nil {
		return nil, fmt.Errorf("workload cannot be nil")
	}

	s.logger.Debug("Creating workload", "namespace", namespaceName, "workload", w.Name)

	// Validate that the referenced component exists
	if err := s.validateComponentExists(ctx, namespaceName, w.Spec.Owner.ComponentName); err != nil {
		return nil, err
	}

	exists, err := s.workloadExists(ctx, namespaceName, w.Name)
	if err != nil {
		s.logger.Error("Failed to check workload existence", "error", err)
		return nil, fmt.Errorf("failed to check workload existence: %w", err)
	}
	if exists {
		s.logger.Warn("Workload already exists", "namespace", namespaceName, "workload", w.Name)
		return nil, ErrWorkloadAlreadyExists
	}

	// Set defaults
	w.TypeMeta = metav1.TypeMeta{
		Kind:       "Workload",
		APIVersion: "openchoreo.dev/v1alpha1",
	}
	w.Namespace = namespaceName
	if w.Labels == nil {
		w.Labels = make(map[string]string)
	}
	w.Labels[labels.LabelKeyProjectName] = w.Spec.Owner.ProjectName
	w.Labels[labels.LabelKeyComponentName] = w.Spec.Owner.ComponentName

	if err := s.k8sClient.Create(ctx, w); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Workload already exists", "namespace", namespaceName, "workload", w.Name)
			return nil, ErrWorkloadAlreadyExists
		}
		s.logger.Error("Failed to create workload CR", "error", err)
		return nil, fmt.Errorf("failed to create workload: %w", err)
	}

	s.logger.Debug("Workload created successfully", "namespace", namespaceName, "workload", w.Name)
	return w, nil
}

func (s *workloadService) UpdateWorkload(ctx context.Context, namespaceName string, w *openchoreov1alpha1.Workload) (*openchoreov1alpha1.Workload, error) {
	if w == nil {
		return nil, fmt.Errorf("workload cannot be nil")
	}

	s.logger.Debug("Updating workload", "namespace", namespaceName, "workload", w.Name)

	existing := &openchoreov1alpha1.Workload{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: w.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Workload not found", "namespace", namespaceName, "workload", w.Name)
			return nil, ErrWorkloadNotFound
		}
		s.logger.Error("Failed to get workload", "error", err)
		return nil, fmt.Errorf("failed to get workload: %w", err)
	}

	// Validate that the referenced component exists
	if err := s.validateComponentExists(ctx, namespaceName, w.Spec.Owner.ComponentName); err != nil {
		return nil, err
	}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	existing.Spec = w.Spec
	existing.Labels = w.Labels
	existing.Annotations = w.Annotations

	// Preserve special labels
	if existing.Labels == nil {
		existing.Labels = make(map[string]string)
	}
	existing.Labels[labels.LabelKeyProjectName] = existing.Spec.Owner.ProjectName
	existing.Labels[labels.LabelKeyComponentName] = existing.Spec.Owner.ComponentName

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if apierrors.IsInvalid(err) {
			s.logger.Error("Workload update rejected by validation", "error", err)
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to update workload CR", "error", err)
		return nil, fmt.Errorf("failed to update workload: %w", err)
	}

	s.logger.Debug("Workload updated successfully", "namespace", namespaceName, "workload", w.Name)
	return existing, nil
}

func (s *workloadService) ListWorkloads(ctx context.Context, namespaceName, componentName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Workload], error) {
	s.logger.Debug("Listing workloads", "namespace", namespaceName, "component", componentName, "limit", opts.Limit, "cursor", opts.Cursor)

	listFn := func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Workload], error) {
		listOpts := []client.ListOption{
			client.InNamespace(namespaceName),
		}
		if pageOpts.Limit > 0 {
			listOpts = append(listOpts, client.Limit(int64(pageOpts.Limit)))
		}
		if pageOpts.Cursor != "" {
			listOpts = append(listOpts, client.Continue(pageOpts.Cursor))
		}

		var wList openchoreov1alpha1.WorkloadList
		if err := s.k8sClient.List(ctx, &wList, listOpts...); err != nil {
			s.logger.Error("Failed to list workloads", "error", err)
			return nil, fmt.Errorf("failed to list workloads: %w", err)
		}

		result := &services.ListResult[openchoreov1alpha1.Workload]{
			Items:      wList.Items,
			NextCursor: wList.Continue,
		}
		if wList.RemainingItemCount != nil {
			remaining := *wList.RemainingItemCount
			result.RemainingCount = &remaining
		}
		return result, nil
	}

	// Apply component filter if specified
	if componentName != "" {
		filteredFn := services.PreFilteredList(
			listFn,
			func(w openchoreov1alpha1.Workload) bool {
				return w.Spec.Owner.ComponentName == componentName
			},
		)
		return filteredFn(ctx, opts)
	}

	return listFn(ctx, opts)
}

func (s *workloadService) GetWorkload(ctx context.Context, namespaceName, workloadName string) (*openchoreov1alpha1.Workload, error) {
	s.logger.Debug("Getting workload", "namespace", namespaceName, "workload", workloadName)

	w := &openchoreov1alpha1.Workload{}
	key := client.ObjectKey{
		Name:      workloadName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, w); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Workload not found", "namespace", namespaceName, "workload", workloadName)
			return nil, ErrWorkloadNotFound
		}
		s.logger.Error("Failed to get workload", "error", err)
		return nil, fmt.Errorf("failed to get workload: %w", err)
	}

	return w, nil
}

func (s *workloadService) DeleteWorkload(ctx context.Context, namespaceName, workloadName string) error {
	s.logger.Debug("Deleting workload", "namespace", namespaceName, "workload", workloadName)

	w := &openchoreov1alpha1.Workload{}
	w.Name = workloadName
	w.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, w); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrWorkloadNotFound
		}
		s.logger.Error("Failed to delete workload CR", "error", err)
		return fmt.Errorf("failed to delete workload: %w", err)
	}

	s.logger.Debug("Workload deleted successfully", "namespace", namespaceName, "workload", workloadName)
	return nil
}

func (s *workloadService) workloadExists(ctx context.Context, namespaceName, workloadName string) (bool, error) {
	w := &openchoreov1alpha1.Workload{}
	key := client.ObjectKey{
		Name:      workloadName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, w)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of workload %s/%s: %w", namespaceName, workloadName, err)
	}
	return true, nil
}

func (s *workloadService) GetWorkloadSchema(_ context.Context) (*extv1.JSONSchemaProps, error) {
	return workloadSpecSchema(), nil
}

func workloadSpecSchema() *extv1.JSONSchemaProps {
	stringType := "string"
	intType := "integer"
	arrayType := "array"
	objectType := "object"

	envVarSchema := extv1.JSONSchemaProps{
		Type:        objectType,
		Description: "Environment variable for the container.",
		Required:    []string{"key"},
		Properties: map[string]extv1.JSONSchemaProps{
			"key": {
				Type:        stringType,
				Description: "The environment variable key.",
			},
			"value": {
				Type:        stringType,
				Description: "The literal value. Mutually exclusive with valueFrom.",
			},
			"valueFrom": {
				Type:        objectType,
				Description: "Extract value from another resource. Mutually exclusive with value.",
				Properties: map[string]extv1.JSONSchemaProps{
					"configurationGroupRef": {
						Type:     objectType,
						Required: []string{"name", "key"},
						Properties: map[string]extv1.JSONSchemaProps{
							"name": {Type: stringType},
							"key":  {Type: stringType},
						},
					},
					"secretRef": {
						Type:     objectType,
						Required: []string{"name", "key"},
						Properties: map[string]extv1.JSONSchemaProps{
							"name": {Type: stringType},
							"key":  {Type: stringType},
						},
					},
				},
			},
		},
	}

	fileVarSchema := extv1.JSONSchemaProps{
		Type:        objectType,
		Description: "File mount configuration for the container.",
		Required:    []string{"key", "mountPath"},
		Properties: map[string]extv1.JSONSchemaProps{
			"key": {
				Type:        stringType,
				Description: "The file key/name.",
			},
			"mountPath": {
				Type:        stringType,
				Description: "The mount path where the file will be mounted.",
			},
			"value": {
				Type:        stringType,
				Description: "The literal content of the file. Mutually exclusive with valueFrom.",
			},
			"valueFrom": {
				Type:        objectType,
				Description: "Extract value from another resource. Mutually exclusive with value.",
				Properties: map[string]extv1.JSONSchemaProps{
					"configurationGroupRef": {
						Type:     objectType,
						Required: []string{"name", "key"},
						Properties: map[string]extv1.JSONSchemaProps{
							"name": {Type: stringType},
							"key":  {Type: stringType},
						},
					},
					"secretRef": {
						Type:     objectType,
						Required: []string{"name", "key"},
						Properties: map[string]extv1.JSONSchemaProps{
							"name": {Type: stringType},
							"key":  {Type: stringType},
						},
					},
				},
			},
		},
	}

	var minPort, maxPort float64 = 1, 65535
	endpointSchema := extv1.JSONSchemaProps{
		Type:        objectType,
		Description: "Network endpoint for port exposure.",
		Required:    []string{"type", "port"},
		Properties: map[string]extv1.JSONSchemaProps{
			"type": {
				Type:        stringType,
				Description: "Protocol/technology of the endpoint.",
				Enum:        []extv1.JSON{{Raw: []byte(`"HTTP"`)}, {Raw: []byte(`"REST"`)}, {Raw: []byte(`"gRPC"`)}, {Raw: []byte(`"GraphQL"`)}, {Raw: []byte(`"Websocket"`)}, {Raw: []byte(`"TCP"`)}, {Raw: []byte(`"UDP"`)}},
			},
			"port": {
				Type:        intType,
				Description: "Port exposed by the endpoint.",
				Minimum:     &minPort,
				Maximum:     &maxPort,
			},
			"targetPort": {
				Type:        intType,
				Description: "Container listening port. Defaults to port if not set.",
				Minimum:     &minPort,
				Maximum:     &maxPort,
			},
			"visibility": {
				Type:        arrayType,
				Description: "Additional endpoint visibilities beyond implicit project visibility.",
				Items: &extv1.JSONSchemaPropsOrArray{
					Schema: &extv1.JSONSchemaProps{
						Type: stringType,
						Enum: []extv1.JSON{{Raw: []byte(`"project"`)}, {Raw: []byte(`"namespace"`)}, {Raw: []byte(`"internal"`)}, {Raw: []byte(`"external"`)}},
					},
				},
			},
			"displayName": {
				Type:        stringType,
				Description: "Human-readable name for the endpoint.",
			},
			"basePath": {
				Type:        stringType,
				Description: "Base path of the API exposed via the endpoint.",
			},
			"schema": {
				Type:        objectType,
				Description: "API definition schema for the endpoint.",
				Properties: map[string]extv1.JSONSchemaProps{
					"type":    {Type: stringType},
					"content": {Type: stringType},
				},
			},
		},
	}

	connectionSchema := extv1.JSONSchemaProps{
		Type:        objectType,
		Description: "Connection to another component's endpoint.",
		Required:    []string{"component", "endpoint", "visibility"},
		Properties: map[string]extv1.JSONSchemaProps{
			"component": {
				Type:        stringType,
				Description: "Target component name.",
			},
			"endpoint": {
				Type:        stringType,
				Description: "Target endpoint name on the target component.",
			},
			"visibility": {
				Type:        stringType,
				Description: "Visibility level at which this connection consumes the endpoint.",
				Enum:        []extv1.JSON{{Raw: []byte(`"namespace"`)}, {Raw: []byte(`"project"`)}},
			},
			"project": {
				Type:        stringType,
				Description: "Target component's project name. If empty, defaults to the same project as the consumer.",
			},
			"envBindings": {
				Type:        objectType,
				Description: "Maps resolved connection address components to environment variable names.",
				Properties: map[string]extv1.JSONSchemaProps{
					"address": {
						Type:        stringType,
						Description: "Env var name for the protocol-appropriate connection string.",
					},
					"basePath": {
						Type:        stringType,
						Description: "Env var name for just the base path.",
					},
					"host": {
						Type:        stringType,
						Description: "Env var name for just the hostname.",
					},
					"port": {
						Type:        stringType,
						Description: "Env var name for just the port number.",
					},
				},
			},
		},
	}

	return &extv1.JSONSchemaProps{
		Type:        objectType,
		Description: "Workload specification defining the runtime configuration for a component.",
		Required:    []string{"container"},
		Properties: map[string]extv1.JSONSchemaProps{
			"container": {
				Type:        objectType,
				Description: "Container specification for this workload.",
				Required:    []string{"image"},
				Properties: map[string]extv1.JSONSchemaProps{
					"image": {
						Type:        stringType,
						Description: "OCI image to run (digest or tag).",
					},
					"command": {
						Type:        arrayType,
						Description: "Container entrypoint.",
						Items:       &extv1.JSONSchemaPropsOrArray{Schema: &extv1.JSONSchemaProps{Type: stringType}},
					},
					"args": {
						Type:        arrayType,
						Description: "Arguments to the entrypoint.",
						Items:       &extv1.JSONSchemaPropsOrArray{Schema: &extv1.JSONSchemaProps{Type: stringType}},
					},
					"env": {
						Type:        arrayType,
						Description: "Environment variables for the container.",
						Items:       &extv1.JSONSchemaPropsOrArray{Schema: &envVarSchema},
					},
					"files": {
						Type:        arrayType,
						Description: "File mount configurations.",
						Items:       &extv1.JSONSchemaPropsOrArray{Schema: &fileVarSchema},
					},
				},
			},
			"endpoints": {
				Type:                 objectType,
				Description:          "Network endpoints for port exposure. Keys are endpoint names.",
				AdditionalProperties: &extv1.JSONSchemaPropsOrBool{Schema: &endpointSchema},
			},
			"connections": {
				Type:                 objectType,
				Description:          "Connections to internal and external resources. Keys are connection names.",
				AdditionalProperties: &extv1.JSONSchemaPropsOrBool{Schema: &connectionSchema},
			},
		},
	}
}

func (s *workloadService) validateComponentExists(ctx context.Context, namespaceName, componentName string) error {
	component := &openchoreov1alpha1.Component{}
	key := client.ObjectKey{
		Name:      componentName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrComponentNotFound
		}
		return fmt.Errorf("failed to validate component: %w", err)
	}
	return nil
}

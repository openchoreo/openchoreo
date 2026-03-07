// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflow

import (
	"context"
	"fmt"
	"log/slog"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/schema"
	"github.com/openchoreo/openchoreo/internal/schema/extractor"
)

// clusterWorkflowService handles cluster workflow business logic without authorization checks.
type clusterWorkflowService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*clusterWorkflowService)(nil)

var clusterWorkflowTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ClusterWorkflow",
}

// NewService creates a new cluster workflow service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &clusterWorkflowService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *clusterWorkflowService) CreateClusterWorkflow(ctx context.Context, cwf *openchoreov1alpha1.ClusterWorkflow) (*openchoreov1alpha1.ClusterWorkflow, error) {
	if cwf == nil {
		return nil, fmt.Errorf("cluster workflow cannot be nil")
	}

	s.logger.Debug("Creating cluster workflow", "clusterWorkflow", cwf.Name)

	// Set defaults
	cwf.Status = openchoreov1alpha1.ClusterWorkflowStatus{}

	if err := s.k8sClient.Create(ctx, cwf); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Cluster workflow already exists", "clusterWorkflow", cwf.Name)
			return nil, ErrClusterWorkflowAlreadyExists
		}
		s.logger.Error("Failed to create cluster workflow CR", "error", err)
		return nil, fmt.Errorf("failed to create cluster workflow: %w", err)
	}

	s.logger.Debug("Cluster workflow created successfully", "clusterWorkflow", cwf.Name)
	cwf.TypeMeta = clusterWorkflowTypeMeta
	return cwf, nil
}

func (s *clusterWorkflowService) UpdateClusterWorkflow(ctx context.Context, cwf *openchoreov1alpha1.ClusterWorkflow) (*openchoreov1alpha1.ClusterWorkflow, error) {
	if cwf == nil {
		return nil, fmt.Errorf("cluster workflow cannot be nil")
	}

	s.logger.Debug("Updating cluster workflow", "clusterWorkflow", cwf.Name)

	existing := &openchoreov1alpha1.ClusterWorkflow{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: cwf.Name}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Cluster workflow not found", "clusterWorkflow", cwf.Name)
			return nil, ErrClusterWorkflowNotFound
		}
		s.logger.Error("Failed to get cluster workflow", "error", err)
		return nil, fmt.Errorf("failed to get cluster workflow: %w", err)
	}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	cwf.Status = openchoreov1alpha1.ClusterWorkflowStatus{}
	existing.Spec = cwf.Spec
	existing.Labels = cwf.Labels
	existing.Annotations = cwf.Annotations

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if apierrors.IsInvalid(err) {
			s.logger.Error("Cluster workflow update rejected by validation", "error", err)
			return nil, fmt.Errorf("cluster workflow validation failed: %s", services.ExtractValidationMessage(err))
		}
		s.logger.Error("Failed to update cluster workflow CR", "error", err)
		return nil, fmt.Errorf("failed to update cluster workflow: %w", err)
	}

	s.logger.Debug("Cluster workflow updated successfully", "clusterWorkflow", cwf.Name)
	existing.TypeMeta = clusterWorkflowTypeMeta
	return existing, nil
}

func (s *clusterWorkflowService) ListClusterWorkflows(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterWorkflow], error) {
	s.logger.Debug("Listing cluster workflows", "limit", opts.Limit, "cursor", opts.Cursor)

	var listOpts []client.ListOption
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var wfList openchoreov1alpha1.ClusterWorkflowList
	if err := s.k8sClient.List(ctx, &wfList, listOpts...); err != nil {
		s.logger.Error("Failed to list cluster workflows", "error", err)
		return nil, fmt.Errorf("failed to list cluster workflows: %w", err)
	}

	for i := range wfList.Items {
		wfList.Items[i].TypeMeta = clusterWorkflowTypeMeta
	}

	result := &services.ListResult[openchoreov1alpha1.ClusterWorkflow]{
		Items:      wfList.Items,
		NextCursor: wfList.Continue,
	}
	if wfList.RemainingItemCount != nil {
		remaining := *wfList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed cluster workflows", "count", len(wfList.Items))
	return result, nil
}

func (s *clusterWorkflowService) GetClusterWorkflow(ctx context.Context, clusterWorkflowName string) (*openchoreov1alpha1.ClusterWorkflow, error) {
	s.logger.Debug("Getting cluster workflow", "clusterWorkflow", clusterWorkflowName)

	cwf := &openchoreov1alpha1.ClusterWorkflow{}
	key := client.ObjectKey{
		Name: clusterWorkflowName,
	}

	if err := s.k8sClient.Get(ctx, key, cwf); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Cluster workflow not found", "clusterWorkflow", clusterWorkflowName)
			return nil, ErrClusterWorkflowNotFound
		}
		s.logger.Error("Failed to get cluster workflow", "error", err)
		return nil, fmt.Errorf("failed to get cluster workflow: %w", err)
	}

	cwf.TypeMeta = clusterWorkflowTypeMeta
	return cwf, nil
}

// DeleteClusterWorkflow removes a cluster-scoped workflow by name.
func (s *clusterWorkflowService) DeleteClusterWorkflow(ctx context.Context, clusterWorkflowName string) error {
	s.logger.Debug("Deleting cluster workflow", "clusterWorkflow", clusterWorkflowName)

	cwf := &openchoreov1alpha1.ClusterWorkflow{}
	cwf.Name = clusterWorkflowName

	if err := s.k8sClient.Delete(ctx, cwf); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrClusterWorkflowNotFound
		}
		s.logger.Error("Failed to delete cluster workflow CR", "error", err)
		return fmt.Errorf("failed to delete cluster workflow: %w", err)
	}

	s.logger.Debug("Cluster workflow deleted successfully", "clusterWorkflow", clusterWorkflowName)
	return nil
}

func (s *clusterWorkflowService) GetClusterWorkflowSchema(ctx context.Context, clusterWorkflowName string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting cluster workflow schema", "clusterWorkflow", clusterWorkflowName)

	cwf, err := s.GetClusterWorkflow(ctx, clusterWorkflowName)
	if err != nil {
		return nil, err
	}

	var schemaMap map[string]any
	if cwf.Spec.Schema != nil && cwf.Spec.Schema.Parameters != nil {
		if err := yaml.Unmarshal(cwf.Spec.Schema.Parameters.Raw, &schemaMap); err != nil {
			return nil, fmt.Errorf("failed to extract schema: %w", err)
		}
	}

	def := schema.Definition{
		Schemas: []map[string]any{schemaMap},
		Options: extractor.Options{
			SkipDefaultValidation: true,
		},
	}

	jsonSchema, err := schema.ToJSONSchema(def)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
	}

	s.logger.Debug("Retrieved cluster workflow schema successfully", "clusterWorkflow", clusterWorkflowName)
	return jsonSchema, nil
}

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the workflow service interface.
// Both the core service (no authz) and the authz-wrapped service implement this.
// Methods accept and return Kubernetes CRD types directly for alignment with
// the K8s-native API design.
type Service interface {
	ListWorkflows(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Workflow], error)
	GetWorkflow(ctx context.Context, namespaceName, workflowName string) (*openchoreov1alpha1.Workflow, error)
	GetWorkflowSchema(ctx context.Context, namespaceName, workflowName string) (*extv1.JSONSchemaProps, error)
	CreateWorkflow(ctx context.Context, namespaceName string, wf *openchoreov1alpha1.Workflow) (*openchoreov1alpha1.Workflow, error)
	UpdateWorkflow(ctx context.Context, namespaceName string, wf *openchoreov1alpha1.Workflow) (*openchoreov1alpha1.Workflow, error)
	DeleteWorkflow(ctx context.Context, namespaceName, workflowName string) error
}

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowplane

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// Client defines the client methods used by WorkflowPlane operations.
type Client interface {
	ListWorkflowPlanes(ctx context.Context, namespaceName string, params *gen.ListWorkflowPlanesParams) (*gen.WorkflowPlaneList, error)
	GetWorkflowPlane(ctx context.Context, namespaceName string, workflowPlaneName string) (*gen.WorkflowPlane, error)
	DeleteWorkflowPlane(ctx context.Context, namespaceName string, workflowPlaneName string) error
}

// WorkflowPlane implements workflow plane operations
type WorkflowPlane struct {
	client Client
}

// New creates a new workflow plane implementation
func New(client Client) *WorkflowPlane {
	return &WorkflowPlane{client: client}
}

// List lists all workflow planes in a namespace
func (b *WorkflowPlane) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceWorkflowPlane, params); err != nil {
		return err
	}

	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.WorkflowPlane, string, error) {
		p := &gen.ListWorkflowPlanesParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := b.client.ListWorkflowPlanes(ctx, params.Namespace, p)
		if err != nil {
			return nil, "", err
		}
		next := ""
		if result.Pagination.NextCursor != nil {
			next = *result.Pagination.NextCursor
		}
		return result.Items, next, nil
	})
	if err != nil {
		return err
	}
	return printList(items)
}

// Get retrieves a single workflow plane and outputs it as YAML
func (b *WorkflowPlane) Get(params GetParams) error {
	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceWorkflowPlane, params); err != nil {
		return err
	}

	ctx := context.Background()

	result, err := b.client.GetWorkflowPlane(ctx, params.Namespace, params.WorkflowPlaneName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow plane to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single workflow plane
func (b *WorkflowPlane) Delete(params DeleteParams) error {
	if err := validation.ValidateParams(validation.CmdDelete, validation.ResourceWorkflowPlane, params); err != nil {
		return err
	}

	ctx := context.Background()

	if err := b.client.DeleteWorkflowPlane(ctx, params.Namespace, params.WorkflowPlaneName); err != nil {
		return err
	}

	fmt.Printf("WorkflowPlane '%s' deleted\n", params.WorkflowPlaneName)
	return nil
}

func printList(items []gen.WorkflowPlane) error {
	if len(items) == 0 {
		fmt.Println("No workflow planes found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, wp := range items {
		age := ""
		if wp.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*wp.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			wp.Metadata.Name,
			age)
	}

	return w.Flush()
}

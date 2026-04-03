// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflow

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflow"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/workflowrun"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// Client defines the client methods used by ClusterWorkflow operations.
type Client interface {
	ListClusterWorkflows(ctx context.Context, params *gen.ListClusterWorkflowsParams) (*gen.ClusterWorkflowList, error)
	GetClusterWorkflow(ctx context.Context, clusterWorkflowName string) (*gen.ClusterWorkflow, error)
	DeleteClusterWorkflow(ctx context.Context, clusterWorkflowName string) error
}

// ClusterWorkflow implements cluster workflow operations
type ClusterWorkflow struct {
	client Client
}

// New creates a new cluster workflow implementation
func New(client Client) *ClusterWorkflow {
	return &ClusterWorkflow{client: client}
}

// List lists all cluster-scoped workflows
func (c *ClusterWorkflow) List() error {
	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ClusterWorkflow, string, error) {
		p := &gen.ListClusterWorkflowsParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := c.client.ListClusterWorkflows(ctx, p)
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

// Get retrieves a single cluster workflow and outputs it as YAML
func (c *ClusterWorkflow) Get(params GetParams) error {
	ctx := context.Background()

	result, err := c.client.GetClusterWorkflow(ctx, params.ClusterWorkflowName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster workflow to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single cluster workflow
func (c *ClusterWorkflow) Delete(params DeleteParams) error {
	ctx := context.Background()

	if err := c.client.DeleteClusterWorkflow(ctx, params.ClusterWorkflowName); err != nil {
		return err
	}

	fmt.Printf("ClusterWorkflow '%s' deleted\n", params.ClusterWorkflowName)
	return nil
}

// StartRun starts a cluster workflow run in the given namespace.
func (c *ClusterWorkflow) StartRun(params StartRunParams) error {
	if params.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if params.WorkflowName == "" {
		return fmt.Errorf("cluster workflow name is required")
	}

	cl, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}
	return workflow.New(cl).StartRun(workflow.StartRunParams{
		Namespace:    params.Namespace,
		WorkflowName: params.WorkflowName,
		WorkflowKind: "ClusterWorkflow",
		Set:          params.Set,
	})
}

// Logs fetches and displays logs for a cluster workflow.
func (c *ClusterWorkflow) Logs(params LogsParams) error {
	if params.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if params.WorkflowName == "" {
		return fmt.Errorf("cluster workflow name is required")
	}

	runName := params.RunName
	if runName == "" {
		var err error
		runName, err = workflow.ResolveLatestRun(params.Namespace, params.WorkflowName, nil)
		if err != nil {
			return err
		}
	}

	return workflowrun.New(nil).Logs(workflowrun.LogsParams{
		Namespace:       params.Namespace,
		WorkflowRunName: runName,
		Follow:          params.Follow,
		Since:           params.Since,
	})
}

func printList(items []gen.ClusterWorkflow) error {
	if len(items) == 0 {
		fmt.Println("No cluster workflows found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, wf := range items {
		age := ""
		if wf.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*wf.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			wf.Metadata.Name,
			age)
	}

	return w.Flush()
}

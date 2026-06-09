// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflowplane

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ClusterWorkflowPlane implements cluster workflow plane operations
type ClusterWorkflowPlane struct {
	client client.Interface
}

// New creates a new cluster workflow plane implementation
func New(c client.Interface) *ClusterWorkflowPlane {
	return &ClusterWorkflowPlane{client: c}
}

// List lists all cluster-scoped workflow planes
func (c *ClusterWorkflowPlane) List() error {
	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ClusterWorkflowPlane, string, error) {
		p := &gen.ListClusterWorkflowPlanesParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := c.client.ListClusterWorkflowPlanes(ctx, p)
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

// Get retrieves a single cluster workflow plane and outputs it as YAML
func (c *ClusterWorkflowPlane) Get(params GetParams) error {
	ctx := context.Background()

	result, err := c.client.GetClusterWorkflowPlane(ctx, params.ClusterWorkflowPlaneName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster workflow plane to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single cluster workflow plane
func (c *ClusterWorkflowPlane) Delete(params DeleteParams) error {
	ctx := context.Background()

	if err := c.client.DeleteClusterWorkflowPlane(ctx, params.ClusterWorkflowPlaneName); err != nil {
		return err
	}

	fmt.Printf("ClusterWorkflowPlane '%s' deleted\n", params.ClusterWorkflowPlaneName)
	return nil
}

func printList(items []gen.ClusterWorkflowPlane) error {
	if len(items) == 0 {
		fmt.Println("No cluster workflow planes found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, cwp := range items {
		age := ""
		if cwp.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*cwp.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			cwp.Metadata.Name,
			age)
	}

	return w.Flush()
}

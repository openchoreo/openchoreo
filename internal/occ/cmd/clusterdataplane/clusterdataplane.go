// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

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

// ClusterDataPlane implements cluster data plane operations
type ClusterDataPlane struct {
	client client.Interface
}

// New creates a new cluster data plane implementation
func New(c client.Interface) *ClusterDataPlane {
	return &ClusterDataPlane{client: c}
}

// List lists all cluster-scoped data planes
func (c *ClusterDataPlane) List() error {
	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ClusterDataPlane, string, error) {
		p := &gen.ListClusterDataPlanesParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := c.client.ListClusterDataPlanes(ctx, p)
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

// Get retrieves a single cluster data plane and outputs it as YAML
func (c *ClusterDataPlane) Get(params GetParams) error {
	ctx := context.Background()

	result, err := c.client.GetClusterDataPlane(ctx, params.ClusterDataPlaneName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster data plane to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single cluster data plane
func (c *ClusterDataPlane) Delete(params DeleteParams) error {
	ctx := context.Background()

	if err := c.client.DeleteClusterDataPlane(ctx, params.ClusterDataPlaneName); err != nil {
		return err
	}

	fmt.Printf("ClusterDataPlane '%s' deleted\n", params.ClusterDataPlaneName)
	return nil
}

func printList(items []gen.ClusterDataPlane) error {
	if len(items) == 0 {
		fmt.Println("No cluster data planes found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, cdp := range items {
		age := ""
		if cdp.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*cdp.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			cdp.Metadata.Name,
			age)
	}

	return w.Flush()
}

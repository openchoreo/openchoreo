// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterobservabilityplane

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// Client defines the client methods used by ClusterObservabilityPlane operations.
type Client interface {
	ListClusterObservabilityPlanes(ctx context.Context, params *gen.ListClusterObservabilityPlanesParams) (*gen.ClusterObservabilityPlaneList, error)
	GetClusterObservabilityPlane(ctx context.Context, clusterObservabilityPlaneName string) (*gen.ClusterObservabilityPlane, error)
	DeleteClusterObservabilityPlane(ctx context.Context, clusterObservabilityPlaneName string) error
}

// ClusterObservabilityPlane implements cluster observability plane operations
type ClusterObservabilityPlane struct {
	client Client
}

// New creates a new cluster observability plane implementation
func New(client Client) *ClusterObservabilityPlane {
	return &ClusterObservabilityPlane{client: client}
}

// List lists all cluster-scoped observability planes
func (c *ClusterObservabilityPlane) List() error {
	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ClusterObservabilityPlane, string, error) {
		p := &gen.ListClusterObservabilityPlanesParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := c.client.ListClusterObservabilityPlanes(ctx, p)
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

// Get retrieves a single cluster observability plane and outputs it as YAML
func (c *ClusterObservabilityPlane) Get(params GetParams) error {
	ctx := context.Background()

	result, err := c.client.GetClusterObservabilityPlane(ctx, params.ClusterObservabilityPlaneName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster observability plane to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single cluster observability plane
func (c *ClusterObservabilityPlane) Delete(params DeleteParams) error {
	ctx := context.Background()

	if err := c.client.DeleteClusterObservabilityPlane(ctx, params.ClusterObservabilityPlaneName); err != nil {
		return err
	}

	fmt.Printf("ClusterObservabilityPlane '%s' deleted\n", params.ClusterObservabilityPlaneName)
	return nil
}

func printList(items []gen.ClusterObservabilityPlane) error {
	if len(items) == 0 {
		fmt.Println("No cluster observability planes found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, cop := range items {
		age := ""
		if cop.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*cop.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			cop.Metadata.Name,
			age)
	}

	return w.Flush()
}

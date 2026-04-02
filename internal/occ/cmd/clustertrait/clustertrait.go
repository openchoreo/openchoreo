// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

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

// Client defines the client methods used by ClusterTrait operations.
type Client interface {
	ListClusterTraits(ctx context.Context, params *gen.ListClusterTraitsParams) (*gen.ClusterTraitList, error)
	GetClusterTrait(ctx context.Context, clusterTraitName string) (*gen.ClusterTrait, error)
	DeleteClusterTrait(ctx context.Context, clusterTraitName string) error
}

// ClusterTrait implements cluster trait operations
type ClusterTrait struct {
	client Client
}

// New creates a new cluster trait implementation
func New(client Client) *ClusterTrait {
	return &ClusterTrait{client: client}
}

// List lists all cluster-scoped traits
func (c *ClusterTrait) List() error {
	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ClusterTrait, string, error) {
		p := &gen.ListClusterTraitsParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := c.client.ListClusterTraits(ctx, p)
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

// Get retrieves a single cluster trait and outputs it as YAML
func (c *ClusterTrait) Get(params GetParams) error {
	ctx := context.Background()

	result, err := c.client.GetClusterTrait(ctx, params.ClusterTraitName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster trait to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single cluster trait
func (c *ClusterTrait) Delete(params DeleteParams) error {
	ctx := context.Background()

	if err := c.client.DeleteClusterTrait(ctx, params.ClusterTraitName); err != nil {
		return err
	}

	fmt.Printf("ClusterTrait '%s' deleted\n", params.ClusterTraitName)
	return nil
}

func printList(items []gen.ClusterTrait) error {
	if len(items) == 0 {
		fmt.Println("No cluster traits found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, trait := range items {
		age := ""
		if trait.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*trait.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			trait.Metadata.Name,
			age)
	}

	return w.Flush()
}

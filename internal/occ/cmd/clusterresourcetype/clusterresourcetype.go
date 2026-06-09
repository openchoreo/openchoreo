// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterresourcetype

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

// ClusterResourceType implements cluster resource type operations
type ClusterResourceType struct {
	client client.Interface
}

// New creates a new cluster resource type implementation
func New(c client.Interface) *ClusterResourceType {
	return &ClusterResourceType{client: c}
}

// List lists all cluster-scoped resource types
func (c *ClusterResourceType) List() error {
	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ClusterResourceType, string, error) {
		p := &gen.ListClusterResourceTypesParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := c.client.ListClusterResourceTypes(ctx, p)
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

// Get retrieves a single cluster resource type and outputs it as YAML
func (c *ClusterResourceType) Get(params GetParams) error {
	ctx := context.Background()

	result, err := c.client.GetClusterResourceType(ctx, params.ClusterResourceTypeName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster resource type to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single cluster resource type
func (c *ClusterResourceType) Delete(params DeleteParams) error {
	ctx := context.Background()

	if err := c.client.DeleteClusterResourceType(ctx, params.ClusterResourceTypeName); err != nil {
		return err
	}

	fmt.Printf("ClusterResourceType '%s' deleted\n", params.ClusterResourceTypeName)
	return nil
}

func printList(items []gen.ClusterResourceType) error {
	if len(items) == 0 {
		fmt.Println("No cluster resource types found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tRETAIN POLICY\tAGE")

	for _, rt := range items {
		retainPolicy := ""
		if rt.Spec != nil && rt.Spec.RetainPolicy != nil {
			retainPolicy = string(*rt.Spec.RetainPolicy)
		}
		age := ""
		if rt.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*rt.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			rt.Metadata.Name,
			retainPolicy,
			age)
	}

	return w.Flush()
}

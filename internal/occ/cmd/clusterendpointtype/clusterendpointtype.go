// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterendpointtype

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ClusterEndpointType implements cluster endpoint type operations
type ClusterEndpointType struct {
	client client.Interface
}

// New creates a new cluster endpoint type implementation
func New(c client.Interface) *ClusterEndpointType {
	return &ClusterEndpointType{client: c}
}

// List lists all cluster-scoped endpoint types
func (c *ClusterEndpointType) List() error {
	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ClusterEndpointType, string, error) {
		p := &gen.ListClusterEndpointTypesParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := c.client.ListClusterEndpointTypes(ctx, p)
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

// Get retrieves a single cluster endpoint type and outputs it as YAML
func (c *ClusterEndpointType) Get(params GetParams) error {
	ctx := context.Background()

	result, err := c.client.GetClusterEndpointType(ctx, params.ClusterEndpointTypeName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster endpoint type to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single cluster endpoint type
func (c *ClusterEndpointType) Delete(params DeleteParams) error {
	ctx := context.Background()

	if err := c.client.DeleteClusterEndpointType(ctx, params.ClusterEndpointTypeName); err != nil {
		return err
	}

	fmt.Printf("ClusterEndpointType '%s' deleted\n", params.ClusterEndpointTypeName)
	return nil
}

func printList(items []gen.ClusterEndpointType) error {
	if len(items) == 0 {
		fmt.Println("No cluster endpoint types found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tRESOURCES\tAGE")

	for _, et := range items {
		ids := make([]string, 0)
		if et.Spec != nil {
			for _, r := range et.Spec.Resources {
				ids = append(ids, r.Id)
			}
		}
		age := ""
		if et.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*et.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			et.Metadata.Name,
			strings.Join(ids, ","),
			age)
	}

	return w.Flush()
}

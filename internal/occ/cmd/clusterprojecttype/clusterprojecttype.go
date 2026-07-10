// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterprojecttype

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ClusterProjectType implements cluster project type operations
type ClusterProjectType struct {
	client client.Interface
}

// New creates a new cluster project type implementation
func New(c client.Interface) *ClusterProjectType {
	return &ClusterProjectType{client: c}
}

// List lists all cluster-scoped project types
func (c *ClusterProjectType) List() error {
	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ClusterProjectType, string, error) {
		p := &gen.ListClusterProjectTypesParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := c.client.ListClusterProjectTypes(ctx, p)
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

// Get retrieves a single cluster project type and outputs it as YAML
func (c *ClusterProjectType) Get(params GetParams) error {
	ctx := context.Background()

	result, err := c.client.GetClusterProjectType(ctx, params.ClusterProjectTypeName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster project type to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single cluster project type
func (c *ClusterProjectType) Delete(params DeleteParams) error {
	ctx := context.Background()

	if err := c.client.DeleteClusterProjectType(ctx, params.ClusterProjectTypeName); err != nil {
		return err
	}

	fmt.Printf("ClusterProjectType '%s' deleted\n", params.ClusterProjectTypeName)
	return nil
}

func printList(items []gen.ClusterProjectType) error {
	if len(items) == 0 {
		fmt.Println("No cluster project types found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tRESOURCES\tAGE")

	for _, pt := range items {
		resources := "0"
		if pt.Spec != nil {
			resources = strconv.Itoa(len(pt.Spec.Resources))
		}
		age := ""
		if pt.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*pt.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			pt.Metadata.Name,
			resources,
			age)
	}

	return w.Flush()
}

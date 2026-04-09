// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterauthzrolebinding

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

// ClusterAuthzRoleBinding implements authz cluster role binding operations
type ClusterAuthzRoleBinding struct {
	client client.Interface
}

// New creates a new authz cluster role binding implementation
func New(c client.Interface) *ClusterAuthzRoleBinding {
	return &ClusterAuthzRoleBinding{client: c}
}

// List lists all cluster-scoped role bindings
func (c *ClusterAuthzRoleBinding) List() error {
	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ClusterAuthzRoleBinding, string, error) {
		p := &gen.ListClusterRoleBindingsParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := c.client.ListClusterRoleBindings(ctx, p)
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
		return fmt.Errorf("failed to list authz cluster role bindings: %w", err)
	}
	return printList(items)
}

// Get retrieves a single authz cluster role binding and outputs it as YAML
func (c *ClusterAuthzRoleBinding) Get(params GetParams) error {
	ctx := context.Background()

	result, err := c.client.GetClusterRoleBinding(ctx, params.Name)
	if err != nil {
		return fmt.Errorf("failed to get authz cluster role binding: %w", err)
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal authz cluster role binding to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single authz cluster role binding
func (c *ClusterAuthzRoleBinding) Delete(params DeleteParams) error {
	ctx := context.Background()

	if err := c.client.DeleteClusterRoleBinding(ctx, params.Name); err != nil {
		return fmt.Errorf("failed to delete authz cluster role binding: %w", err)
	}

	fmt.Printf("Authz cluster role binding '%s' deleted\n", params.Name)
	return nil
}

func printList(items []gen.ClusterAuthzRoleBinding) error {
	if len(items) == 0 {
		fmt.Println("No authz cluster role bindings found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, crb := range items {
		age := ""
		if crb.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*crb.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			crb.Metadata.Name,
			age)
	}

	return w.Flush()
}

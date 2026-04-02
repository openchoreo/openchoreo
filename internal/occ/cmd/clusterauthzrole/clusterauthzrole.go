// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterauthzrole

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

// Client defines the client methods used by ClusterAuthzRole operations.
type Client interface {
	ListClusterRoles(ctx context.Context, params *gen.ListClusterRolesParams) (*gen.ClusterAuthzRoleList, error)
	GetClusterRole(ctx context.Context, name string) (*gen.ClusterAuthzRole, error)
	DeleteClusterRole(ctx context.Context, name string) error
}

// ClusterAuthzRole implements authz cluster role operations
type ClusterAuthzRole struct {
	client Client
}

// New creates a new authz cluster role implementation
func New(client Client) *ClusterAuthzRole {
	return &ClusterAuthzRole{client: client}
}

// List lists all cluster-scoped authorization roles
func (c *ClusterAuthzRole) List() error {
	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ClusterAuthzRole, string, error) {
		p := &gen.ListClusterRolesParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := c.client.ListClusterRoles(ctx, p)
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
		return fmt.Errorf("failed to list authz cluster roles: %w", err)
	}
	return printList(items)
}

// Get retrieves a single authz cluster role and outputs it as YAML
func (c *ClusterAuthzRole) Get(params GetParams) error {
	ctx := context.Background()

	result, err := c.client.GetClusterRole(ctx, params.Name)
	if err != nil {
		return fmt.Errorf("failed to get authz cluster role: %w", err)
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal authz cluster role to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single authz cluster role
func (c *ClusterAuthzRole) Delete(params DeleteParams) error {
	ctx := context.Background()

	if err := c.client.DeleteClusterRole(ctx, params.Name); err != nil {
		return fmt.Errorf("failed to delete authz cluster role: %w", err)
	}

	fmt.Printf("Authz cluster role '%s' deleted\n", params.Name)
	return nil
}

func printList(items []gen.ClusterAuthzRole) error {
	if len(items) == 0 {
		fmt.Println("No authz cluster roles found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tDESCRIPTION\tAGE")

	for _, cr := range items {
		description := ""
		if cr.Spec != nil && cr.Spec.Description != nil {
			description = *cr.Spec.Description
		}
		age := ""
		if cr.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*cr.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			cr.Metadata.Name,
			description,
			age)
	}

	return w.Flush()
}

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authzclusterrolebinding

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

// AuthzClusterRoleBinding implements authz cluster role binding operations
type AuthzClusterRoleBinding struct{}

// New creates a new authz cluster role binding implementation
func New() *AuthzClusterRoleBinding {
	return &AuthzClusterRoleBinding{}
}

// List lists all cluster-scoped role bindings
func (c *AuthzClusterRoleBinding) List() error {
	ctx := context.Background()

	cl, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.AuthzClusterRoleBinding, string, error) {
		p := &gen.ListClusterRoleBindingsParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := cl.ListClusterRoleBindings(ctx, p)
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
func (c *AuthzClusterRoleBinding) Get(params GetParams) error {
	ctx := context.Background()

	cl, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := cl.GetClusterRoleBinding(ctx, params.Name)
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
func (c *AuthzClusterRoleBinding) Delete(params DeleteParams) error {
	ctx := context.Background()

	cl, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	if err := cl.DeleteClusterRoleBinding(ctx, params.Name); err != nil {
		return fmt.Errorf("failed to delete authz cluster role binding: %w", err)
	}

	fmt.Printf("Authz cluster role binding '%s' deleted\n", params.Name)
	return nil
}

func printList(items []gen.AuthzClusterRoleBinding) error {
	if len(items) == 0 {
		fmt.Println("No authz cluster role bindings found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tROLE\tAGE")

	for _, crb := range items {
		roleRef := ""
		if crb.Spec != nil {
			roleRef = string(crb.Spec.RoleRef.Kind) + "/" + crb.Spec.RoleRef.Name
		}
		age := ""
		if crb.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*crb.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			crb.Metadata.Name,
			roleRef,
			age)
	}

	return w.Flush()
}

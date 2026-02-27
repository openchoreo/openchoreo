// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authzrole

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// AuthzRole implements authz role operations
type AuthzRole struct{}

// New creates a new authz role implementation
func New() *AuthzRole {
	return &AuthzRole{}
}

// List lists all authz roles in a namespace
func (r *AuthzRole) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceAuthzRole, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.AuthzRole, string, error) {
		p := &gen.ListNamespaceRolesParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := c.ListNamespaceRoles(ctx, params.Namespace, p)
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
		return fmt.Errorf("failed to list authz roles: %w", err)
	}
	return printList(items)
}

// Get retrieves a single authz role and outputs it as YAML
func (r *AuthzRole) Get(params GetParams) error {
	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceAuthzRole, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.GetNamespaceRole(ctx, params.Namespace, params.Name)
	if err != nil {
		return fmt.Errorf("failed to get authz role: %w", err)
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal authz role to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single authz role
func (r *AuthzRole) Delete(params DeleteParams) error {
	if err := validation.ValidateParams(validation.CmdDelete, validation.ResourceAuthzRole, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	if err := c.DeleteNamespaceRole(ctx, params.Namespace, params.Name); err != nil {
		return fmt.Errorf("failed to delete authz role: %w", err)
	}

	fmt.Printf("Authz role '%s' deleted\n", params.Name)
	return nil
}

func printList(items []gen.AuthzRole) error {
	if len(items) == 0 {
		fmt.Println("No authz roles found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tDESCRIPTION\tAGE")

	for _, r := range items {
		description := ""
		if r.Spec != nil && r.Spec.Description != nil {
			description = *r.Spec.Description
		}
		age := ""
		if r.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*r.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			r.Metadata.Name,
			description,
			age)
	}

	return w.Flush()
}

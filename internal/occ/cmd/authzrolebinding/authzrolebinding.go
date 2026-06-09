// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authzrolebinding

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// AuthzRoleBinding implements authz role binding operations
type AuthzRoleBinding struct {
	client client.Interface
}

// New creates a new authz role binding implementation
func New(c client.Interface) *AuthzRoleBinding {
	return &AuthzRoleBinding{client: c}
}

// List lists all authz role bindings in a namespace
func (r *AuthzRoleBinding) List(params ListParams) error {
	if err := cmdutil.RequireFields("list", "authzrolebinding", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.AuthzRoleBinding, string, error) {
		p := &gen.ListNamespaceRoleBindingsParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := r.client.ListNamespaceRoleBindings(ctx, params.Namespace, p)
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
		return fmt.Errorf("failed to list authz role bindings: %w", err)
	}
	return printList(items)
}

// Get retrieves a single authz role binding and outputs it as YAML
func (r *AuthzRoleBinding) Get(params GetParams) error {
	if err := cmdutil.RequireFields("get", "authzrolebinding", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	result, err := r.client.GetNamespaceRoleBinding(ctx, params.Namespace, params.Name)
	if err != nil {
		return fmt.Errorf("failed to get authz role binding: %w", err)
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal authz role binding to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single authz role binding
func (r *AuthzRoleBinding) Delete(params DeleteParams) error {
	if err := cmdutil.RequireFields("delete", "authzrolebinding", map[string]string{"namespace": params.Namespace, "name": params.Name}); err != nil {
		return err
	}

	ctx := context.Background()

	if err := r.client.DeleteNamespaceRoleBinding(ctx, params.Namespace, params.Name); err != nil {
		return fmt.Errorf("failed to delete authz role binding: %w", err)
	}

	fmt.Printf("Authz role binding '%s' deleted\n", params.Name)
	return nil
}

func printList(items []gen.AuthzRoleBinding) error {
	if len(items) == 0 {
		fmt.Println("No authz role bindings found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, rb := range items {
		age := ""
		if rb.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*rb.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			rb.Metadata.Name,
			age)
	}

	return w.Flush()
}

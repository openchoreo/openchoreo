// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authzclusterrole

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// AuthzClusterRole implements authz cluster role operations
type AuthzClusterRole struct{}

// New creates a new authz cluster role implementation
func New() *AuthzClusterRole {
	return &AuthzClusterRole{}
}

// List lists all cluster-scoped authorization roles
func (c *AuthzClusterRole) List() error {
	ctx := context.Background()

	cl, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := cl.ListClusterRoles(ctx)
	if err != nil {
		return fmt.Errorf("failed to list authz cluster roles: %w", err)
	}

	return printList(result)
}

// Get retrieves a single authz cluster role and outputs it as YAML
func (c *AuthzClusterRole) Get(params GetParams) error {
	ctx := context.Background()

	cl, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := cl.GetClusterRole(ctx, params.Name)
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
func (c *AuthzClusterRole) Delete(params DeleteParams) error {
	ctx := context.Background()

	cl, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	if err := cl.DeleteClusterRole(ctx, params.Name); err != nil {
		return fmt.Errorf("failed to delete authz cluster role: %w", err)
	}

	fmt.Printf("Authz cluster role '%s' deleted\n", params.Name)
	return nil
}

func printList(list *gen.AuthzClusterRoleList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No authz cluster roles found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tDESCRIPTION\tAGE")

	for _, cr := range list.Items {
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

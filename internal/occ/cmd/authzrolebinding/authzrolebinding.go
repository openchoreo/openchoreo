// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authzrolebinding

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// AuthzRoleBinding implements authz role binding operations
type AuthzRoleBinding struct{}

// New creates a new authz role binding implementation
func New() *AuthzRoleBinding {
	return &AuthzRoleBinding{}
}

// List lists all authz role bindings in a namespace
func (r *AuthzRoleBinding) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceAuthzRoleBinding, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListNamespaceRoleBindings(ctx, params.Namespace)
	if err != nil {
		return fmt.Errorf("failed to list authz role bindings: %w", err)
	}

	return printList(result)
}

// Get retrieves a single authz role binding and outputs it as YAML
func (r *AuthzRoleBinding) Get(params GetParams) error {
	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceAuthzRoleBinding, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.GetNamespaceRoleBinding(ctx, params.Namespace, params.Name)
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
	if err := validation.ValidateParams(validation.CmdDelete, validation.ResourceAuthzRoleBinding, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	if err := c.DeleteNamespaceRoleBinding(ctx, params.Namespace, params.Name); err != nil {
		return fmt.Errorf("failed to delete authz role binding: %w", err)
	}

	fmt.Printf("Authz role binding '%s' deleted\n", params.Name)
	return nil
}

func printList(list *gen.AuthzRoleBindingList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No authz role bindings found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tROLE\tAGE")

	for _, rb := range list.Items {
		roleRef := ""
		if rb.Spec != nil {
			roleRef = string(rb.Spec.RoleRef.Kind) + "/" + rb.Spec.RoleRef.Name
		}
		age := ""
		if rb.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*rb.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			rb.Metadata.Name,
			roleRef,
			age)
	}

	return w.Flush()
}

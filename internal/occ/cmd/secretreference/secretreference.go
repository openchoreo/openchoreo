// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secretreference

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

// SecretReference implements secret reference operations
type SecretReference struct{}

// New creates a new secret reference implementation
func New() *SecretReference {
	return &SecretReference{}
}

// List lists all secret references in a namespace
func (s *SecretReference) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceSecretReference, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.SecretReference, string, error) {
		p := &gen.ListSecretReferencesParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := c.ListSecretReferences(ctx, params.Namespace, p)
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

// Get retrieves a single secret reference and outputs it as YAML
func (s *SecretReference) Get(params GetParams) error {
	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceSecretReference, params); err != nil {
		return err
	}

	ctx := context.Background()
	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.GetSecretReference(ctx, params.Namespace, params.SecretReferenceName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal secret reference to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single secret reference
func (s *SecretReference) Delete(params DeleteParams) error {
	if err := validation.ValidateParams(validation.CmdDelete, validation.ResourceSecretReference, params); err != nil {
		return err
	}

	ctx := context.Background()
	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	if err := c.DeleteSecretReference(ctx, params.Namespace, params.SecretReferenceName); err != nil {
		return err
	}

	fmt.Printf("SecretReference '%s' deleted\n", params.SecretReferenceName)
	return nil
}

func printList(items []gen.SecretReference) error {
	if len(items) == 0 {
		fmt.Println("No secret references found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, sr := range items {
		age := "<unknown>"
		if sr.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*sr.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			sr.Metadata.Name,
			age)
	}

	return w.Flush()
}

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package trait

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// Client defines the client methods used by Trait operations.
type Client interface {
	ListTraits(ctx context.Context, namespaceName string, params *gen.ListTraitsParams) (*gen.TraitList, error)
	GetTrait(ctx context.Context, namespaceName string, traitName string) (*gen.Trait, error)
	DeleteTrait(ctx context.Context, namespaceName string, traitName string) error
}

// Trait implements trait operations
type Trait struct {
	client Client
}

// New creates a new trait implementation
func New(client Client) *Trait {
	return &Trait{client: client}
}

// List lists all traits in a namespace
func (t *Trait) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceTrait, params); err != nil {
		return err
	}

	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.Trait, string, error) {
		p := &gen.ListTraitsParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := t.client.ListTraits(ctx, params.Namespace, p)
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

// Get retrieves a single trait and outputs it as YAML
func (t *Trait) Get(params GetParams) error {
	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceTrait, params); err != nil {
		return err
	}

	ctx := context.Background()

	result, err := t.client.GetTrait(ctx, params.Namespace, params.TraitName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal trait to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single trait
func (t *Trait) Delete(params DeleteParams) error {
	if err := validation.ValidateParams(validation.CmdDelete, validation.ResourceTrait, params); err != nil {
		return err
	}

	ctx := context.Background()

	if err := t.client.DeleteTrait(ctx, params.Namespace, params.TraitName); err != nil {
		return err
	}

	fmt.Printf("Trait '%s' deleted\n", params.TraitName)
	return nil
}

func printList(items []gen.Trait) error {
	if len(items) == 0 {
		fmt.Println("No traits found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, trait := range items {
		age := ""
		if trait.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*trait.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			trait.Metadata.Name,
			age)
	}

	return w.Flush()
}

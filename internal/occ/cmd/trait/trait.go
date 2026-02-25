// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package trait

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// Trait implements trait operations
type Trait struct{}

// New creates a new trait implementation
func New() *Trait {
	return &Trait{}
}

// List lists all traits in a namespace
func (t *Trait) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceTrait, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListTraits(ctx, params.Namespace)
	if err != nil {
		return fmt.Errorf("failed to list traits: %w", err)
	}

	return printList(result)
}

func printList(list *gen.TraitList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No traits found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, trait := range list.Items {
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

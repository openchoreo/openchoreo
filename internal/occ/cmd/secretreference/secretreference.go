// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secretreference

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

	result, err := c.ListSecretReferences(ctx, params.Namespace)
	if err != nil {
		return fmt.Errorf("failed to list secret references: %w", err)
	}

	return printList(result)
}

func printList(list *gen.SecretReferenceList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No secret references found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, sr := range list.Items {
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

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package namespace

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// Namespace implements namespace operations
type Namespace struct{}

// New creates a new namespace implementation
func New() *Namespace {
	return &Namespace{}
}

// List lists all namespaces
func (n *Namespace) List() error {
	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListNamespaces(ctx, &gen.ListNamespacesParams{})
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}

	return printList(result)
}

func printList(list *gen.NamespaceList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No namespaces found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, ns := range list.Items {
		age := ""
		if ns.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*ns.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			ns.Metadata.Name,
			age)
	}

	return w.Flush()
}

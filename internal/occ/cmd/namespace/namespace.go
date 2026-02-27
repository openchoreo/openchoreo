// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package namespace

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

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.Namespace, string, error) {
		p := &gen.ListNamespacesParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := c.ListNamespaces(ctx, p)
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

// Get retrieves a single namespace and outputs it as YAML
func (n *Namespace) Get(name string) error {
	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.GetNamespace(ctx, name)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal namespace to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single namespace
func (n *Namespace) Delete(name string) error {
	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	if err := c.DeleteNamespace(ctx, name); err != nil {
		return err
	}

	fmt.Printf("Namespace '%s' deleted\n", name)
	return nil
}

func printList(items []gen.Namespace) error {
	if len(items) == 0 {
		fmt.Println("No namespaces found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, ns := range items {
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

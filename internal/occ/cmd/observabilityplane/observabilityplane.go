// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityplane

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

// ObservabilityPlane implements observability plane operations
type ObservabilityPlane struct{}

// New creates a new observability plane implementation
func New() *ObservabilityPlane {
	return &ObservabilityPlane{}
}

// List lists all observability planes in a namespace
func (o *ObservabilityPlane) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceObservabilityPlane, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ObservabilityPlane, string, error) {
		p := &gen.ListObservabilityPlanesParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := c.ListObservabilityPlanes(ctx, params.Namespace, p)
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

// Get retrieves a single observability plane and outputs it as YAML
func (o *ObservabilityPlane) Get(params GetParams) error {
	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceObservabilityPlane, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.GetObservabilityPlane(ctx, params.Namespace, params.ObservabilityPlaneName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal observability plane to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single observability plane
func (o *ObservabilityPlane) Delete(params DeleteParams) error {
	if err := validation.ValidateParams(validation.CmdDelete, validation.ResourceObservabilityPlane, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	if err := c.DeleteObservabilityPlane(ctx, params.Namespace, params.ObservabilityPlaneName); err != nil {
		return err
	}

	fmt.Printf("ObservabilityPlane '%s' deleted\n", params.ObservabilityPlaneName)
	return nil
}

func printList(items []gen.ObservabilityPlane) error {
	if len(items) == 0 {
		fmt.Println("No observability planes found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, op := range items {
		age := ""
		if op.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*op.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			op.Metadata.Name,
			age)
	}

	return w.Flush()
}

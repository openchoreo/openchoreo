// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

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

// DataPlane implements data plane operations
type DataPlane struct {
	client client.Interface
}

// New creates a new data plane implementation
func New(c client.Interface) *DataPlane {
	return &DataPlane{client: c}
}

// List lists all data planes in a namespace
func (d *DataPlane) List(params ListParams) error {
	if err := cmdutil.RequireFields("list", "dataplane", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.DataPlane, string, error) {
		p := &gen.ListDataPlanesParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := d.client.ListDataPlanes(ctx, params.Namespace, p)
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

// Get retrieves a single data plane and outputs it as YAML
func (d *DataPlane) Get(params GetParams) error {
	if err := cmdutil.RequireFields("get", "dataplane", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	result, err := d.client.GetDataPlane(ctx, params.Namespace, params.DataPlaneName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal data plane to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single data plane
func (d *DataPlane) Delete(params DeleteParams) error {
	if err := cmdutil.RequireFields("delete", "dataplane", map[string]string{"namespace": params.Namespace, "name": params.DataPlaneName}); err != nil {
		return err
	}

	ctx := context.Background()

	if err := d.client.DeleteDataPlane(ctx, params.Namespace, params.DataPlaneName); err != nil {
		return err
	}

	fmt.Printf("DataPlane '%s' deleted\n", params.DataPlaneName)
	return nil
}

func printList(items []gen.DataPlane) error {
	if len(items) == 0 {
		fmt.Println("No data planes found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, dp := range items {
		age := ""
		if dp.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*dp.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			dp.Metadata.Name,
			age)
	}

	return w.Flush()
}

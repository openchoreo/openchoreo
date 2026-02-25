// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

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

// DataPlane implements data plane operations
type DataPlane struct{}

// New creates a new data plane implementation
func New() *DataPlane {
	return &DataPlane{}
}

// List lists all data planes in a namespace
func (d *DataPlane) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceDataPlane, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListDataPlanes(ctx, params.Namespace, &gen.ListDataPlanesParams{})
	if err != nil {
		return fmt.Errorf("failed to list data planes: %w", err)
	}

	return printList(result)
}

// Get retrieves a single data plane and outputs it as YAML
func (d *DataPlane) Get(params GetParams) error {
	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceDataPlane, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.GetDataPlane(ctx, params.Namespace, params.DataPlaneName)
	if err != nil {
		return fmt.Errorf("failed to get data plane: %w", err)
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
	if err := validation.ValidateParams(validation.CmdDelete, validation.ResourceDataPlane, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	if err := c.DeleteDataPlane(ctx, params.Namespace, params.DataPlaneName); err != nil {
		return fmt.Errorf("failed to delete data plane: %w", err)
	}

	fmt.Printf("DataPlane '%s' deleted\n", params.DataPlaneName)
	return nil
}

func printList(list *gen.DataPlaneList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No data planes found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, dp := range list.Items {
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

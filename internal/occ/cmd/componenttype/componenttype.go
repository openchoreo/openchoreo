// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttype

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

// ComponentType implements component type operations
type ComponentType struct{}

// New creates a new component type implementation
func New() *ComponentType {
	return &ComponentType{}
}

// List lists all component types in a namespace
func (ct *ComponentType) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceComponentType, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListComponentTypes(ctx, params.Namespace)
	if err != nil {
		return fmt.Errorf("failed to list component types: %w", err)
	}

	return printList(result)
}

func printList(list *gen.ComponentTypeList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No component types found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tWORKLOAD TYPE\tAGE")

	for _, ct := range list.Items {
		workloadType := ""
		if ct.Spec != nil {
			workloadType = string(ct.Spec.WorkloadType)
		}
		age := ""
		if ct.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*ct.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			ct.Metadata.Name,
			workloadType,
			age)
	}

	return w.Flush()
}

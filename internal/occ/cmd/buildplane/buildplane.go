// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package buildplane

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

// BuildPlane implements build plane operations
type BuildPlane struct{}

// New creates a new build plane implementation
func New() *BuildPlane {
	return &BuildPlane{}
}

// List lists all build planes in a namespace
func (b *BuildPlane) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceBuildPlane, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListBuildPlanes(ctx, params.Namespace)
	if err != nil {
		return fmt.Errorf("failed to list build planes: %w", err)
	}

	return printList(result)
}

func printList(list *gen.BuildPlaneList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No build planes found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, bp := range list.Items {
		age := ""
		if bp.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*bp.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			bp.Metadata.Name,
			age)
	}

	return w.Flush()
}

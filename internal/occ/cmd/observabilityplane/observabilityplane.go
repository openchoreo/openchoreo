// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityplane

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

	result, err := c.ListObservabilityPlanes(ctx, params.Namespace)
	if err != nil {
		return fmt.Errorf("failed to list observability planes: %w", err)
	}

	return printList(result)
}

func printList(list *gen.ObservabilityPlaneList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No observability planes found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, op := range list.Items {
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

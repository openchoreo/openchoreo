// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustercomponenttype

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ClusterComponentType implements cluster component type operations
type ClusterComponentType struct{}

// New creates a new cluster component type implementation
func New() *ClusterComponentType {
	return &ClusterComponentType{}
}

// List lists all cluster-scoped component types
func (c *ClusterComponentType) List() error {
	ctx := context.Background()

	cl, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := cl.ListClusterComponentTypes(ctx)
	if err != nil {
		return fmt.Errorf("failed to list cluster component types: %w", err)
	}

	return printList(result)
}

func printList(list *gen.ClusterComponentTypeList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No cluster component types found")
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

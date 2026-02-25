// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ClusterTrait implements cluster trait operations
type ClusterTrait struct{}

// New creates a new cluster trait implementation
func New() *ClusterTrait {
	return &ClusterTrait{}
}

// List lists all cluster-scoped traits
func (c *ClusterTrait) List() error {
	ctx := context.Background()

	cl, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := cl.ListClusterTraits(ctx)
	if err != nil {
		return err
	}

	return printList(result)
}

// Get retrieves a single cluster trait and outputs it as YAML
func (c *ClusterTrait) Get(params GetParams) error {
	ctx := context.Background()

	cl, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := cl.GetClusterTrait(ctx, params.ClusterTraitName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster trait to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single cluster trait
func (c *ClusterTrait) Delete(params DeleteParams) error {
	ctx := context.Background()

	cl, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	if err := cl.DeleteClusterTrait(ctx, params.ClusterTraitName); err != nil {
		return err
	}

	fmt.Printf("ClusterTrait '%s' deleted\n", params.ClusterTraitName)
	return nil
}

func printList(list *gen.ClusterTraitList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No cluster traits found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, trait := range list.Items {
		age := ""
		if trait.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*trait.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			trait.Metadata.Name,
			age)
	}

	return w.Flush()
}

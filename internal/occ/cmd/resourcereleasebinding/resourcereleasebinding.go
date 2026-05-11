// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

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

// ResourceReleaseBinding implements resource release binding operations
type ResourceReleaseBinding struct {
	client client.Interface
}

// New creates a new resource release binding implementation
func New(c client.Interface) *ResourceReleaseBinding {
	return &ResourceReleaseBinding{client: c}
}

// List lists resource release bindings in a namespace, optionally filtered by resource
func (rrb *ResourceReleaseBinding) List(params ListParams) error {
	if err := cmdutil.RequireFields("list", "resourcereleasebinding", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ResourceReleaseBinding, string, error) {
		p := &gen.ListResourceReleaseBindingsParams{}
		if params.Resource != "" {
			p.Resource = &params.Resource
		}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := rrb.client.ListResourceReleaseBindings(ctx, params.Namespace, p)
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

// Get retrieves a single resource release binding and outputs it as YAML
func (rrb *ResourceReleaseBinding) Get(params GetParams) error {
	if err := cmdutil.RequireFields("get", "resourcereleasebinding", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	result, err := rrb.client.GetResourceReleaseBinding(ctx, params.Namespace, params.ResourceReleaseBindingName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal resource release binding to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single resource release binding
func (rrb *ResourceReleaseBinding) Delete(params DeleteParams) error {
	if err := cmdutil.RequireFields("delete", "resourcereleasebinding", map[string]string{"namespace": params.Namespace, "name": params.ResourceReleaseBindingName}); err != nil {
		return err
	}

	ctx := context.Background()

	if err := rrb.client.DeleteResourceReleaseBinding(ctx, params.Namespace, params.ResourceReleaseBindingName); err != nil {
		return err
	}

	fmt.Printf("ResourceReleaseBinding '%s' deleted\n", params.ResourceReleaseBindingName)
	return nil
}

func printList(items []gen.ResourceReleaseBinding) error {
	if len(items) == 0 {
		fmt.Println("No resource release bindings found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tRESOURCE\tENVIRONMENT\tRELEASE\tSTATUS\tAGE")

	for _, b := range items {
		resourceName := ""
		env := ""
		release := ""
		if b.Spec != nil {
			resourceName = b.Spec.Owner.ResourceName
			env = b.Spec.Environment
			if b.Spec.ResourceRelease != nil {
				release = *b.Spec.ResourceRelease
			}
		}
		status := ""
		if b.Status != nil && b.Status.Conditions != nil {
			for _, c := range *b.Status.Conditions {
				if c.Type == "Ready" {
					status = c.Reason
					break
				}
			}
		}
		age := ""
		if b.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*b.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			b.Metadata.Name,
			resourceName,
			env,
			release,
			status,
			age)
	}

	return w.Flush()
}

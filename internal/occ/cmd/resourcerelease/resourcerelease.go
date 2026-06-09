// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcerelease

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

// ResourceRelease implements resource release operations
type ResourceRelease struct {
	client client.Interface
}

// New creates a new resource release implementation
func New(c client.Interface) *ResourceRelease {
	return &ResourceRelease{client: c}
}

// List lists resource releases in a namespace, optionally filtered by resource
func (rr *ResourceRelease) List(params ListParams) error {
	if err := cmdutil.RequireFields("list", "resourcerelease", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ResourceRelease, string, error) {
		p := &gen.ListResourceReleasesParams{}
		if params.Resource != "" {
			p.Resource = &params.Resource
		}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := rr.client.ListResourceReleases(ctx, params.Namespace, p)
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

// Get retrieves a single resource release and outputs it as YAML
func (rr *ResourceRelease) Get(params GetParams) error {
	if err := cmdutil.RequireFields("get", "resourcerelease", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	result, err := rr.client.GetResourceRelease(ctx, params.Namespace, params.ResourceReleaseName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal resource release to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single resource release
func (rr *ResourceRelease) Delete(params DeleteParams) error {
	if err := cmdutil.RequireFields("delete", "resourcerelease", map[string]string{"namespace": params.Namespace, "name": params.ResourceReleaseName}); err != nil {
		return err
	}

	ctx := context.Background()

	if err := rr.client.DeleteResourceRelease(ctx, params.Namespace, params.ResourceReleaseName); err != nil {
		return err
	}

	fmt.Printf("ResourceRelease '%s' deleted\n", params.ResourceReleaseName)
	return nil
}

func printList(items []gen.ResourceRelease) error {
	if len(items) == 0 {
		fmt.Println("No resource releases found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tRESOURCE\tAGE")

	for _, release := range items {
		resourceName := ""
		if release.Spec != nil {
			resourceName = release.Spec.Owner.ResourceName
		}
		age := ""
		if release.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*release.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			release.Metadata.Name,
			resourceName,
			age)
	}

	return w.Flush()
}

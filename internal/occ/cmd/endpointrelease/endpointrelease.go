// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointrelease

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

// EndpointRelease implements endpoint release operations
type EndpointRelease struct {
	client client.Interface
}

// New creates a new endpoint release implementation
func New(c client.Interface) *EndpointRelease {
	return &EndpointRelease{client: c}
}

// List lists endpoint releases in a namespace, optionally filtered by endpoint
func (er *EndpointRelease) List(params ListParams) error {
	if err := cmdutil.RequireFields("list", "endpointrelease", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.EndpointRelease, string, error) {
		p := &gen.ListEndpointReleasesParams{}
		if params.Endpoint != "" {
			p.Endpoint = &params.Endpoint
		}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := er.client.ListEndpointReleases(ctx, params.Namespace, p)
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

// Get retrieves a single endpoint release and outputs it as YAML
func (er *EndpointRelease) Get(params GetParams) error {
	if err := cmdutil.RequireFields("get", "endpointrelease", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	result, err := er.client.GetEndpointRelease(ctx, params.Namespace, params.EndpointReleaseName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal endpoint release to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single endpoint release
func (er *EndpointRelease) Delete(params DeleteParams) error {
	if err := cmdutil.RequireFields("delete", "endpointrelease", map[string]string{"namespace": params.Namespace, "name": params.EndpointReleaseName}); err != nil {
		return err
	}

	ctx := context.Background()

	if err := er.client.DeleteEndpointRelease(ctx, params.Namespace, params.EndpointReleaseName); err != nil {
		return err
	}

	fmt.Printf("EndpointRelease '%s' deleted\n", params.EndpointReleaseName)
	return nil
}

func printList(items []gen.EndpointRelease) error {
	if len(items) == 0 {
		fmt.Println("No endpoint releases found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tENDPOINT\tAGE")

	for _, release := range items {
		endpointName := ""
		if release.Spec != nil {
			endpointName = release.Spec.Owner.EndpointName
		}
		age := ""
		if release.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*release.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			release.Metadata.Name,
			endpointName,
			age)
	}

	return w.Flush()
}

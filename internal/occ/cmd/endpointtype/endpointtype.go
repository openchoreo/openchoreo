// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointtype

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// EndpointType implements endpoint type operations
type EndpointType struct {
	client client.Interface
}

// New creates a new endpoint type implementation
func New(c client.Interface) *EndpointType {
	return &EndpointType{client: c}
}

// List lists all endpoint types in a namespace
func (et *EndpointType) List(params ListParams) error {
	if err := cmdutil.RequireFields("list", "endpointtype", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.EndpointType, string, error) {
		p := &gen.ListEndpointTypesParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := et.client.ListEndpointTypes(ctx, params.Namespace, p)
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

// Get retrieves a single endpoint type and outputs it as YAML
func (et *EndpointType) Get(params GetParams) error {
	if err := cmdutil.RequireFields("get", "endpointtype", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	result, err := et.client.GetEndpointType(ctx, params.Namespace, params.EndpointTypeName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal endpoint type to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single endpoint type
func (et *EndpointType) Delete(params DeleteParams) error {
	if err := cmdutil.RequireFields("delete", "endpointtype", map[string]string{"namespace": params.Namespace, "name": params.EndpointTypeName}); err != nil {
		return err
	}

	ctx := context.Background()

	if err := et.client.DeleteEndpointType(ctx, params.Namespace, params.EndpointTypeName); err != nil {
		return err
	}

	fmt.Printf("EndpointType '%s' deleted\n", params.EndpointTypeName)
	return nil
}

func printList(items []gen.EndpointType) error {
	if len(items) == 0 {
		fmt.Println("No endpoint types found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tRESOURCES\tAGE")

	for _, et := range items {
		ids := make([]string, 0)
		if et.Spec != nil {
			for _, r := range et.Spec.Resources {
				ids = append(ids, r.Id)
			}
		}
		age := ""
		if et.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*et.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			et.Metadata.Name,
			strings.Join(ids, ","),
			age)
	}

	return w.Flush()
}

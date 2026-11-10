// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

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

// EndpointReleaseBinding implements endpoint release binding operations
type EndpointReleaseBinding struct {
	client client.Interface
}

// New creates a new endpoint release binding implementation
func New(c client.Interface) *EndpointReleaseBinding {
	return &EndpointReleaseBinding{client: c}
}

// List lists endpoint release bindings in a namespace, optionally filtered by endpoint
func (erb *EndpointReleaseBinding) List(params ListParams) error {
	if err := cmdutil.RequireFields("list", "endpointreleasebinding", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.EndpointReleaseBinding, string, error) {
		p := &gen.ListEndpointReleaseBindingsParams{}
		if params.Endpoint != "" {
			p.Endpoint = &params.Endpoint
		}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := erb.client.ListEndpointReleaseBindings(ctx, params.Namespace, p)
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

// Get retrieves a single endpoint release binding and outputs it as YAML
func (erb *EndpointReleaseBinding) Get(params GetParams) error {
	if err := cmdutil.RequireFields("get", "endpointreleasebinding", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	result, err := erb.client.GetEndpointReleaseBinding(ctx, params.Namespace, params.EndpointReleaseBindingName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal endpoint release binding to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single endpoint release binding
func (erb *EndpointReleaseBinding) Delete(params DeleteParams) error {
	if err := cmdutil.RequireFields("delete", "endpointreleasebinding", map[string]string{"namespace": params.Namespace, "name": params.EndpointReleaseBindingName}); err != nil {
		return err
	}

	ctx := context.Background()

	if err := erb.client.DeleteEndpointReleaseBinding(ctx, params.Namespace, params.EndpointReleaseBindingName); err != nil {
		return err
	}

	fmt.Printf("EndpointReleaseBinding '%s' deleted\n", params.EndpointReleaseBindingName)
	return nil
}

func printList(items []gen.EndpointReleaseBinding) error {
	if len(items) == 0 {
		fmt.Println("No endpoint release bindings found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tENDPOINT\tENVIRONMENT\tRELEASE\tSTATUS\tAGE")

	for _, b := range items {
		endpointName := ""
		env := ""
		release := ""
		if b.Spec != nil {
			endpointName = b.Spec.Owner.EndpointName
			env = b.Spec.Environment
			if b.Spec.EndpointRelease != nil {
				release = *b.Spec.EndpointRelease
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
			endpointName,
			env,
			release,
			status,
			age)
	}

	return w.Flush()
}

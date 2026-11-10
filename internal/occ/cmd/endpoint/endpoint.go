// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

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

// Endpoint implements endpoint operations
type Endpoint struct {
	client client.Interface
}

// New creates a new endpoint implementation
func New(c client.Interface) *Endpoint {
	return &Endpoint{client: c}
}

// List lists endpoints in a namespace, optionally filtered by project
func (r *Endpoint) List(params ListParams) error {
	if err := cmdutil.RequireFields("list", "endpoint", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.Endpoint, string, error) {
		p := &gen.ListEndpointsParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		if params.Project != "" {
			p.Project = &params.Project
		}
		result, err := r.client.ListEndpoints(ctx, params.Namespace, p)
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
	return printList(items, params.Project == "")
}

// Get retrieves a single endpoint and outputs it as YAML
func (r *Endpoint) Get(params GetParams) error {
	if err := cmdutil.RequireFields("get", "endpoint", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	result, err := r.client.GetEndpoint(ctx, params.Namespace, params.EndpointName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal endpoint to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single endpoint
func (r *Endpoint) Delete(params DeleteParams) error {
	if err := cmdutil.RequireFields("delete", "endpoint", map[string]string{"namespace": params.Namespace, "name": params.EndpointName}); err != nil {
		return err
	}

	ctx := context.Background()

	if err := r.client.DeleteEndpoint(ctx, params.Namespace, params.EndpointName); err != nil {
		return err
	}

	fmt.Printf("Endpoint '%s' deleted\n", params.EndpointName)
	return nil
}

// Promote advances the EndpointReleaseBinding for the given environment to the
// endpoint's latest release. It is a thin client wrapper: read
// Endpoint.status.latestRelease.name, find the binding for the target environment,
// and PUT it with the new spec.endpointRelease. No dedicated server endpoint.
func (r *Endpoint) Promote(params PromoteParams) error {
	if err := cmdutil.RequireFields("promote", "endpoint", map[string]string{
		"namespace": params.Namespace,
		"name":      params.EndpointName,
		"env":       params.Environment,
	}); err != nil {
		return err
	}

	ctx := context.Background()

	endpoint, err := r.client.GetEndpoint(ctx, params.Namespace, params.EndpointName)
	if err != nil {
		return err
	}
	if endpoint.Status == nil || endpoint.Status.LatestRelease == nil || endpoint.Status.LatestRelease.Name == "" {
		return fmt.Errorf("endpoint %q has no released versions yet", params.EndpointName)
	}
	releaseName := endpoint.Status.LatestRelease.Name

	binding, err := r.findBindingForEnv(ctx, params.Namespace, params.EndpointName, params.Environment)
	if err != nil {
		return err
	}

	binding.Spec.EndpointRelease = &releaseName
	if _, err := r.client.UpdateEndpointReleaseBinding(ctx, params.Namespace, binding.Metadata.Name, *binding); err != nil {
		return err
	}

	fmt.Printf("EndpointReleaseBinding '%s' promoted to endpointrelease '%s'\n", binding.Metadata.Name, releaseName)
	return nil
}

func (r *Endpoint) findBindingForEnv(ctx context.Context, namespace, endpointName, env string) (*gen.EndpointReleaseBinding, error) {
	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.EndpointReleaseBinding, string, error) {
		p := &gen.ListEndpointReleaseBindingsParams{Endpoint: &endpointName}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := r.client.ListEndpointReleaseBindings(ctx, namespace, p)
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
		return nil, err
	}

	for i := range items {
		b := items[i]
		if b.Spec != nil && b.Spec.Environment == env {
			return &b, nil
		}
	}
	return nil, fmt.Errorf("no EndpointReleaseBinding found for endpoint %q in environment %q", endpointName, env)
}

func printList(items []gen.Endpoint, showProject bool) error {
	if len(items) == 0 {
		fmt.Println("No endpoints found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	if showProject {
		fmt.Fprintln(w, "NAME\tTYPE\tVISIBILITY\tPROJECT\tAGE")
	} else {
		fmt.Fprintln(w, "NAME\tTYPE\tVISIBILITY\tAGE")
	}

	for _, ep := range items {
		typeStr := ""
		project := ""
		visibility := make([]string, 0)
		if ep.Spec != nil {
			kind := "EndpointType"
			if ep.Spec.Type.Kind != nil && *ep.Spec.Type.Kind != "" {
				kind = string(*ep.Spec.Type.Kind)
			}
			typeStr = fmt.Sprintf("%s/%s", kind, ep.Spec.Type.Name)
			project = ep.Spec.Owner.ProjectName
			for _, v := range ep.Spec.Visibility {
				visibility = append(visibility, string(v))
			}
		}
		age := ""
		if ep.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*ep.Metadata.CreationTimestamp)
		}
		if showProject {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				ep.Metadata.Name, typeStr, strings.Join(visibility, ","), project, age)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", ep.Metadata.Name, typeStr, strings.Join(visibility, ","), age)
		}
	}

	return w.Flush()
}

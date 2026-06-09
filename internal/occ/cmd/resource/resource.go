// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

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

// Resource implements resource operations
type Resource struct {
	client client.Interface
}

// New creates a new resource implementation
func New(c client.Interface) *Resource {
	return &Resource{client: c}
}

// List lists resources in a namespace, optionally filtered by project
func (r *Resource) List(params ListParams) error {
	if err := cmdutil.RequireFields("list", "resource", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ResourceInstance, string, error) {
		p := &gen.ListResourcesParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		if params.Project != "" {
			p.Project = &params.Project
		}
		result, err := r.client.ListResources(ctx, params.Namespace, p)
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

// Get retrieves a single resource and outputs it as YAML
func (r *Resource) Get(params GetParams) error {
	if err := cmdutil.RequireFields("get", "resource", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	result, err := r.client.GetResource(ctx, params.Namespace, params.ResourceName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal resource to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single resource
func (r *Resource) Delete(params DeleteParams) error {
	if err := cmdutil.RequireFields("delete", "resource", map[string]string{"namespace": params.Namespace, "name": params.ResourceName}); err != nil {
		return err
	}

	ctx := context.Background()

	if err := r.client.DeleteResource(ctx, params.Namespace, params.ResourceName); err != nil {
		return err
	}

	fmt.Printf("Resource '%s' deleted\n", params.ResourceName)
	return nil
}

// Promote advances the ResourceReleaseBinding for the given environment to the
// resource's latest release. It is a thin client wrapper: read
// Resource.status.latestRelease.name, find the binding for the target environment,
// and PUT it with the new spec.resourceRelease. No dedicated server endpoint.
func (r *Resource) Promote(params PromoteParams) error {
	if err := cmdutil.RequireFields("promote", "resource", map[string]string{
		"namespace": params.Namespace,
		"name":      params.ResourceName,
		"env":       params.Environment,
	}); err != nil {
		return err
	}

	ctx := context.Background()

	resource, err := r.client.GetResource(ctx, params.Namespace, params.ResourceName)
	if err != nil {
		return err
	}
	if resource.Status == nil || resource.Status.LatestRelease == nil || resource.Status.LatestRelease.Name == "" {
		return fmt.Errorf("resource %q has no released versions yet", params.ResourceName)
	}
	releaseName := resource.Status.LatestRelease.Name

	binding, err := r.findBindingForEnv(ctx, params.Namespace, params.ResourceName, params.Environment)
	if err != nil {
		return err
	}

	binding.Spec.ResourceRelease = &releaseName
	if _, err := r.client.UpdateResourceReleaseBinding(ctx, params.Namespace, binding.Metadata.Name, *binding); err != nil {
		return err
	}

	fmt.Printf("ResourceReleaseBinding '%s' promoted to resourcerelease '%s'\n", binding.Metadata.Name, releaseName)
	return nil
}

func (r *Resource) findBindingForEnv(ctx context.Context, namespace, resourceName, env string) (*gen.ResourceReleaseBinding, error) {
	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ResourceReleaseBinding, string, error) {
		p := &gen.ListResourceReleaseBindingsParams{Resource: &resourceName}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := r.client.ListResourceReleaseBindings(ctx, namespace, p)
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
	return nil, fmt.Errorf("no ResourceReleaseBinding found for resource %q in environment %q", resourceName, env)
}

func printList(items []gen.ResourceInstance, showProject bool) error {
	if len(items) == 0 {
		fmt.Println("No resources found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	if showProject {
		fmt.Fprintln(w, "NAME\tTYPE\tPROJECT\tAGE")
	} else {
		fmt.Fprintln(w, "NAME\tTYPE\tAGE")
	}

	for _, r := range items {
		typeStr := ""
		project := ""
		if r.Spec != nil {
			kind := "ResourceType"
			if r.Spec.Type.Kind != nil && *r.Spec.Type.Kind != "" {
				kind = string(*r.Spec.Type.Kind)
			}
			typeStr = fmt.Sprintf("%s/%s", kind, r.Spec.Type.Name)
			project = r.Spec.Owner.ProjectName
		}
		age := ""
		if r.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*r.Metadata.CreationTimestamp)
		}
		if showProject {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Metadata.Name, typeStr, project, age)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\n", r.Metadata.Name, typeStr, age)
		}
	}

	return w.Flush()
}

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebindingrevision

import (
	"context"
	"fmt"
	"os"
	"sort"

	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/bindingrevision"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ResourceReleaseBindingRevision implements resource release binding revision operations
type ResourceReleaseBindingRevision struct {
	client  client.Interface
	confirm bindingrevision.Confirmer
}

// New creates a new ResourceReleaseBindingRevision
func New(c client.Interface) *ResourceReleaseBindingRevision {
	return &ResourceReleaseBindingRevision{client: c, confirm: bindingrevision.ConfirmOnTerminal}
}

// List lists a resource release binding's revisions, newest first
func (r *ResourceReleaseBindingRevision) List(params ListParams) error {
	if err := cmdutil.RequireFields("list", "resourcereleasebindingrevision",
		map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}
	if params.Binding == "" {
		return r.listAll(context.Background(), params)
	}
	revs, err := r.revisions(context.Background(), params.Namespace, params.Binding)
	if err != nil {
		return err
	}
	rows := make([]bindingrevision.Row, 0, len(revs))
	for i := range revs {
		rows = append(rows, toRow(&revs[i]))
	}
	return bindingrevision.PrintTable(os.Stdout, rows)
}

// listAll lists the revisions of every resource release binding in the namespace, narrowed to a
// project, one
// resource's bindings, optionally in one environment: binding by binding in
// name order, each binding's revisions newest first.
func (r *ResourceReleaseBindingRevision) listAll(ctx context.Context, params ListParams) error {
	bindings, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ResourceReleaseBinding, string, error) {
		p := &gen.ListResourceReleaseBindingsParams{Limit: &limit}
		if params.Resource != "" {
			p.Resource = &params.Resource
		}
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := r.client.ListResourceReleaseBindings(ctx, params.Namespace, p)
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
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].Metadata.Name < bindings[j].Metadata.Name })

	var rows []bindingrevision.Row
	for _, b := range bindings {
		if b.Spec == nil {
			continue
		}
		environment := b.Spec.Environment
		if params.Environment != "" && environment != params.Environment {
			continue
		}
		if params.Project != "" && b.Spec.Owner.ProjectName != params.Project {
			continue
		}
		revs, err := r.revisionsOf(ctx, params.Namespace, b.Spec.Owner.ResourceName, b.Spec.Environment, b.Metadata.Name)
		if err != nil {
			return fmt.Errorf("list revisions of resource release binding %q: %w", b.Metadata.Name, err)
		}
		for i := range revs {
			row := toRow(&revs[i])
			row.Project = b.Spec.Owner.ProjectName
			row.Binding = b.Metadata.Name
			row.Environment = environment
			rows = append(rows, row)
		}
	}
	return bindingrevision.PrintBindingTable(os.Stdout, rows, params.Project == "")
}

// Get shows one revision of a resource release binding
func (r *ResourceReleaseBindingRevision) Get(params GetParams) error {
	if err := cmdutil.RequireFields("get", "resourcereleasebindingrevision",
		map[string]string{"namespace": params.Namespace, "binding": params.Binding}); err != nil {
		return err
	}
	if params.Revision < 1 {
		return fmt.Errorf("--revision is required and must be at least 1")
	}
	if err := bindingrevision.ValidateOutput(params.Output); err != nil {
		return err
	}

	revs, err := r.revisions(context.Background(), params.Namespace, params.Binding)
	if err != nil {
		return err
	}
	for i := range revs {
		if revs[i].Spec.Revision != params.Revision {
			continue
		}
		if params.Output == bindingrevision.OutputYAML {
			return bindingrevision.PrintYAML(os.Stdout, revs[i])
		}
		return bindingrevision.PrintTable(os.Stdout, []bindingrevision.Row{toRow(&revs[i])})
	}
	return fmt.Errorf("revision %d of resource release binding %q not found", params.Revision, params.Binding)
}

// Delete deletes one revision of a resource release binding: it shows the revision, checks
// with the server that it may be deleted, asks for confirmation unless Yes is
// set, and deletes it. The other revisions keep their numbers.
func (r *ResourceReleaseBindingRevision) Delete(params DeleteParams) error {
	if err := cmdutil.RequireFields("delete", "resourcereleasebindingrevision",
		map[string]string{"namespace": params.Namespace, "binding": params.Binding}); err != nil {
		return err
	}
	if params.Revision < 1 {
		return fmt.Errorf("--revision is required and must be at least 1")
	}

	ctx := context.Background()
	revs, err := r.revisions(ctx, params.Namespace, params.Binding)
	if err != nil {
		return err
	}
	var target *gen.ResourceReleaseBindingRevision
	for i := range revs {
		if revs[i].Spec.Revision == params.Revision {
			target = &revs[i]
		}
	}
	if target == nil || target.Metadata.Name == "" {
		return fmt.Errorf("revision %d of resource release binding %q not found; it may have been deleted",
			params.Revision, params.Binding)
	}

	return bindingrevision.RunDelete(ctx, bindingrevision.DeleteOptions{
		Kind:     "resource release binding",
		Binding:  params.Binding,
		Target:   toRow(target),
		Revision: params.Revision,
		DryRun:   params.DryRun,
		Yes:      params.Yes,
		Out:      os.Stdout,
		Confirm:  r.confirm,
	}, func(ctx context.Context, dryRun bool) (*gen.DeleteBindingRevisionResponse, error) {
		return r.client.DeleteResourceReleaseBindingRevision(ctx, params.Namespace, target.Metadata.Name,
			&gen.DeleteResourceReleaseBindingRevisionParams{Binding: &params.Binding, DryRun: &dryRun})
	})
}

// revisions returns a binding's revisions, newest first. A binding is the one
// binding of its resource in its environment, so its history is that
// resource's revisions in that environment.
func (r *ResourceReleaseBindingRevision) revisions(ctx context.Context, namespace, binding string) ([]gen.ResourceReleaseBindingRevision, error) {
	rb, err := r.client.GetResourceReleaseBinding(ctx, namespace, binding)
	if err != nil {
		return nil, err
	}
	if rb == nil || rb.Spec == nil {
		return nil, fmt.Errorf("resource release binding %q not found in namespace %q", binding, namespace)
	}
	return r.revisionsOf(ctx, namespace, rb.Spec.Owner.ResourceName, rb.Spec.Environment, binding)
}

// revisionsOf returns the revisions of one binding of the resource in one environment. The
// binding is named because a resource can have one binding per release line in an environment.
func (r *ResourceReleaseBindingRevision) revisionsOf(ctx context.Context, namespace, resource, environment, binding string) ([]gen.ResourceReleaseBindingRevision, error) {
	return pagination.FetchAll(func(limit int, cursor string) ([]gen.ResourceReleaseBindingRevision, string, error) {
		p := &gen.ListResourceReleaseBindingRevisionsParams{Resource: resource, Environment: &environment, Binding: &binding, Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := r.client.ListResourceReleaseBindingRevisions(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		next := ""
		if result.Pagination.NextCursor != nil {
			next = *result.Pagination.NextCursor
		}
		return result.Items, next, nil
	})
}

func toRow(rev *gen.ResourceReleaseBindingRevision) bindingrevision.Row {
	return bindingrevision.StatusRow(bindingrevision.Row{
		Name:      bindingrevision.DisplayName(rev.Spec.BindingSpec.Owner.ResourceName, rev.Spec.Revision),
		Cause:     string(rev.Spec.Cause),
		CreatedAt: rev.Metadata.CreationTimestamp,
		Message:   ptr.Deref(rev.Spec.Message, ""),
	}, rev.Status)
}

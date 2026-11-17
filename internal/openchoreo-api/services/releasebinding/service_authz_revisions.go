// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/bindingrevision"
)

// Revisions are a binding's history, so reading them takes releasebinding:view
// on the binding they record; rolling back takes releasebinding:rollback, and
// deleting a revision takes releasebinding:delete-revision.

// revisionCheckRequest builds a check of action on a revision, scoped like a
// check on the binding it records.
func revisionCheckRequest(action, namespaceName string, rev *openchoreov1alpha1.ReleaseBindingRevision) services.CheckRequest {
	spec := rev.Spec.BindingSpec
	return services.CheckRequest{
		Action:       action,
		ResourceType: resourceTypeReleaseBinding,
		ResourceID:   rev.Name,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   spec.Owner.ProjectName,
			Component: spec.Owner.ComponentName,
		},
		Context: authz.Context{
			Resource: authz.ResourceAttribute{
				Environment: services.FormatDualScopedResourceName(namespaceName, spec.Environment, false)},
		},
	}
}

func (s *releaseBindingServiceWithAuthz) ListReleaseBindingRevisions(ctx context.Context, namespaceName, componentName,
	environment, binding string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ReleaseBindingRevision], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ReleaseBindingRevision], error) {
			return s.internal.ListReleaseBindingRevisions(ctx, namespaceName, componentName, environment, binding, pageOpts)
		},
		func(rev openchoreov1alpha1.ReleaseBindingRevision) services.CheckRequest {
			return revisionCheckRequest(authz.ActionViewReleaseBinding, namespaceName, &rev)
		},
	)
}

func (s *releaseBindingServiceWithAuthz) GetReleaseBindingRevision(ctx context.Context, namespaceName,
	revisionName string) (*openchoreov1alpha1.ReleaseBindingRevision, error) {
	rev, err := s.internal.GetReleaseBindingRevision(ctx, namespaceName, revisionName)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Check(ctx, revisionCheckRequest(authz.ActionViewReleaseBinding, namespaceName, rev)); err != nil {
		return nil, err
	}
	return rev, nil
}

func (s *releaseBindingServiceWithAuthz) RollbackReleaseBinding(ctx context.Context, namespaceName, releaseBindingName string,
	req bindingrevision.RollbackRequest) (*bindingrevision.RollbackResult, error) {
	// The binding's owner and environment are immutable, so checking against the
	// stored binding covers the spec the rollback restores.
	rb, err := s.internal.GetReleaseBinding(ctx, namespaceName, releaseBindingName)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionRollbackReleaseBinding,
		ResourceType: resourceTypeReleaseBinding,
		ResourceID:   rb.Name,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   rb.Spec.Owner.ProjectName,
			Component: rb.Spec.Owner.ComponentName,
		},
		Context: authz.Context{
			Resource: authz.ResourceAttribute{
				Environment: services.FormatDualScopedResourceName(namespaceName, rb.Spec.Environment, false)},
		},
	}); err != nil {
		return nil, err
	}
	return s.internal.RollbackReleaseBinding(ctx, namespaceName, releaseBindingName, req)
}

// DeleteReleaseBindingRevision checks releasebinding:delete-revision against the binding the revision
// records, scoped like the view check.
func (s *releaseBindingServiceWithAuthz) DeleteReleaseBindingRevision(ctx context.Context, namespaceName,
	revisionName string, req bindingrevision.DeleteRequest) (*bindingrevision.DeleteResult, error) {
	rev, err := s.internal.GetReleaseBindingRevision(ctx, namespaceName, revisionName)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Check(ctx, revisionCheckRequest(authz.ActionDeleteReleaseBindingRevision, namespaceName, rev)); err != nil {
		return nil, err
	}
	return s.internal.DeleteReleaseBindingRevision(ctx, namespaceName, revisionName, req)
}

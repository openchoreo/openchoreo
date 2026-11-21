// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/bindingrevision"
)

// Revisions are a binding's history, so reading them takes
// resourcereleasebinding:view on the binding they record; rolling back takes
// resourcereleasebinding:rollback, and deleting a revision takes
// resourcereleasebinding:delete-revision.

// revisionCheckRequest builds a check of action on a revision, scoped like a
// check on the binding it records.
func revisionCheckRequest(action, namespaceName string, rev *openchoreov1alpha1.ResourceReleaseBindingRevision) services.CheckRequest {
	spec := rev.Spec.BindingSpec
	return services.CheckRequest{
		Action:       action,
		ResourceType: resourceTypeResourceReleaseBinding,
		ResourceID:   rev.Name,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   spec.Owner.ProjectName,
			Resource:  spec.Owner.ResourceName,
		},
		Context: authz.Context{
			Resource: authz.ResourceAttribute{
				Environment: services.FormatDualScopedResourceName(namespaceName, spec.Environment, false)},
		},
	}
}

func (s *resourceReleaseBindingServiceWithAuthz) ListResourceReleaseBindingRevisions(ctx context.Context, namespaceName,
	resourceName, environment, binding string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ResourceReleaseBindingRevision], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ResourceReleaseBindingRevision], error) {
			return s.internal.ListResourceReleaseBindingRevisions(ctx, namespaceName, resourceName, environment, binding, pageOpts)
		},
		func(rev openchoreov1alpha1.ResourceReleaseBindingRevision) services.CheckRequest {
			return revisionCheckRequest(authz.ActionViewResourceReleaseBinding, namespaceName, &rev)
		},
	)
}

func (s *resourceReleaseBindingServiceWithAuthz) GetResourceReleaseBindingRevision(ctx context.Context, namespaceName,
	revisionName string) (*openchoreov1alpha1.ResourceReleaseBindingRevision, error) {
	rev, err := s.internal.GetResourceReleaseBindingRevision(ctx, namespaceName, revisionName)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Check(ctx, revisionCheckRequest(authz.ActionViewResourceReleaseBinding, namespaceName, rev)); err != nil {
		return nil, err
	}
	return rev, nil
}

func (s *resourceReleaseBindingServiceWithAuthz) RollbackResourceReleaseBinding(ctx context.Context, namespaceName,
	bindingName string, req bindingrevision.RollbackRequest) (*bindingrevision.RollbackResult, error) {
	rb, err := s.internal.GetResourceReleaseBinding(ctx, namespaceName, bindingName)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       authz.ActionRollbackResourceReleaseBinding,
		ResourceType: resourceTypeResourceReleaseBinding,
		ResourceID:   rb.Name,
		Hierarchy: authz.ResourceHierarchy{
			Namespace: namespaceName,
			Project:   rb.Spec.Owner.ProjectName,
			Resource:  rb.Spec.Owner.ResourceName,
		},
		Context: authz.Context{
			Resource: authz.ResourceAttribute{
				Environment: services.FormatDualScopedResourceName(namespaceName, rb.Spec.Environment, false)},
		},
	}); err != nil {
		return nil, err
	}
	return s.internal.RollbackResourceReleaseBinding(ctx, namespaceName, bindingName, req)
}

// DeleteResourceReleaseBindingRevision checks resourcereleasebinding:delete-revision against the binding the revision
// records, scoped like the view check.
func (s *resourceReleaseBindingServiceWithAuthz) DeleteResourceReleaseBindingRevision(ctx context.Context, namespaceName,
	revisionName string, req bindingrevision.DeleteRequest) (*bindingrevision.DeleteResult, error) {
	rev, err := s.internal.GetResourceReleaseBindingRevision(ctx, namespaceName, revisionName)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Check(ctx, revisionCheckRequest(authz.ActionDeleteResourceReleaseBindingRevision, namespaceName, rev)); err != nil {
		return nil, err
	}
	return s.internal.DeleteResourceReleaseBindingRevision(ctx, namespaceName, revisionName, req)
}

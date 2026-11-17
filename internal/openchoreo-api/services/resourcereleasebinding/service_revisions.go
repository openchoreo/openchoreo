// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"context"
	"fmt"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/bindingrevision"
	"github.com/openchoreo/openchoreo/internal/revision"
)

var resourceReleaseBindingRevisionTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ResourceReleaseBindingRevision",
}

// ListResourceReleaseBindingRevisions returns the revisions of a resource's
// release bindings, optionally for one environment, ordered by environment and
// then by revision number, newest first. See the ReleaseBinding service for why
// the history is read whole and paged with an offset cursor.
func (s *resourceReleaseBindingService) ListResourceReleaseBindingRevisions(ctx context.Context, namespaceName,
	resourceName, environment, binding string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ResourceReleaseBindingRevision], error) {
	if err := s.validateResourceExists(ctx, namespaceName, resourceName); err != nil {
		return nil, err
	}

	selector := client.MatchingLabels{labels.LabelKeyResourceName: revision.LabelValue(resourceName)}
	if environment != "" {
		selector[labels.LabelKeyEnvironmentName] = revision.LabelValue(environment)
	}
	var list openchoreov1alpha1.ResourceReleaseBindingRevisionList
	if err := s.k8sClient.List(ctx, &list, client.InNamespace(namespaceName), selector); err != nil {
		return nil, fmt.Errorf("failed to list resource release binding revisions: %w", err)
	}

	items := make([]openchoreov1alpha1.ResourceReleaseBindingRevision, 0, len(list.Items))
	for i := range list.Items {
		rev := list.Items[i]
		spec := rev.Spec.BindingSpec
		if spec.Owner.ResourceName != resourceName || (environment != "" && spec.Environment != environment) {
			continue
		}
		if binding != "" && !bindingrevision.OwnedByBinding(&rev, "ResourceReleaseBinding", binding) {
			continue
		}
		rev.TypeMeta = resourceReleaseBindingRevisionTypeMeta
		items = append(items, rev)
	}
	sort.Slice(items, func(i, j int) bool {
		ei, ej := items[i].Spec.BindingSpec.Environment, items[j].Spec.BindingSpec.Environment
		if ei != ej {
			return ei < ej
		}
		return items[i].Spec.Revision > items[j].Spec.Revision
	})

	return bindingrevision.Paginate(items, opts)
}

// GetResourceReleaseBindingRevision returns one revision by name.
func (s *resourceReleaseBindingService) GetResourceReleaseBindingRevision(ctx context.Context, namespaceName,
	revisionName string) (*openchoreov1alpha1.ResourceReleaseBindingRevision, error) {
	rev := &openchoreov1alpha1.ResourceReleaseBindingRevision{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: revisionName}, rev); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, bindingrevision.ErrRevisionNotFound
		}
		return nil, fmt.Errorf("failed to get resource release binding revision: %w", err)
	}
	rev.TypeMeta = resourceReleaseBindingRevisionTypeMeta
	return rev, nil
}

// RollbackResourceReleaseBinding restores the binding spec recorded by an
// earlier revision; see the ReleaseBinding service's RollbackReleaseBinding.
// The release check is on the revision's ResourceRelease.
//
// A binding a GitOps tool applies from Git is refused with ErrGitOpsManaged,
// dry runs included (see bindingrevision.RejectGitOpsManaged).
//
// A request without a revision (0) rolls back to the revision that was active
// immediately before the current one (bindingrevision.PreviousRevision), and
// leaves the binding unchanged when that revision is gone or unknown.
func (s *resourceReleaseBindingService) RollbackResourceReleaseBinding(ctx context.Context, namespaceName,
	bindingName string, req bindingrevision.RollbackRequest) (*bindingrevision.RollbackResult, error) {
	if req.Revision < 0 {
		return nil, &services.ValidationError{Msg: "revision must be at least 1, or omitted for the previously active revision"}
	}

	binding := &openchoreov1alpha1.ResourceReleaseBinding{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: bindingName}, binding); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrResourceReleaseBindingNotFound
		}
		return nil, fmt.Errorf("failed to get resource release binding: %w", err)
	}

	// A binding applied from Git would be restored from Git on the next sync:
	// refuse, dry runs included, and change nothing.
	if err := bindingrevision.RejectGitOpsManaged(binding, "resource release binding"); err != nil {
		return nil, err
	}

	revs, err := s.bindingRevisions(ctx, binding)
	if err != nil {
		return nil, err
	}
	targetRevision := req.Revision
	var inferred *bindingrevision.Warning
	if targetRevision == 0 {
		specHash, err := revision.ResourceReleaseBindingSpecHash(&binding.Spec)
		if err != nil {
			return nil, err
		}
		candidates := make([]bindingrevision.Candidate, len(revs))
		for i := range revs {
			candidates[i] = bindingrevision.Candidate{Object: &revs[i], Number: revs[i].Spec.Revision,
				SnapshotHash: revs[i].Spec.SnapshotHash}
		}
		if targetRevision, inferred, err = bindingrevision.PreviousRevision(candidates, binding.Status.HighestRevision,
			specHash); err != nil {
			return nil, err
		}
	}
	var target, active *openchoreov1alpha1.ResourceReleaseBindingRevision
	for i := range revs {
		if revs[i].Spec.Revision == targetRevision {
			target = &revs[i]
		}
		if active == nil || revision.MoreRecentlyActive(&revs[i], revs[i].Spec.Revision, active, active.Spec.Revision) {
			active = &revs[i]
		}
	}
	if target == nil {
		return nil, fmt.Errorf("%w: revision %d of resource release binding %q has been deleted or never existed", bindingrevision.ErrRevisionNotFound,
			targetRevision, bindingName)
	}

	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: target.Spec.ResourceReleaseName},
		&openchoreov1alpha1.ResourceRelease{}); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: ResourceRelease %q bound by revision %d", bindingrevision.ErrReleaseDeleted,
				target.Spec.ResourceReleaseName, targetRevision)
		}
		return nil, fmt.Errorf("failed to get resource release: %w", err)
	}

	restored := *target.Spec.BindingSpec.DeepCopy()
	// The history limit configures the history and is not part of what a
	// revision records, so a rollback leaves the binding's current one in place.
	restored.RevisionHistoryLimit = binding.Spec.RevisionHistoryLimit

	currentHash, err := revision.ResourceReleaseBindingSpecHash(&binding.Spec)
	if err != nil {
		return nil, err
	}
	restoredHash, err := revision.ResourceReleaseBindingSpecHash(&restored)
	if err != nil {
		return nil, err
	}
	noSpecChange := currentHash == restoredHash

	changes, err := bindingrevision.SpecChanges(binding.Spec, restored)
	if err != nil {
		return nil, err
	}

	revertedFrom := active.Spec.Revision
	result := &bindingrevision.RollbackResult{
		RevisionName: target.Name,
		RevisionDisplayName: revision.DisplayName(target.Spec.BindingSpec.Owner.ResourceName,
			target.Spec.Revision),
		Revision:       targetRevision,
		TargetRevision: targetRevision,
		RevertedFrom:   &revertedFrom,
		DryRun:         req.DryRun,
		Changes:        changes,
	}

	result.Warnings = bindingrevision.CommonWarnings(targetRevision, target.Status, noSpecChange,
		active.Spec.SnapshotHash != currentHash)
	if inferred != nil {
		result.Warnings = append([]bindingrevision.Warning{*inferred}, result.Warnings...)
	}
	cellDrift, err := bindingrevision.ProjectCellDrift(ctx, s.k8sClient, namespaceName, binding.Spec.Owner.ProjectName,
		binding.Spec.Environment, targetRevision, target.CreationTimestamp)
	if err != nil {
		return nil, err
	}
	if cellDrift != nil {
		result.Warnings = append(result.Warnings, *cellDrift)
	}

	if req.DryRun {
		return result, nil
	}

	base := binding.DeepCopy()
	binding.Spec = restored
	bindingrevision.SetChangeMessage(binding, req.Message)

	if err := s.k8sClient.Patch(ctx, binding,
		client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
		switch {
		case apierrors.IsConflict(err):
			return nil, fmt.Errorf("%w: resource release binding %q", bindingrevision.ErrConcurrentModification, bindingName)
		case apierrors.IsNotFound(err):
			return nil, ErrResourceReleaseBindingNotFound
		}
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		return nil, fmt.Errorf("failed to roll back resource release binding: %w", err)
	}

	s.logger.Info("Rolled back resource release binding", "namespace", namespaceName, "resourceReleaseBinding", bindingName,
		"targetRevision", targetRevision, "revertedFrom", revertedFrom)
	return result, nil
}

// DeleteResourceReleaseBindingRevision deletes one revision of a resource release binding, unless it is
// protected (see bindingrevision.DeleteRevision). The revision's number is not
// reused and the remaining revisions keep theirs: the binding's
// status.highestRevision keeps the controller numbering new revisions above it.
// With DryRun every check runs and nothing is deleted.
func (s *resourceReleaseBindingService) DeleteResourceReleaseBindingRevision(ctx context.Context, namespaceName,
	revisionName string, req bindingrevision.DeleteRequest) (*bindingrevision.DeleteResult, error) {
	rev, err := s.GetResourceReleaseBindingRevision(ctx, namespaceName, revisionName)
	if err != nil {
		return nil, fmt.Errorf("%w: %q", err, revisionName)
	}
	owner := metav1.GetControllerOf(rev)
	if owner == nil || owner.Kind != "ResourceReleaseBinding" || (req.Binding != "" && owner.Name != req.Binding) {
		return nil, fmt.Errorf("%w: %q is not a revision of resource release binding %q", bindingrevision.ErrRevisionNotFound,
			revisionName, req.Binding)
	}

	binding := &openchoreov1alpha1.ResourceReleaseBinding{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: owner.Name}, binding); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrResourceReleaseBindingNotFound
		}
		return nil, fmt.Errorf("failed to get resource release binding: %w", err)
	}
	if binding.UID != owner.UID {
		return nil, ErrResourceReleaseBindingNotFound
	}

	revs, err := s.bindingRevisions(ctx, binding)
	if err != nil {
		return nil, err
	}
	specHash, err := revision.ResourceReleaseBindingSpecHash(&binding.Spec)
	if err != nil {
		return nil, err
	}
	candidates := make([]bindingrevision.Candidate, len(revs))
	for i := range revs {
		candidates[i] = bindingrevision.Candidate{Object: &revs[i], Number: revs[i].Spec.Revision,
			SnapshotHash: revs[i].Spec.SnapshotHash, Status: revs[i].Status}
	}
	if _, err := bindingrevision.DeleteRevision(ctx, s.k8sClient, revisionName, candidates, bindingrevision.BindingState{
		Name:            binding.Name,
		CurrentRevision: binding.Status.CurrentRevision,
		HighestRevision: binding.Status.HighestRevision,
		SpecHash:        specHash,
	}, req.DryRun); err != nil {
		return nil, err
	}

	if !req.DryRun {
		s.logger.Info("Deleted resource release binding revision", "namespace", namespaceName, "resourceReleaseBinding", binding.Name,
			"revision", rev.Spec.Revision)
	}
	return &bindingrevision.DeleteResult{
		RevisionName:        rev.Name,
		RevisionDisplayName: revision.DisplayName(rev.Spec.BindingSpec.Owner.ResourceName, rev.Spec.Revision),
		Revision:            rev.Spec.Revision,
		Binding:             binding.Name,
		Environment:         binding.Spec.Environment,
		DryRun:              req.DryRun,
	}, nil
}

// bindingRevisions returns the binding's own revisions.
func (s *resourceReleaseBindingService) bindingRevisions(ctx context.Context,
	binding *openchoreov1alpha1.ResourceReleaseBinding) ([]openchoreov1alpha1.ResourceReleaseBindingRevision, error) {
	var list openchoreov1alpha1.ResourceReleaseBindingRevisionList
	if err := s.k8sClient.List(ctx, &list, client.InNamespace(binding.Namespace),
		client.MatchingLabels{revision.LabelKeyBindingUID: string(binding.UID)}); err != nil {
		return nil, fmt.Errorf("failed to list resource release binding revisions: %w", err)
	}
	owned := make([]openchoreov1alpha1.ResourceReleaseBindingRevision, 0, len(list.Items))
	for i := range list.Items {
		if ref := metav1.GetControllerOf(&list.Items[i]); ref != nil && ref.UID == binding.UID {
			owned = append(owned, list.Items[i])
		}
	}
	return owned, nil
}

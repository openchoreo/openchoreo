// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

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
	"github.com/openchoreo/openchoreo/internal/releaseline"
	"github.com/openchoreo/openchoreo/internal/revision"
)

var releaseBindingRevisionTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ReleaseBindingRevision",
}

// ListReleaseBindingRevisions returns the revisions of a component's release
// bindings, optionally for one environment, ordered by environment and then by
// revision number, newest first.
//
// A component's history is bounded by the revision history limit, so it is read
// whole, sorted, and paged with an offset cursor; a Kubernetes continue token
// would page in name order, where r10 sorts before r2.
func (s *releaseBindingService) ListReleaseBindingRevisions(ctx context.Context, namespaceName, componentName,
	environment, binding string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ReleaseBindingRevision], error) {
	if err := s.validateComponentExists(ctx, namespaceName, componentName); err != nil {
		return nil, err
	}

	selector := client.MatchingLabels{labels.LabelKeyComponentName: revision.LabelValue(componentName)}
	if environment != "" {
		selector[labels.LabelKeyEnvironmentName] = revision.LabelValue(environment)
	}
	var list openchoreov1alpha1.ReleaseBindingRevisionList
	if err := s.k8sClient.List(ctx, &list, client.InNamespace(namespaceName), selector); err != nil {
		return nil, fmt.Errorf("failed to list release binding revisions: %w", err)
	}

	// Label values are shortened for long names, so match the spec exactly.
	items := make([]openchoreov1alpha1.ReleaseBindingRevision, 0, len(list.Items))
	for i := range list.Items {
		rev := list.Items[i]
		spec := rev.Spec.BindingSpec
		if spec.Owner.ComponentName != componentName || (environment != "" && spec.Environment != environment) {
			continue
		}
		if binding != "" && !bindingrevision.OwnedByBinding(&rev, "ReleaseBinding", binding) {
			continue
		}
		rev.TypeMeta = releaseBindingRevisionTypeMeta
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

// GetReleaseBindingRevision returns one revision by name.
func (s *releaseBindingService) GetReleaseBindingRevision(ctx context.Context, namespaceName,
	revisionName string) (*openchoreov1alpha1.ReleaseBindingRevision, error) {
	rev := &openchoreov1alpha1.ReleaseBindingRevision{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: revisionName}, rev); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, bindingrevision.ErrRevisionNotFound
		}
		return nil, fmt.Errorf("failed to get release binding revision: %w", err)
	}
	rev.TypeMeta = releaseBindingRevisionTypeMeta
	return rev, nil
}

// RollbackReleaseBinding restores the binding spec recorded by an earlier
// revision. It writes the binding -- the target's spec -- conditional on the
// resourceVersion it read, so a concurrent edit
// fails with ErrConcurrentModification. It never writes a revision and no new
// revision is created: the restored spec is the one the target records, so
// the controller makes the target active again under its own name and number
// and points status.currentRevision at it. With DryRun it returns the same
// result and writes nothing.
//
// A binding a GitOps tool applies from Git is refused with ErrGitOpsManaged,
// dry runs included (see bindingrevision.RejectGitOpsManaged).
//
// A binding in the first environment of a component with autoDeploy enabled is
// refused with ErrAutoDeployEnabled, dry runs included: auto-deploy would move
// it straight back to the latest release.
//
// A request without a revision (0) rolls back to the revision that was active
// immediately before the current one (bindingrevision.PreviousRevision), and
// leaves the binding unchanged when that revision is gone or unknown.
func (s *releaseBindingService) RollbackReleaseBinding(ctx context.Context, namespaceName, releaseBindingName string,
	req bindingrevision.RollbackRequest) (*bindingrevision.RollbackResult, error) {
	if req.Revision < 0 {
		return nil, &services.ValidationError{Msg: "revision must be at least 1, or omitted for the previously active revision"}
	}

	binding := &openchoreov1alpha1.ReleaseBinding{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: releaseBindingName}, binding); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrReleaseBindingNotFound
		}
		return nil, fmt.Errorf("failed to get release binding: %w", err)
	}

	// A binding applied from Git would be restored from Git on the next sync:
	// refuse, dry runs included, and change nothing.
	if err := bindingrevision.RejectGitOpsManaged(binding, "release binding"); err != nil {
		return nil, err
	}

	// Auto-deploy keeps the first environment's binding on the latest release of
	// its component or release line, so it would undo the rollback within
	// seconds: refuse it, dry runs included, and change nothing.
	deployer, err := bindingrevision.AutoDeployBlocksRollback(ctx, s.k8sClient, namespaceName,
		binding.Spec.Owner.ComponentName, binding.Spec.Owner.ReleaseLine, binding.Spec.Environment)
	if err != nil {
		return nil, err
	}
	if deployer != "" {
		return nil, fmt.Errorf("%w for %s, and %q is the first environment of its deployment pipeline, "+
			"so auto-deploy would move release binding %q back to the latest release right after the rollback. "+
			"Turn off auto-deploy for the %s, then roll back again", bindingrevision.ErrAutoDeployEnabled,
			deployer, binding.Spec.Environment, releaseBindingName, autoDeployScope(binding.Spec.Owner.ReleaseLine))
	}

	revs, err := s.bindingRevisions(ctx, binding)
	if err != nil {
		return nil, err
	}
	targetRevision := req.Revision
	var inferred *bindingrevision.Warning
	if targetRevision == 0 {
		specHash, err := revision.ReleaseBindingSpecHash(&binding.Spec)
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
	var target, active *openchoreov1alpha1.ReleaseBindingRevision
	for i := range revs {
		if revs[i].Spec.Revision == targetRevision {
			target = &revs[i]
		}
		if active == nil || revision.MoreRecentlyActive(&revs[i], revs[i].Spec.Revision, active, active.Spec.Revision) {
			active = &revs[i]
		}
	}
	if target == nil {
		return nil, fmt.Errorf("%w: revision %d of release binding %q has been deleted or never existed", bindingrevision.ErrRevisionNotFound,
			targetRevision, releaseBindingName)
	}

	componentRelease := &openchoreov1alpha1.ComponentRelease{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: target.Spec.ComponentReleaseName},
		componentRelease); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: ComponentRelease %q bound by revision %d", bindingrevision.ErrReleaseDeleted,
				target.Spec.ComponentReleaseName, targetRevision)
		}
		return nil, fmt.Errorf("failed to get component release: %w", err)
	}

	restored := *target.Spec.BindingSpec.DeepCopy()
	// The history limit configures the history and is not part of what a
	// revision records, so a rollback leaves the binding's current one in place.
	restored.RevisionHistoryLimit = binding.Spec.RevisionHistoryLimit

	currentHash, err := revision.ReleaseBindingSpecHash(&binding.Spec)
	if err != nil {
		return nil, err
	}
	restoredHash, err := revision.ReleaseBindingSpecHash(&restored)
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
		RevisionDisplayName: revision.DisplayName(target.Spec.BindingSpec.Owner.ComponentName,
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
	if restored.State == openchoreov1alpha1.ReleaseStateUndeploy {
		result.Warnings = append(result.Warnings, bindingrevision.Warning{Code: bindingrevision.WarningTargetUndeploys,
			Message: fmt.Sprintf("revision %d has state Undeploy; the rollback removes the deployment", targetRevision)})
	}
	cellDrift, err := bindingrevision.ProjectCellDrift(ctx, s.k8sClient, namespaceName, binding.Spec.Owner.ProjectName,
		binding.Spec.Environment, targetRevision, target.CreationTimestamp)
	if err != nil {
		return nil, err
	}
	if cellDrift != nil {
		result.Warnings = append(result.Warnings, *cellDrift)
	}
	secretDrift, err := s.secretReferenceDrift(ctx, namespaceName, componentRelease, &restored, target)
	if err != nil {
		return nil, err
	}
	result.Warnings = append(result.Warnings, secretDrift...)

	if req.DryRun {
		return result, nil
	}

	if err := s.patchBindingSpec(ctx, binding, restored, req.Message, releaseBindingName); err != nil {
		return nil, err
	}

	s.logger.Info("Rolled back release binding", "namespace", namespaceName, "releaseBinding", releaseBindingName,
		"targetRevision", targetRevision, "revertedFrom", revertedFrom)
	return result, nil
}

// patchBindingSpec writes spec to the binding, conditional on the
// resourceVersion it was read at.
func (s *releaseBindingService) patchBindingSpec(ctx context.Context, binding *openchoreov1alpha1.ReleaseBinding,
	spec openchoreov1alpha1.ReleaseBindingSpec, message, releaseBindingName string) error {
	base := binding.DeepCopy()
	binding.Spec = spec
	bindingrevision.SetChangeMessage(binding, message)
	err := s.k8sClient.Patch(ctx, binding, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}))
	switch {
	case err == nil:
		return nil
	case apierrors.IsConflict(err):
		return fmt.Errorf("%w: release binding %q", bindingrevision.ErrConcurrentModification, releaseBindingName)
	case apierrors.IsNotFound(err):
		return ErrReleaseBindingNotFound
	}
	if vErr := services.ExtractValidationError(err); vErr != nil {
		return vErr
	}
	return fmt.Errorf("failed to roll back release binding: %w", err)
}

// DeleteReleaseBindingRevision deletes one revision of a release binding, unless it is
// protected (see bindingrevision.DeleteRevision). The revision's number is not
// reused and the remaining revisions keep theirs: the binding's
// status.highestRevision keeps the controller numbering new revisions above it.
// With DryRun every check runs and nothing is deleted.
func (s *releaseBindingService) DeleteReleaseBindingRevision(ctx context.Context, namespaceName,
	revisionName string, req bindingrevision.DeleteRequest) (*bindingrevision.DeleteResult, error) {
	rev, err := s.GetReleaseBindingRevision(ctx, namespaceName, revisionName)
	if err != nil {
		return nil, fmt.Errorf("%w: %q", err, revisionName)
	}
	owner := metav1.GetControllerOf(rev)
	if owner == nil || owner.Kind != "ReleaseBinding" || (req.Binding != "" && owner.Name != req.Binding) {
		return nil, fmt.Errorf("%w: %q is not a revision of release binding %q", bindingrevision.ErrRevisionNotFound,
			revisionName, req.Binding)
	}

	binding := &openchoreov1alpha1.ReleaseBinding{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: owner.Name}, binding); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrReleaseBindingNotFound
		}
		return nil, fmt.Errorf("failed to get release binding: %w", err)
	}
	if binding.UID != owner.UID {
		return nil, ErrReleaseBindingNotFound
	}

	revs, err := s.bindingRevisions(ctx, binding)
	if err != nil {
		return nil, err
	}
	specHash, err := revision.ReleaseBindingSpecHash(&binding.Spec)
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
		s.logger.Info("Deleted release binding revision", "namespace", namespaceName, "releaseBinding", binding.Name,
			"revision", rev.Spec.Revision)
	}
	return &bindingrevision.DeleteResult{
		RevisionName:        rev.Name,
		RevisionDisplayName: revision.DisplayName(rev.Spec.BindingSpec.Owner.ComponentName, rev.Spec.Revision),
		Revision:            rev.Spec.Revision,
		Binding:             binding.Name,
		Environment:         binding.Spec.Environment,
		DryRun:              req.DryRun,
	}, nil
}

// bindingRevisions returns the binding's own revisions: those carrying its UID
// label whose controller reference is the binding.
func (s *releaseBindingService) bindingRevisions(ctx context.Context,
	binding *openchoreov1alpha1.ReleaseBinding) ([]openchoreov1alpha1.ReleaseBindingRevision, error) {
	var list openchoreov1alpha1.ReleaseBindingRevisionList
	if err := s.k8sClient.List(ctx, &list, client.InNamespace(binding.Namespace),
		client.MatchingLabels{revision.LabelKeyBindingUID: string(binding.UID)}); err != nil {
		return nil, fmt.Errorf("failed to list release binding revisions: %w", err)
	}
	owned := make([]openchoreov1alpha1.ReleaseBindingRevision, 0, len(list.Items))
	for i := range list.Items {
		if ref := metav1.GetControllerOf(&list.Items[i]); ref != nil && ref.UID == binding.UID {
			owned = append(owned, list.Items[i])
		}
	}
	return owned, nil
}

// secretReferenceDrift warns about each SecretReference the restored spec uses --
// through the ComponentRelease's workload or the binding's workload overrides --
// that no longer exists, lacks a referenced key, or was recreated after the
// target revision was recorded.
func (s *releaseBindingService) secretReferenceDrift(ctx context.Context, namespaceName string,
	componentRelease *openchoreov1alpha1.ComponentRelease, restored *openchoreov1alpha1.ReleaseBindingSpec,
	target *openchoreov1alpha1.ReleaseBindingRevision) ([]bindingrevision.Warning, error) {
	keys := map[string]map[string]struct{}{}
	var names []string
	add := func(ref *openchoreov1alpha1.SecretKeyRef) {
		if ref == nil || ref.Name == "" {
			return
		}
		if _, seen := keys[ref.Name]; !seen {
			keys[ref.Name] = map[string]struct{}{}
			names = append(names, ref.Name)
		}
		keys[ref.Name][ref.Key] = struct{}{}
	}
	addContainer := func(env []openchoreov1alpha1.EnvVar, files []openchoreov1alpha1.FileVar) {
		for i := range env {
			if env[i].ValueFrom != nil {
				add(env[i].ValueFrom.SecretKeyRef)
			}
		}
		for i := range files {
			if files[i].ValueFrom != nil {
				add(files[i].ValueFrom.SecretKeyRef)
			}
		}
	}
	addContainer(componentRelease.Spec.Workload.Container.Env, componentRelease.Spec.Workload.Container.Files)
	if overrides := restored.WorkloadOverrides; overrides != nil && overrides.Container != nil {
		addContainer(overrides.Container.Env, overrides.Container.Files)
	}

	var warnings []bindingrevision.Warning
	for _, name := range names {
		ref := &openchoreov1alpha1.SecretReference{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: name}, ref); err != nil {
			if apierrors.IsNotFound(err) {
				warnings = append(warnings, bindingrevision.Warning{Code: bindingrevision.WarningSecretReferenceDrift,
					Message: fmt.Sprintf("SecretReference %q used by revision %d no longer exists", name, target.Spec.Revision)})
				continue
			}
			return nil, fmt.Errorf("failed to get secret reference %q: %w", name, err)
		}
		if ref.CreationTimestamp.After(target.CreationTimestamp.Time) {
			warnings = append(warnings, bindingrevision.Warning{Code: bindingrevision.WarningSecretReferenceDrift,
				Message: fmt.Sprintf("SecretReference %q was recreated after revision %d", name, target.Spec.Revision)})
		}
		available := map[string]struct{}{}
		for _, data := range ref.Spec.Data {
			available[data.SecretKey] = struct{}{}
		}
		missing := make([]string, 0)
		for key := range keys[name] {
			if _, ok := available[key]; !ok {
				missing = append(missing, key)
			}
		}
		sort.Strings(missing)
		for _, key := range missing {
			warnings = append(warnings, bindingrevision.Warning{Code: bindingrevision.WarningSecretReferenceDrift,
				Message: fmt.Sprintf("SecretReference %q no longer has key %q used by revision %d", name, key, target.Spec.Revision)})
		}
	}
	return warnings, nil
}

// autoDeployScope names what auto-deploy is turned off on for a binding of the
// given release line: the component for its main line, the release line otherwise.
func autoDeployScope(releaseLine string) string {
	if releaseline.Normalize(releaseLine) != "" {
		return "release line"
	}
	return "component"
}

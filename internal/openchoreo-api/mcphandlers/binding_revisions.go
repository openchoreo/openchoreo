// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/bindingrevision"
	"github.com/openchoreo/openchoreo/internal/revision"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// ---------------------------------------------------------------------------
// ReleaseBindingRevision
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListReleaseBindingRevisions(
	ctx context.Context, namespaceName, componentName, environment, bindingName string, opts tools.ListOpts,
) (any, error) {
	result, err := h.services.ReleaseBindingService.ListReleaseBindingRevisions(
		ctx, namespaceName, componentName, environment, bindingName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList(
		"release_binding_revisions", result.Items, result.NextCursor, releaseBindingRevisionSummary,
	), nil
}

func (h *MCPHandler) GetReleaseBindingRevision(
	ctx context.Context, namespaceName, revisionName string,
) (any, error) {
	rev, err := h.services.ReleaseBindingService.GetReleaseBindingRevision(ctx, namespaceName, revisionName)
	if err != nil {
		return nil, err
	}
	m := releaseBindingRevisionSummary(*rev)
	m["snapshotHash"] = rev.Spec.SnapshotHash
	m["bindingSpec"] = specToMap(rev.Spec.BindingSpec)
	if conds := conditionsSummary(rev.Status.Conditions); conds != nil {
		m["conditions"] = conds
	}
	return m, nil
}

func (h *MCPHandler) RollbackReleaseBinding(
	ctx context.Context, namespaceName, bindingName string, revision int64, dryRun bool, message string,
) (any, error) {
	// Every call is audited, dry runs (the default) included: dryRun tells a
	// preview, which changes nothing, from an applied rollback. Set before the
	// rollback runs so that a failed or denied call is marked too.
	// The tools always name a revision: an implicit target (revision 0, the
	// previously active revision) is not offered to agents.
	if revision < 1 {
		return nil, &services.ValidationError{Msg: "revision must be at least 1"}
	}
	message, err := bindingrevision.NormalizeMessage(message)
	if err != nil {
		return nil, err
	}
	audit.SetResource(ctx, &audit.Resource{
		Namespace: namespaceName, Name: bindingName, Metadata: rollbackAuditMetadata(dryRun, message),
	})
	result, err := h.services.ReleaseBindingService.RollbackReleaseBinding(ctx, namespaceName, bindingName,
		bindingrevision.RollbackRequest{Revision: revision, DryRun: dryRun, Message: message})
	if err != nil {
		return nil, err
	}
	return rollbackResult(bindingName, result), nil
}

func releaseBindingRevisionSummary(rev openchoreov1alpha1.ReleaseBindingRevision) map[string]any {
	m := revisionSummary(&rev, rev.Spec.Revision, rev.Spec.Cause, rev.Status)
	m["componentName"] = rev.Spec.BindingSpec.Owner.ComponentName
	m["environment"] = rev.Spec.BindingSpec.Environment
	m["componentRelease"] = rev.Spec.ComponentReleaseName
	m["displayName"] = revision.DisplayName(rev.Spec.BindingSpec.Owner.ComponentName, rev.Spec.Revision)
	m["state"] = string(rev.Spec.BindingSpec.State)
	return m
}

// ---------------------------------------------------------------------------
// ResourceReleaseBindingRevision
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListResourceReleaseBindingRevisions(
	ctx context.Context, namespaceName, resourceName, environment, bindingName string, opts tools.ListOpts,
) (any, error) {
	result, err := h.services.ResourceReleaseBindingService.ListResourceReleaseBindingRevisions(
		ctx, namespaceName, resourceName, environment, bindingName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList(
		"resource_release_binding_revisions", result.Items, result.NextCursor, resourceReleaseBindingRevisionSummary,
	), nil
}

func (h *MCPHandler) GetResourceReleaseBindingRevision(
	ctx context.Context, namespaceName, revisionName string,
) (any, error) {
	rev, err := h.services.ResourceReleaseBindingService.GetResourceReleaseBindingRevision(ctx, namespaceName, revisionName)
	if err != nil {
		return nil, err
	}
	m := resourceReleaseBindingRevisionSummary(*rev)
	m["snapshotHash"] = rev.Spec.SnapshotHash
	m["bindingSpec"] = specToMap(rev.Spec.BindingSpec)
	if conds := conditionsSummary(rev.Status.Conditions); conds != nil {
		m["conditions"] = conds
	}
	return m, nil
}

func (h *MCPHandler) RollbackResourceReleaseBinding(
	ctx context.Context, namespaceName, bindingName string, revision int64, dryRun bool, message string,
) (any, error) {
	// Every call is audited, dry runs (the default) included: dryRun tells a
	// preview, which changes nothing, from an applied rollback. Set before the
	// rollback runs so that a failed or denied call is marked too.
	// The tools always name a revision: an implicit target (revision 0, the
	// previously active revision) is not offered to agents.
	if revision < 1 {
		return nil, &services.ValidationError{Msg: "revision must be at least 1"}
	}
	message, err := bindingrevision.NormalizeMessage(message)
	if err != nil {
		return nil, err
	}
	audit.SetResource(ctx, &audit.Resource{
		Namespace: namespaceName, Name: bindingName, Metadata: rollbackAuditMetadata(dryRun, message),
	})
	result, err := h.services.ResourceReleaseBindingService.RollbackResourceReleaseBinding(ctx, namespaceName,
		bindingName, bindingrevision.RollbackRequest{Revision: revision, DryRun: dryRun, Message: message})
	if err != nil {
		return nil, err
	}
	return rollbackResult(bindingName, result), nil
}

func resourceReleaseBindingRevisionSummary(rev openchoreov1alpha1.ResourceReleaseBindingRevision) map[string]any {
	m := revisionSummary(&rev, rev.Spec.Revision, rev.Spec.Cause, rev.Status)
	m["resourceName"] = rev.Spec.BindingSpec.Owner.ResourceName
	m["environment"] = rev.Spec.BindingSpec.Environment
	m["resourceRelease"] = rev.Spec.ResourceReleaseName
	m["displayName"] = revision.DisplayName(rev.Spec.BindingSpec.Owner.ResourceName, rev.Spec.Revision)
	return m
}

// ---------------------------------------------------------------------------
// Shared
// ---------------------------------------------------------------------------

// revisionSummary returns the fields both revision kinds share. active and
// knownGood are always present, false included, since the tool descriptions
// tell agents to choose a rollback target by them.
func revisionSummary(obj metav1.Object, number int64, cause openchoreov1alpha1.RevisionCause,
	status openchoreov1alpha1.BindingRevisionStatus) map[string]any {
	m := map[string]any{
		"name":      obj.GetName(),
		"revision":  number,
		"cause":     string(cause),
		"active":    status.Active,
		"knownGood": status.KnownGood,
	}
	if ns := obj.GetNamespace(); ns != "" {
		m["namespace"] = ns
	}
	if ts := obj.GetCreationTimestamp(); !ts.IsZero() {
		m["createdAt"] = ts.UTC().Format("2006-01-02T15:04:05Z")
	}
	setIfNotEmpty(m, "health", string(status.Health))
	if status.HealthySince != nil {
		m["healthySince"] = status.HealthySince.UTC().Format("2006-01-02T15:04:05Z")
	}
	return m
}

// rollbackResult reports a rollback or its dry run. applied tells the agent
// whether anything changed; for a dry run, nextStep repeats what the tool
// description requires before applying it.
func rollbackResult(bindingName string, result *bindingrevision.RollbackResult) map[string]any {
	changes := make([]map[string]any, 0, len(result.Changes))
	for _, c := range result.Changes {
		changes = append(changes, map[string]any{"field": c.Field, "current": c.Current, "target": c.Target})
	}
	warnings := make([]map[string]any, 0, len(result.Warnings))
	for _, w := range result.Warnings {
		warnings = append(warnings, map[string]any{"code": string(w.Code), "message": w.Message})
	}

	m := map[string]any{
		"binding":             bindingName,
		"targetRevision":      result.TargetRevision,
		"revision":            result.Revision,
		"revisionName":        result.RevisionName,
		"revisionDisplayName": result.RevisionDisplayName,
		"dryRun":              result.DryRun,
		"applied":             !result.DryRun,
		"changes":             changes,
		"warnings":            warnings,
	}
	if result.RevertedFrom != nil {
		m["revertedFrom"] = *result.RevertedFrom
	}
	if result.DryRun {
		m["nextStep"] = "Nothing was changed. Show the user the changes and every warning, ask them to confirm, " +
			"and only then call again with dry_run=false."
	}
	return m
}

// rollbackAuditMetadata is the audit metadata of a rollback call: whether it was
// a dry run, and its message when one is given.
func rollbackAuditMetadata(dryRun bool, message string) map[string]any {
	metadata := map[string]any{"dryRun": dryRun}
	if message != "" {
		metadata["message"] = message
	}
	return metadata
}

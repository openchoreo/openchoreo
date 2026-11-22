// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package bindingrevision holds what the ReleaseBinding and ResourceReleaseBinding
// services share for revision history, rollback and revision deletion: request
// and result types, errors, the spec diff, the deletion checks and sorted-list
// pagination.
package bindingrevision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/releaseline"
	"github.com/openchoreo/openchoreo/internal/revision"
)

var (
	// ErrRevisionNotFound is returned when the binding has no revision with the
	// requested number.
	ErrRevisionNotFound = errors.New("revision not found")
	// ErrReleaseDeleted is returned when the release a revision binds no longer exists.
	ErrReleaseDeleted = errors.New("release bound by the revision has been deleted")
	// ErrConcurrentModification is returned when the binding changed between being
	// read and being written.
	ErrConcurrentModification = errors.New("binding was modified concurrently")
	// ErrRevisionProtected is returned, as a *ProtectedError, when a revision may
	// not be deleted.
	ErrRevisionProtected = errors.New("revision is protected")
	// ErrNoPreviousRevision is returned when a rollback names no revision and
	// the revision active before the current one cannot be rolled back to: there
	// was none, it was deleted, or the history cannot say which it was.
	ErrNoPreviousRevision = errors.New("no previous revision to roll back to")
	// ErrAutoDeployEnabled is returned when a rollback is refused because the
	// auto-deploy of the binding's component or release line would undo it (see
	// AutoDeployBlocksRollback).
	ErrAutoDeployEnabled = errors.New("rollback blocked: auto-deploy is enabled")
	// ErrGitOpsManaged is returned when a rollback is refused because a GitOps
	// tool applies the binding from Git (see RejectGitOpsManaged).
	ErrGitOpsManaged = errors.New("rollback blocked: the binding is managed by GitOps")
)

// Tracking metadata GitOps tools put on every object they apply. Any one of
// these keys on a binding means a tool re-applies its spec from Git.
const (
	// labelKeyFluxKustomizationName and labelKeyFluxKustomizationNamespace name
	// the Flux Kustomization that applies the object.
	labelKeyFluxKustomizationName      = "kustomize.toolkit.fluxcd.io/name"
	labelKeyFluxKustomizationNamespace = "kustomize.toolkit.fluxcd.io/namespace"
	// labelKeyFluxHelmReleaseName and labelKeyFluxHelmReleaseNamespace name the
	// Flux HelmRelease that installs the object.
	labelKeyFluxHelmReleaseName      = "helm.toolkit.fluxcd.io/name"
	labelKeyFluxHelmReleaseNamespace = "helm.toolkit.fluxcd.io/namespace"
	// annotationKeyArgoCDTrackingID is Argo CD's annotation resource tracking:
	// "{application}:{group}/{kind}:{namespace}/{name}".
	annotationKeyArgoCDTrackingID = "argocd.argoproj.io/tracking-id"
)

// ProtectedError explains why a revision may not be deleted.
type ProtectedError struct {
	Revision int64
	// Reason completes "cannot delete revision N because ...".
	Reason string
}

func (e *ProtectedError) Error() string {
	return fmt.Sprintf("cannot delete revision %d because %s", e.Revision, e.Reason)
}

// Is makes errors.Is(err, ErrRevisionProtected) match.
func (e *ProtectedError) Is(target error) bool { return target == ErrRevisionProtected }

// DeleteRequest asks to delete one revision of a binding.
type DeleteRequest struct {
	// Binding, when set, is the binding the caller expects the revision to
	// belong to; a revision of any other binding is reported as not found.
	Binding string
	// DryRun runs every check without deleting anything.
	DryRun bool
}

// DeleteResult describes a revision deletion, applied or dry-run.
type DeleteResult struct {
	RevisionName string
	// RevisionDisplayName is how the revision is shown to people, {owner}-r{N}:
	// the component or resource name and the revision number.
	RevisionDisplayName string
	Revision            int64
	Binding             string
	Environment         string
	DryRun              bool
}

// Candidate is one of a binding's revisions, as the deletion checks see it.
type Candidate struct {
	Object       client.Object
	Number       int64
	SnapshotHash string
	Status       openchoreov1alpha1.BindingRevisionStatus
}

// BindingState is what the deletion checks need from the binding.
type BindingState struct {
	Name            string
	CurrentRevision string
	HighestRevision int64
	// SpecHash is the snapshot hash of the binding's current spec.
	SpecHash string
}

// DeleteRevision deletes the revision named name, one of the binding's revisions
// revs, unless one of these holds:
//   - it is protected as the history limit protects it (revision.Protected):
//     it is active or the newest known-good revision;
//   - the binding's spec matches it, so the controller is about to make it
//     active again (a rollback to it is being applied);
//   - its number is above the binding's status.highestRevision, which the
//     controller has not written yet; deleting it then could let its number be
//     given to a later revision.
//
// The delete is conditional on the revision's UID and resourceVersion as
// listed, so a revision made active after the checks (the controller patches
// it when it does) is not deleted: ErrConcurrentModification. With dryRun only
// the checks run. Revisions are owned by their binding, never the other way
// round, so deleting one deletes nothing else.
func DeleteRevision(ctx context.Context, c client.Client, name string, revs []Candidate,
	binding BindingState, dryRun bool) (*Candidate, error) {
	var target *Candidate
	active := -1
	for i := range revs {
		if revs[i].Object.GetName() == name {
			target = &revs[i]
		}
		if active < 0 || revision.MoreRecentlyActive(revs[i].Object, revs[i].Number, revs[active].Object, revs[active].Number) {
			active = i
		}
	}
	if target == nil {
		return nil, fmt.Errorf("%w: %q is not a revision of binding %q", ErrRevisionNotFound, name, binding.Name)
	}

	candidates := make([]revision.EvictionCandidate, len(revs))
	var targetCandidate revision.EvictionCandidate
	for i, r := range revs {
		candidates[i] = revision.EvictionCandidate{
			Name:      r.Object.GetName(),
			Number:    r.Number,
			Active:    i == active || r.Status.Active || r.Object.GetName() == binding.CurrentRevision,
			KnownGood: r.Status.KnownGood,
		}
		if &revs[i] == target {
			targetCandidate = candidates[i]
		}
	}
	if p := revision.Protected(candidates, targetCandidate); p != "" {
		return nil, &ProtectedError{Revision: target.Number, Reason: string(p)}
	}
	if target.SnapshotHash == binding.SpecHash {
		return nil, &ProtectedError{Revision: target.Number,
			Reason: "the binding's spec matches it: a rollback to it is being applied"}
	}
	if target.Number > binding.HighestRevision {
		return nil, &ProtectedError{Revision: target.Number,
			Reason: "the controller has not finished recording it; retry shortly"}
	}
	if dryRun {
		return target, nil
	}

	uid, resourceVersion := target.Object.GetUID(), target.Object.GetResourceVersion()
	if err := c.Delete(ctx, target.Object,
		client.Preconditions{UID: &uid, ResourceVersion: &resourceVersion}); err != nil {
		switch {
		case apierrors.IsConflict(err):
			return nil, fmt.Errorf("%w: revision %q changed while it was being deleted; retry", ErrConcurrentModification, name)
		case apierrors.IsNotFound(err):
			return nil, fmt.Errorf("%w: revision %q has already been deleted", ErrRevisionNotFound, name)
		}
		return nil, fmt.Errorf("failed to delete revision %q: %w", name, err)
	}
	return target, nil
}

// RollbackRequest asks to restore the binding spec of an earlier revision.
type RollbackRequest struct {
	// Revision is the number of the revision to roll back to, or 0 for the
	// revision that was active before the current one (see PreviousRevision).
	Revision int64
	// DryRun runs every check and computes the result without writing anything.
	DryRun bool
	// Message is an optional reason for the rollback. It is written to the
	// binding's change-message annotation with the restored spec; the
	// controller puts it on the RevisionReactivated event.
	Message string
	// BindingUID, when set, is the UID of the binding the caller authorized.
	// The rollback refuses a binding of the same name with another UID.
	BindingUID types.UID
}

// NormalizeMessage returns a rollback message without surrounding whitespace,
// or a ValidationError when it is longer than a change message may be.
func NormalizeMessage(message string) (string, error) {
	message = strings.TrimSpace(message)
	if utf8.RuneCountInString(message) > revision.MaxChangeMessageLength {
		return "", &services.ValidationError{
			Msg: fmt.Sprintf("message must be at most %d characters", revision.MaxChangeMessageLength)}
	}
	return message, nil
}

// RollbackResult describes a rollback, applied or dry-run.
type RollbackResult struct {
	// RevisionName and Revision identify the revision active after the rollback.
	// A rollback creates no revision: the target revision becomes active again
	// under its own name and number, so these always name the target.
	RevisionName string
	Revision     int64
	// RevisionDisplayName is how the target is shown to people,
	// {owner}-r{N} (e.g. frontend-r1).
	RevisionDisplayName string
	// TargetRevision is the revision whose spec is restored.
	TargetRevision int64
	// RevertedFrom is the revision that was active when the rollback was made.
	RevertedFrom *int64
	DryRun       bool
	Changes      []SpecChange
	Warnings     []Warning
}

// SpecChange is one top-level binding spec field a rollback changes. Current and
// Target are nil when the field is unset.
type SpecChange struct {
	Field   string
	Current any
	Target  any
}

// WarningCode identifies a pre-flight finding. The values match the API.
type WarningCode string

const (
	WarningTargetNotKnownGood   WarningCode = "TargetNotKnownGood"
	WarningTargetUnhealthy      WarningCode = "TargetUnhealthy"
	WarningTargetUndeploys      WarningCode = "TargetUndeploys"
	WarningNoSpecChange         WarningCode = "NoSpecChange"
	WarningUnrecordedChange     WarningCode = "UnrecordedChange"
	WarningProjectCellDrift     WarningCode = "ProjectCellDrift"
	WarningSecretReferenceDrift WarningCode = "SecretReferenceDrift"
	// WarningPreviousRevisionInferred is set when no revision was named and the
	// previously active revision was inferred from activation order (see
	// PreviousRevision).
	WarningPreviousRevisionInferred WarningCode = "PreviousRevisionInferred"
)

// Warning is a pre-flight finding that does not block the rollback.
type Warning struct {
	Code    WarningCode
	Message string
}

// SpecChanges compares two binding specs field by field and returns the
// top-level fields whose values differ, in field-name order. Values are
// compared with revision.CanonicalHash, so key order inside free-form config and
// nil-versus-empty do not count as changes -- the same rules that decide whether
// the controller records a new revision.
func SpecChanges(current, target any) ([]SpecChange, error) {
	cur, err := asFields(current)
	if err != nil {
		return nil, err
	}
	tgt, err := asFields(target)
	if err != nil {
		return nil, err
	}

	names := make(map[string]struct{})
	for k := range cur {
		names[k] = struct{}{}
	}
	for k := range tgt {
		names[k] = struct{}{}
	}
	sorted := make([]string, 0, len(names))
	for k := range names {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	changes := []SpecChange{}
	for _, field := range sorted {
		curHash, err := revision.CanonicalHash(cur[field])
		if err != nil {
			return nil, err
		}
		tgtHash, err := revision.CanonicalHash(tgt[field])
		if err != nil {
			return nil, err
		}
		if curHash != tgtHash {
			changes = append(changes, SpecChange{Field: field, Current: cur[field], Target: tgt[field]})
		}
	}
	return changes, nil
}

// asFields decodes a spec into its top-level JSON fields. Numbers are kept as
// written (json.Number), so a large integer in a config is neither rounded in
// the comparison nor in the values a change reports.
func asFields(spec any) (map[string]any, error) {
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}
	fields := map[string]any{}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&fields); err != nil {
		return nil, fmt.Errorf("decode spec: %w", err)
	}
	return fields, nil
}

// SortKey orders a list of revisions: by environment, then newest revision
// first, then by object name, which makes the key unique to one revision.
type SortKey struct {
	Environment string `json:"e"`
	Revision    int64  `json:"r"`
	Name        string `json:"n"`
}

// Before reports whether a revision with key k is listed before one with key o.
func (k SortKey) Before(o SortKey) bool {
	switch {
	case k.Environment != o.Environment:
		return k.Environment < o.Environment
	case k.Revision != o.Revision:
		return k.Revision > o.Revision
	}
	return k.Name < o.Name
}

// SortRevisions sorts items in list order (see SortKey).
func SortRevisions[T any](items []T, key func(T) SortKey) {
	sort.Slice(items, func(i, j int) bool { return key(items[i]).Before(key(items[j])) })
}

// Paginate returns one page of items, sorted with SortRevisions. The cursor is
// opaque and holds the key of the last revision returned, so the next page
// resumes after that revision even when revisions are recorded or deleted in
// between -- an offset would repeat or skip revisions then. A Kubernetes
// continue token cannot give this order: it returns objects in name order,
// where r10 sorts before r2.
func Paginate[T any](items []T, key func(T) SortKey, opts services.ListOptions) (*services.ListResult[T], error) {
	start := 0
	if opts.Cursor != "" {
		var after SortKey
		raw, err := base64.RawURLEncoding.DecodeString(opts.Cursor)
		if err == nil {
			err = json.Unmarshal(raw, &after)
		}
		if err != nil {
			return nil, &services.ValidationError{Msg: "invalid cursor"}
		}
		start = sort.Search(len(items), func(i int) bool { return after.Before(key(items[i])) })
	}

	end := len(items)
	if opts.Limit > 0 && start+opts.Limit < end {
		end = start + opts.Limit
	}

	result := &services.ListResult[T]{Items: items[start:end]}
	if end < len(items) {
		raw, err := json.Marshal(key(items[end-1]))
		if err != nil {
			return nil, fmt.Errorf("encode cursor: %w", err)
		}
		result.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
		remaining := int64(len(items) - end)
		result.RemainingCount = &remaining
	}
	return result, nil
}

// SpecPatch returns the patch a rollback writes: one JSON Patch that replaces
// the binding's spec whole with spec -- keeping explicit nulls in its free-form
// configs, which a merge patch would delete -- and sets the change-message
// annotation to message. Without a message it removes the annotation: a message
// left by a change the rollback discards does not describe the rollback. It
// carries the resourceVersion binding was read at, so the write fails with a
// conflict if the binding changed since, or was replaced by another of the same
// name.
func SpecPatch(binding client.Object, spec any, message string) (client.Patch, error) {
	ops := []map[string]any{
		{"op": "replace", "path": "/metadata/resourceVersion", "value": binding.GetResourceVersion()},
		{"op": "replace", "path": "/spec", "value": spec},
	}
	_, hadMessage := binding.GetAnnotations()[openchoreov1alpha1.AnnotationKeyChangeMessage]
	if message != "" || hadMessage {
		annotations := make(map[string]string, len(binding.GetAnnotations())+1)
		for k, v := range binding.GetAnnotations() {
			annotations[k] = v
		}
		if message != "" {
			annotations[openchoreov1alpha1.AnnotationKeyChangeMessage] = message
		} else {
			delete(annotations, openchoreov1alpha1.AnnotationKeyChangeMessage)
		}
		ops = append(ops, map[string]any{"op": "add", "path": "/metadata/annotations", "value": annotations})
	}
	raw, err := json.Marshal(ops)
	if err != nil {
		return nil, fmt.Errorf("encode rollback patch: %w", err)
	}
	return client.RawPatch(types.JSONPatchType, raw), nil
}

// CheckBindingUID returns notFound when uid is set and the binding read has
// another UID: the binding checked for authorization was deleted and another
// of the same name created since, which the caller was not authorized for.
func CheckBindingUID(binding metav1.Object, uid types.UID, notFound error) error {
	if uid != "" && binding.GetUID() != uid {
		return notFound
	}
	return nil
}

// CommonWarnings returns the pre-flight warnings every binding kind shares,
// from the target revision's status and how the target compares with the
// binding's current spec.
func CommonWarnings(target int64, status openchoreov1alpha1.BindingRevisionStatus,
	noSpecChange, unrecordedChange bool) []Warning {
	var warnings []Warning
	if !status.KnownGood {
		warnings = append(warnings, Warning{Code: WarningTargetNotKnownGood,
			Message: fmt.Sprintf("revision %d never stayed Healthy for the known-good window", target)})
	}
	if status.Health == openchoreov1alpha1.RevisionHealthDegraded || status.Health == openchoreov1alpha1.RevisionHealthFailed {
		warnings = append(warnings, Warning{Code: WarningTargetUnhealthy,
			Message: fmt.Sprintf("revision %d was %s when it stopped being active", target, status.Health)})
	}
	if noSpecChange {
		warnings = append(warnings, Warning{Code: WarningNoSpecChange,
			Message: fmt.Sprintf("revision %d records the current spec; nothing changes", target)})
	}
	if unrecordedChange {
		warnings = append(warnings, Warning{Code: WarningUnrecordedChange,
			Message: "the binding has a spec change that is not recorded as a revision yet; the rollback discards it"})
	}
	return warnings
}

// AutoDeployBlocksRollback reports whether a rollback of a binding in
// environment must be refused because auto-deploy would undo it, and if so
// names what auto-deploys it for the error message: the component, or the
// release line of the component the binding belongs to. It mirrors the
// controllers that auto-deploy. The component controller keeps the main
// line's binding in the deployment pipeline's root (first) environment on the
// component's latest release when spec.autoDeploy is true. The ReleaseLine
// controller does the same for a release line's binding, with the line's
// spec.autoDeploy overriding the component's when set. Both reconcile as soon
// as that binding changes. Bindings in any other environment, and lines
// without auto-deploy, are never moved. When the release line, project or
// pipeline cannot be resolved the controllers do not auto-deploy either, so
// that does not block.
func AutoDeployBlocksRollback(ctx context.Context, c client.Client, namespace, componentName, releaseLine,
	environment string) (string, error) {
	comp := &openchoreov1alpha1.Component{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, comp); err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get component: %w", err)
	}
	autoDeploy := comp.Spec.AutoDeploy
	deployer := fmt.Sprintf("component %q", componentName)
	if line := releaseline.Normalize(releaseLine); line != "" {
		rl := &openchoreov1alpha1.ReleaseLine{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: line}, rl); err != nil {
			if apierrors.IsNotFound(err) {
				return "", nil
			}
			return "", fmt.Errorf("failed to get release line: %w", err)
		}
		if rl.Spec.Owner.ComponentName != comp.Name || rl.Spec.Owner.ProjectName != comp.Spec.Owner.ProjectName {
			return "", nil
		}
		if rl.Spec.AutoDeploy != nil {
			autoDeploy = *rl.Spec.AutoDeploy
		}
		deployer = fmt.Sprintf("release line %q of component %q", line, componentName)
	}
	if !autoDeploy {
		return "", nil
	}
	project := &openchoreov1alpha1.Project{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: comp.Spec.Owner.ProjectName}, project); err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get project: %w", err)
	}
	if project.Spec.DeploymentPipelineRef.Name == "" {
		return "", nil
	}
	pipeline := &openchoreov1alpha1.DeploymentPipeline{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: project.Spec.DeploymentPipelineRef.Name},
		pipeline); err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get deployment pipeline: %w", err)
	}
	root, err := pipeline.RootEnvironment()
	if err != nil {
		return "", nil //nolint:nilerr // an invalid pipeline means no auto-deploy, as in the controller
	}
	if environment != root {
		return "", nil
	}
	return deployer, nil
}

// GitOpsManager returns the GitOps tool that applies obj from Git, as it is
// shown to people (`Flux Kustomization "flux-system/apps"`), or "" when obj
// carries no GitOps tracking metadata. Any one of a tool's keys is enough,
// whatever its value: partial or malformed tracking metadata was still written
// by a tool, so it is never taken to mean the binding is not managed from Git.
// The exception is a complete Argo CD tracking ID naming another object.
func GitOpsManager(obj metav1.Object) string {
	objLabels := obj.GetLabels()
	if m := fluxManager(objLabels, "Kustomization", labelKeyFluxKustomizationName,
		labelKeyFluxKustomizationNamespace); m != "" {
		return m
	}
	if m := fluxManager(objLabels, "HelmRelease", labelKeyFluxHelmReleaseName,
		labelKeyFluxHelmReleaseNamespace); m != "" {
		return m
	}
	if id, ok := obj.GetAnnotations()[annotationKeyArgoCDTrackingID]; ok {
		app, namespace, name, complete := parseArgoCDTrackingID(id)
		if complete && (namespace != obj.GetNamespace() || name != obj.GetName()) {
			// The ID tracks another object: a copy that kept the original's
			// annotation is not synced by Argo CD through it.
			return ""
		}
		if app != "" {
			return fmt.Sprintf("Argo CD application %q", app)
		}
		return "Argo CD"
	}
	return ""
}

// parseArgoCDTrackingID splits an Argo CD tracking ID,
// {app}:{group}/{kind}:{namespace}/{name}. complete is false when the ID does
// not have that form; app is then still returned when it can be read.
func parseArgoCDTrackingID(id string) (app, namespace, name string, complete bool) {
	app, rest, found := strings.Cut(id, ":")
	if !found {
		return "", "", "", false
	}
	groupKind, object, found := strings.Cut(rest, ":")
	if !found || !strings.Contains(groupKind, "/") {
		return app, "", "", false
	}
	namespace, name, found = strings.Cut(object, "/")
	if !found || name == "" {
		return app, "", "", false
	}
	return app, namespace, name, app != ""
}

// fluxManager describes the Flux object named by the nameKey and nsKey labels,
// "Flux" when only part of that name is recorded, or "" when neither label is set.
func fluxManager(objLabels map[string]string, kind, nameKey, nsKey string) string {
	name, hasName := objLabels[nameKey]
	ns, hasNS := objLabels[nsKey]
	switch {
	case !hasName && !hasNS:
		return ""
	case name != "" && ns != "":
		return fmt.Sprintf("Flux %s %q", kind, ns+"/"+name)
	default:
		return "Flux"
	}
}

// RejectGitOpsManaged returns ErrGitOpsManaged when a GitOps tool applies the
// binding from Git (GitOpsManager), and nil otherwise. A rollback writes the
// binding directly, so the tool would restore the spec in Git on its next sync
// and silently undo it: the rollback has to be made in Git instead. kind names
// the binding kind for people, e.g. "release binding".
func RejectGitOpsManaged(binding client.Object, kind string) error {
	manager := GitOpsManager(binding)
	if manager == "" {
		return nil
	}
	return fmt.Errorf("%w: %s %q is applied from Git by %s. Rollback through OpenChoreo revision history is not "+
		"supported for GitOps-managed bindings, because GitOps would restore the configuration in Git on its next "+
		"sync. Revert the binding's configuration in Git and let GitOps reconcile the change",
		ErrGitOpsManaged, kind, binding.GetName(), manager)
}

// ProjectCellDrift reports when the project's cell in the environment -- its
// ProjectReleaseBinding -- is bound to a ProjectRelease created after the target
// revision, so the restored spec would run in a cell it was never deployed with.
// It returns nil when there is no drift or no ProjectReleaseBinding.
func ProjectCellDrift(ctx context.Context, c client.Reader, namespace, project, environment string,
	target int64, recordedAt metav1.Time) (*Warning, error) {
	var bindings openchoreov1alpha1.ProjectReleaseBindingList
	if err := c.List(ctx, &bindings, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("list project release bindings: %w", err)
	}
	for i := range bindings.Items {
		prb := &bindings.Items[i]
		if prb.Spec.Owner.ProjectName != project || prb.Spec.Environment != environment || prb.Spec.ProjectRelease == "" {
			continue
		}
		release := &openchoreov1alpha1.ProjectRelease{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: prb.Spec.ProjectRelease}, release); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("get project release %q: %w", prb.Spec.ProjectRelease, err)
		}
		if release.CreationTimestamp.After(recordedAt.Time) {
			return &Warning{Code: WarningProjectCellDrift, Message: fmt.Sprintf(
				"project %q in environment %q is now bound to ProjectRelease %q, created after revision %d",
				project, environment, release.Name, target)}, nil
		}
		return nil, nil
	}
	return nil, nil
}

// PreviousRevision returns the number of the revision that was active before
// the binding's current revision, for a rollback that names no revision. It
// follows activation history (revision.PreviousActive), never revision-number
// order, and never substitutes another revision for a recorded one that is
// gone. When the current revision predates activation tracking and the history
// has gaps, the target is inferred from activation order and returned with a
// WarningPreviousRevisionInferred for the caller to show.
//
// It refuses while the binding's spec differs from its current revision's: the
// controller has not recorded the latest change yet, so "previous" would be
// measured from a revision that is about to stop being current.
func PreviousRevision(revs []Candidate, highest int64, bindingSpecHash string) (int64, *Warning, error) {
	activations := make([]revision.Activation, len(revs))
	for i := range revs {
		activations[i] = revision.Activation{Object: revs[i].Object, Number: revs[i].Number}
	}
	current, previous, inferred, err := revision.PreviousActive(activations, highest)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %w; name the revision to roll back to", ErrNoPreviousRevision, err)
	}
	for i := range revs {
		if revs[i].Number == current.Number && revs[i].SnapshotHash != bindingSpecHash {
			return 0, nil, fmt.Errorf("%w: the binding has a change the controller has not recorded yet",
				ErrConcurrentModification)
		}
	}
	if inferred != "" {
		return previous.Number, &Warning{Code: WarningPreviousRevisionInferred, Message: inferred}, nil
	}
	return previous.Number, nil, nil
}

// OwnedByBinding reports whether rev is a revision of the binding of the given
// kind and name: its controller reference names that binding. A component (or
// resource) can have several bindings in one environment, one per release
// line, so the environment alone does not tell their revisions apart.
func OwnedByBinding(rev metav1.Object, kind, name string) bool {
	ref := metav1.GetControllerOf(rev)
	return ref != nil && ref.Kind == kind && ref.Name == name
}

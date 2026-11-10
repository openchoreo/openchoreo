// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"errors"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// maxReportedReadFailures caps how many unreadable observed resources the
// ObservedResourcesRead condition names one by one.
const maxReportedReadFailures = 10

// effectiveMode returns the release's mode, Manage when unset.
func effectiveMode(release *openchoreov1alpha1.RenderedRelease) openchoreov1alpha1.RenderedReleaseMode {
	if release.Spec.Mode == "" {
		return openchoreov1alpha1.RenderedReleaseModeManage
	}
	return release.Spec.Mode
}

// objectIdentity names an object on a plane independently of the API version
// it is read in.
type objectIdentity struct {
	group, kind, namespace, name string
}

func (o objectIdentity) String() string {
	if o.namespace == "" {
		return fmt.Sprintf("%s %s", o.groupKind(), o.name)
	}
	return fmt.Sprintf("%s %s/%s", o.groupKind(), o.namespace, o.name)
}

func (o objectIdentity) groupKind() string {
	return schema.GroupKind{Group: o.group, Kind: o.kind}.String()
}

func identityOf(obj *unstructured.Unstructured) objectIdentity {
	gvk := obj.GroupVersionKind()
	return objectIdentity{group: gvk.Group, kind: gvk.Kind, namespace: obj.GetNamespace(), name: obj.GetName()}
}

// observedEntry is a spec.observedResources entry with its effective id.
type observedEntry struct {
	id  string
	gvk schema.GroupVersionKind
	// apiVersionErr is set when the entry's apiVersion does not parse. The CRD
	// pattern rejects such values, so this only guards against objects written
	// before it existed.
	apiVersionErr error
	namespace     string
	name          string
}

func (e observedEntry) identity() objectIdentity {
	return objectIdentity{group: e.gvk.Group, kind: e.gvk.Kind, namespace: e.namespace, name: e.name}
}

// describe names the entry in condition messages.
func (e observedEntry) describe() string {
	apiVersion := e.gvk.GroupVersion().String()
	if e.namespace == "" {
		return fmt.Sprintf("%s (%s %s %s)", e.id, apiVersion, e.gvk.Kind, e.name)
	}
	return fmt.Sprintf("%s (%s %s %s/%s)", e.id, apiVersion, e.gvk.Kind, e.namespace, e.name)
}

// resolveObservedResources returns the observed entries with their effective ids
// and, in spec order, every id that more than one entry ends up with.
//
// Ids that are set are unique among themselves (CEL), so a repeated id involves a
// generated one: a set id that equals the id generated for another entry, or one
// object listed twice without ids.
func resolveObservedResources(specs []openchoreov1alpha1.ObservedResource) ([]observedEntry, []string) {
	entries := make([]observedEntry, 0, len(specs))
	counts := make(map[string]int, len(specs))
	for _, spec := range specs {
		gv, err := schema.ParseGroupVersion(spec.APIVersion)
		if err != nil {
			gv = schema.GroupVersion{Version: spec.APIVersion}
		}
		entry := observedEntry{
			id:            spec.ID,
			gvk:           gv.WithKind(spec.Kind),
			apiVersionErr: err,
			namespace:     spec.Namespace,
			name:          spec.Name,
		}
		if entry.id == "" {
			entry.id = generateObservedResourceID(gv.Group, spec.Kind, spec.Namespace, spec.Name)
		}
		counts[entry.id]++
		entries = append(entries, entry)
	}

	var conflicts []string
	for _, entry := range entries {
		if counts[entry.id] > 1 {
			conflicts = append(conflicts, entry.id)
			counts[entry.id] = 0 // name each conflicting id once
		}
	}
	return entries, conflicts
}

// generateObservedResourceID returns the id of an observed resource that has none
// set: the lowercased kind and the name, with a short hash of the group, kind,
// namespace and name, such as "deployment-payments-api-1a2b3c4d".
//
// It depends on nothing but those four values, so an entry keeps its id across
// reconciles and across a change of API version. The kind and name alone can
// repeat (the same name in two namespaces, or the same kind in two groups), which
// is what the hash is for. Ids fit in a label value, like the ids of rendered
// resources, and long kinds and names are shortened to fit.
func generateObservedResourceID(group, kind, namespace, name string) string {
	hashInput := strings.Join([]string{group, kind, namespace, name}, "/")
	return dpkubernetes.GenerateK8sNameWithHashInput(dpkubernetes.MaxLabelNameLength, hashInput, kind, name)
}

// observedReadFailure records an observed resource that could not be read.
type observedReadFailure struct {
	entry  observedEntry
	reason controller.ConditionReason
	detail string
}

// observedReadResult is what one pass of reading the observed resources found.
type observedReadResult struct {
	// statuses holds one entry per observed resource, in spec order.
	statuses []openchoreov1alpha1.RenderedManifestStatus
	// failures holds the resources that could not be read, in spec order.
	failures []observedReadFailure
	// live holds the objects that were read.
	live []*unstructured.Unstructured
}

// readObservedResources reads every observed resource from the plane and reports
// it the way buildResourceStatus reports applied ones: its .status, its health
// from the same checks, and a LastObservedTime kept while neither changes.
//
// It only reads. An object that does not exist, a kind the plane does not serve,
// and a read the agent's RBAC forbids are reported as Unknown and recorded as
// failures for the ObservedResourcesRead condition.
func (r *Reconciler) readObservedResources(ctx context.Context, planeClient client.Client,
	old *openchoreov1alpha1.RenderedRelease, entries []observedEntry) observedReadResult {
	logger := log.FromContext(ctx)

	prev := make(map[observedStatusKey]*openchoreov1alpha1.RenderedManifestStatus, len(old.Status.ObservedResources))
	for i := range old.Status.ObservedResources {
		prev[observedStatusKeyOf(&old.Status.ObservedResources[i])] = &old.Status.ObservedResources[i]
	}

	result := observedReadResult{
		statuses: make([]openchoreov1alpha1.RenderedManifestStatus, 0, len(entries)),
	}
	for _, entry := range entries {
		status := openchoreov1alpha1.RenderedManifestStatus{
			ID:           entry.id,
			Group:        entry.gvk.Group,
			Version:      entry.gvk.Version,
			Kind:         entry.gvk.Kind,
			Name:         entry.name,
			Namespace:    entry.namespace,
			HealthStatus: openchoreov1alpha1.HealthStatusUnknown,
		}

		live, failure := readObservedResource(ctx, planeClient, entry)
		if failure != nil {
			logger.Info("Observed resource could not be read", "id", entry.id,
				"gvk", entry.gvk.String(), "namespace", entry.namespace, "name", entry.name,
				"reason", failure.reason, "detail", failure.detail)
			result.failures = append(result.failures, *failure)
			result.statuses = append(result.statuses, status)
			continue
		}

		status.Status, status.HealthStatus = liveStatusAndHealth(logger, entry.id, entry.gvk, live)
		status.LastObservedTime = nextLastObservedTime(prev[observedStatusKeyOf(&status)], status.Status, status.HealthStatus)
		result.statuses = append(result.statuses, status)
		result.live = append(result.live, live)
	}
	return result
}

// readObservedResource gets one observed object. It returns the object, or why it
// could not be read.
func readObservedResource(ctx context.Context, planeClient client.Client, entry observedEntry) (*unstructured.Unstructured, *observedReadFailure) {
	if entry.apiVersionErr != nil {
		return nil, &observedReadFailure{entry: entry, reason: ReasonObservedReadFailed,
			detail: fmt.Sprintf("invalid apiVersion: %v", entry.apiVersionErr)}
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(entry.gvk)
	err := planeClient.Get(ctx, client.ObjectKey{Namespace: entry.namespace, Name: entry.name}, obj)
	if err == nil {
		// Some clients drop the kind when decoding; the health check is chosen by it.
		obj.SetGroupVersionKind(entry.gvk)
		return obj, nil
	}

	failure := &observedReadFailure{entry: entry}
	switch {
	case apimeta.IsNoMatchError(err) || (apierrors.IsNotFound(err) && !namesObject(err)):
		failure.reason = ReasonObservedKindNotFound
		failure.detail = "kind is not served by the plane"
	case apierrors.IsNotFound(err):
		failure.reason = ReasonObservedResourceNotFound
		failure.detail = "not found"
	case apierrors.IsForbidden(err):
		failure.reason = ReasonObservedReadForbidden
		failure.detail = "forbidden by the plane agent's RBAC"
	default:
		failure.reason = ReasonObservedReadFailed
		failure.detail = err.Error()
	}
	return nil, failure
}

// namesObject reports whether a NotFound error is about a named object. The API
// server answers a request for a resource type it does not serve with a NotFound
// that names nothing, and a missing object with one that names it.
func namesObject(err error) bool {
	var status apierrors.APIStatus
	if !errors.As(err, &status) {
		return false
	}
	details := status.Status().Details
	return details != nil && details.Name != ""
}

// observedStatusKey pairs a previous status.observedResources entry with the
// current one: ids can repeat while they conflict, so the object is part of it.
type observedStatusKey struct {
	id                             string
	group, version, kind, ns, name string
}

func observedStatusKeyOf(s *openchoreov1alpha1.RenderedManifestStatus) observedStatusKey {
	return observedStatusKey{id: s.ID, group: s.Group, version: s.Version, kind: s.Kind, ns: s.Namespace, name: s.Name}
}

// setObservedConditions records how reading the observed resources went. Both
// conditions are present only while the release lists observed resources, so
// releases that do not use them are unchanged.
func setObservedConditions(release *openchoreov1alpha1.RenderedRelease, entries []observedEntry,
	failures []observedReadFailure, idConflicts []string) {
	if len(entries) == 0 {
		apimeta.RemoveStatusCondition(&release.Status.Conditions, string(ConditionObservedResourcesRead))
		apimeta.RemoveStatusCondition(&release.Status.Conditions, string(ConditionObservedResourceIDsUnique))
		return
	}

	if len(failures) == 0 {
		controller.MarkTrueCondition(release, ConditionObservedResourcesRead, ReasonObservedReadSucceeded,
			fmt.Sprintf("Read all %d observed resources", len(entries)))
	} else {
		controller.MarkFalseCondition(release, ConditionObservedResourcesRead, readFailureReason(failures),
			readFailureMessage(len(entries), failures))
	}

	if len(idConflicts) == 0 {
		controller.MarkTrueCondition(release, ConditionObservedResourceIDsUnique, ReasonObservedIDsUnique,
			"Every observed resource has its own id")
	} else {
		controller.MarkFalseCondition(release, ConditionObservedResourceIDsUnique, ReasonObservedIDConflict,
			fmt.Sprintf("More than one observed resource has the id %s. A set id equals one generated for "+
				"another entry, or one object is listed twice", strings.Join(idConflicts, ", ")))
	}
}

// readFailureReason returns the reason the failures share, or ReadFailed when
// they have more than one.
func readFailureReason(failures []observedReadFailure) controller.ConditionReason {
	reason := failures[0].reason
	for _, f := range failures[1:] {
		if f.reason != reason {
			return ReasonObservedReadFailed
		}
	}
	return reason
}

// readFailureMessage names each unreadable resource and why, in spec order, up
// to maxReportedReadFailures of them.
func readFailureMessage(total int, failures []observedReadFailure) string {
	parts := make([]string, 0, min(len(failures), maxReportedReadFailures))
	for i, f := range failures {
		if i == maxReportedReadFailures {
			break
		}
		parts = append(parts, fmt.Sprintf("%s: %s", f.entry.describe(), f.detail))
	}
	msg := fmt.Sprintf("%d of %d observed resources could not be read: %s", len(failures), total, strings.Join(parts, "; "))
	if extra := len(failures) - maxReportedReadFailures; extra > 0 {
		msg += fmt.Sprintf("; and %d more", extra)
	}
	return msg
}

// excludeObserved drops the objects that are listed as observed resources.
// Observed objects are never deleted, even one this release applied earlier and
// that still carries its labels.
func excludeObserved(objs []*unstructured.Unstructured, entries []observedEntry) []*unstructured.Unstructured {
	if len(entries) == 0 || len(objs) == 0 {
		return objs
	}
	observed := make(map[objectIdentity]bool, len(entries))
	for _, entry := range entries {
		observed[entry.identity()] = true
	}
	kept := make([]*unstructured.Unstructured, 0, len(objs))
	for _, obj := range objs {
		if !observed[identityOf(obj)] {
			kept = append(kept, obj)
		}
	}
	return kept
}

// checkObservedOverlap returns an error naming the first desired resource that is
// also listed as an observed resource. Applying it would write to an object the
// release only observes.
func checkObservedOverlap(desired []*unstructured.Unstructured, entries []observedEntry) error {
	if len(entries) == 0 {
		return nil
	}
	observed := make(map[objectIdentity]string, len(entries))
	for _, entry := range entries {
		observed[entry.identity()] = entry.id
	}
	for _, obj := range desired {
		if id, found := observed[identityOf(obj)]; found {
			return fmt.Errorf("resource %s (%s) is also observed resource %s, and observed resources are never written",
				obj.GetLabels()[labels.LabelKeyRenderedReleaseResourceID], identityOf(obj), id)
		}
	}
	return nil
}

// hasResurrectableLiveWorkload reports whether any of the given live objects is a
// Deployment or StatefulSet scaled to zero without being paused (see
// hasResurrectableWorkload).
func hasResurrectableLiveWorkload(live []*unstructured.Unstructured) bool {
	for _, obj := range live {
		if isResurrectable(obj) {
			return true
		}
	}
	return false
}

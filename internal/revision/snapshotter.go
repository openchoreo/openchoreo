// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package revision

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	// LabelKeyBindingUID selects the revisions of one binding. The UID is used
	// rather than the name because label values are capped at 63 characters and
	// because a binding deleted and recreated under the same name must not pick
	// up the old binding's revisions while garbage collection catches up.
	LabelKeyBindingUID = "openchoreo.dev/binding-uid"

	// LabelKeyReleaseBinding carries the owning ReleaseBinding's name, for humans
	// and kubectl selectors. Names longer than a label value allows are shortened
	// with LabelValue.
	LabelKeyReleaseBinding = "openchoreo.dev/release-binding"

	// LabelKeyResourceReleaseBinding carries the owning ResourceReleaseBinding's name.
	LabelKeyResourceReleaseBinding = "openchoreo.dev/resource-release-binding"

	// AnnotationKeyBindingGeneration records the binding metadata.generation at
	// which a revision last became active: when it was created, and again each
	// time a spec it records makes it active again. The revision carrying the
	// highest value is the binding's current revision. A reconcile working from
	// a binding no newer than that (informer cache lag) must not record or
	// activate anything.
	AnnotationKeyBindingGeneration = "openchoreo.dev/binding-generation"

	// AnnotationKeyPreviousRevision records, each time a revision becomes active,
	// the number of the revision that was active just before it: "0" when there
	// was none. It is rewritten with AnnotationKeyBindingGeneration, so on the
	// current revision it names the revision a rollback without an explicit
	// target returns to (see PreviousActive). Revision numbers are never reused,
	// so it cannot name a different revision later.
	AnnotationKeyPreviousRevision = "openchoreo.dev/previous-revision"

	// maxLabelValueLength is the Kubernetes label value limit.
	maxLabelValueLength = 63

	// EventReasonRevisionReactivated is emitted on a binding when its spec
	// returns to one an earlier revision recorded and that revision becomes
	// active again.
	EventReasonRevisionReactivated = "RevisionReactivated"

	// DefaultKnownGoodWindow is how long a revision must stay Healthy before it
	// is marked known-good, when no window is configured.
	DefaultKnownGoodWindow = 60 * time.Second
)

// Adapter supplies the kind-specific parts of recording a revision. B is the
// binding type and R the revision type it records; both are pointer types.
type Adapter[B client.Object, R client.Object] interface {
	// NewRevisionList returns an empty list of revisions.
	NewRevisionList() client.ObjectList
	// Items returns the revisions held by a list from NewRevisionList.
	Items(list client.ObjectList) []R
	// Number returns spec.revision.
	Number(rev R) int64
	// HighestRevision returns the binding's status.highestRevision.
	HighestRevision(binding B) int64
	// CurrentRevision returns the binding's status.currentRevision.
	CurrentRevision(binding B) string
	// SnapshotHash returns spec.snapshotHash.
	SnapshotHash(rev R) string
	// RecordedCause returns spec.cause.
	RecordedCause(rev R) openchoreov1alpha1.RevisionCause
	// Status returns the revision's status for in-place modification.
	Status(rev R) *openchoreov1alpha1.BindingRevisionStatus
	// HashInput returns the part of the binding spec whose changes create a revision.
	HashInput(binding B) any
	// Labels returns the kind-specific labels for the binding's revisions.
	Labels(binding B) map[string]string
	// Build returns a revision with its spec filled in. Metadata is set by the Snapshotter.
	Build(binding B, number int64, cause openchoreov1alpha1.RevisionCause, hash string) R
	// Cause classifies a spec change from the spec alone, for changes the
	// binding's cause annotation does not decide. latest is the revision being
	// replaced, the binding's current one; hasLatest is false when the binding
	// has no revision yet.
	Cause(binding B, latest R, hasLatest bool) openchoreov1alpha1.RevisionCause
	// KnownGoodEligible reports whether the revision can become known-good. A
	// revision that undeploys is not a rollback target, however healthy.
	KnownGoodEligible(rev R) bool
}

// Options configures a Snapshotter.
type Options struct {
	// KnownGoodWindow is how long the active revision must stay Healthy before
	// it is marked known-good. Zero marks it known-good on its first Healthy
	// observation.
	KnownGoodWindow time.Duration
	// Recorder receives events about the binding. Optional.
	Recorder record.EventRecorder
	// Now returns the current time. Defaults to time.Now.
	Now func() time.Time
	// APIReader reads revisions from the apiserver, bypassing the cache, when
	// the cached list is behind the binding: its status names a current
	// revision the list does not have yet. Defaults to the Client.
	APIReader client.Reader
}

// Result reports what Record did.
type Result struct {
	// Number is the revision that is active after Record, or 0 if none exists.
	Number int64
	// Name is the name of that revision, or empty if none exists. Bindings
	// publish it as status.currentRevision.
	Name string
	// Created is true when this call created the revision.
	Created bool
	// Reactivated is true when this call made an existing revision active again
	// because the binding spec returned to the one it records.
	Reactivated bool
	// Cause is the cause of a revision this call created or found already
	// created; empty when no new revision was needed.
	Cause openchoreov1alpha1.RevisionCause
	// Highest is the highest revision number the binding has recorded: its
	// status.highestRevision or any revision Record saw, whichever is higher.
	// Bindings publish it as status.highestRevision.
	Highest int64
}

// Snapshotter records each distinct binding spec as an immutable revision and
// keeps status.active pointing at the revision that records the current spec.
//
// It is idempotent: a revision's name is derived from the binding name and its
// number, so two attempts to record the same revision collide on the name and
// the second is treated as success. Revision specs are only ever written by
// Create; status is only ever written through the status subresource.
type Snapshotter[B client.Object, R client.Object] struct {
	Client  client.Client
	Scheme  *runtime.Scheme
	Adapter Adapter[B, R]
	Options
}

// Record makes the revision that records the binding's current spec the active
// one, creating it only when no revision records that spec yet.
//
// Revisions are unique per spec. When the spec returns to one an earlier
// revision recorded -- a rollback, or any change back to an identical spec --
// that revision becomes active again under its own name and number, and nothing
// is created. A spec no revision records becomes a new revision numbered one
// above the highest, carrying the binding's change message. Every other revision
// is marked inactive, and once the current spec is recorded the binding's cause
// and change-message annotations are removed.
//
// Which revision is active only changes for a binding read newer than the
// generation the current revision was activated at (AnnotationKeyBindingGeneration):
// an older read is informer cache lag and changes nothing. The revisions are
// read from the apiserver instead of the cache when the cache is behind the
// binding (see listForRecord), so a revision the cache has not seen yet is
// never recorded a second time.
//
// Removing the annotation patches the binding's metadata. Only the binding's
// annotations and resourceVersion are refreshed from the response, so status
// the caller has changed in memory is kept and its later status update does not
// conflict.
func (s *Snapshotter[B, R]) Record(ctx context.Context, binding B) (Result, error) {
	revs, err := s.listForRecord(ctx, binding)
	if err != nil {
		return Result{}, err
	}
	current, hasCurrent := s.current(revs)

	hash, err := CanonicalHash(s.Adapter.HashInput(binding))
	if err != nil {
		return Result{}, fmt.Errorf("hash binding spec: %w", err)
	}

	result := Result{Highest: s.highest(binding, revs)}
	if hasCurrent {
		result.Number = s.Adapter.Number(current)
		result.Name = current.GetName()
	}
	// previous is the number of the revision this call replaces, if it makes
	// another one active.
	previous := result.Number
	// fresh reports whether this binding read may change the active revision.
	fresh := !hasCurrent || binding.GetGeneration() > recordedGeneration(current)

	// recorded reports whether a revision of the binding's current spec exists,
	// which is when the cause and change-message annotations have served their
	// purpose. A stale binding read whose spec is not recorded keeps them for the
	// reconcile that records it.
	recorded := false
	// reactivate is the existing revision this call makes active again, if any.
	var reactivate R
	if match, ok := s.matching(revs, hash); ok {
		recorded = true
		if s.Adapter.Number(match) != result.Number && fresh {
			reactivate = match
			result.Number = s.Adapter.Number(match)
			result.Name = match.GetName()
			result.Reactivated = true
		}
	} else if fresh {
		number, rev, found, created, err := s.create(ctx, binding, revs, current, hasCurrent, hash)
		if err != nil {
			return result, err
		}
		result.Number = number
		result.Name = Name(binding.GetName(), number)
		result.Created = created
		result.Highest = max(result.Highest, number)
		// A revision found already created is only projected when this client can
		// read it; otherwise the next reconcile lists it and projects it then.
		if found {
			result.Cause = s.Adapter.RecordedCause(rev)
			revs = append(revs, rev)
			recorded = s.Adapter.SnapshotHash(rev) == hash
		}
	}

	// A revision made active again starts a new health window, even if its
	// status still reads active because deactivating it failed earlier.
	statusErr := s.projectStatus(ctx, revs, result.Number, result.Reactivated)
	if result.Reactivated {
		// The revision is marked as the binding's current one only once its
		// status is written. Until then, the binding still reads as moving to
		// it, so a reconcile after a failed status write reactivates it again
		// and restarts its health window, instead of finding it current with a
		// window carried over from before.
		if statusErr != nil {
			return result, statusErr
		}
		if err := s.markActivated(ctx, reactivate, binding.GetGeneration(), previous); err != nil {
			return result, err
		}
		if s.Recorder != nil {
			// No revision records this change, so its message goes on the event.
			note := ""
			if msg := ChangeMessage(binding.GetAnnotations()); msg != "" {
				note = fmt.Sprintf(". Message: %q", msg)
			}
			s.Recorder.Eventf(binding, corev1.EventTypeNormal, EventReasonRevisionReactivated,
				"Revision %d (%s) is active again: the binding spec matches the one it recorded%s",
				result.Number, result.Name, note)
		}
	}
	errs := []error{statusErr}
	if recorded {
		errs = append(errs, s.clearChangeAnnotations(ctx, binding))
	}
	return result, kerrors.NewAggregate(errs)
}

// ObserveHealth records health on the binding's active revision -- the current
// one, and only when it records the binding's current spec. Every other revision
// keeps the health it last had when it stopped being active.
//
// Health and observedAt change together, and only when the health changes.
// healthySince is set when the revision becomes Healthy and cleared when it
// leaves Healthy. Once the revision has been Healthy for KnownGoodWindow it is
// marked known-good, and stays known-good whatever happens next. While that
// window is still running, ObserveHealth returns how long is left, for the
// caller to requeue after; nothing here sleeps.
func (s *Snapshotter[B, R]) ObserveHealth(ctx context.Context, binding B,
	health openchoreov1alpha1.RevisionHealth) (time.Duration, error) {
	revs, err := s.list(ctx, s.Client, binding)
	if err != nil {
		return 0, err
	}
	active, ok := s.current(revs)
	if !ok {
		return 0, nil
	}
	hash, err := CanonicalHash(s.Adapter.HashInput(binding))
	if err != nil {
		return 0, fmt.Errorf("hash binding spec: %w", err)
	}
	if s.Adapter.SnapshotHash(active) != hash {
		// The current spec has no revision yet; the health describes a spec the
		// active revision does not record.
		return 0, nil
	}
	if !s.Adapter.Status(active).Active {
		// Its status is not yet marked active, as when a write failed: its
		// health window starts once it is.
		return 0, nil
	}

	eligible := s.Adapter.KnownGoodEligible(active)
	var requeueAfter time.Duration
	err = s.updateStatus(ctx, active, func(status *openchoreov1alpha1.BindingRevisionStatus) bool {
		requeueAfter = 0
		now := metav1.NewTime(s.now())
		changed := false

		if status.Health != health {
			status.Health = health
			status.ObservedAt = &now
			changed = true
		}
		switch {
		case health == openchoreov1alpha1.RevisionHealthHealthy && status.HealthySince == nil:
			status.HealthySince = &now
			changed = true
		case health != openchoreov1alpha1.RevisionHealthHealthy && status.HealthySince != nil:
			status.HealthySince = nil
			changed = true
		}

		if !status.KnownGood && eligible && health == openchoreov1alpha1.RevisionHealthHealthy {
			if remaining := s.KnownGoodWindow - now.Sub(status.HealthySince.Time); remaining <= 0 {
				status.KnownGood = true
				changed = true
			} else {
				requeueAfter = remaining
			}
		}
		return changed
	})
	if err != nil {
		return 0, fmt.Errorf("update status of revision %q: %w", active.GetName(), err)
	}
	return requeueAfter, nil
}

// Deactivate marks every revision of the binding inactive, keeping their
// health as last observed. A binding that stops applying revisions (one that
// switches to observing resources it does not manage) calls it so that no
// revision reads active while none is applied. It is safe to call on every
// reconcile: only revisions still marked active are written.
func (s *Snapshotter[B, R]) Deactivate(ctx context.Context, binding B) error {
	revs, err := s.list(ctx, s.Client, binding)
	if err != nil {
		return err
	}
	return s.projectStatus(ctx, revs, 0, false)
}

// listForRecord returns the binding's revisions for Record. They are listed
// from the cache, unless the cache is behind the binding: the binding's status
// names a current revision the cached list does not have. Revisions and
// bindings are cached by separate informers, so a binding whose status already
// counts a new revision can arrive before that revision does; recording from
// that list would record the same spec again under the next number. The list
// is then read from the apiserver, which also tells a revision not cached yet
// from one that was deleted.
func (s *Snapshotter[B, R]) listForRecord(ctx context.Context, binding B) ([]R, error) {
	revs, err := s.list(ctx, s.Client, binding)
	if err != nil {
		return nil, err
	}
	current := s.Adapter.CurrentRevision(binding)
	if current == "" || slices.ContainsFunc(revs, func(rev R) bool { return rev.GetName() == current }) {
		return revs, nil
	}
	reader := s.APIReader
	if reader == nil {
		reader = s.Client
	}
	return s.list(ctx, reader, binding)
}

// list returns the binding's revisions, read through reader: those carrying
// its UID label whose controller reference is the binding.
func (s *Snapshotter[B, R]) list(ctx context.Context, reader client.Reader, binding B) ([]R, error) {
	list := s.Adapter.NewRevisionList()
	if err := reader.List(ctx, list,
		client.InNamespace(binding.GetNamespace()),
		client.MatchingLabels{LabelKeyBindingUID: string(binding.GetUID())},
	); err != nil {
		return nil, fmt.Errorf("list revisions: %w", err)
	}

	items := s.Adapter.Items(list)
	owned := make([]R, 0, len(items))
	for _, rev := range items {
		if ref := metav1.GetControllerOf(rev); ref != nil && ref.UID == binding.GetUID() {
			owned = append(owned, rev)
		}
	}
	return owned, nil
}

// highest returns the highest revision number the binding has recorded: its
// status.highestRevision, or the highest spec.revision among revs when a
// revision is numbered above it (one created before the status was written).
// Deleted revisions only count through the status, which is why it is kept.
func (s *Snapshotter[B, R]) highest(binding B, revs []R) int64 {
	highest := s.Adapter.HighestRevision(binding)
	for _, rev := range revs {
		highest = max(highest, s.Adapter.Number(rev))
	}
	return highest
}

// current returns the revision most recently made active: the one with the
// highest AnnotationKeyBindingGeneration, the highest spec.revision breaking
// ties. Without reactivations this is the latest revision.
func (s *Snapshotter[B, R]) current(revs []R) (R, bool) {
	var current R
	found := false
	for _, rev := range revs {
		if !found {
			current, found = rev, true
			continue
		}
		if MoreRecentlyActive(rev, s.Adapter.Number(rev), current, s.Adapter.Number(current)) {
			current = rev
		}
	}
	return current, found
}

// matching returns the revision that records the spec with this hash. Specs
// are unique per binding, so there is at most one; should there be more, the
// newest is used.
func (s *Snapshotter[B, R]) matching(revs []R, hash string) (R, bool) {
	var match R
	found := false
	for _, rev := range revs {
		if s.Adapter.SnapshotHash(rev) != hash {
			continue
		}
		if !found || s.Adapter.Number(rev) > s.Adapter.Number(match) {
			match, found = rev, true
		}
	}
	return match, found
}

// markActivated records on the revision the binding generation it became
// active at, making it the binding's current revision, and the number of the
// revision it replaces (0 for none). Only metadata is patched; the spec stays
// immutable.
func (s *Snapshotter[B, R]) markActivated(ctx context.Context, rev R, generation, previous int64) error {
	patched, ok := rev.DeepCopyObject().(R)
	if !ok {
		return fmt.Errorf("copy revision %q", rev.GetName())
	}
	annotations := make(map[string]string, len(rev.GetAnnotations())+1)
	for k, v := range rev.GetAnnotations() {
		annotations[k] = v
	}
	annotations[AnnotationKeyBindingGeneration] = strconv.FormatInt(generation, 10)
	annotations[AnnotationKeyPreviousRevision] = strconv.FormatInt(previous, 10)
	patched.SetAnnotations(annotations)
	if err := s.Client.Patch(ctx, patched, client.MergeFrom(rev)); err != nil {
		return fmt.Errorf("activate revision %q: %w", rev.GetName(), err)
	}
	rev.SetAnnotations(patched.GetAnnotations())
	rev.SetResourceVersion(patched.GetResourceVersion())
	return nil
}

// create builds and creates the binding's next revision, numbered one above
// the highest it has recorded (see highest), so each binding counts r1, r2,
// r3... on its own and never reuses the number of a deleted revision. current
// is the revision it replaces, used to classify the change.
// It returns the revision number and, when found is true, the stored revision:
// the one created, or the one that already existed under that name. found is
// false when the existing revision cannot be read yet.
func (s *Snapshotter[B, R]) create(ctx context.Context, binding B, revs []R, current R, hasCurrent bool,
	hash string) (number int64, rev R, found, created bool, err error) {
	number = s.highest(binding, revs) + 1

	built := s.Adapter.Build(binding, number, s.cause(binding, current, hasCurrent), hash)
	built.SetName(Name(binding.GetName(), number))
	built.SetNamespace(binding.GetNamespace())

	revLabels := s.Adapter.Labels(binding)
	revLabels[LabelKeyBindingUID] = string(binding.GetUID())
	built.SetLabels(revLabels)
	previous := int64(0)
	if hasCurrent {
		previous = s.Adapter.Number(current)
	}
	built.SetAnnotations(map[string]string{
		AnnotationKeyBindingGeneration: strconv.FormatInt(binding.GetGeneration(), 10),
		AnnotationKeyPreviousRevision:  strconv.FormatInt(previous, 10),
	})
	if err := controllerutil.SetControllerReference(binding, built, s.Scheme); err != nil {
		return number, rev, false, false, fmt.Errorf("set owner reference on revision %q: %w", built.GetName(), err)
	}

	if err := s.Client.Create(ctx, built); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return number, rev, false, false, fmt.Errorf("create revision %q: %w", built.GetName(), err)
		}
		// Another attempt recorded this revision number first: a retry after a lost
		// response, or a reconcile working from a cache that had not yet seen it.
		// When it records this spec, that is success; the stored revision is read
		// back, never overwritten.
		existing, ok := built.DeepCopyObject().(R)
		if !ok {
			return number, rev, false, false, fmt.Errorf("copy revision %q", built.GetName())
		}
		if err := s.Client.Get(ctx, client.ObjectKeyFromObject(built), existing); err != nil {
			if apierrors.IsNotFound(err) {
				return number, rev, false, false, nil
			}
			return number, rev, false, false, fmt.Errorf("get existing revision %q: %w", built.GetName(), err)
		}
		// The name can also be held by a revision of an earlier binding with the
		// same name, deleted and recreated before garbage collection removed its
		// revisions. That revision is not ours: retry until it is gone.
		if ref := metav1.GetControllerOf(existing); ref == nil || ref.UID != binding.GetUID() {
			return number, rev, false, false, fmt.Errorf(
				"revision name %q is held by a revision of another binding; waiting for it to be garbage collected", built.GetName())
		}
		// A revision of this binding that records another spec: the revisions
		// this reconcile listed are out of date. It is not this spec's revision;
		// the retry lists the revisions again and numbers this spec above it.
		if s.Adapter.SnapshotHash(existing) != hash {
			return number, rev, false, false, fmt.Errorf(
				"revision %q already records a different spec; the revision list was out of date", built.GetName())
		}
		return number, existing, true, false, nil
	}
	return number, built, true, true, nil
}

// cause classifies the change. A Promote cause annotation decides a binding's
// first revision; anything else is classified by the adapter from the spec.
func (s *Snapshotter[B, R]) cause(binding B, current R, hasCurrent bool) openchoreov1alpha1.RevisionCause {
	if !hasCurrent && openchoreov1alpha1.RevisionCause(
		binding.GetAnnotations()[openchoreov1alpha1.AnnotationKeyRevisionCause]) == openchoreov1alpha1.RevisionCausePromote {
		return openchoreov1alpha1.RevisionCausePromote
	}
	return s.Adapter.Cause(binding, current, hasCurrent)
}

// changeAnnotations are set on a binding to describe one change and removed
// once a revision records it.
var changeAnnotations = []string{
	openchoreov1alpha1.AnnotationKeyRevisionCause,
	openchoreov1alpha1.AnnotationKeyChangeMessage,
}

// clearChangeAnnotations removes the cause and change-message annotations from
// the binding, if it has any. The patch is applied to a copy; only annotations
// and resourceVersion are carried back to binding.
//
// The patch carries the binding's resourceVersion. If the binding changed after
// it was read -- a new spec and change message written meanwhile -- the patch
// fails with a conflict instead of removing annotations meant for that newer
// change; the next reconcile records the newer spec and removes them then.
func (s *Snapshotter[B, R]) clearChangeAnnotations(ctx context.Context, binding B) error {
	current := binding.GetAnnotations()
	found := false
	for _, k := range changeAnnotations {
		if _, ok := current[k]; ok {
			found = true
		}
	}
	if !found {
		return nil
	}

	patched, ok := binding.DeepCopyObject().(client.Object)
	if !ok {
		return fmt.Errorf("copy binding %q", binding.GetName())
	}
	remaining := make(map[string]string, len(current))
	for k, v := range current {
		if !slices.Contains(changeAnnotations, k) {
			remaining[k] = v
		}
	}
	patched.SetAnnotations(remaining)

	if err := s.Client.Patch(ctx, patched, client.MergeFromWithOptions(binding, client.MergeFromWithOptimisticLock{})); err != nil {
		return fmt.Errorf("remove revision change annotations: %w", err)
	}
	binding.SetAnnotations(patched.GetAnnotations())
	binding.SetResourceVersion(patched.GetResourceVersion())
	return nil
}

// projectStatus makes the revision numbered active the only active revision;
// 0 makes none active. A revision becoming active starts at health
// Progressing: a new one, and one made active again, since its spec is being
// applied anew. restart starts the active revision over in the same way even
// when its status already reads active, as it does when deactivating it failed
// before it was made active again. Its known-good mark is kept. A revision that
// stops being active keeps its health as last observed. Only revisions whose
// status changes are written, so this is safe to run on every reconcile.
//
// The other revisions are marked inactive before the active one is marked
// active, so a client reading between the writes sees at most one active
// revision, never the old and the new together. When a deactivation fails, the
// active one is not written and the error is returned, so a retry completes the
// deactivation first.
func (s *Snapshotter[B, R]) projectStatus(ctx context.Context, revs []R, active int64, restart bool) error {
	var errs []error
	project := func(rev R, want bool) {
		err := s.updateStatus(ctx, rev, func(status *openchoreov1alpha1.BindingRevisionStatus) bool {
			start := want && (restart || !status.Active || status.Health == "")
			if status.Active == want && !start {
				return false
			}
			if start {
				now := metav1.NewTime(s.now())
				status.Health = openchoreov1alpha1.RevisionHealthProgressing
				status.ObservedAt = &now
				status.HealthySince = nil
			}
			status.Active = want
			return true
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("update status of revision %q: %w", rev.GetName(), err))
		}
	}
	for _, rev := range revs {
		if s.Adapter.Number(rev) != active {
			project(rev, false)
		}
	}
	if len(errs) > 0 {
		return kerrors.NewAggregate(errs)
	}
	for _, rev := range revs {
		if s.Adapter.Number(rev) == active {
			project(rev, true)
		}
	}
	return kerrors.NewAggregate(errs)
}

// updateStatus applies mutate to the revision's status and writes it through
// the status subresource when mutate reports a change. On a conflict it reads
// the revision again and reapplies mutate to the fresh status, so concurrent
// writers never overwrite each other's changes.
func (s *Snapshotter[B, R]) updateStatus(ctx context.Context, rev R,
	mutate func(status *openchoreov1alpha1.BindingRevisionStatus) bool) error {
	first := true
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if !first {
			if err := s.Client.Get(ctx, client.ObjectKeyFromObject(rev), rev); err != nil {
				return err
			}
		}
		first = false
		if !mutate(s.Adapter.Status(rev)) {
			return nil
		}
		return s.Client.Status().Update(ctx, rev)
	})
}

// now returns the current time from Options.Now, or time.Now.
func (s *Snapshotter[B, R]) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// MoreRecentlyActive reports whether revision a, numbered aNumber, became
// active more recently than revision b, numbered bNumber: it carries a higher
// AnnotationKeyBindingGeneration, or the same one and a higher number. The
// binding's current revision is the one no other revision is more recently
// active than. Callers outside the controller use it to find that revision
// the same way the controller does.
func MoreRecentlyActive(a client.Object, aNumber int64, b client.Object, bNumber int64) bool {
	ga, gb := recordedGeneration(a), recordedGeneration(b)
	return ga > gb || (ga == gb && aNumber > bNumber)
}

// recordedGeneration returns the binding generation a revision last became
// active at, or 0 if it is not recorded.
func recordedGeneration(rev client.Object) int64 {
	gen, err := strconv.ParseInt(rev.GetAnnotations()[AnnotationKeyBindingGeneration], 10, 64)
	if err != nil {
		return 0
	}
	return gen
}

// LabelValue returns s if it is a valid label value, and otherwise a stable
// shortened form: a prefix of s followed by "-" and 10 hex characters of the
// SHA-256 of s. Kubernetes object names only contain characters that are
// valid in label values, so the result is valid for any object name.
func LabelValue(s string) string {
	if len(s) <= maxLabelValueLength {
		return s
	}
	sum := sha256.Sum256([]byte(s))
	hash := hex.EncodeToString(sum[:])[:10]
	prefix := strings.TrimRight(s[:maxLabelValueLength-len(hash)-1], "-._")
	return prefix + "-" + hash
}

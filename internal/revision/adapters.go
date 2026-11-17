// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package revision

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// NewReleaseBindingSnapshotter returns a Snapshotter that records ReleaseBindings
// as ReleaseBindingRevisions.
func NewReleaseBindingSnapshotter(c client.Client, scheme *runtime.Scheme, opts Options) *Snapshotter[*openchoreov1alpha1.ReleaseBinding, *openchoreov1alpha1.ReleaseBindingRevision] {
	return &Snapshotter[*openchoreov1alpha1.ReleaseBinding, *openchoreov1alpha1.ReleaseBindingRevision]{
		Client:  c,
		Scheme:  scheme,
		Adapter: releaseBindingAdapter{},
		Options: opts,
	}
}

// NewResourceReleaseBindingSnapshotter returns a Snapshotter that records
// ResourceReleaseBindings as ResourceReleaseBindingRevisions.
func NewResourceReleaseBindingSnapshotter(c client.Client, scheme *runtime.Scheme, opts Options) *Snapshotter[*openchoreov1alpha1.ResourceReleaseBinding, *openchoreov1alpha1.ResourceReleaseBindingRevision] {
	return &Snapshotter[*openchoreov1alpha1.ResourceReleaseBinding, *openchoreov1alpha1.ResourceReleaseBindingRevision]{
		Client:  c,
		Scheme:  scheme,
		Adapter: resourceReleaseBindingAdapter{},
		Options: opts,
	}
}

// ─────────────────────────────────────────────────────────────
// ReleaseBinding
// ─────────────────────────────────────────────────────────────

type releaseBindingAdapter struct{}

var _ Adapter[*openchoreov1alpha1.ReleaseBinding, *openchoreov1alpha1.ReleaseBindingRevision] = releaseBindingAdapter{}

func (releaseBindingAdapter) NewRevisionList() client.ObjectList {
	return &openchoreov1alpha1.ReleaseBindingRevisionList{}
}

func (releaseBindingAdapter) Items(list client.ObjectList) []*openchoreov1alpha1.ReleaseBindingRevision {
	l := list.(*openchoreov1alpha1.ReleaseBindingRevisionList)
	items := make([]*openchoreov1alpha1.ReleaseBindingRevision, len(l.Items))
	for i := range l.Items {
		items[i] = &l.Items[i]
	}
	return items
}

func (releaseBindingAdapter) Number(rev *openchoreov1alpha1.ReleaseBindingRevision) int64 {
	return rev.Spec.Revision
}

func (releaseBindingAdapter) HighestRevision(binding *openchoreov1alpha1.ReleaseBinding) int64 {
	return binding.Status.HighestRevision
}

func (releaseBindingAdapter) SnapshotHash(rev *openchoreov1alpha1.ReleaseBindingRevision) string {
	return rev.Spec.SnapshotHash
}

func (releaseBindingAdapter) RecordedCause(rev *openchoreov1alpha1.ReleaseBindingRevision) openchoreov1alpha1.RevisionCause {
	return rev.Spec.Cause
}

func (releaseBindingAdapter) Status(rev *openchoreov1alpha1.ReleaseBindingRevision) *openchoreov1alpha1.BindingRevisionStatus {
	return &rev.Status
}

// KnownGoodEligible excludes revisions that undeploy: one is healthy once its
// resources are gone, which makes it no target to roll back to.
func (releaseBindingAdapter) KnownGoodEligible(rev *openchoreov1alpha1.ReleaseBindingRevision) bool {
	return rev.Spec.BindingSpec.State != openchoreov1alpha1.ReleaseStateUndeploy
}

// HashInput is the recorded part of the binding spec: every field that decides
// what is deployed. See ReleaseBindingRecordedSpec.
func (releaseBindingAdapter) HashInput(binding *openchoreov1alpha1.ReleaseBinding) any {
	return releaseBindingHashInput(&binding.Spec)
}

func releaseBindingHashInput(spec *openchoreov1alpha1.ReleaseBindingSpec) any {
	return ReleaseBindingRecordedSpec(spec)
}

// ReleaseBindingRecordedSpec returns the part of spec a revision records: a copy
// of spec without the fields that configure the history itself and not what is
// deployed (revisionHistoryLimit). A revision is hashed from, and stores, this
// part only, as a Deployment's revision is its pod template and not its
// revisionHistoryLimit. Changing an excluded field therefore creates no
// revision, and a rollback must carry the binding's current value over.
func ReleaseBindingRecordedSpec(spec *openchoreov1alpha1.ReleaseBindingSpec) *openchoreov1alpha1.ReleaseBindingSpec {
	recorded := spec.DeepCopy()
	recorded.RevisionHistoryLimit = nil
	return recorded
}

func (releaseBindingAdapter) HistoryLimit(binding *openchoreov1alpha1.ReleaseBinding) *int32 {
	return binding.Spec.RevisionHistoryLimit
}

// ReleaseBindingSpecHash returns the snapshotHash a ReleaseBindingRevision of
// spec would carry. Two specs with the same hash record no new revision.
func ReleaseBindingSpecHash(spec *openchoreov1alpha1.ReleaseBindingSpec) (string, error) {
	return CanonicalHash(releaseBindingHashInput(spec))
}

func (releaseBindingAdapter) Labels(binding *openchoreov1alpha1.ReleaseBinding) map[string]string {
	return map[string]string{
		LabelKeyReleaseBinding:         LabelValue(binding.Name),
		labels.LabelKeyComponentName:   LabelValue(binding.Spec.Owner.ComponentName),
		labels.LabelKeyProjectName:     LabelValue(binding.Spec.Owner.ProjectName),
		labels.LabelKeyEnvironmentName: LabelValue(binding.Spec.Environment),
	}
}

func (releaseBindingAdapter) Build(binding *openchoreov1alpha1.ReleaseBinding, number int64,
	cause openchoreov1alpha1.RevisionCause, hash string) *openchoreov1alpha1.ReleaseBindingRevision {
	return &openchoreov1alpha1.ReleaseBindingRevision{
		Spec: openchoreov1alpha1.ReleaseBindingRevisionSpec{
			Revision:             number,
			Cause:                cause,
			ComponentReleaseName: binding.Spec.ReleaseName,
			SnapshotHash:         hash,
			Message:              ChangeMessage(binding.Annotations),
			BindingSpec:          *ReleaseBindingRecordedSpec(&binding.Spec),
		},
	}
}

// Cause classifies a ReleaseBinding spec change, checking in order:
//   - Undeploy: state switched to Undeploy (or the first revision is undeployed).
//   - Redeploy: state switched back from Undeploy.
//   - Deploy: the first revision, or releaseName changed.
//   - Override: anything else -- only per-environment overrides changed.
func (releaseBindingAdapter) Cause(binding *openchoreov1alpha1.ReleaseBinding,
	latest *openchoreov1alpha1.ReleaseBindingRevision, hasLatest bool) openchoreov1alpha1.RevisionCause {
	cur := binding.Spec
	undeployed := cur.State == openchoreov1alpha1.ReleaseStateUndeploy

	if !hasLatest {
		if undeployed {
			return openchoreov1alpha1.RevisionCauseUndeploy
		}
		return openchoreov1alpha1.RevisionCauseDeploy
	}

	prev := latest.Spec.BindingSpec
	wasUndeployed := prev.State == openchoreov1alpha1.ReleaseStateUndeploy
	switch {
	case undeployed && !wasUndeployed:
		return openchoreov1alpha1.RevisionCauseUndeploy
	case !undeployed && wasUndeployed:
		return openchoreov1alpha1.RevisionCauseRedeploy
	case cur.ReleaseName != prev.ReleaseName:
		return openchoreov1alpha1.RevisionCauseDeploy
	default:
		return openchoreov1alpha1.RevisionCauseOverride
	}
}

// ─────────────────────────────────────────────────────────────
// ResourceReleaseBinding
// ─────────────────────────────────────────────────────────────

type resourceReleaseBindingAdapter struct{}

var _ Adapter[*openchoreov1alpha1.ResourceReleaseBinding, *openchoreov1alpha1.ResourceReleaseBindingRevision] = resourceReleaseBindingAdapter{}

func (resourceReleaseBindingAdapter) NewRevisionList() client.ObjectList {
	return &openchoreov1alpha1.ResourceReleaseBindingRevisionList{}
}

func (resourceReleaseBindingAdapter) Items(list client.ObjectList) []*openchoreov1alpha1.ResourceReleaseBindingRevision {
	l := list.(*openchoreov1alpha1.ResourceReleaseBindingRevisionList)
	items := make([]*openchoreov1alpha1.ResourceReleaseBindingRevision, len(l.Items))
	for i := range l.Items {
		items[i] = &l.Items[i]
	}
	return items
}

func (resourceReleaseBindingAdapter) Number(rev *openchoreov1alpha1.ResourceReleaseBindingRevision) int64 {
	return rev.Spec.Revision
}

func (resourceReleaseBindingAdapter) HighestRevision(binding *openchoreov1alpha1.ResourceReleaseBinding) int64 {
	return binding.Status.HighestRevision
}

func (resourceReleaseBindingAdapter) SnapshotHash(rev *openchoreov1alpha1.ResourceReleaseBindingRevision) string {
	return rev.Spec.SnapshotHash
}

func (resourceReleaseBindingAdapter) RecordedCause(rev *openchoreov1alpha1.ResourceReleaseBindingRevision) openchoreov1alpha1.RevisionCause {
	return rev.Spec.Cause
}

func (resourceReleaseBindingAdapter) Status(rev *openchoreov1alpha1.ResourceReleaseBindingRevision) *openchoreov1alpha1.BindingRevisionStatus {
	return &rev.Status
}

// KnownGoodEligible is always true: ResourceReleaseBindings cannot undeploy.
func (resourceReleaseBindingAdapter) KnownGoodEligible(*openchoreov1alpha1.ResourceReleaseBindingRevision) bool {
	return true
}

// HashInput is the recorded part of the binding spec; see releaseBindingAdapter.HashInput.
func (resourceReleaseBindingAdapter) HashInput(binding *openchoreov1alpha1.ResourceReleaseBinding) any {
	return resourceReleaseBindingHashInput(&binding.Spec)
}

func resourceReleaseBindingHashInput(spec *openchoreov1alpha1.ResourceReleaseBindingSpec) any {
	return ResourceReleaseBindingRecordedSpec(spec)
}

// ResourceReleaseBindingRecordedSpec returns the part of spec a revision
// records; see ReleaseBindingRecordedSpec.
func ResourceReleaseBindingRecordedSpec(spec *openchoreov1alpha1.ResourceReleaseBindingSpec) *openchoreov1alpha1.ResourceReleaseBindingSpec {
	recorded := spec.DeepCopy()
	recorded.RevisionHistoryLimit = nil
	return recorded
}

func (resourceReleaseBindingAdapter) HistoryLimit(binding *openchoreov1alpha1.ResourceReleaseBinding) *int32 {
	return binding.Spec.RevisionHistoryLimit
}

// ResourceReleaseBindingSpecHash returns the snapshotHash a
// ResourceReleaseBindingRevision of spec would carry.
func ResourceReleaseBindingSpecHash(spec *openchoreov1alpha1.ResourceReleaseBindingSpec) (string, error) {
	return CanonicalHash(resourceReleaseBindingHashInput(spec))
}

func (resourceReleaseBindingAdapter) Labels(binding *openchoreov1alpha1.ResourceReleaseBinding) map[string]string {
	return map[string]string{
		LabelKeyResourceReleaseBinding: LabelValue(binding.Name),
		labels.LabelKeyResourceName:    LabelValue(binding.Spec.Owner.ResourceName),
		labels.LabelKeyProjectName:     LabelValue(binding.Spec.Owner.ProjectName),
		labels.LabelKeyEnvironmentName: LabelValue(binding.Spec.Environment),
	}
}

func (resourceReleaseBindingAdapter) Build(binding *openchoreov1alpha1.ResourceReleaseBinding, number int64,
	cause openchoreov1alpha1.RevisionCause, hash string) *openchoreov1alpha1.ResourceReleaseBindingRevision {
	return &openchoreov1alpha1.ResourceReleaseBindingRevision{
		Spec: openchoreov1alpha1.ResourceReleaseBindingRevisionSpec{
			Revision:            number,
			Cause:               cause,
			ResourceReleaseName: binding.Spec.ResourceRelease,
			SnapshotHash:        hash,
			Message:             ChangeMessage(binding.Annotations),
			BindingSpec:         *ResourceReleaseBindingRecordedSpec(&binding.Spec),
		},
	}
}

// Cause classifies a ResourceReleaseBinding spec change. ResourceReleaseBindings
// have no state field, so the only spec-derived causes are Deploy (the first
// revision, or the pinned resourceRelease changed) and Override.
func (resourceReleaseBindingAdapter) Cause(binding *openchoreov1alpha1.ResourceReleaseBinding,
	latest *openchoreov1alpha1.ResourceReleaseBindingRevision, hasLatest bool) openchoreov1alpha1.RevisionCause {
	if !hasLatest || latest.Spec.BindingSpec.ResourceRelease != binding.Spec.ResourceRelease {
		return openchoreov1alpha1.RevisionCauseDeploy
	}
	return openchoreov1alpha1.RevisionCauseOverride
}

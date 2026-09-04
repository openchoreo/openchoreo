// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// Delivery lifecycle event reasons consumed by the observer's DORA aggregator
// (internal/observer/aggregator). Reason strings and the payload schema are a
// contract with that consumer.
const (
	reasonDeploymentStarted   = "DeploymentStarted"
	reasonDeploymentSucceeded = "DeploymentSucceeded"
	reasonDeploymentFailed    = "DeploymentFailed"
	reasonDeploymentRecovered = "DeploymentRecovered"

	// deliveryReportingController identifies this controller as the event author.
	deliveryReportingController = "openchoreo.dev/renderedrelease-controller"

	// failureReasonApplyFailed marks a rollout that never reached the data plane.
	failureReasonApplyFailed = "ApplyFailed"
	// failureReasonDegraded is the fallback when a degraded resource offers no
	// more specific reason.
	failureReasonDegraded = "Degraded"

	cronJobKind = "CronJob"

	// reasonProgressDeadlineExceeded is the Deployment Progressing condition
	// reason for a rollout that never became available.
	reasonProgressDeadlineExceeded = "ProgressDeadlineExceeded"
)

// deliveryEventPayload is embedded as JSON in the event message so the
// aggregator gets release and scope identity independent of collector
// enrichment (Kubernetes Events do not inherit the involved object's labels).
type deliveryEventPayload struct {
	RenderedReleaseUID   string `json:"renderedReleaseUid"`
	ComponentReleaseName string `json:"componentReleaseName"`
	// OrgNamespace is the control-plane namespace the rollout belongs to. It is in
	// the payload for the same reason the UIDs are: a Kubernetes Event does not
	// inherit the involved object's labels, so anything the consumer needs has to
	// travel in the message. It is the one field the store requires, and depending
	// on collector enrichment for it meant an un-enriched event could not be folded.
	OrgNamespace     string `json:"orgNamespace,omitempty"`
	ProjectUID       string `json:"projectUid,omitempty"`
	ComponentUID     string `json:"componentUid,omitempty"`
	EnvironmentUID   string `json:"environmentUid,omitempty"`
	Commit           string `json:"commit,omitempty"`
	CommitAuthoredAt string `json:"commitAuthoredAt,omitempty"`
	Phase            string `json:"phase"`
	FailureReason    string `json:"failureReason,omitempty"`
	// FailureEpisode identifies which failure->recovery cycle of this rollout the
	// event belongs to. The emitter already distinguishes episodes -- it suffixes
	// event names -e1, -e2 -- but a consumer keying a recovery on the rollout alone
	// merges them, and the merged duration then spans the healthy interval between
	// them. Carried on Failed and Recovered; zero on Started and Succeeded.
	FailureEpisode int32 `json:"failureEpisode,omitempty"`
}

// deliveryContext is everything needed to emit delivery events for one
// reconcile, resolved once up front. It exists only for component-owned
// data-plane releases that render a primary workload resource.
type deliveryContext struct {
	// rolloutID is the per-rollout identity: the immutable ComponentRelease UID
	// joined with the RenderedRelease UID. The RenderedRelease object is reused
	// across rollouts (named {component}-{environment}), so its UID alone cannot
	// identify a rollout; the ComponentRelease UID alone is shared by every
	// environment the release is bound to. The pair is unique and stable.
	rolloutID            string
	componentReleaseName string
	// primary is the desired primary workload resource (Deployment, StatefulSet,
	// or CronJob) the events anchor to as involvedObject.
	primary *unstructured.Unstructured
	// commit and commitAuthoredAt are the rollout's commit provenance, read from
	// the owning ComponentRelease by resolveDeliveryProvenance.
	//
	// They are NOT read off `primary`: the render pipeline injects
	// MetadataContext.Labels onto every resource but never its Annotations (see
	// postProcessResources, which calls addLabels only), so a commit placed in the
	// metadata context never reaches a rendered resource. The UID fields below do
	// come from labels, which is why those work. Reading the ComponentRelease
	// directly also drops a round-trip: the release is already identified by
	// LabelKeyComponentReleaseName.
	commit           string
	commitAuthoredAt string
}

// primaryWorkloadGVKs are the resource kinds whose health defines rollout
// outcome and which delivery events anchor to, mirroring GetHealthCheckFunc.
var primaryWorkloadGVKs = map[schema.GroupVersionKind]bool{
	{Group: appsAPIGroup, Version: "v1", Kind: deploymentKind}:  true,
	{Group: appsAPIGroup, Version: "v1", Kind: statefulSetKind}: true,
	{Group: "batch", Version: "v1", Kind: cronJobKind}:          true,
}

// deliveryContextFor resolves the delivery context, or nil when this release
// does not participate in delivery events (non-component owners, observability
// plane, no workload resource, or the ComponentRelease labels are not stamped
// yet by the releasebinding controller).
func deliveryContextFor(release *openchoreov1alpha1.RenderedRelease, desiredResources []*unstructured.Unstructured) *deliveryContext {
	if release.Spec.TargetPlane == targetPlaneObservabilityPlane {
		return nil
	}
	if release.Spec.Owner.ComponentName == "" {
		return nil
	}
	crName := release.Labels[labels.LabelKeyComponentReleaseName]
	crUID := release.Labels[labels.LabelKeyComponentReleaseUID]
	if crName == "" || crUID == "" {
		return nil
	}

	var primary *unstructured.Unstructured
	for _, obj := range desiredResources {
		if primaryWorkloadGVKs[obj.GroupVersionKind()] {
			primary = obj
			break
		}
	}
	if primary == nil {
		return nil
	}

	return &deliveryContext{
		rolloutID:            fmt.Sprintf("%s.%s", crUID, release.UID),
		componentReleaseName: crName,
		primary:              primary,
	}
}

// deliveryState returns the release's delivery markers for the current rollout,
// resetting them when the rollout identity changed.
func deliveryState(release *openchoreov1alpha1.RenderedRelease, dc *deliveryContext) *openchoreov1alpha1.DeliveryStatus {
	if release.Status.Delivery == nil || release.Status.Delivery.RolloutID != dc.rolloutID {
		release.Status.Delivery = &openchoreov1alpha1.DeliveryStatus{RolloutID: dc.rolloutID}
	}
	return release.Status.Delivery
}

// hasOpenFailureEpisode reports whether a DeploymentFailed was emitted without
// a later DeploymentRecovered.
func hasOpenFailureEpisode(d *openchoreov1alpha1.DeliveryStatus) bool {
	if d.FailedAt == nil {
		return false
	}
	return d.RecoveredAt == nil || d.RecoveredAt.Time.Before(d.FailedAt.Time)
}

// resolveDeliveryProvenance fills the rollout's commit provenance from the owning
// ComponentRelease, which holds the embedded Workload and therefore its Source.
//
// Provenance is best-effort on purpose. It feeds Lead Time for Changes only, so a
// ComponentRelease that has been deleted or is not yet readable must not fail the
// reconcile and block the rollout; the events are still emitted, just without a
// commit, which the aggregator already tolerates (it leaves LeadTimeMs unset).
func (r *Reconciler) resolveDeliveryProvenance(
	ctx context.Context,
	release *openchoreov1alpha1.RenderedRelease,
	dc *deliveryContext,
) {
	if dc == nil {
		return
	}
	logger := log.FromContext(ctx)

	componentRelease := &openchoreov1alpha1.ComponentRelease{}
	key := types.NamespacedName{
		Name:      dc.componentReleaseName,
		Namespace: release.Namespace,
	}
	if err := r.Get(ctx, key, componentRelease); err != nil {
		logger.V(1).Info("Delivery provenance unavailable; lead time will not be computed for this rollout",
			"componentRelease", key.String(), "error", err.Error())
		return
	}

	source := componentRelease.Spec.Workload.Source
	if source == nil {
		return
	}
	dc.commit = source.Commit
	if source.AuthoredAt != nil {
		dc.commitAuthoredAt = source.AuthoredAt.UTC().Format(time.RFC3339)
	}
}

// emitDeliveryEvents runs the delivery lifecycle emission for releases that have
// one (component workloads on the data plane); it is a no-op otherwise. Returns
// the first emission failure, wrapped with the rollout it belongs to, so the
// caller can persist the markers that did succeed before requeueing.
func (r *Reconciler) emitDeliveryEvents(
	ctx context.Context,
	planeClient client.Client,
	release *openchoreov1alpha1.RenderedRelease,
	dc *deliveryContext,
	resourceStatuses []openchoreov1alpha1.RenderedManifestStatus,
	liveResources []*unstructured.Unstructured,
) error {
	if dc == nil {
		return nil
	}
	if err := r.reconcileDeliveryEvents(ctx, planeClient, release, dc, resourceStatuses, liveResources); err != nil {
		// Wrapped rather than logged here: controller-runtime already reports the
		// error at the reconcile boundary, so logging it too would report the same
		// failure twice. The rollout identity is what that report otherwise lacks.
		return fmt.Errorf("failed to emit delivery lifecycle events for rollout %s, "+
			"remaining phases deferred: %w", dc.rolloutID, err)
	}
	return nil
}

// reconcileDeliveryEvents emits the delivery lifecycle events implied by the
// current health of the release's resources. Called after a successful apply
// with freshly built resource statuses; markers are only set when the event
// reached the data plane, so a failed emission retries next reconcile.
//
// Returns the first emission error and stops, leaving the remaining phases for
// the retry. Phase order is part of the contract: a consumer folding these
// chronologically must never see Succeeded or Failed before Started, which is
// what would happen if a failed Started emission let the health transition run
// anyway and emitted Started on a later reconcile. The caller persists whatever
// markers were set before the failure, then requeues.
func (r *Reconciler) reconcileDeliveryEvents(
	ctx context.Context,
	planeClient client.Client,
	release *openchoreov1alpha1.RenderedRelease,
	dc *deliveryContext,
	resourceStatuses []openchoreov1alpha1.RenderedManifestStatus,
	liveResources []*unstructured.Unstructured,
) error {
	d := deliveryState(release, dc)
	now := metav1.Now()

	if d.StartedAt == nil {
		if err := r.emitDeliveryEvent(ctx, planeClient, dc, reasonDeploymentStarted, "", 0); err != nil {
			return err
		}
		d.StartedAt = &now
	}

	allHealthy, degradedID := summarizeHealth(resourceStatuses)

	if allHealthy && d.FailedAt == nil {
		if err := r.restoreLostFailureEpisode(ctx, planeClient, dc, d); err != nil {
			return err
		}
	}

	openEpisode := hasOpenFailureEpisode(d)

	switch {
	case degradedID != "" && !openEpisode:
		reason := degradedFailureReason(degradedID, liveResources)
		episode := d.FailureEpisode + 1
		if err := r.emitDeliveryEvent(
			ctx, planeClient, dc, reasonDeploymentFailed, reason, episode); err != nil {
			return err
		}
		d.FailedAt = &now
		d.FailureEpisode = episode
	case allHealthy:
		if d.SucceededAt == nil {
			if err := r.emitDeliveryEvent(ctx, planeClient, dc, reasonDeploymentSucceeded, "", 0); err != nil {
				return err
			}
			d.SucceededAt = &now
		}
		if openEpisode {
			// Same episode number as the failure it closes.
			if err := r.emitDeliveryEvent(
				ctx, planeClient, dc, reasonDeploymentRecovered, "",
				d.FailureEpisode); err != nil {
				return err
			}
			d.RecoveredAt = &now
		}
	}
	return nil
}

// restoreLostFailureEpisode rebuilds the markers for a failure episode whose
// DeploymentFailed reached the data plane but whose status update did not.
//
// Without it such an episode never closes. DeploymentRecovered is gated on
// FailedAt, so once that marker is lost a release that goes straight back to
// healthy emits nothing, and the failure stays open forever with no recovery to
// measure against it. The degraded path repairs itself -- while resources stay
// unhealthy the same episode is re-derived and re-emitted -- but a
// degraded-to-healthy transition in the window between the lost write and the
// retry does not.
//
// The Events are the durable record and their names are derived purely from the
// rollout identity, so the marker can be read back from the data plane. Only the
// episode immediately after the last recorded one is checked: markers are lost by
// a single failed status write, so at most one episode can be missing.
func (r *Reconciler) restoreLostFailureEpisode(
	ctx context.Context,
	planeClient client.Client,
	dc *deliveryContext,
	d *openchoreov1alpha1.DeliveryStatus,
) error {
	episode := d.FailureEpisode + 1
	key := client.ObjectKey{
		Namespace: dc.primary.GetNamespace(),
		Name:      deliveryEventName(dc, reasonDeploymentFailed, episodeSuffix(episode)),
	}

	event := &corev1.Event{}
	if err := planeClient.Get(ctx, key, event); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // nothing was lost
		}
		return err
	}

	// Prefer the event's own timestamp over "now" so the recovery duration a
	// consumer derives spans the real outage rather than starting at the repair.
	failedAt := event.FirstTimestamp
	if failedAt.IsZero() {
		failedAt = metav1.NewTime(event.EventTime.Time)
	}
	d.FailedAt = &failedAt
	d.FailureEpisode = episode
	return nil
}

// episodeSuffix names the events of one failure episode. Derived from the persisted
// counter rather than the clock, so a re-emission after a lost status update produces
// the same event name and collapses via AlreadyExists.
func episodeSuffix(episode int32) string {
	return fmt.Sprintf("e%d", episode)
}

// deliveryEventName derives the Event name for one phase of a rollout. It is a
// pure function of the rollout identity, which is what makes a re-emission after
// a lost status update collapse into AlreadyExists instead of creating a
// duplicate -- and what lets a lost marker be rebuilt by looking the Event up
// again. Emission and recovery share it so the two cannot drift apart.
func deliveryEventName(dc *deliveryContext, reason, nameSuffix string) string {
	name := fmt.Sprintf("oc-delivery-%s-%s", shortHash(dc.rolloutID),
		strings.ToLower(strings.TrimPrefix(reason, "Deployment")))
	if nameSuffix != "" {
		name = fmt.Sprintf("%s-%s", name, nameSuffix)
	}
	return name
}

// markDeliveryApplyFailure emits DeploymentFailed for a rollout whose resources
// could not be applied to the data plane. Best-effort: the retrying reconcile
// re-attempts emission until the marker is set.
func (r *Reconciler) markDeliveryApplyFailure(
	ctx context.Context,
	planeClient client.Client,
	release *openchoreov1alpha1.RenderedRelease,
	dc *deliveryContext,
) bool {
	before := release.Status.Delivery
	d := deliveryState(release, dc)
	if hasOpenFailureEpisode(d) {
		return before != release.Status.Delivery
	}
	now := metav1.Now()
	episode := d.FailureEpisode + 1
	if err := r.emitDeliveryEvent(
		ctx, planeClient, dc, reasonDeploymentFailed, failureReasonApplyFailed,
		episode); err != nil {
		return before != release.Status.Delivery
	}
	d.FailedAt = &now
	d.FailureEpisode = episode
	return true
}

// summarizeHealth reduces resource statuses to the rollout-level signal:
// whether everything settled healthy, and the ID of a degraded resource if any.
// Suspended (deliberately paused / scaled to zero) counts as settled.
func summarizeHealth(statuses []openchoreov1alpha1.RenderedManifestStatus) (allHealthy bool, degradedID string) {
	if len(statuses) == 0 {
		return false, ""
	}
	allHealthy = true
	for _, s := range statuses {
		switch s.HealthStatus {
		case openchoreov1alpha1.HealthStatusDegraded:
			if degradedID == "" {
				degradedID = s.ID
			}
			allHealthy = false
		case openchoreov1alpha1.HealthStatusHealthy, openchoreov1alpha1.HealthStatusSuspended:
			// settled
		default:
			allHealthy = false
		}
	}
	return allHealthy, degradedID
}

// degradedFailureReason inspects the live resource behind a degraded status and
// maps it to a coarse failure reason for the event payload.
func degradedFailureReason(resourceID string, liveResources []*unstructured.Unstructured) string {
	var live *unstructured.Unstructured
	for _, obj := range liveResources {
		if obj.GetLabels()[labels.LabelKeyRenderedReleaseResourceID] == resourceID {
			live = obj
			break
		}
	}
	if live == nil {
		return failureReasonDegraded
	}

	gvk := live.GroupVersionKind()
	switch {
	case gvk.Group == appsAPIGroup && gvk.Kind == deploymentKind:
		var deployment appsv1.Deployment
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(live.Object, &deployment); err != nil {
			return failureReasonDegraded
		}
		_, progressingCond, replicaFailCond := extractDeploymentConditions(deployment.Status.Conditions)
		if progressingCond != nil && progressingCond.Reason == reasonProgressDeadlineExceeded {
			return reasonProgressDeadlineExceeded
		}
		if replicaFailCond != nil && replicaFailCond.Status == corev1.ConditionTrue {
			return "DeploymentReplicaFailure"
		}
	case gvk.Group == "" && gvk.Kind == "Pod":
		var pod corev1.Pod
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(live.Object, &pod); err != nil {
			return failureReasonDegraded
		}
		if pod.Status.Phase == corev1.PodFailed {
			return "PodFailed"
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
				return cs.State.Waiting.Reason
			}
		}
	}
	return failureReasonDegraded
}

// emitDeliveryEvent creates one delivery lifecycle event in the data plane.
// Started/Succeeded use a name deterministic per rollout, making re-emission
// after a lost status update collapse via AlreadyExists; Failed/Recovered can
// legitimately recur per rollout, so callers pass an episode suffix.
func (r *Reconciler) emitDeliveryEvent(
	ctx context.Context,
	planeClient client.Client,
	dc *deliveryContext,
	reason string,
	failureReason string,
	// episode is the failure episode this event belongs to, 0 for the phases that
	// have none (Started, Succeeded). The event name suffix is derived from it here
	// rather than passed alongside, so the name and the payload cannot disagree.
	episode int32,
) error {
	logger := log.FromContext(ctx)

	nameSuffix := ""
	if episode > 0 {
		nameSuffix = episodeSuffix(episode)
	}

	payload := deliveryEventPayload{
		RenderedReleaseUID:   dc.rolloutID,
		ComponentReleaseName: dc.componentReleaseName,
		OrgNamespace:         dc.primary.GetLabels()[labels.LabelKeyNamespaceName],
		ProjectUID:           dc.primary.GetLabels()[labels.LabelKeyProjectUID],
		ComponentUID:         dc.primary.GetLabels()[labels.LabelKeyComponentUID],
		EnvironmentUID:       dc.primary.GetLabels()[labels.LabelKeyEnvironmentUID],
		Commit:               dc.commit,
		CommitAuthoredAt:     dc.commitAuthoredAt,
		Phase:                strings.TrimPrefix(reason, "Deployment"),
		FailureReason:        failureReason,
		FailureEpisode:       episode,
	}
	message, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal delivery event payload: %w", err)
	}

	eventType := corev1.EventTypeNormal
	if reason == reasonDeploymentFailed {
		eventType = corev1.EventTypeWarning
	}

	name := deliveryEventName(dc, reason, nameSuffix)

	now := metav1.Now()
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: dc.primary.GetNamespace(),
		},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: dc.primary.GetAPIVersion(),
			Kind:       dc.primary.GetKind(),
			Namespace:  dc.primary.GetNamespace(),
			Name:       dc.primary.GetName(),
		},
		Type:           eventType,
		Reason:         reason,
		Action:         reason,
		Message:        string(message),
		FirstTimestamp: now,
		LastTimestamp:  now,
		// EventTime is the events.k8s.io/v1 timestamp. Setting it alongside
		// ReportingController/ReportingInstance/Action satisfies the stricter
		// validation path, and gives consumers reading through the newer API a
		// populated occurrence time instead of a zero value.
		EventTime:           metav1.MicroTime(now),
		Count:               1,
		Source:              corev1.EventSource{Component: deliveryReportingController},
		ReportingController: deliveryReportingController,
		ReportingInstance:   ControllerName,
	}

	if err := planeClient.Create(ctx, event); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Emitted by a previous reconcile whose status update was lost.
			return nil
		}
		logger.Error(err, "Failed to emit delivery event", "reason", reason, "event", name)
		return err
	}

	logger.Info("Emitted delivery event", "reason", reason, "event", name, "rolloutID", dc.rolloutID)
	return nil
}

// shortHash gives a compact stable identifier for event names.
func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:10]
}

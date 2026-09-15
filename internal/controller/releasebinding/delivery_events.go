// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

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
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/renderedrelease"
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
	deliveryReportingController = "openchoreo.dev/releasebinding-controller"

	// failureReasonApplyFailed marks a rollout that never reached the data plane.
	failureReasonApplyFailed = "ApplyFailed"
	// failureReasonDegraded is the fallback when a degraded resource offers no
	// more specific reason.
	failureReasonDegraded = "Degraded"

	// controllerName identifies this controller as the event's reporting instance.
	controllerName = "releasebinding-controller"

	// targetPlaneObservabilityPlane marks a RenderedRelease bound for the
	// observability plane, which carries no deployable workload.
	targetPlaneObservabilityPlane = "observabilityplane"

	deploymentKind  = "Deployment"
	statefulSetKind = "StatefulSet"
	cronJobKind     = "CronJob"

	// reasonProgressDeadlineExceeded is the Deployment Progressing condition
	// reason for a rollout that never became available.
	reasonProgressDeadlineExceeded = "ProgressDeadlineExceeded"
)

// deliveryEventPayload is embedded as JSON in the event message so the
// aggregator gets release and scope identity independent of collector
// enrichment (Kubernetes Events do not inherit the involved object's labels).
type deliveryEventPayload struct {
	// RolloutID identifies one rollout as
	// "<componentReleaseUID>.<renderedReleaseUID>.<renderedReleaseGeneration>".
	// It is not a RenderedRelease UID on its own: the same RenderedRelease is
	// reused across ComponentReleases, so neither UID distinguishes a rollout
	// alone. The generation is what distinguishes redeployments of a spec that has
	// been deployed before -- see deliveryContext.rolloutID.
	RolloutID            string `json:"rolloutId"`
	ComponentReleaseName string `json:"componentReleaseName"`
	// NamespaceName is the OpenChoreo namespace the rollout belongs to -- the
	// control-plane namespace, not the data-plane namespace the involved object
	// lives in. It is in the payload for the same reason the UIDs are: a
	// Kubernetes Event does not inherit the involved object's labels, so anything
	// a consumer needs has to travel in the message. It is the one field the store
	// requires, and depending on collector enrichment for it meant an un-enriched
	// event could not be folded.
	NamespaceName  string `json:"namespaceName,omitempty"`
	ProjectUID     string `json:"projectUid,omitempty"`
	ComponentUID   string `json:"componentUid,omitempty"`
	EnvironmentUID string `json:"environmentUid,omitempty"`
	Phase          string `json:"phase"`
	FailureReason  string `json:"failureReason,omitempty"`
	// FailureEpisode identifies which failure->recovery cycle of this rollout the
	// event belongs to. The emitter already distinguishes episodes -- it suffixes
	// event names -e1, -e2 -- but a consumer keying a recovery on the rollout alone
	// merges them, and the merged duration then spans the healthy interval between
	// them. Carried on Failed and Recovered; zero on Started and Succeeded.
	FailureEpisode int32 `json:"failureEpisode,omitempty"`
	// Commit is the VCS commit the running image was built from, and
	// CommitAuthoredAt is when that commit was authored, in RFC 3339. Both are
	// carried on every phase, not just the terminal one: the consumer has no route
	// back to the ComponentRelease to look them up. Empty when the workload records
	// no source.
	Commit           string `json:"commit,omitempty"`
	CommitAuthoredAt string `json:"commitAuthoredAt,omitempty"`
}

// deliveryContext is everything needed to emit delivery events for one
// reconcile, resolved once up front. It exists only for component-owned
// data-plane releases that render a primary workload resource.
type deliveryContext struct {
	// rolloutID is the per-rollout identity: the ComponentRelease UID, the
	// RenderedRelease UID, and the RenderedRelease generation. Neither UID alone is
	// enough -- the RenderedRelease is reused across rollouts, and the
	// ComponentRelease UID is shared by every environment it is bound to -- and the
	// generation separates redeployments of a spec that ran before, since
	// ComponentReleases are content-addressed and a rollback resolves to the same
	// one. It is stable while a rollout converges, so every phase shares an identity.
	//
	// A restart does not advance it: openchoreo.dev/restartedAt is an annotation that
	// never enters the spec, so a restart records no deployment -- intended, since it
	// ships no change.
	rolloutID            string
	componentReleaseName string
	// namespaceName is the OpenChoreo namespace the rollout belongs to, taken from
	// the ReleaseBinding itself rather than from a label on the rendered resource.
	// The store requires it, and the label is not guaranteed: it is injected
	// through MetadataContext.Labels, which not every render path populates, so
	// reading it off the resource could marshal an empty value that `omitempty`
	// then drops from the payload entirely. The object's own namespace is the same
	// value and always set.
	namespaceName string
	// commit and commitAuthoredAt carry the source provenance of the image being
	// rolled out, read from the ComponentRelease's frozen workload copy. Empty
	// when the workload records no source.
	commit           string
	commitAuthoredAt string
	// primary is the desired primary workload resource (Deployment, StatefulSet,
	// or CronJob) the events anchor to as involvedObject.
	primary *unstructured.Unstructured
}

// primaryWorkloadGVKs are the resource kinds whose health defines rollout
// outcome and which delivery events anchor to, mirroring GetHealthCheckFunc.
var primaryWorkloadGVKs = map[schema.GroupVersionKind]bool{
	{Group: appsAPIGroup, Version: "v1", Kind: deploymentKind}:  true,
	{Group: appsAPIGroup, Version: "v1", Kind: statefulSetKind}: true,
	{Group: "batch", Version: "v1", Kind: cronJobKind}:          true,
}

// reconcileDelivery emits the delivery lifecycle events implied by the rollout's
// current health, or nothing for a rollout that does not participate (see
// deliveryContextFor).
//
// Health comes from the RenderedRelease this binding owns. Because the binding
// Owns() that object, a status change there reconciles the binding, so health
// changes reach this without having to watch the data plane itself. Markers are
// set on the binding's own status and persisted by the deferred status update in
// Reconcile, so an event and the record that it was emitted are written together.
//
// A status change is not the only thing that reconciles the binding, though, so
// this cannot assume the health it reads describes the rollout it is reporting on
// -- see deliveryHealthIsCurrent.
//
// It returns nothing: every failure here is a lost metric, never a lost rollout,
// so there is no outcome for the caller to act on.
func (r *Reconciler) reconcileDelivery(
	ctx context.Context,
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	componentRelease *openchoreov1alpha1.ComponentRelease,
	renderedRelease *openchoreov1alpha1.RenderedRelease,
	resources []map[string]any,
	applyFailed bool,
) {
	dc := deliveryContextFor(releaseBinding, componentRelease, renderedRelease, asUnstructured(resources))
	if dc == nil {
		return
	}

	planeClient, err := r.getDPClient(ctx, releaseBinding.Namespace, releaseBinding.Spec.Environment)
	if err != nil {
		// Delivery events are a metric, not part of making the deployment work.
		// This controller does not otherwise need a plane client, so a plane it
		// cannot reach -- or an install with no provider configured -- must not
		// fail the rollout. Nothing is recorded, so the next reconcile retries.
		log.FromContext(ctx).V(1).Info("Skipping delivery events: no data plane client",
			"environment", releaseBinding.Spec.Environment, "error", err.Error())
		return
	}

	// An apply failure means the rollout never reached the plane, so there is no
	// resource health to summarize -- report the failure directly.
	if applyFailed {
		r.markDeliveryApplyFailure(ctx, planeClient, releaseBinding, dc)
		return
	}

	if !deliveryHealthIsCurrent(renderedRelease) {
		// Reporting the previous revision's health as this rollout's outcome would
		// emit DeploymentSucceeded before the rollout had begun. Nothing is
		// recorded, so the reconcile that observes this generation reports it.
		log.FromContext(ctx).V(1).Info("Deferring delivery events: resource health not yet observed for this generation",
			"rolloutID", dc.rolloutID, "generation", renderedRelease.Generation)
		return
	}

	if err := r.emitDeliveryEvents(ctx, planeClient, releaseBinding, dc, renderedRelease.Status.Resources); err != nil {
		// Delivery events are a metric, for the same reason the plane client above
		// is not fatal: a plane that refuses the write -- an agent without RBAC to
		// create events, say -- must not put every binding into permanent reconcile
		// failure. Logged rather than returned, and whatever phases did get emitted
		// are still recorded, so the next reconcile continues from there.
		log.FromContext(ctx).Error(err, "Failed to emit delivery events", "rolloutID", dc.rolloutID)
	}
}

// deliveryHealthIsCurrent reports whether Status.Resources describes the spec the
// rollout being reported on rendered.
//
// The rollout identity advances as soon as a new ComponentRelease is rendered,
// but the resource health it is compared against is written asynchronously by the
// renderedrelease controller. On the first reconcile after a re-render that status
// still summarizes the revision being replaced -- which is healthy, having been
// running until now -- so evaluating it would emit DeploymentSucceeded for a
// rollout whose pods do not exist yet. That is not a cosmetic ordering problem:
// succeededAt is what Lead Time for Changes measures to, so it would exclude the
// rollout it is supposed to be timing.
//
// The applied condition is the marker for this: the renderedrelease controller
// builds Status.Resources and sets the condition in the same reconcile, so a
// successful condition reporting this generation means the health beside it
// describes this spec.
//
// The status is checked as well as the generation, so this does not depend on
// the caller having intercepted an apply failure first: a rollout whose apply
// failed for this generation is reported through markDeliveryApplyFailure, and
// there is no resource health to summarize for it either way.
func deliveryHealthIsCurrent(renderedRelease *openchoreov1alpha1.RenderedRelease) bool {
	applied := apimeta.FindStatusCondition(renderedRelease.Status.Conditions,
		renderedrelease.ConditionResourcesApplied)
	return applied != nil &&
		applied.Status == metav1.ConditionTrue &&
		applied.ObservedGeneration == renderedRelease.Generation
}

// asUnstructured views the rendered resource maps as unstructured objects, which
// is what the primary-workload lookup needs. It does not copy the maps.
func asUnstructured(resources []map[string]any) []*unstructured.Unstructured {
	out := make([]*unstructured.Unstructured, 0, len(resources))
	for _, obj := range resources {
		out = append(out, &unstructured.Unstructured{Object: obj})
	}
	return out
}

// getDPClient resolves a client for the data plane backing an environment. The
// delivery events are created in that plane, alongside the workload they
// describe, so they are collected by the same event pipeline as any other
// Kubernetes event.
func (r *Reconciler) getDPClient(
	ctx context.Context, namespaceName, environmentName string,
) (client.Client, error) {
	env := &openchoreov1alpha1.Environment{}
	if err := r.Get(ctx, client.ObjectKey{Name: environmentName, Namespace: namespaceName}, env); err != nil {
		return nil, fmt.Errorf("failed to get environment %s: %w", environmentName, err)
	}

	dataPlaneResult, err := controller.GetDataPlaneFromRef(ctx, r.Client, env.Namespace, env.Spec.DataPlaneRef)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve dataplane for environment %s: %w", environmentName, err)
	}

	dpClient, err := dataPlaneResult.GetK8sClient(r.PlaneClientProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create dataplane client for %s: %w", dataPlaneResult.GetName(), err)
	}

	return dpClient, nil
}

// deliveryContextFor resolves the delivery context, or nil when this rollout does
// not participate in delivery events: the RenderedRelease has not been created
// yet, it targets the observability plane, or the render produced no primary
// workload resource whose health defines a deployment.
//
// The ComponentRelease name and UID come from the object itself rather than from
// labels on the RenderedRelease, since this controller holds both.
func deliveryContextFor(
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	componentRelease *openchoreov1alpha1.ComponentRelease,
	renderedRelease *openchoreov1alpha1.RenderedRelease,
	desiredResources []*unstructured.Unstructured,
) *deliveryContext {
	if componentRelease == nil || renderedRelease == nil || renderedRelease.UID == "" {
		return nil
	}
	if renderedRelease.Spec.TargetPlane == targetPlaneObservabilityPlane {
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

	commit, authoredAt := deliveryProvenance(componentRelease)

	return &deliveryContext{
		rolloutID: fmt.Sprintf("%s.%s.%d",
			componentRelease.UID, renderedRelease.UID, renderedRelease.Generation),
		componentReleaseName: componentRelease.Name,
		namespaceName:        releaseBinding.Namespace,
		commit:               commit,
		commitAuthoredAt:     authoredAt,
		primary:              primary,
	}
}

// deliveryProvenance reads the source commit and its authoring time off the
// ComponentRelease's frozen workload copy, formatted for the payload.
//
// The ComponentRelease is the right source for this rather than the live
// Workload: it is the immutable snapshot the rollout was rendered from, so the
// provenance stays the provenance of what actually shipped even after the
// Workload moves on. Returns empty strings when no source was recorded, which
// `omitempty` then keeps out of the payload entirely.
func deliveryProvenance(componentRelease *openchoreov1alpha1.ComponentRelease) (commit, authoredAt string) {
	source := componentRelease.Spec.Workload.Source
	if source == nil {
		return "", ""
	}
	if source.AuthoredAt != nil {
		authoredAt = source.AuthoredAt.UTC().Format(time.RFC3339)
	}
	return source.Commit, authoredAt
}

// deliveryState returns the binding's delivery markers for the current rollout,
// resetting them when the rollout identity changed.
func deliveryState(
	releaseBinding *openchoreov1alpha1.ReleaseBinding, dc *deliveryContext,
) *openchoreov1alpha1.DeliveryStatus {
	if releaseBinding.Status.Delivery == nil || releaseBinding.Status.Delivery.RolloutID != dc.rolloutID {
		releaseBinding.Status.Delivery = &openchoreov1alpha1.DeliveryStatus{RolloutID: dc.rolloutID}
	}
	return releaseBinding.Status.Delivery
}

// hasOpenFailureEpisode reports whether a DeploymentFailed was emitted without
// a later DeploymentRecovered.
func hasOpenFailureEpisode(d *openchoreov1alpha1.DeliveryStatus) bool {
	if d.FailedAt == nil {
		return false
	}
	return d.RecoveredAt == nil || d.RecoveredAt.Time.Before(d.FailedAt.Time)
}

// emitDeliveryEvents runs the delivery lifecycle emission for releases that have
// one (component workloads on the data plane); it is a no-op otherwise. Returns
// the first emission failure, wrapped with the rollout it belongs to, so the
// caller can persist the markers that did succeed before requeueing.
func (r *Reconciler) emitDeliveryEvents(
	ctx context.Context,
	planeClient client.Client,
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	dc *deliveryContext,
	resourceStatuses []openchoreov1alpha1.RenderedManifestStatus,
) error {
	if dc == nil {
		return nil
	}
	if err := r.reconcileDeliveryEvents(ctx, planeClient, releaseBinding, dc, resourceStatuses); err != nil {
		// Wrapped rather than logged here, because the caller decides what to do
		// with it -- and since this no longer reaches the reconcile boundary, the
		// caller is what logs it. The wrap is what carries the rollout identity and
		// the deferral in the message itself, so the error read on its own says
		// which rollout stopped and that the remaining phases retry.
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
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	dc *deliveryContext,
	resourceStatuses []openchoreov1alpha1.RenderedManifestStatus,
) error {
	d := deliveryState(releaseBinding, dc)
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
		reason := degradedFailureReason(resourceStatuses, degradedID)
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
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	dc *deliveryContext,
) bool {
	before := releaseBinding.Status.Delivery
	d := deliveryState(releaseBinding, dc)
	if hasOpenFailureEpisode(d) {
		return before != releaseBinding.Status.Delivery
	}
	now := metav1.Now()

	// A failure on the first apply attempt would otherwise be the rollout's first
	// event, and the Started for it would only be emitted once an apply finally
	// succeeded -- after the Failed, breaking the phase order a consumer folds on.
	// Started goes out first, and if it cannot, the failure waits for the retry.
	if d.StartedAt == nil {
		if err := r.emitDeliveryEvent(ctx, planeClient, dc, reasonDeploymentStarted, "", 0); err != nil {
			return before != releaseBinding.Status.Delivery
		}
		d.StartedAt = &now
	}

	episode := d.FailureEpisode + 1
	if err := r.emitDeliveryEvent(
		ctx, planeClient, dc, reasonDeploymentFailed, failureReasonApplyFailed,
		episode); err != nil {
		return true // the Started marker above still needs persisting
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
func degradedFailureReason(
	statuses []openchoreov1alpha1.RenderedManifestStatus, resourceID string,
) string {
	var rs *openchoreov1alpha1.RenderedManifestStatus
	for i := range statuses {
		if statuses[i].ID == resourceID {
			rs = &statuses[i]
			break
		}
	}
	if rs == nil || rs.Status == nil {
		return failureReasonDegraded
	}

	switch {
	case rs.Group == appsAPIGroup && rs.Kind == deploymentKind:
		var st appsv1.DeploymentStatus
		if err := json.Unmarshal(rs.Status.Raw, &st); err != nil {
			return failureReasonDegraded
		}
		for i := range st.Conditions {
			c := st.Conditions[i]
			if c.Type == appsv1.DeploymentProgressing && c.Reason == reasonProgressDeadlineExceeded {
				return reasonProgressDeadlineExceeded
			}
			if c.Type == appsv1.DeploymentReplicaFailure && c.Status == corev1.ConditionTrue {
				return "DeploymentReplicaFailure"
			}
		}
	case rs.Group == "" && rs.Kind == "Pod":
		var st corev1.PodStatus
		if err := json.Unmarshal(rs.Status.Raw, &st); err != nil {
			return failureReasonDegraded
		}
		if st.Phase == corev1.PodFailed {
			return "PodFailed"
		}
		for _, cs := range st.ContainerStatuses {
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
		RolloutID:            dc.rolloutID,
		ComponentReleaseName: dc.componentReleaseName,
		NamespaceName:        dc.namespaceName,
		ProjectUID:           dc.primary.GetLabels()[labels.LabelKeyProjectUID],
		ComponentUID:         dc.primary.GetLabels()[labels.LabelKeyComponentUID],
		EnvironmentUID:       dc.primary.GetLabels()[labels.LabelKeyEnvironmentUID],
		Phase:                strings.TrimPrefix(reason, "Deployment"),
		FailureReason:        failureReason,
		FailureEpisode:       episode,
		Commit:               dc.commit,
		CommitAuthoredAt:     dc.commitAuthoredAt,
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
		ReportingInstance:   controllerName,
	}

	if err := planeClient.Create(ctx, event); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Emitted by a previous reconcile whose status update was lost.
			return nil
		}
		// Not logged here: the caller returns this to the reconcile boundary, which
		// reports it, so logging would duplicate the record for every failure.
		return fmt.Errorf("failed to create %s delivery event %q: %w", reason, name, err)
	}

	logger.Info("Emitted delivery event", "reason", reason, "event", name, "rolloutID", dc.rolloutID)
	return nil
}

// shortHash gives a compact stable identifier for event names.
func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:10]
}

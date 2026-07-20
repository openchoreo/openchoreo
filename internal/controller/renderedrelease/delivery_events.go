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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	kindDeployment  = "Deployment"
	kindStatefulSet = "StatefulSet"
	kindCronJob     = "CronJob"

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
	ProjectUID           string `json:"projectUid,omitempty"`
	ComponentUID         string `json:"componentUid,omitempty"`
	EnvironmentUID       string `json:"environmentUid,omitempty"`
	Commit               string `json:"commit,omitempty"`
	CommitAuthoredAt     string `json:"commitAuthoredAt,omitempty"`
	Phase                string `json:"phase"`
	FailureReason        string `json:"failureReason,omitempty"`
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
}

// primaryWorkloadGVKs are the resource kinds whose health defines rollout
// outcome and which delivery events anchor to, mirroring GetHealthCheckFunc.
var primaryWorkloadGVKs = map[schema.GroupVersionKind]bool{
	{Group: "apps", Version: "v1", Kind: kindDeployment}:  true,
	{Group: "apps", Version: "v1", Kind: kindStatefulSet}: true,
	{Group: "batch", Version: "v1", Kind: kindCronJob}:    true,
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

// reconcileDeliveryEvents emits the delivery lifecycle events implied by the
// current health of the release's resources. Called after a successful apply
// with freshly built resource statuses; markers are only set when the event
// reached the data plane, so a failed emission retries next reconcile.
func (r *Reconciler) reconcileDeliveryEvents(
	ctx context.Context,
	planeClient client.Client,
	release *openchoreov1alpha1.RenderedRelease,
	dc *deliveryContext,
	resourceStatuses []openchoreov1alpha1.RenderedManifestStatus,
	liveResources []*unstructured.Unstructured,
) {
	d := deliveryState(release, dc)
	now := metav1.Now()

	if d.StartedAt == nil {
		if err := r.emitDeliveryEvent(ctx, planeClient, dc, reasonDeploymentStarted, "", ""); err == nil {
			d.StartedAt = &now
		}
	}

	allHealthy, degradedID := summarizeHealth(resourceStatuses)
	openEpisode := hasOpenFailureEpisode(d)

	switch {
	case degradedID != "" && !openEpisode:
		reason := degradedFailureReason(degradedID, liveResources)
		suffix := fmt.Sprintf("%d", now.Unix())
		if err := r.emitDeliveryEvent(ctx, planeClient, dc, reasonDeploymentFailed, reason, suffix); err == nil {
			d.FailedAt = &now
		}
	case allHealthy:
		if d.SucceededAt == nil {
			if err := r.emitDeliveryEvent(ctx, planeClient, dc, reasonDeploymentSucceeded, "", ""); err == nil {
				d.SucceededAt = &now
			}
		}
		if openEpisode {
			suffix := fmt.Sprintf("%d", now.Unix())
			if err := r.emitDeliveryEvent(ctx, planeClient, dc, reasonDeploymentRecovered, "", suffix); err == nil {
				d.RecoveredAt = &now
			}
		}
	}
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
	suffix := fmt.Sprintf("%d", now.Unix())
	if err := r.emitDeliveryEvent(ctx, planeClient, dc, reasonDeploymentFailed, failureReasonApplyFailed, suffix); err != nil {
		return before != release.Status.Delivery
	}
	d.FailedAt = &now
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
	case gvk.Group == appsAPIGroup && gvk.Kind == kindDeployment:
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
	nameSuffix string,
) error {
	logger := log.FromContext(ctx)

	payload := deliveryEventPayload{
		RenderedReleaseUID:   dc.rolloutID,
		ComponentReleaseName: dc.componentReleaseName,
		ProjectUID:           dc.primary.GetLabels()[labels.LabelKeyProjectUID],
		ComponentUID:         dc.primary.GetLabels()[labels.LabelKeyComponentUID],
		EnvironmentUID:       dc.primary.GetLabels()[labels.LabelKeyEnvironmentUID],
		Phase:                strings.TrimPrefix(reason, "Deployment"),
		FailureReason:        failureReason,
	}
	message, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal delivery event payload: %w", err)
	}

	eventType := corev1.EventTypeNormal
	if reason == reasonDeploymentFailed {
		eventType = corev1.EventTypeWarning
	}

	name := fmt.Sprintf("oc-delivery-%s-%s", shortHash(dc.rolloutID), strings.ToLower(payload.Phase))
	if nameSuffix != "" {
		name = fmt.Sprintf("%s-%s", name, nameSuffix)
	}

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
		Type:                eventType,
		Reason:              reason,
		Action:              reason,
		Message:             string(message),
		FirstTimestamp:      now,
		LastTimestamp:       now,
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

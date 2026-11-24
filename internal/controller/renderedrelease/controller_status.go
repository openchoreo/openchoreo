// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// updateStatus updates the Release status with applied and observed resources
// Returns true if the status was updated, false if unchanged
func (r *Reconciler) updateStatus(ctx context.Context, old, release *openchoreov1alpha1.RenderedRelease,
	resourceStatuses, observedStatuses []openchoreov1alpha1.RenderedManifestStatus) (bool, error) {
	logger := log.FromContext(ctx)

	// Update the status
	release.Status.Resources = resourceStatuses
	release.Status.ObservedResources = observedStatuses

	// Conditions are deliberately part of the comparison. The apply-success
	// condition is no longer persisted mid-reconcile, so this update is what
	// publishes it -- masking it here would drop it whenever the resource
	// statuses happened not to change.

	// Check if the entire status actually changed and skip update if not
	if apiequality.Semantic.DeepEqual(old.Status, release.Status) {
		return false, nil
	}

	// Update the Release status
	if err := r.Status().Update(ctx, release); err != nil {
		logger.Error(err, "Failed to update Release status")
		return false, fmt.Errorf("failed to update status: %w", err)
	}

	return true, nil
}

// resourceKey names an object on a plane as it was read: the GVK, namespace and name.
type resourceKey struct {
	gvk             schema.GroupVersionKind
	namespace, name string
}

// keyOf returns the key obj is indexed and looked up under.
func keyOf(obj *unstructured.Unstructured) resourceKey {
	return resourceKey{gvk: obj.GroupVersionKind(), namespace: obj.GetNamespace(), name: obj.GetName()}
}

// liveResourceIndex pairs desired resources with the live objects they were applied as.
//
// It matches by object, not by resource ID. The resource ID is a label, and
// operators that copy labels onto the objects they create hand it to objects that
// are not ours: ESO copies an ExternalSecret's labels onto its Secret when the
// ExternalSecret has no target template, so both are listed under one resource ID.
// listLiveResourcesByGVKs stamps every live item with the GVK it was listed under,
// which is where the GVK in the key comes from.
//
// The version is part of the key. The same object is listed once per version the
// controller knows for its kind (the spec's and the one in the previous status),
// each copy in that version's shape, and status has to be read in the version the
// spec asks for. Every desired GVK is listed (findAllKnownGVKs), so a desired
// resource that exists is found.
type liveResourceIndex map[resourceKey]*unstructured.Unstructured

// indexLiveResources indexes live resources by the GVK they were listed under,
// their namespace and name.
func indexLiveResources(liveResources []*unstructured.Unstructured) liveResourceIndex {
	index := make(liveResourceIndex, len(liveResources))
	for _, live := range liveResources {
		index[keyOf(live)] = live
	}
	return index
}

// lookup returns the live object desired was applied as, read in desired's version.
func (idx liveResourceIndex) lookup(desired *unstructured.Unstructured) (*unstructured.Unstructured, bool) {
	live, found := idx[keyOf(desired)]
	return live, found
}

// buildResourceStatus converts applied unstructured objects to ResourceStatus entries using live resources
func (r *Reconciler) buildResourceStatus(ctx context.Context, old *openchoreov1alpha1.RenderedRelease, desiredResources, liveResources []*unstructured.Unstructured) []openchoreov1alpha1.RenderedManifestStatus {
	logger := log.FromContext(ctx)
	liveIndex := indexLiveResources(liveResources)

	// Build a map of old resource statuses for quick lookup by resource ID
	oldResourceMap := make(map[string]openchoreov1alpha1.RenderedManifestStatus)
	for _, oldResource := range old.Status.Resources {
		oldResourceMap[oldResource.ID] = oldResource
	}

	resourceStatuses := make([]openchoreov1alpha1.RenderedManifestStatus, 0, len(desiredResources))

	for _, desiredObj := range desiredResources {
		gvk := desiredObj.GroupVersionKind()
		resourceID := desiredObj.GetLabels()[labels.LabelKeyRenderedReleaseResourceID]

		var resourceStatus *runtime.RawExtension
		var lastObservedTime *metav1.Time
		healthStatus := openchoreov1alpha1.HealthStatusUnknown

		if liveResource, found := liveIndex.lookup(desiredObj); found {
			resourceStatus, healthStatus = liveStatusAndHealth(logger, resourceID, gvk, liveResource)

			// The live snapshot can predate the apply this reconcile just made:
			// server-side apply is a quorum write, while List is served from the
			// API server's watch cache, which can lag it. Judging that snapshot
			// reports the previous revision's health under the current generation
			// -- for a Deployment mid-rollout, Healthy for a revision whose new
			// ReplicaSet does not exist yet. The apply response carries the
			// server's generation for the spec that was written, so a live object
			// behind it has not been observed yet.
			if snapshotPrecedesApply(desiredObj, liveResource) {
				healthStatus = openchoreov1alpha1.HealthStatusProgressing
			}

			var oldResource *openchoreov1alpha1.RenderedManifestStatus
			if prev, exists := oldResourceMap[resourceID]; exists {
				oldResource = &prev
			}
			lastObservedTime = nextLastObservedTime(oldResource, resourceStatus, healthStatus)
		}

		status := openchoreov1alpha1.RenderedManifestStatus{
			ID:               resourceID,
			Group:            gvk.Group,
			Version:          gvk.Version,
			Kind:             gvk.Kind,
			Name:             desiredObj.GetName(),
			Namespace:        desiredObj.GetNamespace(),
			Status:           resourceStatus,
			HealthStatus:     healthStatus,
			LastObservedTime: lastObservedTime,
		}

		resourceStatuses = append(resourceStatuses, status)
	}

	return resourceStatuses
}

// liveStatusAndHealth returns a live object's .status and its health from the
// health check for gvk. A status that cannot be marshaled is left out and a
// failed health check reads as Unknown; both are logged.
func liveStatusAndHealth(logger logr.Logger, resourceID string, gvk schema.GroupVersionKind,
	live *unstructured.Unstructured) (*runtime.RawExtension, openchoreov1alpha1.HealthStatus) {
	var resourceStatus *runtime.RawExtension
	if statusField, found, _ := unstructured.NestedFieldCopy(live.Object, "status"); found && statusField != nil {
		statusBytes, err := json.Marshal(statusField)
		if err == nil {
			resourceStatus = &runtime.RawExtension{Raw: statusBytes}
		} else {
			logger.Error(err, "Failed to marshal resource status",
				"resourceID", resourceID,
				"gvk", gvk.String(),
				"namespace", live.GetNamespace(),
				"name", live.GetName())
		}
	}

	// Kinds without a check of their own get the presence-only check, which
	// reports Healthy for any object that exists.
	health, err := GetHealthCheckFunc(gvk)(live)
	if err != nil {
		logger.Error(err, "Failed to check resource health",
			"resourceID", resourceID,
			"gvk", gvk.String(),
			"namespace", live.GetNamespace(),
			"name", live.GetName())
		return resourceStatus, openchoreov1alpha1.HealthStatusUnknown
	}
	return resourceStatus, health
}

// nextLastObservedTime keeps prev's LastObservedTime while the status and health
// are unchanged from prev, so an unchanged resource does not rewrite the release
// status. Otherwise, or when there is no previous entry, it is now.
func nextLastObservedTime(prev *openchoreov1alpha1.RenderedManifestStatus, status *runtime.RawExtension,
	health openchoreov1alpha1.HealthStatus) *metav1.Time {
	if prev != nil && prev.LastObservedTime != nil &&
		apiequality.Semantic.DeepEqual(prev.Status, status) && prev.HealthStatus == health {
		return prev.LastObservedTime
	}
	return ptr.To(metav1.Now())
}

// snapshotPrecedesApply reports whether live was read before the spec that
// applied wrote became visible.
//
// applied is the object handed to server-side apply, which controller-runtime
// replaces with the server's response, so its generation is the one the write
// produced. Generation is absent for kinds that do not track it (ConfigMap,
// Secret, HTTPRoute status aside), which reads as zero on both sides and so
// never trips this.
func snapshotPrecedesApply(applied, live *unstructured.Unstructured) bool {
	appliedGen, liveGen := applied.GetGeneration(), live.GetGeneration()
	return appliedGen > 0 && liveGen > 0 && liveGen < appliedGen
}

// hasTransitioningResources checks if any resources are in a transitioning state
func (r *Reconciler) hasTransitioningResources(resources []openchoreov1alpha1.RenderedManifestStatus) bool {
	for _, resource := range resources {
		// Check health status to determine if resource is transitioning
		// - Progressing: actively changing state (rolling update, scaling, etc.)
		// - Unknown: can't determine state (could be transitioning)
		// - Degraded: in error state but Kubernetes may be retrying (CrashLoopBackOff, ImagePullBackOff, etc.)
		if resource.HealthStatus == openchoreov1alpha1.HealthStatusProgressing ||
			resource.HealthStatus == openchoreov1alpha1.HealthStatusUnknown ||
			resource.HealthStatus == openchoreov1alpha1.HealthStatusDegraded {
			return true
		}
	}
	return false
}

// hasResurrectableWorkload reports whether any managed Deployment or StatefulSet is scaled to
// zero without being paused, so an autoscaler (HPA/KEDA scale-from-zero) could bring it back
// up at any time. Replicas are read from the live object, not the rendered spec: the control
// plane strips spec.replicas from autoscaled workloads, so only the live object reflects what
// the autoscaler set. Workloads are picked by the desired GVK and paired to their live object
// by GVK, namespace and name (see liveResourceIndex).
func hasResurrectableWorkload(desiredResources, liveResources []*unstructured.Unstructured) bool {
	liveIndex := indexLiveResources(liveResources)

	for _, desired := range desiredResources {
		gvk := desired.GroupVersionKind()
		if gvk.Group != appsAPIGroup || (gvk.Kind != deploymentKind && gvk.Kind != statefulSetKind) {
			continue
		}
		live, ok := liveIndex.lookup(desired)
		if !ok {
			continue
		}
		if isResurrectable(live) {
			return true
		}
	}
	return false
}

// isResurrectable reports whether a live Deployment or StatefulSet is scaled to
// zero without being paused. It is false for every other kind.
func isResurrectable(live *unstructured.Unstructured) bool {
	gvk := live.GroupVersionKind()
	if gvk.Group != appsAPIGroup || (gvk.Kind != deploymentKind && gvk.Kind != statefulSetKind) {
		return false
	}
	if paused, _, _ := unstructured.NestedBool(live.Object, "spec", "paused"); paused {
		return false
	}
	replicas, found, err := unstructured.NestedInt64(live.Object, "spec", "replicas")
	return err == nil && found && replicas == 0
}

func GetHealthCheckFunc(gvk schema.GroupVersionKind) func(obj *unstructured.Unstructured) (openchoreov1alpha1.HealthStatus, error) {
	if check, ok := kindHealthCheckFunc(gvk); ok {
		return check
	}
	return getUnknownResourceHealth
}

// HasKindHealthCheck reports whether gvk has a calculator that reads the
// object's spec and status, as opposed to the presence-only fallback every other
// kind gets. Callers holding a partial object need the difference: a
// presence-only answer stays true of a projection that kept only metadata, while
// a spec/status calculator reading one is answering about fields that are not
// there.
func HasKindHealthCheck(gvk schema.GroupVersionKind) bool {
	_, ok := kindHealthCheckFunc(gvk)
	return ok
}

func kindHealthCheckFunc(gvk schema.GroupVersionKind) (func(obj *unstructured.Unstructured) (openchoreov1alpha1.HealthStatus, error), bool) {
	switch {
	case gvk.Group == appsAPIGroup && gvk.Kind == deploymentKind:
		return getDeploymentHealth, true
	case gvk.Group == appsAPIGroup && gvk.Kind == statefulSetKind:
		return getStatefulSetHealth, true
	case gvk.Group == "" && gvk.Kind == "Pod":
		return getPodHealth, true
	case gvk.Group == batchAPIGroup && gvk.Kind == cronJobKind:
		return getCronJobHealth, true
		// TODO: Add gateway http route health check, and other resources as needed
	}
	return nil, false
}

func getDeploymentHealth(obj *unstructured.Unstructured) (openchoreov1alpha1.HealthStatus, error) {
	// Convert unstructured object to Deployment
	var deployment appsv1.Deployment
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &deployment); err != nil {
		return openchoreov1alpha1.HealthStatusUnknown, fmt.Errorf("failed to convert to deployment: %w", err)
	}

	// Check if deployment is paused (or deliberately scaled to zero) -> Suspended
	if deployment.Spec.Paused {
		return openchoreov1alpha1.HealthStatusSuspended, nil
	}
	if deployment.Spec.Replicas != nil && *deployment.Spec.Replicas == 0 {
		return openchoreov1alpha1.HealthStatusSuspended, nil
	}

	// New spec not yet observed -> Progressing
	if deployment.Status.ObservedGeneration == 0 || deployment.Generation > deployment.Status.ObservedGeneration {
		return openchoreov1alpha1.HealthStatusProgressing, nil
	}

	// Extract replica details
	desiredReplicas := int32(1)
	if deployment.Spec.Replicas != nil {
		desiredReplicas = *deployment.Spec.Replicas
	}
	updatedReplicas := deployment.Status.UpdatedReplicas
	readyReplicas := deployment.Status.ReadyReplicas
	availableReplicas := deployment.Status.AvailableReplicas
	unavailableReplicas := deployment.Status.UnavailableReplicas

	// Extract deployment conditions
	availableCond, progressingCond, replicaFailCond := extractDeploymentConditions(deployment.Status.Conditions)

	// Progress deadline or replica failure -> Degraded
	if progressingCond != nil && progressingCond.Reason == reasonProgressDeadlineExceeded {
		return openchoreov1alpha1.HealthStatusDegraded, nil
	}
	if replicaFailCond != nil && replicaFailCond.Status == corev1.ConditionTrue {
		return openchoreov1alpha1.HealthStatusDegraded, nil
	}

	// All pods on new revision but none are available yet -> check if rollout is still in progress
	if desiredReplicas == updatedReplicas && availableReplicas == 0 && desiredReplicas > 0 {
		if progressingCond != nil && progressingCond.Status == corev1.ConditionTrue {
			// "NewReplicaSetAvailable" means the rollout completed (new ReplicaSet was created
			// and scaled up). If pods are still not available after rollout completion, it
			// indicates a runtime failure (e.g., CrashLoopBackOff, failing readiness probes).
			if progressingCond.Reason == "NewReplicaSetAvailable" {
				return openchoreov1alpha1.HealthStatusDegraded, nil
			}
			// Other reasons (e.g., "ReplicaSetUpdated") mean pods are still being created
			return openchoreov1alpha1.HealthStatusProgressing, nil
		}
		// Progressing is False or unknown -> degraded
		return openchoreov1alpha1.HealthStatusDegraded, nil
	}

	// Serving traffic impacted? -> Degraded
	// We only check if we have seen at least one available pod or ready replica
	if availableCond != nil && availableCond.Status != corev1.ConditionTrue &&
		unavailableReplicas > 0 && (readyReplicas > 0 || availableReplicas > 0) {
		return openchoreov1alpha1.HealthStatusDegraded, nil
	}

	// All replicas are up to date, ready, and available -> healthy
	allReplicasMatch := desiredReplicas == updatedReplicas && //nolint:gocritic // badCond: Valid deployment health check logic
		desiredReplicas == readyReplicas &&
		desiredReplicas == availableReplicas
	if allReplicasMatch && unavailableReplicas == 0 {
		return openchoreov1alpha1.HealthStatusHealthy, nil
	}

	// Deployment is in progress if:
	// - Not all replicas are updated
	// - Not all replicas are ready
	// - Not all replicas are available
	// - Some replicas are unavailable
	return openchoreov1alpha1.HealthStatusProgressing, nil
}

func extractDeploymentConditions(conditions []appsv1.DeploymentCondition) (available, progressing, replicaFailure *appsv1.DeploymentCondition) {
	for i := range conditions {
		c := &conditions[i]
		switch c.Type {
		case appsv1.DeploymentAvailable:
			available = c
		case appsv1.DeploymentProgressing:
			progressing = c
		case appsv1.DeploymentReplicaFailure:
			replicaFailure = c
		}
	}
	return available, progressing, replicaFailure
}

// TODO: Check the statefulset health tracking and update logic
func getStatefulSetHealth(obj *unstructured.Unstructured) (openchoreov1alpha1.HealthStatus, error) {
	// Convert an unstructured object to StatefulSet
	var statefulSet appsv1.StatefulSet
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &statefulSet); err != nil {
		return openchoreov1alpha1.HealthStatusUnknown, fmt.Errorf("failed to convert to statefulset: %w", err)
	}

	// If status is not populated yet, it's progressing
	if statefulSet.Status.ObservedGeneration == 0 || statefulSet.Generation > statefulSet.Status.ObservedGeneration {
		return openchoreov1alpha1.HealthStatusProgressing, nil
	}

	// Get desired replicas (default to 1 if not specified)
	desiredReplicas := int32(1)
	if statefulSet.Spec.Replicas != nil {
		desiredReplicas = *statefulSet.Spec.Replicas
	}

	// Check for update in progress
	if statefulSet.Status.CurrentRevision != statefulSet.Status.UpdateRevision {
		return openchoreov1alpha1.HealthStatusProgressing, nil
	}

	// Determine health based on replica counts
	readyReplicas := statefulSet.Status.ReadyReplicas
	availableReplicas := statefulSet.Status.AvailableReplicas
	updatedReplicas := statefulSet.Status.UpdatedReplicas

	// All replicas are ready, available, and updated
	allReplicasMatch := desiredReplicas == readyReplicas && //nolint:gocritic // badCond: Valid StatefulSet health check logic
		desiredReplicas == availableReplicas &&
		desiredReplicas == updatedReplicas
	if allReplicasMatch {
		return openchoreov1alpha1.HealthStatusHealthy, nil
	}

	// If we have some ready replicas but not all, it's progressing
	return openchoreov1alpha1.HealthStatusProgressing, nil
}

func getPodHealth(obj *unstructured.Unstructured) (openchoreov1alpha1.HealthStatus, error) {
	// Convert an unstructured object to Pod
	var pod corev1.Pod
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &pod); err != nil {
		return openchoreov1alpha1.HealthStatusUnknown, fmt.Errorf("failed to convert to pod: %w", err)
	}

	// Check pod phase
	switch pod.Status.Phase {
	case corev1.PodPending:
		return openchoreov1alpha1.HealthStatusProgressing, nil
	case corev1.PodRunning:
		// Check if all containers are ready
		for _, containerStatus := range pod.Status.ContainerStatuses {
			// Check if container is in a waiting state with error before checking readiness
			if containerStatus.State.Waiting != nil {
				if containerStatus.State.Waiting.Reason == "CrashLoopBackOff" ||
					containerStatus.State.Waiting.Reason == "ImagePullBackOff" ||
					containerStatus.State.Waiting.Reason == "ErrImagePull" {
					return openchoreov1alpha1.HealthStatusDegraded, nil
				}
			}
			if !containerStatus.Ready {
				return openchoreov1alpha1.HealthStatusProgressing, nil
			}
		}
		return openchoreov1alpha1.HealthStatusHealthy, nil
	case corev1.PodSucceeded:
		return openchoreov1alpha1.HealthStatusHealthy, nil
	case corev1.PodFailed:
		return openchoreov1alpha1.HealthStatusDegraded, nil
	case corev1.PodUnknown:
		return openchoreov1alpha1.HealthStatusUnknown, nil
	default:
		return openchoreov1alpha1.HealthStatusUnknown, nil
	}
}

func getCronJobHealth(obj *unstructured.Unstructured) (openchoreov1alpha1.HealthStatus, error) {
	// Convert unstructured object to CronJob
	var cronJob batchv1.CronJob
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &cronJob); err != nil {
		return openchoreov1alpha1.HealthStatusUnknown, fmt.Errorf("failed to convert to cronjob: %w", err)
	}

	// Check if CronJob is suspended
	if cronJob.Spec.Suspend != nil && *cronJob.Spec.Suspend {
		return openchoreov1alpha1.HealthStatusSuspended, nil
	}

	// Check active jobs
	activeJobs := len(cronJob.Status.Active)

	// TODO: Fine tune - what makes a CronJob healthy in OpenChoreo?

	// If there's an active job, it's progressing
	if activeJobs > 0 {
		return openchoreov1alpha1.HealthStatusProgressing, nil
	}

	// Check last schedule time
	if cronJob.Status.LastScheduleTime != nil {
		// CronJob has run at least once, consider it healthy
		return openchoreov1alpha1.HealthStatusHealthy, nil
	}

	// CronJob hasn't run yet - could be newly created or waiting for schedule
	return openchoreov1alpha1.HealthStatusProgressing, nil
}

func getUnknownResourceHealth(obj *unstructured.Unstructured) (openchoreov1alpha1.HealthStatus, error) {
	// For unknown resources, we can't determine health status reliably
	// Resources like ConfigMaps, Secrets, Services, etc. don't have meaningful health states
	// They are either present or not, so if we got here, they exist
	return openchoreov1alpha1.HealthStatusHealthy, nil
}

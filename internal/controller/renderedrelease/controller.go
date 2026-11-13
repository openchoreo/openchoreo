// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
)

const (
	// ControllerName is the name of the controller managing RenderedRelease resources
	ControllerName = "renderedrelease-controller"

	targetPlaneDataPlane          = "dataplane"
	targetPlaneObservabilityPlane = "observabilityplane"

	appsAPIGroup    = "apps"
	deploymentKind  = "Deployment"
	statefulSetKind = "StatefulSet"

	batchAPIGroup = "batch"
	cronJobKind   = "CronJob"

	// reasonProgressDeadlineExceeded is the Deployment Progressing condition reason
	// Kubernetes sets when a rollout exceeds progressDeadlineSeconds.
	reasonProgressDeadlineExceeded = "ProgressDeadlineExceeded"

	// DefaultStableInterval is the stable requeue interval when neither the release nor
	// the controller sets one. Every stable release costs one reconcile per interval, so
	// with many releases on a remote plane it can need more workers than the controller
	// has; raise --renderedrelease-stable-interval there.
	DefaultStableInterval = 1 * time.Minute
	// DefaultProgressingInterval is the progressing requeue interval when neither the
	// release nor the controller sets one.
	DefaultProgressingInterval = 10 * time.Second
)

// Reconciler reconciles a RenderedRelease object
type Reconciler struct {
	client.Client
	PlaneClientProvider kubernetesClient.PlaneClientProvider
	Scheme              *runtime.Scheme

	// StableInterval is how long a release whose resources are all stable waits before
	// the next check, when its spec.interval is unset. Zero uses DefaultStableInterval.
	StableInterval time.Duration
	// ProgressingInterval is how long a release with transitioning resources waits before
	// the next check, when its spec.progressingInterval is unset. Zero uses
	// DefaultProgressingInterval.
	ProgressingInterval time.Duration
}

// TODO: Optimize to apply resource only if spec has changed
// TODO: Add events and conditions

// +kubebuilder:rbac:groups=openchoreo.dev,resources=renderedreleases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=renderedreleases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=renderedreleases/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterdataplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterobservabilityplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create
// +kubebuilder:rbac:groups="",resources=events,verbs=create
// +kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the RenderedRelease instance
	release := &openchoreov1alpha1.RenderedRelease{}
	if err := r.Get(ctx, req.NamespacedName, release); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("RenderedRelease resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get RenderedRelease")
		return ctrl.Result{}, err
	}

	old := release.DeepCopy()

	// Handle the deletion of the Release
	if !release.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing Release")
		return r.finalize(ctx, old, release)
	}

	// Ensure the finalizer is added to the Release
	if finalizerAdded, err := r.ensureFinalizer(ctx, release); err != nil || finalizerAdded {
		// Return after adding the finalizer to ensure the finalizer is persisted
		return ctrl.Result{}, err
	}

	// Get plane client (dataplane or observabilityplane) for the environment based on targetPlane
	targetPlane := release.Spec.TargetPlane
	if targetPlane == "" {
		targetPlane = targetPlaneDataPlane // Default to dataplane if not specified (avoid breaking change)
	}

	var planeClient client.Client
	var err error
	switch targetPlane {
	case targetPlaneObservabilityPlane:
		planeClient, err = r.getOPClient(ctx, release.Namespace, release.Spec.EnvironmentName)
		if err != nil {
			logger.Error(err, "Failed to get observability plane client")
			return ctrl.Result{}, err
		}
	case targetPlaneDataPlane:
		fallthrough
	default:
		planeClient, err = r.getDPClient(ctx, release.Namespace, release.Spec.EnvironmentName)
		if err != nil {
			logger.Error(err, "Failed to get dataplane client")
			return ctrl.Result{}, err
		}
	}

	// Observed resources are read in both modes and never written. They are read
	// before the apply so that a failed apply still publishes them.
	observedEntries := resolveObservedResources(release.Spec.ObservedResources)
	observed := r.readObservedResources(ctx, planeClient, old, observedEntries)
	setObservedConditions(release, observedEntries, observed.failures)

	// The mode only decides what happens to spec.resources. In Observe mode nothing
	// is desired, so the cleanup below removes whatever this release applied
	// earlier: that is the rollback when a binding moves back to observing.
	var desiredResources []*unstructured.Unstructured
	if effectiveMode(release) == openchoreov1alpha1.RenderedReleaseModeManage {
		desiredResources, err = r.makeDesiredResources(release)
		if err != nil {
			logger.Error(err, "Failed to make desired resources")
			return ctrl.Result{}, err
		}

		// An object that is both rendered and observed would be written by the apply.
		if err := checkObservedOverlap(desiredResources, observedEntries); err != nil {
			logger.Error(err, "Rendered resources overlap observed resources")
			return r.markApplyFailed(ctx, old, release, observed.statuses, err)
		}

		// Ensure namespaces exist before applying resources on the observability plane only.
		// The data plane's cell namespace is owned by the ProjectReleaseBinding, so the DP
		// apply path must not regain implicit namespace creation. On the observability plane
		// nothing else creates the target namespace, so we create it here.
		if targetPlane == targetPlaneObservabilityPlane {
			desiredNamespaces := r.makeDesiredNamespaces(release, desiredResources)
			if err := r.ensureNamespaces(ctx, planeClient, desiredNamespaces); err != nil {
				logger.Error(err, "Failed to ensure namespaces on observability plane")
				return ctrl.Result{}, err
			}
		}

		// PHASE 1: Apply desired resources to the target plane
		// This ensures all resources in the spec are created/updated with proper tracking labels
		if err := r.applyResources(ctx, planeClient, desiredResources); err != nil {
			logger.Error(err, "Failed to apply resources to target plane", "targetPlane", targetPlane)
			return r.markApplyFailed(ctx, old, release, observed.statuses, err)
		}

		// Mark resources as successfully applied, but leave persisting it to the status
		// update at the end of this reconcile.
		// Writing it here would publish a state that contradicts itself: the condition
		// would report this generation as applied while Status.Resources still holds
		// the health of the generation being replaced. A consumer that reads the
		// condition to decide whether the health beside it is current -- which is the
		// only signal available for that -- would act on the previous revision's
		// health. The delivery events built on it reported a rollout as succeeded
		// before its pods existed.
		//
		// The apply-failure branch above still persists immediately, because there the
		// point is to surface the error before returning.
		controller.MarkTrueCondition(release, controller.ConditionType(ConditionResourcesApplied),
			controller.ConditionReason(ReasonApplySucceeded), "All resources applied successfully")
	} else {
		// Nothing in spec.resources is applied in Observe mode, so the condition is
		// False, with a reason of its own to tell it from an apply failure. How the
		// observed resources were read is ObservedResourcesRead's to report. Like the
		// apply success above, this is persisted with the status at the end, not at
		// once like an apply failure.
		controller.MarkFalseCondition(release, controller.ConditionType(ConditionResourcesApplied),
			controller.ConditionReason(ReasonObserveMode),
			"Observe mode: spec.resources are not applied, and anything applied earlier is removed")
	}

	// PHASE 2: Discover live resources that we manage in the target plane
	// This queries both current resource types (from spec) and previous resource types (from status)
	// to ensure we find all resources that might need cleanup, preventing resource leaks
	gvks := findAllKnownGVKs(desiredResources, release.Status.Resources, targetPlane)
	liveResources, err := r.listLiveResourcesByGVKs(ctx, planeClient, release, gvks)
	if err != nil {
		logger.Error(err, "Failed to list live resources from target plane", "targetPlane", targetPlane)
		return ctrl.Result{}, err
	}

	// PHASE 3: Find and delete stale resources (cleanup orphaned resources)
	// Stale = live resources that are no longer in the desired spec (e.g., user removed a ConfigMap)
	// This implements Flux-style inventory cleanup to prevent resource accumulation over time
	// An observed object is never deleted, even one this release applied before it
	// was listed as observed.
	staleResources := excludeObserved(r.findStaleResources(liveResources, desiredResources), observedEntries)
	if err := r.deleteResources(ctx, planeClient, staleResources); err != nil {
		logger.Error(err, "Failed to delete stale resources")
		return ctrl.Result{}, err
	}

	// Build the resource statuses first so delivery events and the status update
	// share one health evaluation
	resourceStatuses := r.buildResourceStatus(ctx, old, desiredResources, liveResources)

	// PHASE 4: Update status with applied resources inventory (done last after all operations)
	// This maintains an inventory of what we applied for future cleanup operations.
	//
	// A successful write does not end the reconcile. The watch drops the
	// controller's own status writes (ignoreStatusOnlyUpdates), so the requeue
	// below is what brings this release back. Re-queuing on the write instead
	// loops forever when a live object's status changes on its own, such as an
	// ExternalSecret's refreshTime.
	if _, err := r.updateStatus(ctx, old, release, resourceStatuses, observed.statuses); err != nil {
		return ctrl.Result{}, err
	}

	// Pick the next check from the state just observed. Both the delay and the
	// queue priority are set on every path, so they follow the release's state
	// rather than whatever priority the request happened to be dequeued with.
	// Polls run at low priority so that real changes (spec, restart annotation,
	// deletion), which arrive through the watch at priority 0, never wait behind them.
	// Observed resources count too; transitioningRequeuePriority weighs them. So does
	// a failed read, whose entry may still show its last health.
	if r.hasTransitioningResources(release.Status.Resources) || r.hasTransitioningResources(release.Status.ObservedResources) ||
		len(observed.failures) > 0 {
		requeueAfter := getProgressingRequeueInterval(release, r.ProgressingInterval)
		priority := transitioningRequeuePriority(release.Status.Resources, release.Status.ObservedResources)
		logger.Info("Resources are transitioning, requeuing with configured interval",
			"requeueAfter", requeueAfter, "priority", priority)
		// A zero progressingInterval disables the requeue: RequeueAfter 0 means
		// none, and with the controller's own status writes filtered nothing else
		// brings the release back until its spec or metadata changes.
		return ctrl.Result{RequeueAfter: requeueAfter, Priority: ptr.To(priority)}, nil
	}

	// Poll workloads scaled to zero faster so an autoscaler-driven scale-up is noticed promptly.
	if hasResurrectableWorkload(desiredResources, liveResources) || hasResurrectableLiveWorkload(observed.live) {
		requeueAfter := getResurrectableRequeueInterval()
		logger.Info("Workload is scaled to zero, requeuing to detect autoscaler-driven scale-up",
			"requeueAfter", requeueAfter)
		return ctrl.Result{RequeueAfter: requeueAfter, Priority: ptr.To(requeuePriorityPoll)}, nil
	}

	requeueAfter := getStableRequeueInterval(release, r.StableInterval)
	logger.Info("Successfully applied the Release resources to the target plane",
		"targetPlane", targetPlane, "requeueAfter", requeueAfter)
	return ctrl.Result{RequeueAfter: requeueAfter, Priority: ptr.To(requeuePriorityPoll)}, nil
}

// markApplyFailed persists ResourcesApplied=False with err and returns err. It is
// written at once, so upstream controllers (e.g., ReleaseBinding) can surface the
// error before the reconcile returns. The observed resources read in this pass are
// written with it; the applied inventory is kept as it was, since nothing new was
// applied.
func (r *Reconciler) markApplyFailed(ctx context.Context, old, release *openchoreov1alpha1.RenderedRelease,
	observedStatuses []openchoreov1alpha1.RenderedManifestStatus, err error) (ctrl.Result, error) {
	controller.MarkFalseCondition(release, controller.ConditionType(ConditionResourcesApplied),
		controller.ConditionReason(ReasonApplyFailed),
		fmt.Sprintf("Failed to apply resources to target plane: %v", err))
	if _, statusErr := r.updateStatus(ctx, old, release, release.Status.Resources, observedStatuses); statusErr != nil {
		log.FromContext(ctx).Error(statusErr, "Failed to update Release status with apply error")
	}
	return ctrl.Result{}, err
}

// getDPClient gets the dataplane client for the specified environment
func (r *Reconciler) getDPClient(ctx context.Context, namespaceName string, environmentName string) (client.Client, error) {
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

// getOPClient gets the observability plane client for the specified environment
// It follows the chain: Environment -> DataPlane -> ObservabilityPlane
func (r *Reconciler) getOPClient(ctx context.Context, namespaceName string, environmentName string) (client.Client, error) {
	env := &openchoreov1alpha1.Environment{}
	if err := r.Get(ctx, client.ObjectKey{Name: environmentName, Namespace: namespaceName}, env); err != nil {
		return nil, fmt.Errorf("failed to get environment %s: %w", environmentName, err)
	}

	dataPlaneResult, err := controller.GetDataPlaneFromRef(ctx, r.Client, env.Namespace, env.Spec.DataPlaneRef)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve dataplane for environment %s: %w", environmentName, err)
	}

	obsResult, err := dataPlaneResult.GetObservabilityPlane(ctx, r.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve observability plane for dataplane %s: %w", dataPlaneResult.GetName(), err)
	}

	opClient, err := obsResult.GetK8sClient(r.PlaneClientProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create observability plane client for %s: %w", obsResult.GetName(), err)
	}

	return opClient, nil
}

// applyResources applies the given resources to the target plane
func (r *Reconciler) applyResources(ctx context.Context, planeClient client.Client, resources []*unstructured.Unstructured) error {
	for _, obj := range resources {
		resourceID := obj.GetLabels()[labels.LabelKeyRenderedReleaseResourceID]

		// Apply the resource using server-side apply
		if err := planeClient.Patch(ctx, obj, client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName)); err != nil {
			return fmt.Errorf("failed to apply resource %s: %w", resourceID, err)
		}
	}

	return nil
}

// makeDesiredResources creates the desired resources from the Release spec
func (r *Reconciler) makeDesiredResources(release *openchoreov1alpha1.RenderedRelease) ([]*unstructured.Unstructured, error) {
	desiredObjects := make([]*unstructured.Unstructured, 0, len(release.Spec.Resources))

	restartedAt := release.Annotations[controller.AnnotationKeyRestartedAt]

	for _, resource := range release.Spec.Resources {
		// Convert RawExtension to Unstructured
		obj := &unstructured.Unstructured{}
		if err := obj.UnmarshalJSON(resource.Object.Raw); err != nil {
			return nil, fmt.Errorf("failed to unmarshal resource %s: %w", resource.ID, err)
		}

		// Add tracking labels
		resourceLabels := obj.GetLabels()
		if resourceLabels == nil {
			resourceLabels = make(map[string]string)
		}
		resourceLabels[labels.LabelKeyManagedBy] = ControllerName
		resourceLabels[labels.LabelKeyRenderedReleaseResourceID] = resource.ID
		resourceLabels[labels.LabelKeyRenderedReleaseUID] = string(release.UID)
		resourceLabels[labels.LabelKeyRenderedReleaseName] = release.Name
		resourceLabels[labels.LabelKeyRenderedReleaseNamespace] = release.Namespace

		obj.SetLabels(resourceLabels)

		if restartedAt != "" {
			if err := injectRestartedAt(obj, restartedAt); err != nil {
				return nil, fmt.Errorf("failed to inject restartedAt on resource %s: %w", resource.ID, err)
			}
		}

		desiredObjects = append(desiredObjects, obj)
	}

	return desiredObjects, nil
}

// injectRestartedAt sets openchoreo.dev/restartedAt on the pod template of an
// apps/v1 Deployment so a change to the value causes the data plane to perform
// a rolling restart. It is a no-op for any other kind. Returns an error if the
// manifest is malformed (e.g. annotations is not a string map), so the caller
// can surface it instead of silently dropping the restart trigger.
func injectRestartedAt(obj *unstructured.Unstructured, value string) error {
	gvk := obj.GroupVersionKind()
	if gvk.Group != appsAPIGroup || gvk.Kind != deploymentKind {
		return nil
	}
	annotations, _, err := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "annotations")
	if err != nil {
		return fmt.Errorf("read pod template annotations: %w", err)
	}
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[controller.AnnotationKeyRestartedAt] = value
	if err := unstructured.SetNestedStringMap(obj.Object, annotations, "spec", "template", "metadata", "annotations"); err != nil {
		return fmt.Errorf("set pod template annotations: %w", err)
	}
	return nil
}

// makeDesiredNamespaces creates namespace objects from the desired resources with proper labels
func (r *Reconciler) makeDesiredNamespaces(release *openchoreov1alpha1.RenderedRelease, resources []*unstructured.Unstructured) []*corev1.Namespace {
	namespaceMap := make(map[string]*corev1.Namespace)

	for _, obj := range resources {
		namespaceName := obj.GetNamespace()
		if namespaceName != "" {
			if _, exists := namespaceMap[namespaceName]; !exists {
				namespaceMap[namespaceName] = &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: namespaceName,
						Labels: map[string]string{
							// Audit labels - track which release created this namespace
							labels.LabelKeyCreatedBy:                ControllerName,
							labels.LabelKeyRenderedReleaseName:      release.Name,
							labels.LabelKeyRenderedReleaseNamespace: release.Namespace,
							labels.LabelKeyRenderedReleaseUID:       string(release.UID),

							// Identification labels - track where this namespace belongs
							labels.LabelKeyNamespaceName:   release.Namespace,
							labels.LabelKeyEnvironmentName: release.Spec.EnvironmentName,
							labels.LabelKeyProjectName:     release.Spec.Owner.ProjectName,
						},
					},
				}
			}
		}
	}

	// Convert the map to a slice
	namespaces := make([]*corev1.Namespace, 0, len(namespaceMap))
	for _, ns := range namespaceMap {
		namespaces = append(namespaces, ns)
	}

	return namespaces
}

// ensureNamespaces ensures all required namespaces exist in the target plane
func (r *Reconciler) ensureNamespaces(ctx context.Context, planeClient client.Client, namespaces []*corev1.Namespace) error {
	for _, namespace := range namespaces {
		existingNs := &corev1.Namespace{}
		err := planeClient.Get(ctx, client.ObjectKey{Name: namespace.Name}, existingNs)

		// Namespace already exists, skip to next
		if err == nil {
			continue
		}

		// Error other than NotFound
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to check namespace %s: %w", namespace.Name, err)
		}

		// Namespace doesn't exist, create it
		if err := planeClient.Create(ctx, namespace); err != nil {
			if apierrors.IsAlreadyExists(err) {
				// Another controller/release created it concurrently - that's fine
				continue
			}
			return fmt.Errorf("failed to create namespace %s: %w", namespace.Name, err)
		}
	}

	return nil
}

// findStaleResources finds resources that this controller applied but are no longer in the
// desired spec. Live objects it did not apply are never stale, whatever labels they carry
// (see appliedByController).
func (r *Reconciler) findStaleResources(liveResources, desiredResources []*unstructured.Unstructured) []*unstructured.Unstructured {
	// Build a set of desired resource IDs for fast lookup
	desiredResourceIDs := make(map[string]bool)
	for _, obj := range desiredResources {
		resourceID := obj.GetLabels()[labels.LabelKeyRenderedReleaseResourceID]
		if resourceID != "" {
			desiredResourceIDs[resourceID] = true
		}
	}

	// Find live resources that are not in the desired set
	var staleResources []*unstructured.Unstructured
	for _, liveObj := range appliedByController(liveResources) {
		liveResourceID := liveObj.GetLabels()[labels.LabelKeyRenderedReleaseResourceID]
		if liveResourceID != "" {
			// If this live resource ID is not in the desired set, it's stale
			if !desiredResourceIDs[liveResourceID] {
				staleResources = append(staleResources, liveObj)
			}
		}
	}

	return staleResources
}

// appliedByController returns the live resources this controller applied: those whose
// managedFields hold a server-side apply entry from its field manager.
//
// The tracking labels alone do not say that. Operators copy labels onto the objects they
// create (ESO copies an ExternalSecret's labels onto its Secret when the ExternalSecret has
// no target template), and such an object is listed as if it were ours. managedFields are
// not copied, and applyResources records an Apply entry under ControllerName on everything
// it writes. Deleting only what carries that entry leaves those objects to their owner: the
// Secret of an ExternalSecret with creationPolicy Orphan outlives it, and one with Merge
// existed before it.
func appliedByController(liveResources []*unstructured.Unstructured) []*unstructured.Unstructured {
	applied := make([]*unstructured.Unstructured, 0, len(liveResources))
	for _, obj := range liveResources {
		for _, entry := range obj.GetManagedFields() {
			if entry.Manager == ControllerName && entry.Operation == metav1.ManagedFieldsOperationApply {
				applied = append(applied, obj)
				break
			}
		}
	}
	return applied
}

// deleteResources deletes the given stale resources from the target plane
func (r *Reconciler) deleteResources(ctx context.Context, planeClient client.Client, staleResources []*unstructured.Unstructured) error {
	for _, obj := range staleResources {
		resourceID := obj.GetLabels()[labels.LabelKeyRenderedReleaseResourceID]

		// Skip resources already terminating on the target plane (re-deleting over
		// the gateway tunnel is a wasted round-trip on the most expensive I/O path).
		if obj.GetDeletionTimestamp() != nil {
			continue
		}

		// Delete the resource from the target plane
		if err := planeClient.Delete(ctx, obj); err != nil {
			return fmt.Errorf("failed to delete stale resource %s: %w", resourceID, err)
		}
	}

	return nil
}

// wellKnownDataPlaneGVKs is the safety-net list for data-plane rendered releases.
// It covers common Kubernetes resource types that the data-plane agent has permission
// to list, so orphaned resources are caught even when status update fails.
var wellKnownDataPlaneGVKs = []schema.GroupVersionKind{
	// Core Kubernetes Resources
	{Group: "", Version: "v1", Kind: "Service"},
	{Group: "", Version: "v1", Kind: "ConfigMap"},
	{Group: "", Version: "v1", Kind: "Secret"},
	{Group: "", Version: "v1", Kind: "ServiceAccount"},
	{Group: "", Version: "v1", Kind: "Namespace"},
	{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"},

	// Apps
	{Group: appsAPIGroup, Version: "v1", Kind: deploymentKind},
	{Group: appsAPIGroup, Version: "v1", Kind: statefulSetKind},

	// Batch
	{Group: "batch", Version: "v1", Kind: "Job"},
	{Group: "batch", Version: "v1", Kind: "CronJob"},

	// Autoscaling & Policy
	{Group: "autoscaling", Version: "v2", Kind: "HorizontalPodAutoscaler"},
	{Group: "policy", Version: "v1", Kind: "PodDisruptionBudget"},

	// Networking
	{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"},
	{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"},

	// RBAC
	{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
	{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"},
	{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"},
	{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRoleBinding"},

	// Gateway API
	{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute"},
	{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway"},
}

// wellKnownObservabilityPlaneGVKs is the safety-net list for observability-plane rendered
// releases. It is intentionally narrow: only the CRDs that the observability-plane agent
// reconciles and has RBAC permission to list. Broad Kubernetes types (Services, Deployments,
// etc.) and control-plane-only CRDs (e.g. ObservabilityAlertsNotificationChannel) are
// excluded because the observability-plane agent's RBAC does not cover them.
var wellKnownObservabilityPlaneGVKs = []schema.GroupVersionKind{
	{Group: "openchoreo.dev", Version: "v1alpha1", Kind: "ObservabilityAlertRule"},
}

// findAllKnownGVKs finds all GroupVersionKinds that we should query for cleanup.
//
// This function is critical for preventing resource leaks during cleanup. It combines resource types
// from three sources to ensure comprehensive coverage:
//
// 1. DESIRED RESOURCES (current spec): Resource types the user wants now
//   - Handles new resource types added to the spec
//   - Ensures we query current resource types for updates
//
// 2. APPLIED RESOURCES (previously applied): Resource types we managed before
//   - Handles resource types that were removed from the spec
//   - Prevents orphaned resources when user removes entire resource types
//
// 3. WELL-KNOWN TYPES: Plane-specific resource types we typically manage
//   - Handles edge cases where resources exist but status update failed
//   - Provides safety net for orphaned resources from failed reconciliations
//   - Scoped per target plane so only types the plane's agent can list are queried
//
// Example scenario:
//   - Previous reconciliation: Applied ConfigMap + Secret
//   - Current reconciliation: User removed ConfigMap, kept Secret
//   - Without status: Would only query Secret, miss orphaned ConfigMap
//   - With status: Queries both Secret + ConfigMap, finds and deletes orphaned ConfigMap
func findAllKnownGVKs(desiredResources []*unstructured.Unstructured, appliedResources []openchoreov1alpha1.RenderedManifestStatus, targetPlane string) []schema.GroupVersionKind {
	gvkSet := make(map[schema.GroupVersionKind]bool)

	// Add GVKs from desired resources (current spec)
	// This ensures we query resource types the user wants now
	for _, obj := range desiredResources {
		gvk := obj.GroupVersionKind()
		gvkSet[gvk] = true
	}

	// Add GVKs from applied resources (previously applied)
	// This ensures we query resource types we managed before, even if removed from spec
	for _, appliedResource := range appliedResources {
		gvk := schema.GroupVersionKind{
			Group:   appliedResource.Group,
			Version: appliedResource.Version,
			Kind:    appliedResource.Kind,
		}
		gvkSet[gvk] = true
	}

	// Add plane-specific well-known GVKs as a safety net for orphaned resources.
	// Using a plane-scoped list ensures we only probe types the plane agent can list,
	// avoiding spurious RBAC errors from querying types the plane doesn't manage.
	wellKnownGVKs := wellKnownDataPlaneGVKs
	if targetPlane == targetPlaneObservabilityPlane {
		wellKnownGVKs = wellKnownObservabilityPlaneGVKs
	}
	for _, gvk := range wellKnownGVKs {
		gvkSet[gvk] = true
	}

	// Convert set to slice after all sources have been merged
	gvks := make([]schema.GroupVersionKind, 0, len(gvkSet))
	for gvk := range gvkSet {
		gvks = append(gvks, gvk)
	}

	return gvks
}

// listLiveResourcesByGVKs queries specific resource types with label selector
func (r *Reconciler) listLiveResourcesByGVKs(ctx context.Context, planeClient client.Client, release *openchoreov1alpha1.RenderedRelease, gvks []schema.GroupVersionKind) ([]*unstructured.Unstructured, error) {
	var allLiveResources []*unstructured.Unstructured

	// Query each GVK with our label selector
	for _, gvk := range gvks {
		// Create unstructured list for this GVK
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   gvk.Group,
			Version: gvk.Version,
			Kind:    gvk.Kind + "List", // e.g., "Deployment" -> "DeploymentList"
		})

		// Build label selector
		labelSelector := metav1.LabelSelector{
			MatchLabels: map[string]string{
				labels.LabelKeyManagedBy:          ControllerName,
				labels.LabelKeyRenderedReleaseUID: string(release.UID),
			},
		}
		selector, err := metav1.LabelSelectorAsSelector(&labelSelector)
		if err != nil {
			return nil, fmt.Errorf("failed to create label selector: %w", err)
		}

		// List resources with label selector
		if err := planeClient.List(ctx, list, &client.ListOptions{
			LabelSelector: selector,
		}); err != nil {
			if apimeta.IsNoMatchError(err) {
				// GVK is not registered in this cluster — silently skip
				continue
			}
			return nil, fmt.Errorf("failed to list resources for %s: %w", gvk.String(), err)
		}

		// Stamp each item with the GVK it was listed under: live resources are paired
		// with desired ones by group and kind as well as name. The unstructured decoder
		// only fills an item's kind from the list when the response names the list's
		// own, so this does not rely on the plane client to do it.
		for i := range list.Items {
			item := &list.Items[i]
			item.SetGroupVersionKind(gvk)
			allLiveResources = append(allLiveResources, item)
		}
	}

	return allLiveResources, nil
}

// getStableRequeueInterval returns the requeue interval for stable resources: the
// release's spec.interval when set, else defaultInterval, else DefaultStableInterval.
// Returns zero duration if spec.interval is set to 0 (no requeue)
func getStableRequeueInterval(release *openchoreov1alpha1.RenderedRelease, defaultInterval time.Duration) time.Duration {
	baseInterval := defaultInterval
	if baseInterval <= 0 {
		baseInterval = DefaultStableInterval
	}
	if release.Spec.Interval != nil {
		baseInterval = release.Spec.Interval.Duration
		// If set to 0, don't requeue
		if baseInterval == 0 {
			return 0
		}
	}

	// Add 20% jitter
	jitterMax := time.Duration(float64(baseInterval) * 0.2)
	return addJitter(baseInterval, jitterMax)
}

// getResurrectableRequeueInterval returns the requeue interval for workloads scaled to zero
// that an autoscaler may resurrect: faster than the stable cadence so a scale-up is noticed
// promptly, but slow enough not to hammer the plane agent when many workloads sit idle.
func getResurrectableRequeueInterval() time.Duration {
	baseInterval := 1 * time.Minute
	jitterMax := time.Duration(float64(baseInterval) * 0.2)
	return addJitter(baseInterval, jitterMax)
}

// getProgressingRequeueInterval returns the requeue interval for transitioning resources:
// the release's spec.progressingInterval when set, else defaultInterval, else
// DefaultProgressingInterval.
// Returns zero duration if progressingInterval is set to 0 (no requeue)
func getProgressingRequeueInterval(release *openchoreov1alpha1.RenderedRelease, defaultInterval time.Duration) time.Duration {
	baseInterval := defaultInterval
	if baseInterval <= 0 {
		baseInterval = DefaultProgressingInterval
	}
	if release.Spec.ProgressingInterval != nil {
		baseInterval = release.Spec.ProgressingInterval.Duration
		// If set to 0, don't requeue
		if baseInterval == 0 {
			return 0
		}
	}

	// Add 20% jitter
	jitterMax := time.Duration(float64(baseInterval) * 0.2)
	return addJitter(baseInterval, jitterMax)
}

// addJitter adds a random jitter to the base duration to prevent thundering herd
// For example, addJitter(10*time.Second, 5*time.Second) returns 10-15 seconds
func addJitter(base time.Duration, maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return base
	}
	jitter := time.Duration(rand.Intn(int(maxJitter))) //nolint:gosec // Non-cryptographic randomness is acceptable for jitter
	return base + jitter
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.RenderedRelease{}, builder.WithPredicates(ignoreStatusOnlyUpdates())).
		Named("renderedrelease").
		Complete(r)
}

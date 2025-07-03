// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package release

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	dpKubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
)

const (
	// Controller name for managed-by label
	ControllerName = "release-controller"
)

// Reconciler reconciles a Release object
type Reconciler struct {
	client.Client
	DpClientMgr *dpKubernetes.KubeClientManager
	Scheme      *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.choreo.dev,resources=releases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=releases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=releases/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Release instance
	release := &choreov1.Release{}
	if err := r.Get(ctx, req.NamespacedName, release); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Release resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Release")
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

	// Get dataplane client for the environment
	dpClient, err := r.getDPClient(ctx, release.Spec.EnvironmentName)
	if err != nil {
		logger.Error(err, "Failed to get dataplane client")
		return ctrl.Result{}, err
	}

	// Get desired resources from spec
	desiredResources, err := r.makeDesiredResources(release)
	if err != nil {
		logger.Error(err, "Failed to make desired resources")
		return ctrl.Result{}, err
	}

	// PHASE 1: Apply desired resources to the dataplane
	// This ensures all resources in the spec are created/updated with proper tracking labels
	if err := r.applyResources(ctx, dpClient, desiredResources); err != nil {
		logger.Error(err, "Failed to apply resources to dataplane")
		return ctrl.Result{}, err
	}

	// PHASE 2: Discover live resources that we manage in the dataplane
	// This queries both current resource types (from spec) and previous resource types (from status)
	// to ensure we find all resources that might need cleanup, preventing resource leaks
	gvks := findAllKnownGVKs(desiredResources, release.Status.Resources)
	liveResources, err := r.listLiveResourcesByGVKs(ctx, dpClient, release, gvks)
	if err != nil {
		logger.Error(err, "Failed to list live resources from dataplane")
		return ctrl.Result{}, err
	}

	// PHASE 3: Find and delete stale resources (cleanup orphaned resources)
	// Stale = live resources that are no longer in the desired spec (e.g., user removed a ConfigMap)
	// This implements Flux-style inventory cleanup to prevent resource accumulation over time
	staleResources := r.findStaleResources(liveResources, desiredResources)
	if err := r.deleteResources(ctx, dpClient, staleResources); err != nil {
		logger.Error(err, "Failed to delete stale resources")
		return ctrl.Result{}, err
	}

	// PHASE 4: Update status with applied resources inventory (done last after all operations)
	// This maintains an inventory of what we applied for future cleanup operations
	if err := r.updateStatus(ctx, release, desiredResources); err != nil {
		logger.Error(err, "Failed to update Release status")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully applied Release resources to dataplane")
	return ctrl.Result{}, nil
}

// getDPClient gets the dataplane client for the specified environment
func (r *Reconciler) getDPClient(ctx context.Context, environmentName string) (client.Client, error) {
	// Fetch the environment from default namespace
	env := &choreov1.Environment{}
	if err := r.Get(ctx, client.ObjectKey{Name: environmentName, Namespace: "default"}, env); err != nil {
		return nil, fmt.Errorf("failed to get environment %s: %w", environmentName, err)
	}

	// Get the dataplane using the direct reference from default namespace
	dataplane := &choreov1.DataPlane{}
	if err := r.Get(ctx, client.ObjectKey{Name: env.Spec.DataPlaneRef, Namespace: "default"}, dataplane); err != nil {
		return nil, fmt.Errorf("failed to get dataplane %s for environment %s: %w", env.Spec.DataPlaneRef, environmentName, err)
	}

	// Get the dataplane client
	dpClient, err := r.DpClientMgr.GetClient(dataplane.Name, dataplane.Spec.KubernetesCluster.Credentials)
	if err != nil {
		return nil, fmt.Errorf("failed to create dataplane client for %s: %w", dataplane.Name, err)
	}

	return dpClient, nil
}

// applyResources applies the given resources to the dataplane
func (r *Reconciler) applyResources(ctx context.Context, dpClient client.Client, resources []*unstructured.Unstructured) error {
	for _, obj := range resources {
		resourceID := obj.GetLabels()[labels.LabelKeyReleaseResourceID]

		// Apply the resource using server-side apply
		if err := dpClient.Patch(ctx, obj, client.Apply, client.ForceOwnership, client.FieldOwner(ControllerName)); err != nil {
			return fmt.Errorf("failed to apply resource %s: %w", resourceID, err)
		}
	}

	return nil
}

// makeDesiredResources creates the desired resources from the Release spec
func (r *Reconciler) makeDesiredResources(release *choreov1.Release) ([]*unstructured.Unstructured, error) {
	var desiredObjects []*unstructured.Unstructured

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
		resourceLabels[labels.LabelKeyReleaseResourceID] = resource.ID
		resourceLabels[labels.LabelKeyReleaseUID] = string(release.UID)

		obj.SetLabels(resourceLabels)

		desiredObjects = append(desiredObjects, obj)
	}

	return desiredObjects, nil
}

// makeResourceStatus converts applied unstructured objects to ResourceStatus entries
func (r *Reconciler) makeResourceStatus(resources []*unstructured.Unstructured) []choreov1.ResourceStatus {
	var resourceStatuses []choreov1.ResourceStatus

	for _, obj := range resources {
		gvk := obj.GroupVersionKind()
		resourceID := obj.GetLabels()[labels.LabelKeyReleaseResourceID]

		status := choreov1.ResourceStatus{
			ID:        resourceID,
			Group:     gvk.Group,
			Version:   gvk.Version,
			Kind:      gvk.Kind,
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		}

		resourceStatuses = append(resourceStatuses, status)
	}

	return resourceStatuses
}

// updateStatus updates the Release status with applied resources
func (r *Reconciler) updateStatus(ctx context.Context, release *choreov1.Release, appliedResources []*unstructured.Unstructured) error {
	logger := log.FromContext(ctx)

	// Build resource status from applied resources
	resourceStatuses := r.makeResourceStatus(appliedResources)

	// Update the status
	release.Status.Resources = resourceStatuses

	// Update the Release status
	if err := r.Status().Update(ctx, release); err != nil {
		logger.Error(err, "Failed to update Release status")
		return fmt.Errorf("failed to update status: %w", err)
	}

	logger.Info("Successfully updated Release status", "resourceCount", len(resourceStatuses))
	return nil
}

// findStaleResources finds resources that were previously managed but are no longer in the desired spec
func (r *Reconciler) findStaleResources(liveResources, desiredResources []*unstructured.Unstructured) []*unstructured.Unstructured {
	// Build a set of desired resource IDs for fast lookup
	desiredResourceIDs := make(map[string]bool)
	for _, obj := range desiredResources {
		resourceID := obj.GetLabels()[labels.LabelKeyReleaseResourceID]
		if resourceID != "" {
			desiredResourceIDs[resourceID] = true
		}
	}

	// Find live resources that are not in the desired set
	var staleResources []*unstructured.Unstructured
	for _, liveObj := range liveResources {
		liveResourceID := liveObj.GetLabels()[labels.LabelKeyReleaseResourceID]
		if liveResourceID != "" {
			// If this live resource ID is not in the desired set, it's stale
			if !desiredResourceIDs[liveResourceID] {
				staleResources = append(staleResources, liveObj)
			}
		}
	}

	return staleResources
}

// deleteResources deletes the given stale resources from the dataplane
func (r *Reconciler) deleteResources(ctx context.Context, dpClient client.Client, staleResources []*unstructured.Unstructured) error {
	for _, obj := range staleResources {
		resourceID := obj.GetLabels()[labels.LabelKeyReleaseResourceID]

		// Delete the resource from the dataplane
		if err := dpClient.Delete(ctx, obj); err != nil {
			return fmt.Errorf("failed to delete stale resource %s: %w", resourceID, err)
		}
	}

	return nil
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
// 3. WELL-KNOWN TYPES: Common Kubernetes resource types we typically manage
//   - Handles edge cases where resources exist but status update failed
//   - Provides safety net for orphaned resources from failed reconciliations
//
// Example scenario:
//   - Previous reconciliation: Applied ConfigMap + Secret
//   - Current reconciliation: User removed ConfigMap, kept Secret
//   - Without status: Would only query Secret, miss orphaned ConfigMap
//   - With status: Queries both Secret + ConfigMap, finds and deletes orphaned ConfigMap
//
// This approach automatically supports any CRDs (Gateway, Cilium, etc.) without hardcoded lists.
func findAllKnownGVKs(desiredResources []*unstructured.Unstructured, appliedResources []choreov1.ResourceStatus) []schema.GroupVersionKind {
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

	// Convert set to slice for iteration
	var gvks []schema.GroupVersionKind
	for gvk := range gvkSet {
		gvks = append(gvks, gvk)
	}

	// Add well-known GVKs that are commonly managed by controllers
	// This provides a safety net for resources that might be orphaned due to failed status updates
	wellKnownGVKs := []schema.GroupVersionKind{
		// Core Kubernetes Resources
		{Group: "", Version: "v1", Kind: "Service"},
		{Group: "", Version: "v1", Kind: "ConfigMap"},
		{Group: "", Version: "v1", Kind: "Secret"},
		{Group: "", Version: "v1", Kind: "ServiceAccount"},
		{Group: "", Version: "v1", Kind: "Namespace"},
		{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"},

		// Apps
		{Group: "apps", Version: "v1", Kind: "Deployment"},
		{Group: "apps", Version: "v1", Kind: "StatefulSet"},

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

		// Envoy Gateway
		{Group: "gateway.envoyproxy.io", Version: "v1alpha1", Kind: "SecurityPolicy"},
		{Group: "gateway.envoyproxy.io", Version: "v1alpha1", Kind: "BackendTrafficPolicy"},
		{Group: "gateway.envoyproxy.io", Version: "v1alpha1", Kind: "HTTPRouteFilter"},

		// Third-party CRDs
		{Group: "cilium.io", Version: "v2", Kind: "CiliumNetworkPolicy"},
		{Group: "secrets-store.csi.x-k8s.io", Version: "v1", Kind: "SecretProviderClass"},
	}
	for _, gvk := range wellKnownGVKs {
		gvkSet[gvk] = true
	}

	return gvks
}

// listLiveResourcesByGVKs queries specific resource types with label selector
func (r *Reconciler) listLiveResourcesByGVKs(ctx context.Context, dpClient client.Client, release *choreov1.Release, gvks []schema.GroupVersionKind) ([]*unstructured.Unstructured, error) {
	logger := log.FromContext(ctx)

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
				labels.LabelKeyManagedBy:  ControllerName,
				labels.LabelKeyReleaseUID: string(release.UID),
			},
		}
		selector, err := metav1.LabelSelectorAsSelector(&labelSelector)
		if err != nil {
			return nil, fmt.Errorf("failed to create label selector: %w", err)
		}

		// List resources with label selector
		if err := dpClient.List(ctx, list, &client.ListOptions{
			LabelSelector: selector,
		}); err != nil {
			logger.Error(err, "Failed to list resources", "gvk", gvk.String())
			continue // Continue with other GVKs instead of failing
		}

		// Add all items to result
		for i := range list.Items {
			allLiveResources = append(allLiveResources, &list.Items[i])
		}
	}

	logger.Info("Total live resources found", "count", len(allLiveResources))
	return allLiveResources, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.Release{}).
		Named("release").
		Complete(r)
}

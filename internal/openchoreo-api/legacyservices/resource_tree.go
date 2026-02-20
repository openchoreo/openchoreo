// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/controller"
	releasecontroller "github.com/openchoreo/openchoreo/internal/controller/release"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

const (
	planeTypeDataPlane = "dataplane"
	maxResponseBytes   = 10 * 1024 * 1024 // 10MB
)

// GetReleaseResourceTree returns a hierarchical view of all live Kubernetes resources
// deployed by a Release, including child resources (ReplicaSets, Pods, Jobs).
func (s *ComponentService) GetReleaseResourceTree(ctx context.Context, namespaceName, projectName, componentName,
	environmentName string) (*models.ResourceTreeResponse, error) {
	s.logger.Debug("Getting release resource tree", "namespace", namespaceName, "project", projectName,
		"component", componentName, "environment", environmentName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewComponent, ResourceTypeComponent, componentName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
		return nil, err
	}

	if s.gatewayClient == nil {
		return nil, fmt.Errorf("gateway client is not configured")
	}

	// 1. Get the Release (reuse pattern from GetEnvironmentRelease)
	componentKey := client.ObjectKey{
		Namespace: namespaceName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrComponentNotFound
		}
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if component.Spec.Owner.ProjectName != projectName {
		return nil, ErrComponentNotFound
	}

	var releaseList openchoreov1alpha1.ReleaseList
	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
		client.MatchingLabels{
			labels.LabelKeyNamespaceName:   namespaceName,
			labels.LabelKeyProjectName:     projectName,
			labels.LabelKeyComponentName:   componentName,
			labels.LabelKeyEnvironmentName: environmentName,
		},
	}

	if err := s.k8sClient.List(ctx, &releaseList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}

	if len(releaseList.Items) == 0 {
		return nil, ErrReleaseNotFound
	}

	if len(releaseList.Items) > 1 {
		return nil, fmt.Errorf("expected 1 release for component/environment, found %d", len(releaseList.Items))
	}

	release := &releaseList.Items[0]

	// 2. Resolve data plane info
	env := &openchoreov1alpha1.Environment{}
	envKey := client.ObjectKey{
		Name:      environmentName,
		Namespace: namespaceName,
	}
	if err := s.k8sClient.Get(ctx, envKey, env); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	dpResult, err := controller.GetDataPlaneOrClusterDataPlaneOfEnv(ctx, s.k8sClient, env)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve data plane: %w", err)
	}

	planeID, crNamespace, crName := resolveDataPlaneInfo(dpResult)

	// 3. Build resource nodes from Release.Status.Resources
	allNodes := make([]models.ResourceNode, 0, len(release.Status.Resources))

	for i := range release.Status.Resources {
		rs := &release.Status.Resources[i]

		// Resolve the plural resource name via the Kubernetes discovery API
		plural, err := s.resolveResourcePlural(rs.Group, rs.Version, rs.Kind)
		if err != nil {
			s.logger.Warn("Failed to resolve resource plural, skipping", "kind", rs.Kind, "name", rs.Name, "error", err)
			continue
		}

		// Fetch the live resource from the data plane
		k8sPath := buildK8sGetPath(rs.Group, rs.Version, plural, rs.Namespace, rs.Name)
		obj, err := s.fetchLiveResource(ctx, planeID, crNamespace, crName, k8sPath)
		if err != nil {
			s.logger.Warn("Failed to fetch live resource, skipping", "kind", rs.Kind, "name", rs.Name, "error", err)
			continue
		}

		node, ok := buildResourceNode(obj, nil, rs.HealthStatus)
		if !ok {
			s.logger.Warn("Skipping resource node with missing required fields", "kind", rs.Kind, "name", rs.Name)
			continue
		}
		allNodes = append(allNodes, node)

		// 4. Fetch child resources for specific kinds
		childNodes := s.fetchChildResources(ctx, planeID, crNamespace, crName, obj, rs)
		allNodes = append(allNodes, childNodes...)
	}

	return &models.ResourceTreeResponse{Nodes: allNodes}, nil
}

// resolveDataPlaneInfo extracts planeID, crNamespace, crName from a DataPlaneResult.
func resolveDataPlaneInfo(dpResult *controller.DataPlaneResult) (planeID, crNamespace, crName string) {
	if dpResult.DataPlane != nil {
		dp := dpResult.DataPlane
		planeID = dp.Spec.PlaneID
		if planeID == "" {
			planeID = dp.Name
		}
		return planeID, dp.Namespace, dp.Name
	}
	if dpResult.ClusterDataPlane != nil {
		cdp := dpResult.ClusterDataPlane
		planeID = cdp.Spec.PlaneID
		if planeID == "" {
			planeID = cdp.Name
		}
		return planeID, "_cluster", cdp.Name
	}
	return "", "", ""
}

// fetchLiveResource fetches a single K8s resource from the data plane via the gateway proxy.
func (s *ComponentService) fetchLiveResource(ctx context.Context, planeID, crNamespace, crName,
	k8sPath string) (map[string]any, error) {
	resp, err := s.gatewayClient.ProxyK8sRequest(ctx, planeTypeDataPlane, planeID, crNamespace, crName, k8sPath, "")
	if err != nil {
		return nil, fmt.Errorf("proxy request failed: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, k8sPath)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return obj, nil
}

// fetchK8sList fetches a list of K8s resources from the data plane via the gateway proxy.
func (s *ComponentService) fetchK8sList(ctx context.Context, planeID, crNamespace, crName, k8sPath,
	rawQuery string) ([]map[string]any, error) {
	resp, err := s.gatewayClient.ProxyK8sRequest(ctx, planeTypeDataPlane, planeID, crNamespace, crName, k8sPath, rawQuery)
	if err != nil {
		return nil, fmt.Errorf("proxy request failed: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, k8sPath)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var listObj map[string]any
	if err := json.Unmarshal(body, &listObj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal list response: %w", err)
	}

	items, ok := listObj["items"].([]any)
	if !ok {
		return nil, nil
	}

	var result []map[string]any
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			result = append(result, m)
		}
	}

	return result, nil
}

// fetchChildResources fetches child resources for known parent kinds.
func (s *ComponentService) fetchChildResources(ctx context.Context, planeID, crNamespace, crName string,
	parentObj map[string]any, rs *openchoreov1alpha1.ResourceStatus) []models.ResourceNode {
	var nodes []models.ResourceNode

	parentUID := getNestedString(parentObj, "metadata", "uid")
	parentRef := models.ResourceRef{
		Group:     rs.Group,
		Version:   rs.Version,
		Kind:      rs.Kind,
		Namespace: rs.Namespace,
		Name:      rs.Name,
		UID:       parentUID,
	}

	switch rs.Kind {
	case "Deployment":
		// Skip ReplicaSets, fetch Pods directly via ReplicaSets (as intermediate lookup only)
		replicaSets := s.fetchOwnedResources(ctx, planeID, crNamespace, crName, "apps", "ReplicaSet", rs.Namespace, parentUID)
		for _, rsObj := range replicaSets {
			rsUID := getNestedString(rsObj, "metadata", "uid")
			pods := s.fetchOwnedResources(ctx, planeID, crNamespace, crName, "", "Pod", rs.Namespace, rsUID)
			for _, podObj := range pods {
				if podNode, ok := buildResourceNode(podObj, &parentRef, ""); ok {
					nodes = append(nodes, podNode)
				}
			}
		}

	case "CronJob":
		// Fetch Jobs owned by this CronJob
		jobs := s.fetchOwnedResources(ctx, planeID, crNamespace, crName, "batch", "Job", rs.Namespace, parentUID)
		for _, jobObj := range jobs {
			jobNode, ok := buildResourceNode(jobObj, &parentRef, "")
			if !ok {
				continue
			}
			nodes = append(nodes, jobNode)

			// Fetch Pods owned by each Job
			jobUID := getNestedString(jobObj, "metadata", "uid")
			jobRef := models.ResourceRef{
				Group:     "batch",
				Version:   "v1",
				Kind:      "Job",
				Namespace: getNestedString(jobObj, "metadata", "namespace"),
				Name:      getNestedString(jobObj, "metadata", "name"),
				UID:       jobUID,
			}
			pods := s.fetchOwnedResources(ctx, planeID, crNamespace, crName, "", "Pod", rs.Namespace, jobUID)
			for _, podObj := range pods {
				if podNode, ok := buildResourceNode(podObj, &jobRef, ""); ok {
					nodes = append(nodes, podNode)
				}
			}
		}

	case "Job":
		// Fetch Pods owned by this Job (standalone Jobs, not owned by CronJob)
		pods := s.fetchOwnedResources(ctx, planeID, crNamespace, crName, "", "Pod", rs.Namespace, parentUID)
		for _, podObj := range pods {
			if podNode, ok := buildResourceNode(podObj, &parentRef, ""); ok {
				nodes = append(nodes, podNode)
			}
		}
	}

	return nodes
}

// fetchOwnedResources lists resources of a given kind in a namespace and filters by ownerReferences UID.
// It always uses API version "v1" for the resource lookup.
// It also injects kind and apiVersion into each item, since K8s list responses omit them from individual items.
func (s *ComponentService) fetchOwnedResources(ctx context.Context, planeID, crNamespace, crName, group, kind,
	namespace, ownerUID string) []map[string]any {
	plural, err := s.resolveResourcePlural(group, "v1", kind)
	if err != nil {
		s.logger.Warn("Failed to resolve resource plural", "kind", kind, "group", group, "error", err)
		return nil
	}
	k8sPath := buildK8sListPath(group, "v1", plural, namespace)
	items, err := s.fetchK8sList(ctx, planeID, crNamespace, crName, k8sPath, "")
	if err != nil {
		s.logger.Warn("Failed to fetch child resources", "kind", kind, "namespace", namespace, "error", err)
		return nil
	}

	// Build the apiVersion string (e.g., "v1" or "apps/v1")
	apiVersion := "v1"
	if group != "" {
		apiVersion = group + "/v1"
	}

	var owned []map[string]any
	for _, item := range items {
		if hasOwnerReference(item, ownerUID) {
			// K8s list responses don't include kind/apiVersion on individual items, so inject them
			item["kind"] = kind
			item["apiVersion"] = apiVersion
			owned = append(owned, item)
		}
	}

	return owned
}

// buildResourceNode creates a ResourceNode from a raw K8s object.
// If healthStatus is provided (from Release.Status), it is used directly.
// Otherwise, health is computed from the live object using the same logic as the release controller.
// Returns the node and true if all required fields (version, kind, name, uid) are present,
// or a zero value and false if any required field is missing.
func buildResourceNode(obj map[string]any, parentRef *models.ResourceRef,
	healthStatus openchoreov1alpha1.HealthStatus) (models.ResourceNode, bool) {
	metadata, _ := obj["metadata"].(map[string]any)

	group := getAPIGroup(obj)
	version := getAPIVersion(obj)
	kind := getStringField(obj, "kind")
	name := getNestedString(obj, "metadata", "name")
	uid := getNestedString(obj, "metadata", "uid")

	// Validate required fields per the OpenAPI ResourceNode schema.
	if version == "" || kind == "" || name == "" || uid == "" {
		return models.ResourceNode{}, false
	}

	node := models.ResourceNode{
		Group:           group,
		Version:         version,
		Kind:            kind,
		Namespace:       getNestedString(obj, "metadata", "namespace"),
		Name:            name,
		UID:             uid,
		ResourceVersion: getNestedString(obj, "metadata", "resourceVersion"),
		Object:          sanitizeObject(obj, kind),
	}

	if createdStr, ok := metadata["creationTimestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, createdStr); err == nil {
			node.CreatedAt = &t
		}
	}

	if parentRef != nil {
		node.ParentRefs = []models.ResourceRef{*parentRef}
	}

	if healthStatus != "" {
		node.Health = &models.HealthInfo{Status: string(healthStatus)}
	} else {
		// Compute health from the live object
		node.Health = computeHealthFromObject(obj, group, kind)
	}

	return node, true
}

// computeHealthFromObject computes health status from a live K8s object
// using the release controller's health check logic.
func computeHealthFromObject(obj map[string]any, group, kind string) *models.HealthInfo {
	gvk := schema.GroupVersionKind{Group: group, Kind: kind}
	healthCheckFunc := releasecontroller.GetHealthCheckFunc(gvk)
	if healthCheckFunc == nil {
		return nil
	}

	unstrObj := &unstructured.Unstructured{Object: obj}
	health, err := healthCheckFunc(unstrObj)
	if err != nil {
		return &models.HealthInfo{Status: string(openchoreov1alpha1.HealthStatusUnknown), Message: err.Error()}
	}

	return &models.HealthInfo{Status: string(health)}
}

// sanitizeObject returns a copy of the Kubernetes object with sensitive fields removed.
// For Secrets, the data and stringData fields are redacted.
// For all objects, managedFields is stripped to reduce response size.
func sanitizeObject(obj map[string]any, kind string) map[string]any {
	// Shallow-copy the top level so we don't mutate the original.
	sanitized := make(map[string]any, len(obj))
	for k, v := range obj {
		sanitized[k] = v
	}

	// Strip managedFields from metadata (noisy and not useful for display).
	if metadata, ok := sanitized["metadata"].(map[string]any); ok {
		metaCopy := make(map[string]any, len(metadata))
		for k, v := range metadata {
			metaCopy[k] = v
		}
		delete(metaCopy, "managedFields")
		sanitized["metadata"] = metaCopy
	}

	// Redact sensitive fields from Secrets.
	if kind == "Secret" {
		delete(sanitized, "data")
		delete(sanitized, "stringData")
	}

	return sanitized
}

// resolveResourcePlural resolves the plural resource name for a given group, version, and kind
// using the Kubernetes discovery API (via RESTMapper) instead of naive string pluralization.
func (s *ComponentService) resolveResourcePlural(group, version, kind string) (string, error) {
	gk := schema.GroupKind{Group: group, Kind: kind}
	mapping, err := s.k8sClient.RESTMapper().RESTMapping(gk, version)
	if err != nil {
		return "", fmt.Errorf("failed to resolve plural for %s.%s/%s: %w", kind, group, version, err)
	}
	return mapping.Resource.Resource, nil
}

// buildK8sGetPath builds the K8s API path for fetching a single resource.
func buildK8sGetPath(group, version, plural, namespace, name string) string {
	var basePath string
	if group == "" {
		basePath = fmt.Sprintf("api/%s", version)
	} else {
		basePath = fmt.Sprintf("apis/%s/%s", group, version)
	}
	if namespace != "" {
		return fmt.Sprintf("%s/namespaces/%s/%s/%s", basePath, namespace, plural, name)
	}
	return fmt.Sprintf("%s/%s/%s", basePath, plural, name)
}

// buildK8sListPath builds the K8s API path for listing resources.
func buildK8sListPath(group, version, plural, namespace string) string {
	var basePath string
	if group == "" {
		basePath = fmt.Sprintf("api/%s", version)
	} else {
		basePath = fmt.Sprintf("apis/%s/%s", group, version)
	}
	if namespace != "" {
		return fmt.Sprintf("%s/namespaces/%s/%s", basePath, namespace, plural)
	}
	return fmt.Sprintf("%s/%s", basePath, plural)
}

// hasOwnerReference checks if the object has an ownerReference with the given UID.
func hasOwnerReference(obj map[string]any, ownerUID string) bool {
	metadata, ok := obj["metadata"].(map[string]any)
	if !ok {
		return false
	}
	refs, ok := metadata["ownerReferences"].([]any)
	if !ok {
		return false
	}
	for _, ref := range refs {
		refMap, ok := ref.(map[string]any)
		if !ok {
			continue
		}
		if uid, ok := refMap["uid"].(string); ok && uid == ownerUID {
			return true
		}
	}
	return false
}

// getNestedString retrieves a nested string value from a map.
func getNestedString(obj map[string]any, keys ...string) string {
	current := obj
	for i, key := range keys {
		if i == len(keys)-1 {
			if v, ok := current[key].(string); ok {
				return v
			}
			return ""
		}
		next, ok := current[key].(map[string]any)
		if !ok {
			return ""
		}
		current = next
	}
	return ""
}

// getStringField retrieves a top-level string field from a map.
func getStringField(obj map[string]any, key string) string {
	if v, ok := obj[key].(string); ok {
		return v
	}
	return ""
}

// getAPIGroup extracts the API group from apiVersion field (e.g., "apps/v1" -> "apps", "v1" -> "").
func getAPIGroup(obj map[string]any) string {
	apiVersion := getStringField(obj, "apiVersion")
	if idx := strings.Index(apiVersion, "/"); idx >= 0 {
		return apiVersion[:idx]
	}
	return ""
}

// getAPIVersion extracts the version from apiVersion field (e.g., "apps/v1" -> "v1", "v1" -> "v1").
func getAPIVersion(obj map[string]any) string {
	apiVersion := getStringField(obj, "apiVersion")
	if idx := strings.Index(apiVersion, "/"); idx >= 0 {
		return apiVersion[idx+1:]
	}
	return apiVersion
}

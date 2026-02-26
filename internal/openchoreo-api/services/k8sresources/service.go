// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/clients/gateway"
	"github.com/openchoreo/openchoreo/internal/controller"
	releasecontroller "github.com/openchoreo/openchoreo/internal/controller/release"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

const (
	planeTypeDataPlane          = "dataplane"
	planeTypeObservabilityPlane = "observabilityplane"
	maxResponseBytes            = 10 * 1024 * 1024 // 10MB
)

// planeInfo holds the resolved plane coordinates for gateway proxy calls.
type planeInfo struct {
	planeType   string
	planeID     string
	crNamespace string
	crName      string
}

// releaseContext holds a release and its resolved plane info.
type releaseContext struct {
	release   *openchoreov1alpha1.Release
	plane     planeInfo
	namespace string // data plane namespace derived from Release.Status.Resources
}

type k8sResourcesService struct {
	k8sClient     client.Client
	gatewayClient *gateway.Client
	logger        *slog.Logger
}

// NewService creates a new k8s resources service.
func NewService(k8sClient client.Client, gatewayClient *gateway.Client, logger *slog.Logger) Service {
	return &k8sResourcesService{
		k8sClient:     k8sClient,
		gatewayClient: gatewayClient,
		logger:        logger,
	}
}

// GetResourceTree returns hierarchical views of all live Kubernetes resources
// deployed by the releases owned by a release binding.
func (s *k8sResourcesService) GetResourceTree(ctx context.Context, namespaceName, releaseBindingName string) (*K8sResourceTreeResult, error) {
	s.logger.Debug("Getting k8s resource tree", "namespace", namespaceName, "releaseBinding", releaseBindingName)

	if s.gatewayClient == nil {
		return nil, fmt.Errorf("gateway client is not configured")
	}

	releaseContexts, err := s.resolveReleaseContexts(ctx, namespaceName, releaseBindingName)
	if err != nil {
		return nil, err
	}

	releases := make([]ReleaseResourceTree, 0, len(releaseContexts))
	for _, rc := range releaseContexts {
		nodes := s.buildResourceTreeNodes(ctx, &rc)
		releases = append(releases, ReleaseResourceTree{
			Name:        rc.release.Name,
			TargetPlane: rc.release.Spec.TargetPlane,
			Nodes:       nodes,
		})
	}

	return &K8sResourceTreeResult{Releases: releases}, nil
}

// GetResourceEvents returns Kubernetes events for a specific resource in the release binding's resource tree.
func (s *k8sResourcesService) GetResourceEvents(ctx context.Context, namespaceName, releaseBindingName, group, version, kind, name string) (*models.ResourceEventsResponse, error) {
	s.logger.Debug("Getting k8s resource events", "namespace", namespaceName, "releaseBinding", releaseBindingName,
		"group", group, "version", version, "kind", kind, "name", name)

	if s.gatewayClient == nil {
		return nil, fmt.Errorf("gateway client is not configured")
	}

	releaseContexts, err := s.resolveReleaseContexts(ctx, namespaceName, releaseBindingName)
	if err != nil {
		return nil, err
	}

	// Find which release context contains the requested resource
	rc, resourceNS := s.findResourceRelease(releaseContexts, group, version, kind, name)
	if rc == nil {
		return nil, ErrResourceNotFound
	}

	// Build field selector to filter events
	fieldSelector := fmt.Sprintf("involvedObject.kind=%s,involvedObject.name=%s", kind, name)
	if resourceNS != "" {
		fieldSelector += ",involvedObject.namespace=" + resourceNS
	}

	eventsPath := "api/v1/events"
	if resourceNS != "" {
		eventsPath = fmt.Sprintf("api/v1/namespaces/%s/events", resourceNS)
	}

	rawQuery := "fieldSelector=" + fieldSelector

	items, err := s.fetchK8sList(ctx, rc.plane, eventsPath, rawQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch events: %w", err)
	}

	events := make([]models.ResourceEvent, 0, len(items))
	for _, item := range items {
		events = append(events, mapEventItem(item))
	}

	return &models.ResourceEventsResponse{Events: events}, nil
}

// GetResourceLogs returns logs for a specific pod in the release binding's resource tree.
func (s *k8sResourcesService) GetResourceLogs(ctx context.Context, namespaceName, releaseBindingName, podName string, sinceSeconds *int64) (*models.ResourcePodLogsResponse, error) {
	s.logger.Debug("Getting k8s resource logs", "namespace", namespaceName, "releaseBinding", releaseBindingName, "pod", podName)

	if s.gatewayClient == nil {
		return nil, fmt.Errorf("gateway client is not configured")
	}

	releaseContexts, err := s.resolveReleaseContexts(ctx, namespaceName, releaseBindingName)
	if err != nil {
		return nil, err
	}

	// Find a release context with a dataplane target that has parent resources for pods
	var targetRC *releaseContext
	for i := range releaseContexts {
		rc := &releaseContexts[i]
		if rc.release.Spec.TargetPlane == planeTypeDataPlane && hasParentResourceInRelease("Pod", rc.release.Status.Resources) {
			targetRC = rc
			break
		}
	}
	if targetRC == nil {
		return nil, ErrResourceNotFound
	}

	rawLogs, err := s.gatewayClient.GetPodLogsFromPlane(ctx, targetRC.plane.planeType, targetRC.plane.planeID,
		targetRC.plane.crNamespace, targetRC.plane.crName,
		&gateway.PodReference{
			Namespace: targetRC.namespace,
			Name:      podName,
		},
		&gateway.PodLogsOptions{
			IncludeTimestamps: true,
			SinceSeconds:      sinceSeconds,
		})
	if err != nil {
		if gateway.IsPermanentError(err) {
			return nil, ErrResourceNotFound
		}
		return nil, fmt.Errorf("failed to fetch pod logs: %w", err)
	}

	logEntries := parseLogLines(rawLogs)
	return &models.ResourcePodLogsResponse{LogEntries: logEntries}, nil
}

// resolveReleaseContexts fetches the ReleaseBinding, finds its owned Releases,
// and resolves plane info for each.
func (s *k8sResourcesService) resolveReleaseContexts(ctx context.Context, namespaceName, releaseBindingName string) ([]releaseContext, error) {
	// 1. Fetch the ReleaseBinding
	var rb openchoreov1alpha1.ReleaseBinding
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: releaseBindingName}, &rb); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrReleaseBindingNotFound
		}
		return nil, fmt.Errorf("failed to get release binding: %w", err)
	}

	// 2. List Releases in the same namespace, filter by owner
	var releaseList openchoreov1alpha1.ReleaseList
	if err := s.k8sClient.List(ctx, &releaseList, client.InNamespace(namespaceName)); err != nil {
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}

	var ownedReleases []*openchoreov1alpha1.Release
	for i := range releaseList.Items {
		release := &releaseList.Items[i]
		if metav1.IsControlledBy(release, &rb) {
			ownedReleases = append(ownedReleases, release)
		}
	}

	if len(ownedReleases) == 0 {
		return nil, ErrReleaseNotFound
	}

	// 3. Resolve environment and plane info
	env := &openchoreov1alpha1.Environment{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: rb.Spec.Environment}, env); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	dpResult, err := controller.GetDataPlaneOrClusterDataPlaneOfEnv(ctx, s.k8sClient, env)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve data plane: %w", err)
	}

	contexts := make([]releaseContext, 0, len(ownedReleases))
	for _, release := range ownedReleases {
		pi, err := s.resolvePlaneInfo(ctx, release, dpResult)
		if err != nil {
			s.logger.Warn("Failed to resolve plane info for release, skipping",
				"release", release.Name, "targetPlane", release.Spec.TargetPlane, "error", err)
			continue
		}

		ns := deriveNamespace(release)

		contexts = append(contexts, releaseContext{
			release:   release,
			plane:     pi,
			namespace: ns,
		})
	}

	if len(contexts) == 0 {
		return nil, ErrReleaseNotFound
	}

	return contexts, nil
}

// resolvePlaneInfo resolves gateway proxy coordinates for a release based on its target plane.
func (s *k8sResourcesService) resolvePlaneInfo(ctx context.Context, release *openchoreov1alpha1.Release, dpResult *controller.DataPlaneResult) (planeInfo, error) {
	switch release.Spec.TargetPlane {
	case planeTypeObservabilityPlane:
		obsResult, err := dpResult.GetObservabilityPlane(ctx, s.k8sClient)
		if err != nil {
			return planeInfo{}, fmt.Errorf("failed to resolve observability plane: %w", err)
		}
		return resolveObservabilityPlaneInfo(obsResult), nil
	default: // dataplane
		pi := resolveDataPlaneInfo(dpResult)
		return pi, nil
	}
}

// resolveDataPlaneInfo extracts planeInfo from a DataPlaneResult.
func resolveDataPlaneInfo(dpResult *controller.DataPlaneResult) planeInfo {
	if dpResult.DataPlane != nil {
		dp := dpResult.DataPlane
		id := dp.Spec.PlaneID
		if id == "" {
			id = dp.Name
		}
		return planeInfo{planeType: planeTypeDataPlane, planeID: id, crNamespace: dp.Namespace, crName: dp.Name}
	}
	if dpResult.ClusterDataPlane != nil {
		cdp := dpResult.ClusterDataPlane
		id := cdp.Spec.PlaneID
		if id == "" {
			id = cdp.Name
		}
		return planeInfo{planeType: planeTypeDataPlane, planeID: id, crNamespace: "_cluster", crName: cdp.Name}
	}
	return planeInfo{}
}

// resolveObservabilityPlaneInfo extracts planeInfo from an ObservabilityPlaneResult.
func resolveObservabilityPlaneInfo(obsResult *controller.ObservabilityPlaneResult) planeInfo {
	if obsResult.ObservabilityPlane != nil {
		op := obsResult.ObservabilityPlane
		id := op.Spec.PlaneID
		if id == "" {
			id = op.Name
		}
		return planeInfo{planeType: planeTypeObservabilityPlane, planeID: id, crNamespace: op.Namespace, crName: op.Name}
	}
	if obsResult.ClusterObservabilityPlane != nil {
		cop := obsResult.ClusterObservabilityPlane
		id := cop.Spec.PlaneID
		if id == "" {
			id = cop.Name
		}
		return planeInfo{planeType: planeTypeObservabilityPlane, planeID: id, crNamespace: "_cluster", crName: cop.Name}
	}
	return planeInfo{}
}

// deriveNamespace extracts the data plane namespace from the first resource in the release status.
func deriveNamespace(release *openchoreov1alpha1.Release) string {
	if len(release.Status.Resources) > 0 {
		return release.Status.Resources[0].Namespace
	}
	return ""
}

// buildResourceTreeNodes builds resource nodes for a single release.
func (s *k8sResourcesService) buildResourceTreeNodes(ctx context.Context, rc *releaseContext) []models.ResourceNode {
	allNodes := make([]models.ResourceNode, 0, len(rc.release.Status.Resources))

	for i := range rc.release.Status.Resources {
		rs := &rc.release.Status.Resources[i]

		plural, err := s.resolveResourcePlural(rs.Group, rs.Version, rs.Kind)
		if err != nil {
			s.logger.Warn("Failed to resolve resource plural, skipping", "kind", rs.Kind, "name", rs.Name, "error", err)
			continue
		}

		k8sPath := buildK8sGetPath(rs.Group, rs.Version, plural, rs.Namespace, rs.Name)
		obj, err := s.fetchLiveResource(ctx, rc.plane, k8sPath)
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

		childNodes := s.fetchChildResources(ctx, rc.plane, obj, rs)
		allNodes = append(allNodes, childNodes...)
	}

	return allNodes
}

// findResourceRelease finds which release context contains the requested resource
// and returns its namespace.
func (s *k8sResourcesService) findResourceRelease(contexts []releaseContext, group, version, kind, name string) (*releaseContext, string) {
	// First try exact match in release status resources
	for i := range contexts {
		rc := &contexts[i]
		for _, rs := range rc.release.Status.Resources {
			if rs.Group == group && rs.Version == version && rs.Kind == kind && rs.Name == name {
				return rc, rs.Namespace
			}
		}
	}

	// For child resource kinds (Pod, ReplicaSet, Job), check if a parent exists
	if isChildResourceKind(kind) {
		for i := range contexts {
			rc := &contexts[i]
			if hasParentResourceInRelease(kind, rc.release.Status.Resources) {
				return rc, rc.namespace
			}
		}
	}

	return nil, ""
}

// --- Fetching helpers ---

func (s *k8sResourcesService) fetchLiveResource(ctx context.Context, pi planeInfo, k8sPath string) (map[string]any, error) {
	resp, err := s.gatewayClient.ProxyK8sRequest(ctx, pi.planeType, pi.planeID, pi.crNamespace, pi.crName, k8sPath, "")
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

func (s *k8sResourcesService) fetchK8sList(ctx context.Context, pi planeInfo, k8sPath, rawQuery string) ([]map[string]any, error) {
	resp, err := s.gatewayClient.ProxyK8sRequest(ctx, pi.planeType, pi.planeID, pi.crNamespace, pi.crName, k8sPath, rawQuery)
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

func (s *k8sResourcesService) fetchChildResources(ctx context.Context, pi planeInfo, parentObj map[string]any, rs *openchoreov1alpha1.ResourceStatus) []models.ResourceNode {
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
		replicaSets := s.fetchOwnedResources(ctx, pi, "apps", "ReplicaSet", rs.Namespace, parentUID)
		for _, rsObj := range replicaSets {
			rsUID := getNestedString(rsObj, "metadata", "uid")
			pods := s.fetchOwnedResources(ctx, pi, "", "Pod", rs.Namespace, rsUID)
			for _, podObj := range pods {
				if podNode, ok := buildResourceNode(podObj, &parentRef, ""); ok {
					nodes = append(nodes, podNode)
				}
			}
		}

	case "CronJob":
		jobs := s.fetchOwnedResources(ctx, pi, "batch", "Job", rs.Namespace, parentUID)
		for _, jobObj := range jobs {
			jobNode, ok := buildResourceNode(jobObj, &parentRef, "")
			if !ok {
				continue
			}
			nodes = append(nodes, jobNode)

			jobUID := getNestedString(jobObj, "metadata", "uid")
			jobRef := models.ResourceRef{
				Group:     "batch",
				Version:   "v1",
				Kind:      "Job",
				Namespace: getNestedString(jobObj, "metadata", "namespace"),
				Name:      getNestedString(jobObj, "metadata", "name"),
				UID:       jobUID,
			}
			pods := s.fetchOwnedResources(ctx, pi, "", "Pod", rs.Namespace, jobUID)
			for _, podObj := range pods {
				if podNode, ok := buildResourceNode(podObj, &jobRef, ""); ok {
					nodes = append(nodes, podNode)
				}
			}
		}

	case "Job":
		pods := s.fetchOwnedResources(ctx, pi, "", "Pod", rs.Namespace, parentUID)
		for _, podObj := range pods {
			if podNode, ok := buildResourceNode(podObj, &parentRef, ""); ok {
				nodes = append(nodes, podNode)
			}
		}
	}

	return nodes
}

func (s *k8sResourcesService) fetchOwnedResources(ctx context.Context, pi planeInfo, group, kind, namespace, ownerUID string) []map[string]any {
	plural, err := s.resolveResourcePlural(group, "v1", kind)
	if err != nil {
		s.logger.Warn("Failed to resolve resource plural", "kind", kind, "group", group, "error", err)
		return nil
	}
	k8sPath := buildK8sListPath(group, "v1", plural, namespace)
	items, err := s.fetchK8sList(ctx, pi, k8sPath, "")
	if err != nil {
		s.logger.Warn("Failed to fetch child resources", "kind", kind, "namespace", namespace, "error", err)
		return nil
	}

	apiVersion := "v1"
	if group != "" {
		apiVersion = group + "/v1"
	}

	var owned []map[string]any
	for _, item := range items {
		if hasOwnerReference(item, ownerUID) {
			item["kind"] = kind
			item["apiVersion"] = apiVersion
			owned = append(owned, item)
		}
	}

	return owned
}

func (s *k8sResourcesService) resolveResourcePlural(group, version, kind string) (string, error) {
	gk := schema.GroupKind{Group: group, Kind: kind}
	mapping, err := s.k8sClient.RESTMapper().RESTMapping(gk, version)
	if err != nil {
		return "", fmt.Errorf("failed to resolve plural for %s.%s/%s: %w", kind, group, version, err)
	}
	return mapping.Resource.Resource, nil
}

// --- Pure utility functions ---

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

func buildResourceNode(obj map[string]any, parentRef *models.ResourceRef, healthStatus openchoreov1alpha1.HealthStatus) (models.ResourceNode, bool) {
	metadata, _ := obj["metadata"].(map[string]any)

	group := getAPIGroup(obj)
	version := getAPIVersion(obj)
	kind := getStringField(obj, "kind")
	name := getNestedString(obj, "metadata", "name")
	uid := getNestedString(obj, "metadata", "uid")

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
		node.Health = computeHealthFromObject(obj, group, kind)
	}

	return node, true
}

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

func sanitizeObject(obj map[string]any, kind string) map[string]any {
	sanitized := make(map[string]any, len(obj))
	maps.Copy(sanitized, obj)

	if metadata, ok := sanitized["metadata"].(map[string]any); ok {
		metaCopy := make(map[string]any, len(metadata))
		maps.Copy(metaCopy, metadata)
		delete(metaCopy, "managedFields")
		sanitized["metadata"] = metaCopy
	}

	if kind == "Secret" {
		delete(sanitized, "data")
		delete(sanitized, "stringData")
	}

	return sanitized
}

func mapEventItem(item map[string]any) models.ResourceEvent {
	event := models.ResourceEvent{
		Type:    getNestedString(item, "type"),
		Reason:  getNestedString(item, "reason"),
		Message: getNestedString(item, "message"),
	}

	if countVal, ok := item["count"]; ok {
		if v, ok := countVal.(float64); ok {
			c := int32(v) //nolint:gosec // event count will not overflow int32
			event.Count = &c
		}
	}

	if ts := getNestedString(item, "firstTimestamp"); ts != "" {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			event.FirstTimestamp = &t
		}
	} else if ts := getNestedString(item, "eventTime"); ts != "" {
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			event.FirstTimestamp = &t
		}
	}

	if ts := getNestedString(item, "lastTimestamp"); ts != "" {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			event.LastTimestamp = &t
		}
	} else if event.FirstTimestamp != nil {
		event.LastTimestamp = event.FirstTimestamp
	}

	if src := getNestedString(item, "source", "component"); src != "" {
		event.Source = src
	} else {
		event.Source = getNestedString(item, "reportingComponent")
	}

	return event
}

func parseLogLines(rawLogs string) []models.PodLogEntry {
	lines := strings.Split(rawLogs, "\n")
	entries := make([]models.PodLogEntry, 0, len(lines))
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		spaceIndex := strings.Index(trimmedLine, " ")
		if spaceIndex > 0 {
			timestampCandidate := trimmedLine[:spaceIndex]
			if _, err := time.Parse(time.RFC3339, timestampCandidate); err == nil {
				entries = append(entries, models.PodLogEntry{
					Timestamp: timestampCandidate,
					Log:       trimmedLine[spaceIndex+1:],
				})
				continue
			}
			if _, err := time.Parse(time.RFC3339Nano, timestampCandidate); err == nil {
				entries = append(entries, models.PodLogEntry{
					Timestamp: timestampCandidate,
					Log:       trimmedLine[spaceIndex+1:],
				})
				continue
			}
		}
	}
	return entries
}

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

func getStringField(obj map[string]any, key string) string {
	if v, ok := obj[key].(string); ok {
		return v
	}
	return ""
}

func getAPIGroup(obj map[string]any) string {
	apiVersion := getStringField(obj, "apiVersion")
	if idx := strings.Index(apiVersion, "/"); idx >= 0 {
		return apiVersion[:idx]
	}
	return ""
}

func getAPIVersion(obj map[string]any) string {
	apiVersion := getStringField(obj, "apiVersion")
	if idx := strings.Index(apiVersion, "/"); idx >= 0 {
		return apiVersion[idx+1:]
	}
	return apiVersion
}

var childResourceParentKinds = map[string][]string{
	"Pod":        {"Deployment", "Job", "CronJob"},
	"ReplicaSet": {"Deployment"},
	"Job":        {"CronJob"},
}

func isChildResourceKind(kind string) bool {
	_, ok := childResourceParentKinds[kind]
	return ok
}

func hasParentResourceInRelease(childKind string, resources []openchoreov1alpha1.ResourceStatus) bool {
	parentKinds := childResourceParentKinds[childKind]
	for i := range resources {
		if slices.Contains(parentKinds, resources[i].Kind) {
			return true
		}
	}
	return false
}

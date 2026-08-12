// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/clients/gateway"
	"github.com/openchoreo/openchoreo/internal/controller"
	renderedreleasecontroller "github.com/openchoreo/openchoreo/internal/controller/renderedrelease"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
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
	release   *openchoreov1alpha1.RenderedRelease
	plane     planeInfo
	namespace string // data plane namespace derived from Release.Status.Resources
}

type k8sResourcesService struct {
	k8sClient     client.Client
	gatewayClient *gateway.Client
	// rules is never nil: NewService is the only way to build this type outside
	// the package and always compiles it, and compileRules always returns an
	// allocated value. Methods therefore dereference it without guarding — a nil
	// here is an in-package literal that forgot to set it, and panicking on that
	// is better than silently walking a tree with no rules and reporting a
	// release whose children merely appear to be gone.
	rules  *compiledRules
	logger *slog.Logger
}

// NewService creates a new k8s resources service. The traversal rules are
// compiled once here: they were validated at startup, so compilation cannot
// fail.
func NewService(k8sClient client.Client, gatewayClient *gateway.Client, treeCfg config.ResourceTreeConfig, logger *slog.Logger) Service {
	return &k8sResourcesService{
		k8sClient:     k8sClient,
		gatewayClient: gatewayClient,
		rules:         compileRules(treeCfg),
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

	releaseContexts, _, err := s.resolveReleaseContexts(ctx, namespaceName, releaseBindingName)
	if err != nil {
		return nil, err
	}

	renderedReleases := make([]ReleaseResourceTree, 0, len(releaseContexts))
	for _, rc := range releaseContexts {
		nodes, _ := s.buildResourceTreeNodes(ctx, &rc)
		renderedReleases = append(renderedReleases, ReleaseResourceTree{
			Name:        rc.release.Name,
			TargetPlane: rc.release.Spec.TargetPlane,
			Nodes:       nodes,
			Release:     rc.release,
		})
	}

	return &K8sResourceTreeResult{RenderedReleases: renderedReleases}, nil
}

// GetResourceEvents returns Kubernetes events for a specific resource in the release binding's resource tree.
func (s *k8sResourcesService) GetResourceEvents(ctx context.Context, namespaceName, releaseBindingName, group, version, kind, name string) (*models.ResourceEventsResponse, error) {
	s.logger.Debug("Getting k8s resource events", "namespace", namespaceName, "releaseBinding", releaseBindingName,
		"group", group, "version", version, "kind", kind, "name", name)

	if s.gatewayClient == nil {
		return nil, fmt.Errorf("gateway client is not configured")
	}

	releaseContexts, dropped, err := s.resolveReleaseContexts(ctx, namespaceName, releaseBindingName)
	if err != nil {
		return nil, err
	}

	// Events can name a resource in any plane, so any dropped release leaves the
	// search incomplete.
	rc, resourceNS, err := s.resolveTreeResource(ctx, releaseContexts, group, version, kind, name, len(dropped) > 0)
	if err != nil {
		return nil, err
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
// When container is empty, logs from every container in the pod are returned, each entry
// tagged with its container name and merged into a single timeline ordered by timestamp.
// When container is set, only that container's logs are returned.
func (s *k8sResourcesService) GetResourceLogs(ctx context.Context, namespaceName, releaseBindingName, podName, container string, sinceSeconds *int64) (*models.ResourcePodLogsResponse, error) {
	s.logger.Debug("Getting k8s resource logs", "namespace", namespaceName, "releaseBinding", releaseBindingName,
		"pod", podName, "container", container)

	if s.gatewayClient == nil {
		return nil, fmt.Errorf("gateway client is not configured")
	}

	releaseContexts, dropped, err := s.resolveReleaseContexts(ctx, namespaceName, releaseBindingName)
	if err != nil {
		return nil, err
	}

	// Pods live in the data plane, so filter to data-plane releases before
	// resolving. Otherwise a same-named member in an observability-plane release
	// ordered first would shadow the real pod and produce a false 404, and every
	// non-data-plane tree would be walked needlessly.
	dataPlaneContexts := contextsForPlane(releaseContexts, planeTypeDataPlane)
	// Only a dropped data-plane release could have held the pod, so only that
	// leaves the pod search incomplete.
	dataPlaneDropped := slices.ContainsFunc(dropped, func(r *openchoreov1alpha1.RenderedRelease) bool {
		return r.Spec.TargetPlane == planeTypeDataPlane
	})
	targetRC, podNamespace, err := s.resolveTreeResource(ctx, dataPlaneContexts, "", "v1", "Pod", podName, dataPlaneDropped)
	if err != nil {
		return nil, err
	}

	// Determine which containers to fetch logs from. A pod's Kubernetes log API requires an
	// explicit container name once the pod has more than one container, so when the caller
	// does not pick one we enumerate the pod's containers and aggregate their logs.
	var containers []string
	if container != "" {
		containers = []string{container}
	} else {
		containers, err = s.resolvePodContainers(ctx, targetRC.plane, podNamespace, podName)
		if err != nil {
			s.logger.Warn("Failed to resolve pod containers", "pod", podName, "error", err)
			var statusErr *liveResourceStatusError
			if errors.As(err, &statusErr) && statusErr.statusCode == http.StatusNotFound {
				return nil, ErrResourceNotFound
			}
			// The pod is present but unreadable (transient/agent error) or the
			// response was otherwise unexpected
			return nil, fmt.Errorf("failed to resolve pod containers: %w", err)
		}
	}

	entries := make([]models.PodLogEntry, 0)
	for _, containerName := range containers {
		rawLogs, err := s.gatewayClient.GetPodLogsFromPlane(ctx, targetRC.plane.planeType, targetRC.plane.planeID,
			targetRC.plane.crNamespace, targetRC.plane.crName,
			&gateway.PodReference{
				Namespace: podNamespace,
				Name:      podName,
			},
			&gateway.PodLogsOptions{
				ContainerName:     containerName,
				IncludeTimestamps: true,
				SinceSeconds:      sinceSeconds,
			})
		if err != nil {
			var permErr *gateway.PermanentError
			if errors.As(err, &permErr) {
				if container != "" {
					switch permErr.StatusCode {
					case http.StatusNotFound:
						return nil, ErrResourceNotFound
					case http.StatusBadRequest:
						return nil, ErrInvalidContainer
					default:
						return nil, fmt.Errorf("failed to fetch logs for container %q: %w", containerName, err)
					}
				}
				// Aggregating: a container that is still starting or already gone
				// returns 400/404 and is safely skipped, but surface anything else
				// (e.g. a 403 from a misconfigured agent) instead of returning a
				// partial result.
				if permErr.StatusCode == http.StatusBadRequest || permErr.StatusCode == http.StatusNotFound {
					s.logger.Warn("Skipping container with no readable logs",
						"pod", podName, "container", containerName, "status", permErr.StatusCode)
					continue
				}
				return nil, fmt.Errorf("failed to fetch logs for container %q: %w", containerName, err)
			}
			return nil, fmt.Errorf("failed to fetch pod logs: %w", err)
		}

		for _, entry := range parseLogLines(rawLogs) {
			entry.Container = containerName
			entries = append(entries, entry)
		}
	}

	// Merge per-container logs into a single timeline ordered by timestamp.
	sortLogEntriesByTimestamp(entries)

	return &models.ResourcePodLogsResponse{LogEntries: entries}, nil
}

// resolvePodContainers fetches the live pod and returns the names of its containers.
func (s *k8sResourcesService) resolvePodContainers(ctx context.Context, pi planeInfo, namespace, podName string) ([]string, error) {
	podPath := buildK8sGetPath("", "v1", "pods", namespace, podName)
	pod, err := s.fetchLiveResource(ctx, pi, podPath)
	if err != nil {
		return nil, err
	}

	names := extractContainerNames(pod)
	if len(names) == 0 {
		return nil, fmt.Errorf("no containers found for pod %s", podName)
	}
	return names, nil
}

// resolveReleaseContexts fetches the ReleaseBinding, finds its owned Releases,
// and resolves plane info for each. The second return lists releases whose plane
// could not be resolved and were therefore dropped: a membership check must treat
// that as an incomplete search rather than proof the resource is absent, so it is
// surfaced rather than only logged.
func (s *k8sResourcesService) resolveReleaseContexts(ctx context.Context, namespaceName, releaseBindingName string) ([]releaseContext, []*openchoreov1alpha1.RenderedRelease, error) {
	// 1. Fetch the ReleaseBinding
	var rb openchoreov1alpha1.ReleaseBinding
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: releaseBindingName}, &rb); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, nil, ErrReleaseBindingNotFound
		}
		return nil, nil, fmt.Errorf("failed to get release binding: %w", err)
	}

	// 2. List Releases in the same namespace, filter by owner
	var releaseList openchoreov1alpha1.RenderedReleaseList
	if err := s.k8sClient.List(ctx, &releaseList, client.InNamespace(namespaceName)); err != nil {
		return nil, nil, fmt.Errorf("failed to list releases: %w", err)
	}

	var ownedReleases []*openchoreov1alpha1.RenderedRelease
	for i := range releaseList.Items {
		release := &releaseList.Items[i]
		if metav1.IsControlledBy(release, &rb) {
			ownedReleases = append(ownedReleases, release)
		}
	}

	if len(ownedReleases) == 0 {
		// ReleaseBinding exists but has no owned releases yet (e.g. not reconciled).
		// Return an empty list so the caller can return an empty tree.
		return nil, nil, nil
	}

	// 3. Resolve environment and plane info
	env := &openchoreov1alpha1.Environment{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: rb.Spec.Environment}, env); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, nil, ErrEnvironmentNotFound
		}
		return nil, nil, fmt.Errorf("failed to get environment: %w", err)
	}

	dpResult, err := controller.GetDataPlaneFromRef(ctx, s.k8sClient, env.Namespace, env.Spec.DataPlaneRef)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve data plane: %w", err)
	}

	contexts := make([]releaseContext, 0, len(ownedReleases))
	var dropped []*openchoreov1alpha1.RenderedRelease
	for _, release := range ownedReleases {
		pi, err := s.resolvePlaneInfo(ctx, release, dpResult)
		if err != nil {
			s.logger.Warn("Failed to resolve plane info for release, skipping",
				"release", release.Name, "targetPlane", release.Spec.TargetPlane, "error", err)
			dropped = append(dropped, release)
			continue
		}

		ns := deriveNamespace(release)

		contexts = append(contexts, releaseContext{
			release:   release,
			plane:     pi,
			namespace: ns,
		})
	}

	return contexts, dropped, nil
}

// resolvePlaneInfo resolves gateway proxy coordinates for a release based on its target plane.
func (s *k8sResourcesService) resolvePlaneInfo(ctx context.Context, release *openchoreov1alpha1.RenderedRelease, dpResult *controller.DataPlaneResult) (planeInfo, error) {
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
func deriveNamespace(release *openchoreov1alpha1.RenderedRelease) string {
	if len(release.Status.Resources) > 0 {
		return release.Status.Resources[0].Namespace
	}
	return ""
}

// buildResourceTreeNodes builds resource nodes for a single release. The
// accumulator is created once here and shared by every root, because dedupe is
// release-wide: the same Pod can be reachable from two roots and must surface as
// one node carrying both parents.
//
// The second return reports whether the walk was incomplete: a root that could
// not be resolved or fetched, or any child edge that came back forbidden, errored
// or truncated. A membership check must not read absence from an incomplete walk
// as proof the target does not exist.
func (s *k8sResourcesService) buildResourceTreeNodes(ctx context.Context, rc *releaseContext) ([]models.ResourceNode, bool) {
	acc := newNodeAccumulator(len(rc.release.Status.Resources))
	incomplete := false

	for i := range rc.release.Status.Resources {
		rs := &rc.release.Status.Resources[i]

		plural, err := s.rootResourcePlural(rs)
		if err != nil {
			s.logger.Warn("Failed to resolve resource plural, skipping", "kind", rs.Kind, "name", rs.Name, "error", err)
			incomplete = true
			continue
		}

		k8sPath := buildK8sGetPath(rs.Group, rs.Version, plural, rs.Namespace, rs.Name)
		obj, err := s.fetchLiveResource(ctx, rc.plane, k8sPath)
		if err != nil {
			s.logger.Warn("Failed to fetch live resource, skipping", "kind", rs.Kind, "name", rs.Name, "error", err)
			incomplete = true
			continue
		}

		rootGVR := schema.GroupVersionResource{Group: rs.Group, Version: rs.Version, Resource: plural}
		node, ok := buildResourceNode(obj, rootGVR, nil, rs.HealthStatus)
		if !ok {
			s.logger.Warn("Skipping resource node with missing required fields", "kind", rs.Kind, "name", rs.Name)
			incomplete = true
			continue
		}
		acc.add(node)

		s.expandChildren(ctx, rc.plane, acc, node.UID, obj, rs)
	}

	// A child edge that came back forbidden, errored or truncated is recorded as a
	// ChildDiscoveryStatus on its parent; any such status means the descendant set
	// under that node is incomplete.
	for i := range acc.nodes {
		if len(acc.nodes[i].ChildrenStatus) > 0 {
			incomplete = true
			break
		}
	}

	return acc.nodes, incomplete
}

// rootResourcePlural resolves a root's REST plural from its traversal rule when
// one is configured, and only otherwise from the RESTMapper. The RESTMapper is
// the CONTROL plane's, so a CRD that exists only in the data plane cannot be
// resolved through it — the rule carries the exact plural for that case.
func (s *k8sResourcesService) rootResourcePlural(rs *openchoreov1alpha1.RenderedManifestStatus) (string, error) {
	for _, rule := range s.rules.roots[groupKind{Group: rs.Group, Kind: rs.Kind}] {
		if rule.Kind.Resource != "" {
			return rule.Kind.Resource, nil
		}
	}
	return s.resolveResourcePlural(rs.Group, rs.Version, rs.Kind)
}

// resolveTreeResource keeps exact rendered-root lookup behavior, then resolves
// actual live descendants. Reachability of a kind alone does not authorize a
// resource name.
//
// It returns a typed error rather than a bare nil so the caller can tell three
// outcomes apart: ErrResourceNotFound (the walks completed and no live member
// matched), ErrResourceTreeIncomplete (a walk failed, so absence is not
// authoritative — fail closed but do not claim the resource is gone), and
// ErrResourceMatchAmbiguous (more than one live member matched the identity).
// searchIncomplete seeds resolveTreeResource with incompleteness that happened
// before the walk — a release dropped because its plane could not be resolved is
// part of the search that did not happen, so absence cannot be authoritative.
func (s *k8sResourcesService) resolveTreeResource(ctx context.Context, contexts []releaseContext,
	group, version, kind, name string, searchIncomplete bool) (*releaseContext, string, error) {
	// An exact rendered root is authoritative on its own and needs no walk.
	// A rendered root is declared in its release's status, so it is authoritative
	// that the resource exists and is owned there, independent of any live child
	// walk. Count across all releases rather than taking the first: two releases
	// declaring the same identity is ambiguous, and a single root cannot be proven
	// unique while a dropped release (whose status we could not resolve a plane
	// for) might declare it too.
	exactMatches := 0
	var exactRC *releaseContext
	var exactNamespace string
	for i := range contexts {
		rc := &contexts[i]
		for _, rs := range rc.release.Status.Resources {
			if rs.Group == group && rs.Version == version && rs.Kind == kind && rs.Name == name {
				exactMatches++
				exactRC = rc
				exactNamespace = rs.Namespace
			}
		}
	}
	switch {
	case exactMatches > 1:
		return nil, "", ErrResourceMatchAmbiguous
	case exactMatches == 1 && searchIncomplete:
		return nil, "", ErrResourceTreeIncomplete
	case exactMatches == 1:
		return exactRC, exactNamespace, nil
	}

	var (
		matchRC        *releaseContext
		matchNamespace string
		matches        int
		anyIncomplete  = searchIncomplete
	)
	for i := range contexts {
		rc := &contexts[i]
		nodes, incomplete := s.buildResourceTreeNodes(ctx, rc)
		if incomplete {
			anyIncomplete = true
		}
		// Count every matching node, not just the first: a single release can hold
		// two distinct live members of the same group/version/kind/name (different
		// namespaces, different UIDs), and that is an ambiguous identity just as
		// much as a collision across releases.
		for _, node := range nodes {
			if node.Group == group && node.Version == version && node.Kind == kind && node.Name == name {
				matches++
				matchRC = rc
				matchNamespace = node.Namespace
			}
		}
	}

	switch {
	case matches > 1:
		return nil, "", ErrResourceMatchAmbiguous
	case anyIncomplete:
		// A failed walk means absence is not proof of non-membership, and a lone
		// match cannot be proven unique while another candidate tree is incomplete.
		// Fail closed on the operational error rather than authorize from a partial
		// view.
		return nil, "", ErrResourceTreeIncomplete
	case matches == 1:
		return matchRC, matchNamespace, nil
	default:
		return nil, "", ErrResourceNotFound
	}
}

// contextsForPlane returns only the release contexts whose release targets the
// given plane. Filtering before resolution keeps a member from another plane
// from shadowing the intended one and avoids walking trees that cannot hold the
// requested resource.
func contextsForPlane(contexts []releaseContext, plane string) []releaseContext {
	filtered := make([]releaseContext, 0, len(contexts))
	for _, rc := range contexts {
		if rc.release.Spec.TargetPlane == plane {
			filtered = append(filtered, rc)
		}
	}
	return filtered
}

// --- Fetching helpers ---

// liveResourceStatusError is returned by fetchLiveResource when the proxied
// Kubernetes GET responds with a non-200 status. It preserves the status code so
// callers can tell a genuine 404 apart from transient or otherwise unexpected
// failures instead of collapsing them all into "not found".
type liveResourceStatusError struct {
	statusCode int
	path       string
}

func (e *liveResourceStatusError) Error() string {
	return fmt.Sprintf("unexpected status %d for %s", e.statusCode, e.path)
}

func (s *k8sResourcesService) fetchLiveResource(ctx context.Context, pi planeInfo, k8sPath string) (map[string]any, error) {
	resp, err := s.gatewayClient.ProxyK8sRequest(ctx, pi.planeType, pi.planeID, pi.crNamespace, pi.crName, k8sPath, "")
	if err != nil {
		return nil, fmt.Errorf("proxy request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &liveResourceStatusError{statusCode: resp.StatusCode, path: k8sPath}
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
		// The status code is preserved so the tree walk's fallback can tell a
		// missing RBAC grant apart from a genuine failure.
		return nil, &liveResourceStatusError{statusCode: resp.StatusCode, path: k8sPath}
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

// fetchOwnedResources lists one child kind in one namespace and keeps the items
// owned by ownerUID. It is the control-plane half of the ownerRef matcher, used
// only when the cluster agent cannot match server-side.
//
// A list failure is returned rather than logged away: "forbidden" and "no
// children" look identical to the caller otherwise, and the tree reports the
// difference.
func (s *k8sResourcesService) fetchOwnedResources(ctx context.Context, pi planeInfo,
	group, version, kind, resource, namespace, ownerUID string) ([]map[string]any, error) {
	items, err := s.fetchChildKindList(ctx, pi, group, version, kind, resource, namespace, "")
	if err != nil {
		return nil, err
	}

	owned := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if hasOwnerReference(item, ownerUID) {
			owned = append(owned, item)
		}
	}
	return owned, nil
}

// fetchLabelSelectedResources is the control-plane half of the labelSelector
// matcher: the same proxied list, scoped by an already substituted server-side
// selector and with no owner filtering, because a label match has no ownership
// to check.
func (s *k8sResourcesService) fetchLabelSelectedResources(ctx context.Context, pi planeInfo,
	group, version, kind, resource, namespace, selector string) ([]map[string]any, error) {
	rawQuery := url.Values{"labelSelector": []string{selector}}.Encode()
	return s.fetchChildKindList(ctx, pi, group, version, kind, resource, namespace, rawQuery)
}

// fetchChildKindList lists one child kind in one namespace. The plural comes
// from the rule when it carries one — a dataplane-only CRD is not in the control
// plane's RESTMapper — and apiVersion/kind are backfilled, since list items are
// commonly served without them.
func (s *k8sResourcesService) fetchChildKindList(ctx context.Context, pi planeInfo,
	group, version, kind, resource, namespace, rawQuery string) ([]map[string]any, error) {
	plural := resource
	if plural == "" {
		var err error
		plural, err = s.resolveResourcePlural(group, version, kind)
		if err != nil {
			return nil, err
		}
	}

	items, err := s.fetchK8sList(ctx, pi, buildK8sListPath(group, version, plural, namespace), rawQuery)
	if err != nil {
		return nil, err
	}

	apiVersion := version
	if group != "" {
		apiVersion = group + "/" + version
	}
	for _, item := range items {
		if getStringField(item, "kind") == "" {
			item["kind"] = kind
		}
		if getStringField(item, "apiVersion") == "" {
			item["apiVersion"] = apiVersion
		}
	}

	return items, nil
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

// buildResourceNode turns a live object into a tree node. gvr is the resource
// the object was actually listed or fetched through, which is what the
// sanitizer keys its Secret strip off — see sanitizeObject.
func buildResourceNode(obj map[string]any, gvr schema.GroupVersionResource,
	parentRef *models.ResourceRef, healthStatus openchoreov1alpha1.HealthStatus) (models.ResourceNode, bool) {
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
		Object:          sanitizeObject(obj, gvr),
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
	healthCheckFunc := renderedreleasecontroller.GetHealthCheckFunc(gvk)
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

// sanitizeObject trims an object before it leaves this API. gvr is the resource
// the object was selected through, never the object's own kind field: on the
// legacy fallback path a list item arrives without a kind and gets one
// backfilled from the operator's rule text, so a rule saying `kind: secret`
// would spell the strip away. The GVR is what the list call was built from and
// cannot be misspelled without the list failing outright.
func sanitizeObject(obj map[string]any, gvr schema.GroupVersionResource) map[string]any {
	sanitized := make(map[string]any, len(obj))
	maps.Copy(sanitized, obj)

	if metadata, ok := sanitized["metadata"].(map[string]any); ok {
		metaCopy := make(map[string]any, len(metadata))
		maps.Copy(metaCopy, metadata)
		delete(metaCopy, "managedFields")
		sanitized["metadata"] = metaCopy
	}

	if isCoreSecretResource(gvr.Group, gvr.Resource) {
		delete(sanitized, "data")
		delete(sanitized, "stringData")
		// The deletes above are only half the strip: kubectl's last-applied
		// annotation holds the whole serialized Secret, data block and all, so a
		// Secret that was ever applied with kubectl would carry its own contents
		// past them. This branch exists to keep Secret data out of the response,
		// and that is not true of applied Secrets without this.
		if metadata, ok := sanitized["metadata"].(map[string]any); ok {
			sanitized["metadata"] = withoutLastAppliedConfig(metadata)
		}
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

// extractContainerNames returns the names of the containers defined in a pod's spec.
func extractContainerNames(pod map[string]any) []string {
	spec, ok := pod["spec"].(map[string]any)
	if !ok {
		return nil
	}
	containers, ok := spec["containers"].([]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(containers))
	for _, c := range containers {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := cm["name"].(string); ok && name != "" {
			names = append(names, name)
		}
	}
	return names
}

// sortLogEntriesByTimestamp orders log entries chronologically. Timestamps are parsed once
// each so entries aggregated from multiple containers interleave into a single timeline.
func sortLogEntriesByTimestamp(entries []models.PodLogEntry) {
	type keyed struct {
		ts    time.Time
		entry models.PodLogEntry
	}
	keyedEntries := make([]keyed, len(entries))
	for i, e := range entries {
		keyedEntries[i] = keyed{ts: parseLogTimestamp(e.Timestamp), entry: e}
	}
	sort.SliceStable(keyedEntries, func(i, j int) bool {
		return keyedEntries[i].ts.Before(keyedEntries[j].ts)
	})
	for i := range keyedEntries {
		entries[i] = keyedEntries[i].entry
	}
}

// parseLogTimestamp parses an RFC3339(Nano) timestamp, returning the zero time on failure.
func parseLogTimestamp(ts string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t
	}
	return time.Time{}
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

// isChildResourceKind reports whether the traversal rules reach this kind
// through some parent. Such a kind is never listed in a release's own status,
// so events and logs for it have to be attributed through that parent. The
// lookup is group aware: a CRD reusing a built-in Kind name is a different kind
// and no rule reaches it.
func (s *k8sResourcesService) isChildResourceKind(group, kind string) bool {
	_, ok := s.rules.childParents[groupKind{Group: group, Kind: kind}]
	return ok
}

// hasParentResourceInRelease reports whether the release owns any root the
// child kind is reachable from, which is what attributes the child to that
// release. How the rule matches the child — owner reference or label selector —
// does not enter into it. The parent slice belongs to the compiled rules and is
// only read here.
func (s *k8sResourcesService) hasParentResourceInRelease(group, childKind string,
	resources []openchoreov1alpha1.RenderedManifestStatus) bool {
	parents := s.rules.childParents[groupKind{Group: group, Kind: childKind}]
	if len(parents) == 0 {
		return false
	}
	for i := range resources {
		if slices.Contains(parents, groupKind{Group: resources[i].Group, Kind: resources[i].Kind}) {
			return true
		}
	}
	return false
}

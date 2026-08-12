// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/clients/gateway"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/resourcetree/protocol"
)

const (
	// discoveryStateForbidden and discoveryStateError are the two states a
	// ChildDiscoveryStatus reports. They are kept apart because a console can act
	// on the first — grant the agent RBAC — and can only retry the second.
	discoveryStateForbidden = "forbidden"
	discoveryStateError     = "error"

	// matchedByLabelSelector badges nodes found by a heuristic matcher. Exact
	// (ownerRef) matches carry no badge, so the console highlights only the
	// relationships that were inferred.
	matchedByLabelSelector = "labelSelector"
)

// nodeAccumulator collects the nodes of ONE release. It is created once per
// release and shared by every root, because dedupe is release-wide: the same Pod
// can be reachable from two roots and must surface as one node carrying both
// parents.
//
// Nodes are addressed by INDEX, never by pointer. append reallocates the backing
// array, so a *models.ResourceNode captured before an append can point into an
// abandoned array and silently drop every later mutation.
type nodeAccumulator struct {
	nodes      []models.ResourceNode
	indexByUID map[string]int

	// warnedLegacyFallback keeps the version-skew warning to one line per
	// release rather than one per level per root. It lives here because the
	// accumulator is the only state spanning a release's whole walk.
	warnedLegacyFallback bool
}

func newNodeAccumulator(capacity int) *nodeAccumulator {
	return &nodeAccumulator{
		nodes:      make([]models.ResourceNode, 0, capacity),
		indexByUID: make(map[string]int, capacity),
	}
}

// add emits a node, or merges its parents into the node already emitted for the
// same UID.
func (a *nodeAccumulator) add(node models.ResourceNode) {
	if idx, ok := a.indexByUID[node.UID]; ok {
		existing := &a.nodes[idx]
		existing.ParentRefs = mergeParentRefs(existing.ParentRefs, node.ParentRefs)
		if node.MetadataOnly && !existing.MetadataOnly {
			existing.MetadataOnly = true
			existing.Object = projectMetadata(existing.Object)
		}
		if node.MatchedBy == matchedByLabelSelector {
			existing.MatchedBy = matchedByLabelSelector
		}
		return
	}
	a.indexByUID[node.UID] = len(a.nodes)
	a.nodes = append(a.nodes, node)
}

// addStatus records a discovery failure on an already emitted node. A UID with
// no node is dropped: the only way to reach one is a root whose live GET failed,
// which is already reported as a skipped resource.
//
// An identical status is recorded once. One query covers every parent sharing an
// edge, so a single failure is reported for each of them — and several hidden
// parents (the ReplicaSets under one Deployment) resolve to the SAME anchor node,
// which would otherwise collect the same line once per sibling.
func (a *nodeAccumulator) addStatus(uid string, status models.ChildDiscoveryStatus) {
	idx, ok := a.indexByUID[uid]
	if !ok {
		return
	}
	if slices.Contains(a.nodes[idx].ChildrenStatus, status) {
		return
	}
	a.nodes[idx].ChildrenStatus = append(a.nodes[idx].ChildrenStatus, status)
}

func appendRefUnique(refs []models.ResourceRef, ref models.ResourceRef) []models.ResourceRef {
	if slices.ContainsFunc(refs, func(r models.ResourceRef) bool { return r.UID == ref.UID }) {
		return refs
	}
	return append(refs, ref)
}

func mergeParentRefs(existing, added []models.ResourceRef) []models.ResourceRef {
	for _, ref := range added {
		existing = appendRefUnique(existing, ref)
	}
	return existing
}

// walkParent is one live object whose children are still to be discovered.
type walkParent struct {
	uid       string
	namespace string
	name      string
	// anchorRef is BOTH the ref this parent's children carry and the node its own
	// discovery failures attach to. For an emitted parent it is the parent's own
	// ref; for a hidden one — a ReplicaSet, say — it is inherited from the nearest
	// emitted ancestor, so a hidden parent's children hang off, and its failures
	// land on, a node the user can actually see.
	anchorRef models.ResourceRef
	edges     []*compiledChild
}

// edgeGroup is the set of parents that share one rule edge. They share a query
// too: that is what lets a level cost one round trip instead of one per parent.
type edgeGroup struct {
	edge         *compiledChild
	parents      []*walkParent
	parentsByUID map[string]*walkParent
}

func (g *edgeGroup) addParent(parent *walkParent) {
	if _, ok := g.parentsByUID[parent.uid]; ok {
		return
	}
	g.parentsByUID[parent.uid] = parent
	g.parents = append(g.parents, parent)
}

// levelQuery is one wire query of a level: one edge group, and one chunk of that
// group's parents. A group whose parents fit a single query becomes exactly one
// of these, so the common shape is unchanged.
type levelQuery struct {
	// id is the wire query ID: the group's EdgeID, or EdgeID#<n> for chunk n of
	// a group that had to be split.
	id string
	// group indexes the groups slice the queries were built from.
	group   int
	parents []*walkParent
}

// buildLevelQueries turns a level's groups into the queries that carry it,
// splitting any group with more parents than the agent accepts in one query.
//
// A group that fits keeps the bare EdgeID as its wire ID; a split one suffixes
// #<n>, which cannot collide with another edge's ID because an EdgeID is built
// from group, kind and children[i] alone and never contains a '#'.
//
// The EdgeID is what identifies a query in the first place because it is the
// edge's structural path: level plus parent kind plus child kind is not unique,
// since the same kind edge can appear in two branches.
func buildLevelQueries(groups []*edgeGroup) []levelQuery {
	queries := make([]levelQuery, 0, len(groups))

	for i, group := range groups {
		if len(group.parents) <= protocol.MaxParentsPerQuery {
			queries = append(queries, levelQuery{id: group.edge.EdgeID, group: i, parents: group.parents})
			continue
		}

		chunk := 0
		for parents := range slices.Chunk(group.parents, protocol.MaxParentsPerQuery) {
			queries = append(queries, levelQuery{
				id:      fmt.Sprintf("%s#%d", group.edge.EdgeID, chunk),
				group:   i,
				parents: parents,
			})
			chunk++
		}
	}

	return queries
}

// buildMatchRequest packs one batch of queries into a request.
func buildMatchRequest(batch []levelQuery, groups []*edgeGroup) *protocol.MatchRequest {
	req := &protocol.MatchRequest{
		// The version is always sent: the client compares the response against
		// it, so an empty one would turn the agent's per-result UnsupportedVersion
		// codes into a transport-level mismatch that swallows them.
		Version: protocol.Version,
		Queries: make([]protocol.MatchQuery, 0, len(batch)),
	}
	for _, lq := range batch {
		req.Queries = append(req.Queries, buildMatchQuery(groups[lq.group].edge, lq.id, lq.parents))
	}
	return req
}

// buildMatchQuery builds the query for one edge over one chunk of its parents.
// Criteria is the compile-time marshaled payload, shared by every request that
// uses this edge: it is assigned as is and never mutated.
//
// Chunks omit Limit, which the agent reads as DefaultMatchLimit, so every chunk
// gets the full allowance and a split group can return more matches than an
// unsplit one would — which is the point: those matches exist and the unsplit
// query could not have asked for them. The agent's byte budget still bounds
// each response, and mergeMatchesByUID unions a child two chunks both reported.
func buildMatchQuery(edge *compiledChild, id string, parents []*walkParent) protocol.MatchQuery {
	refs := make([]protocol.ParentRef, 0, len(parents))
	for _, parent := range parents {
		refs = append(refs, protocol.ParentRef{
			UID:       parent.uid,
			Namespace: parent.namespace,
			Name:      parent.name,
		})
	}

	return protocol.MatchQuery{
		ID:       id,
		Matcher:  edge.Matcher,
		Criteria: edge.Criteria,
		Parents:  refs,
		Child: protocol.ChildKind{
			Group:    edge.Kind.Group,
			Version:  edge.Kind.Version,
			Kind:     edge.Kind.Kind,
			Resource: edge.Kind.Resource,
		},
		MetadataOnly: edge.MetadataOnly,
	}
}

// childMatch is one discovered object and the parents it was attributed to,
// normalized across the agent path and the legacy fallback so the rest of the
// walk cannot tell which found it.
type childMatch struct {
	obj        map[string]any
	parentUIDs []string
}

// treeWalk carries the per-root walk state.
type treeWalk struct {
	svc *k8sResourcesService
	pi  planeInfo
	acc *nodeAccumulator
}

// expandChildren walks one root's rule tree breadth first, appending every
// discovered node into the release-wide accumulator.
func (s *k8sResourcesService) expandChildren(ctx context.Context, pi planeInfo, acc *nodeAccumulator,
	rootUID string, rootObj map[string]any, rs *openchoreov1alpha1.RenderedManifestStatus) {
	edges := s.rootEdges(rs)
	if len(edges) == 0 {
		// No rule for this kind means no children and no status, matching the
		// behavior a kind outside the traversal always had.
		return
	}

	namespace := getNestedString(rootObj, "metadata", "namespace")
	if namespace == "" {
		namespace = rs.Namespace
	}
	name := getNestedString(rootObj, "metadata", "name")
	if name == "" {
		name = rs.Name
	}

	root := &walkParent{
		uid:       rootUID,
		namespace: namespace,
		name:      name,
		anchorRef: models.ResourceRef{
			Group:     rs.Group,
			Version:   rs.Version,
			Kind:      rs.Kind,
			Namespace: namespace,
			Name:      name,
			UID:       rootUID,
		},
		edges: edges,
	}

	walk := &treeWalk{svc: s, pi: pi, acc: acc}
	for level := []*walkParent{root}; len(level) > 0; {
		level = walk.expandLevel(ctx, level)
	}
}

// rootEdges returns the edges configured for a release resource's kind,
// concatenating every rule that matches rather than taking only the first.
//
// That is not, on its own, enough to make duplicate roots work: EdgeIDs are
// derived from the root ref alone, so two rules sharing a root group+kind would
// mint colliding IDs and the second rule's edges would silently fold into the
// first group, losing its matcher, criteria and child kind. Startup validation
// rejecting duplicate roots is what makes this safe — the concatenation only
// keeps the lookup from depending on map iteration order.
func (s *k8sResourcesService) rootEdges(rs *openchoreov1alpha1.RenderedManifestStatus) []*compiledChild {
	rules := s.rules.roots[groupKind{Group: rs.Group, Kind: rs.Kind}]
	edges := make([]*compiledChild, 0, len(rules))
	for _, rule := range rules {
		edges = append(edges, rule.Children...)
	}
	return edges
}

// expandLevel resolves one breadth-first level and returns the parents of the
// next one. An empty return ends the walk, either because the rules bottomed out
// or because discovery failed and must not descend further.
func (w *treeWalk) expandLevel(ctx context.Context, level []*walkParent) []*walkParent {
	groups := w.groupByEdge(level)
	if len(groups) == 0 {
		return nil
	}

	matches, ok := w.resolveGroups(ctx, groups)
	if !ok {
		return nil
	}

	var next []*walkParent
	for i, group := range groups {
		next = append(next, w.consumeGroup(group, matches[i])...)
	}
	return next
}

// groupByEdge collects the parents sharing a rule edge into one group, in
// first-seen order so requests are deterministic.
//
// A cluster-scoped parent is queried only when a label selector explicitly
// scopes the search and does not derive a value from the absent parent namespace.
func (w *treeWalk) groupByEdge(level []*walkParent) []*edgeGroup {
	groups := make([]*edgeGroup, 0, len(level))
	byEdgeID := make(map[string]*edgeGroup, len(level))

	for _, parent := range level {
		for _, edge := range parent.edges {
			if parent.namespace == "" && !safeForClusterScopedParent(edge) {
				w.acc.addStatus(parent.anchorRef.UID,
					statusFor(edge, discoveryStateError, "cluster-scoped parents are not supported"))
				continue
			}
			group, ok := byEdgeID[edge.EdgeID]
			if !ok {
				group = &edgeGroup{edge: edge, parentsByUID: map[string]*walkParent{}}
				byEdgeID[edge.EdgeID] = group
				groups = append(groups, group)
			}
			group.addParent(parent)
		}
	}

	return groups
}

func safeForClusterScopedParent(edge *compiledChild) bool {
	if edge.Matcher != protocol.MatcherLabelSelector {
		return false
	}
	var criteria protocol.LabelSelectorCriteria
	if err := json.Unmarshal(edge.Criteria, &criteria); err != nil || len(criteria.Namespaces) == 0 {
		return false
	}
	_, usesNamespace := criteria.ParentFieldUse()
	return !usesNamespace
}

// resolveGroups asks the cluster agent for one level's children and returns the
// matches per group. The bool reports whether the walk may descend: false means
// discovery failed outright, every affected parent already carries a status, and
// the nodes emitted so far stand.
//
// A level costs as many requests as the agent's per-request query cap demands,
// sent one after another, stopping at the first failure. Answered batches are
// held until every batch has answered, and only then applied, because a failure
// is a failure of the whole level: a batch already applied would have written
// its per-query statuses onto nodes, and neither the fallback below nor the
// caller can take those back.
func (w *treeWalk) resolveGroups(ctx context.Context, groups []*edgeGroup) ([][]childMatch, bool) {
	batches := slices.Collect(slices.Chunk(buildLevelQueries(groups), protocol.MaxQueriesPerRequest))

	responses := make([]*protocol.MatchResponse, 0, len(batches))
	for _, batch := range batches {
		resp, err := w.svc.gatewayClient.MatchResourceTreeChildren(ctx,
			w.pi.planeType, w.pi.planeID, w.pi.crNamespace, w.pi.crName, buildMatchRequest(batch, groups))
		if err == nil {
			responses = append(responses, resp)
			continue
		}

		// ONLY the version-skew sentinel permits the legacy walk, and it arrives
		// wrapped with diagnostic text, so it is matched with errors.Is. A timeout,
		// a 5xx, a malformed body or a response that does not answer the queries
		// sent are real failures: falling back on those would quietly turn an
		// outage into a slower, differently shaped answer.
		//
		// An agent does not gain or lose resource-tree support between two batches
		// of one level, so the remaining batches are not sent: the whole level is
		// resolved control-plane side instead, discarding what the earlier batches
		// answered so the level has one shape rather than two.
		if errors.Is(err, gateway.ErrResourceTreeUnsupported) {
			w.warnLegacyFallback()
			return w.resolveGroupsLegacy(ctx, groups), true
		}

		for _, group := range groups {
			for _, parent := range group.parents {
				w.acc.addStatus(parent.anchorRef.UID, statusFor(group.edge, discoveryStateError, err.Error()))
			}
		}
		return nil, false
	}

	matches := make([][]childMatch, len(groups))
	for i, batch := range batches {
		w.collectMatches(batch, groups, responses[i], matches)
	}
	return matches, true
}

// collectMatches lines one response up with the queries that produced it,
// appending into the level's per-group matches and recording any per-query
// failure on the parents it affected.
//
// A failure is reported to the chunk's parents, not the group's: the other
// chunks of a split group were answered separately and may well have succeeded.
func (w *treeWalk) collectMatches(queries []levelQuery, groups []*edgeGroup,
	resp *protocol.MatchResponse, matches [][]childMatch) {
	byID := make(map[string]*protocol.MatchResult, len(resp.Results))
	for i := range resp.Results {
		byID[resp.Results[i].ID] = &resp.Results[i]
	}

	for _, lq := range queries {
		edge := groups[lq.group].edge
		// The client already verified that the results cover the queries sent
		// exactly, so a missing result cannot happen; skipping rather than
		// indexing blindly keeps that assumption from becoming a panic.
		result, ok := byID[lq.id]
		if !ok {
			continue
		}

		for _, matched := range result.Matches {
			var obj map[string]any
			if err := json.Unmarshal(matched.Object, &obj); err != nil {
				w.svc.logger.Warn("Skipping unreadable matched object",
					"kind", edge.Kind.Kind, "queryID", lq.id, "error", err)
				continue
			}
			matches[lq.group] = append(matches[lq.group], childMatch{obj: obj, parentUIDs: matched.ParentUIDs})
		}

		if status, failed := resultStatus(edge, result); failed {
			for _, parent := range lq.parents {
				w.acc.addStatus(parent.anchorRef.UID, status)
			}
		}
	}
}

// resultStatus turns a per-query failure into a node status. A truncated result
// still carries the matches it managed to collect, so it reports alongside them
// rather than instead of them.
func resultStatus(edge *compiledChild, result *protocol.MatchResult) (models.ChildDiscoveryStatus, bool) {
	switch {
	case result.Error != nil:
		state := discoveryStateError
		if result.Error.Code == protocol.CodeForbidden {
			state = discoveryStateForbidden
		}
		return statusFor(edge, state, result.Error.Message), true
	case result.Truncated:
		return statusFor(edge, discoveryStateError, "the result was truncated"), true
	default:
		return models.ChildDiscoveryStatus{}, false
	}
}

// consumeGroup emits one edge's matches and returns the parents they contribute
// to the next level.
func (w *treeWalk) consumeGroup(group *edgeGroup, matches []childMatch) []*walkParent {
	var next []*walkParent

	for _, match := range mergeMatchesByUID(matches) {
		refs, misParented := w.parentRefsFor(group, match.parentUIDs)
		// A hidden parent has no node of its own, so its children inherit the ref
		// it inherited — the nearest node the user can see.
		anchor := refs[0]
		if misParented {
			// The match named no parent this query asked about. It is still attached
			// (to the first parent) so it stays reachable, but flag that parent so
			// the mis-attribution is visible instead of a silent wrong edge.
			w.acc.addStatus(group.parents[0].anchorRef.UID,
				statusFor(group.edge, discoveryStateError,
					"a matched child named no known parent and was attached to the first parent"))
		}

		if !group.edge.Hide {
			node, ok := buildResourceNode(match.obj, group.edge.gvr(), nil, "")
			if !ok {
				// A match that passed the wire contract but cannot be built into a
				// node (missing identity fields) is dropped. Flag the anchor so the
				// child set reads as incomplete rather than silently short — a
				// membership lookup must not treat a discarded match as "not there".
				w.svc.logger.Warn("Skipping child node with missing required fields",
					"kind", group.edge.Kind.Kind, "queryID", group.edge.EdgeID)
				w.acc.addStatus(anchor.UID, statusFor(group.edge, discoveryStateError,
					"a matched child was dropped for missing required fields"))
				continue
			}
			node.ParentRefs = refs
			node.MetadataOnly = group.edge.MetadataOnly
			if group.edge.Matcher == protocol.MatcherLabelSelector {
				node.MatchedBy = matchedByLabelSelector
			}
			if node.MetadataOnly {
				node.Object = projectMetadata(node.Object)
			}
			w.acc.add(node)

			anchor = models.ResourceRef{
				Group:     node.Group,
				Version:   node.Version,
				Kind:      node.Kind,
				Namespace: node.Namespace,
				Name:      node.Name,
				UID:       node.UID,
			}
		}

		if len(group.edge.Children) == 0 {
			continue
		}
		next = append(next, &walkParent{
			uid:       getNestedString(match.obj, "metadata", "uid"),
			namespace: getNestedString(match.obj, "metadata", "namespace"),
			name:      getNestedString(match.obj, "metadata", "name"),
			anchorRef: anchor,
			edges:     group.edge.Children,
		})
	}

	return next
}

// mergeMatchesByUID collapses one edge's matches to one entry per object. The
// agent already dedupes within a query, but the legacy fallback runs per parent
// and reports a shared child once for each of them.
func mergeMatchesByUID(matches []childMatch) []childMatch {
	merged := make([]childMatch, 0, len(matches))
	indexByUID := make(map[string]int, len(matches))

	for _, match := range matches {
		uid := getNestedString(match.obj, "metadata", "uid")
		if uid == "" {
			// Without a UID the object can neither be deduped nor be queried as a
			// parent; buildResourceNode rejects it for the same reason.
			continue
		}
		if idx, ok := indexByUID[uid]; ok {
			for _, parentUID := range match.parentUIDs {
				if !slices.Contains(merged[idx].parentUIDs, parentUID) {
					merged[idx].parentUIDs = append(merged[idx].parentUIDs, parentUID)
				}
			}
			continue
		}
		indexByUID[uid] = len(merged)
		// The parent UIDs are cloned because the source slice belongs to the
		// decoded response and is appended to below.
		merged = append(merged, childMatch{obj: match.obj, parentUIDs: slices.Clone(match.parentUIDs)})
	}

	return merged
}

// parentRefsFor maps the parent UIDs a match was attributed to onto the refs its
// node carries. Two hidden ReplicaSets under one Deployment collapse to a single
// Deployment ref, because both inherited it. The result is never empty; the
// second return reports whether it fell back to the first parent because none of
// the attributed UIDs were recognized.
func (w *treeWalk) parentRefsFor(group *edgeGroup, parentUIDs []string) ([]models.ResourceRef, bool) {
	refs := make([]models.ResourceRef, 0, len(parentUIDs))
	for _, uid := range parentUIDs {
		if parent, ok := group.parentsByUID[uid]; ok {
			refs = appendRefUnique(refs, parent.anchorRef)
		}
	}
	if len(refs) == 0 {
		// An object attributed to no parent this query asked about still belongs
		// somewhere: parenting it to the group's first parent keeps it reachable
		// instead of orphaning it in the tree. It is also a mis-parenting a
		// conforming agent never causes, so the caller records a status on that
		// parent rather than letting a wrong edge appear with nothing to explain it.
		w.svc.logger.Debug("Match attributed to unknown parents; parenting it to the first parent in the query",
			"queryID", group.edge.EdgeID, "kind", group.edge.Kind.Kind,
			"parentUIDs", parentUIDs, "parent", group.parents[0].name)
		refs = append(refs, group.parents[0].anchorRef)
		return refs, true
	}
	return refs, false
}

// projectMetadata reduces an object to its identity and metadata. The agent
// already trims metadataOnly matches, but the legacy fallback transfers whole
// objects, so the projection is repeated here: a Secret's contents must never
// leave this API regardless of which path found it.
func projectMetadata(obj map[string]any) map[string]any {
	projected := make(map[string]any, 3)
	for _, key := range []string{"apiVersion", "kind", "metadata"} {
		if value, ok := obj[key]; ok {
			projected[key] = value
		}
	}
	if metadata, ok := projected["metadata"].(map[string]any); ok {
		projected["metadata"] = withoutLastAppliedConfig(metadata)
	}
	return projected
}

// withoutLastAppliedConfig returns metadata without kubectl's last-applied
// annotation, whose value is the whole serialized object. Keeping it would let a
// metadata-only node carry exactly the contents the projection exists to
// withhold, for every Secret that was ever `kubectl apply`-ed — reducing the
// guarantee to "Secrets nobody applied with kubectl".
//
// Only the maps it changes are copied, and only when the annotation is actually
// present: the metadata it is handed can be shared with the caller's object.
func withoutLastAppliedConfig(metadata map[string]any) map[string]any {
	annotations, ok := metadata["annotations"].(map[string]any)
	if !ok {
		return metadata
	}
	if _, present := annotations[protocol.LastAppliedConfigAnnotation]; !present {
		return metadata
	}

	trimmedAnnotations := make(map[string]any, len(annotations)-1)
	for key, value := range annotations {
		if key != protocol.LastAppliedConfigAnnotation {
			trimmedAnnotations[key] = value
		}
	}

	trimmed := maps.Clone(metadata)
	trimmed["annotations"] = trimmedAnnotations
	return trimmed
}

func statusFor(edge *compiledChild, state, message string) models.ChildDiscoveryStatus {
	return models.ChildDiscoveryStatus{
		Group:   edge.Kind.Group,
		Version: edge.Kind.Version,
		Kind:    edge.Kind.Kind,
		State:   state,
		Message: message,
	}
}

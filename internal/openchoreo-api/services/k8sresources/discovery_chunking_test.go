// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/resourcetree/protocol"
)

// --- chunking fixtures ---

func widgetKind(i int) string   { return fmt.Sprintf("Widget%d", i) }
func widgetPlural(i int) string { return fmt.Sprintf("widget%ds", i) }

// widgetEdgesConfig hangs n sibling edges off the Deployment root, each on its
// own child kind, so one level can carry more edges than a single request may.
// Config validation caps a rule set at 256 edges with no per-level cap, so this
// is a shape an operator can legitimately configure.
func widgetEdgesConfig(n int) config.ResourceTreeConfig {
	children := make([]config.ChildRule, 0, n)
	for i := range n {
		children = append(children, config.ChildRule{
			Kind: config.KindRef{Group: "example.com", Version: "v1", Kind: widgetKind(i), Resource: widgetPlural(i)},
		})
	}
	return config.ResourceTreeConfig{Rules: []config.ResourceTreeRule{{
		Root:     config.KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
		Children: children,
	}}}
}

// widgetTree gives every widget edge exactly one child owned by the root, so a
// missing node names the edge that was dropped.
func widgetTree(n int) *fakeCluster {
	objects := make([]map[string]any, 0, n)
	kindByPlural := make(map[string]string, n)
	for i := range n {
		objects = append(objects, liveObject("example.com/v1", widgetKind(i), treeNamespace,
			fmt.Sprintf("widget-%d", i), fmt.Sprintf("widget-%d-uid", i), ownedBy("deploy-uid")))
		kindByPlural[widgetPlural(i)] = widgetKind(i)
	}
	return &fakeCluster{objects: objects, kindByPlural: kindByPlural}
}

// replicaSetPodConfig keeps the ReplicaSets visible, unlike the built-in rules,
// so a Pod shared by two of them carries two parent refs rather than collapsing
// onto the Deployment anchor they would both inherit.
func replicaSetPodConfig() config.ResourceTreeConfig {
	return config.ResourceTreeConfig{Rules: []config.ResourceTreeRule{{
		Root: config.KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
		Children: []config.ChildRule{{
			Kind:     config.KindRef{Group: "apps", Version: "v1", Kind: "ReplicaSet", Resource: "replicasets"},
			Children: []config.ChildRule{{Kind: config.KindRef{Version: "v1", Kind: "Pod", Resource: "pods"}}},
		}},
	}}}
}

func replicaSetName(i int) string { return fmt.Sprintf("web-%03d", i) }
func replicaSetUID(i int) string  { return fmt.Sprintf("rs-%03d-uid", i) }

// replicaSetTree builds n ReplicaSets under the Deployment root — one level-two
// parent each — plus three Pods that pin where a chunk boundary falls: one owned
// by the first ReplicaSet, one by the last, and one by both.
func replicaSetTree(n int) *fakeCluster {
	objects := make([]map[string]any, 0, n+3)
	for i := range n {
		objects = append(objects, liveObject("apps/v1", "ReplicaSet", treeNamespace,
			replicaSetName(i), replicaSetUID(i), ownedBy("deploy-uid")))
	}
	objects = append(objects,
		liveObject("v1", "Pod", treeNamespace, "first-pod", "first-pod-uid", ownedBy(replicaSetUID(0))),
		liveObject("v1", "Pod", treeNamespace, "last-pod", "last-pod-uid", ownedBy(replicaSetUID(n-1))),
		liveObject("v1", "Pod", treeNamespace, "shared-pod", "shared-pod-uid",
			ownedBy(replicaSetUID(0), replicaSetUID(n-1))),
	)
	return &fakeCluster{objects: objects}
}

// podEdgeID is the wire ID the Pod edge of replicaSetPodConfig would carry
// unsplit, which the chunk IDs are expected to extend.
func podEdgeID(t *testing.T, svc *k8sResourcesService) string {
	t.Helper()
	rules := svc.rules.roots[groupKind{Group: "apps", Kind: "Deployment"}]
	require.Len(t, rules, 1)
	require.Len(t, rules[0].Children, 1)
	require.Len(t, rules[0].Children[0].Children, 1)
	return rules[0].Children[0].Children[0].EdgeID
}

// queriesForKind returns every query sent for a child kind, in wire order.
func queriesForKind(rec *gatewayRecorder, childKind string) []protocol.MatchQuery {
	var queries []protocol.MatchQuery
	for _, req := range rec.matchRequests() {
		for _, q := range req.Queries {
			if q.Child.Kind == childKind {
				queries = append(queries, q)
			}
		}
	}
	return queries
}

// --- chunking ---

func TestExpandChildrenChunking(t *testing.T) {
	t.Run("a level with more edges than one request allows is split into batches", func(t *testing.T) {
		const edges = protocol.MaxQueriesPerRequest + 1

		rec := &gatewayRecorder{}
		cluster := widgetTree(edges)
		svc := treeService(treeGateway(t, rec, cluster.match(t), nil), widgetEdgesConfig(edges))

		nodes := runWalk(t, svc, deploymentRootObject(), deploymentRootStatus())

		batches := rec.matchRequests()
		require.Equal(t, 2, len(batches),
			"a request carrying more than %d queries is rejected whole", protocol.MaxQueriesPerRequest)
		assert.Equal(t, protocol.MaxQueriesPerRequest, len(batches[0].Queries))
		assert.Equal(t, 1, len(batches[1].Queries))

		require.Equal(t, edges+1, len(nodes), "every edge's child must survive the split")
		assert.Empty(t, nodes[0].ChildrenStatus, "a level that resolved must report nothing")

		ids := rec.queryIDs()
		assert.Len(t, slices.Compact(slices.Sorted(slices.Values(ids))), edges, "ids must stay unique across batches")
		for _, id := range ids {
			assert.NotContains(t, id, "#", "a single-chunk group keeps its bare edge id")
		}
	})

	t.Run("an edge with more parents than one query allows is split into chunks", func(t *testing.T) {
		const parents = protocol.MaxParentsPerQuery + 1

		rec := &gatewayRecorder{}
		cluster := replicaSetTree(parents)
		svc := treeService(treeGateway(t, rec, cluster.match(t), nil), replicaSetPodConfig())

		nodes := runWalk(t, svc, deploymentRootObject(), deploymentRootStatus())

		podQueries := queriesForKind(rec, podChildKind)
		require.Equal(t, 2, len(podQueries),
			"a query carrying more than %d parents is rejected", protocol.MaxParentsPerQuery)
		edgeID := podEdgeID(t, svc)
		assert.Equal(t, edgeID+"#0", podQueries[0].ID)
		assert.Equal(t, edgeID+"#1", podQueries[1].ID)
		assert.Equal(t, protocol.MaxParentsPerQuery, len(podQueries[0].Parents))
		assert.Equal(t, 1, len(podQueries[1].Parents))

		batches := rec.matchRequests()
		require.Equal(t, 2, len(batches), "one batch per level: the chunks of one edge fit in one request")
		assert.Equal(t, 2, len(batches[1].Queries), "both Pod chunks travel in the level's single request")

		// Root, every ReplicaSet, and the three Pods the chunks between them found.
		require.Equal(t, 1+parents+3, len(nodes), "the matches of both chunks must be kept")
		findNode(t, nodes, "Pod", "first-pod")
		findNode(t, nodes, "Pod", "last-pod")

		shared := findNode(t, nodes, "Pod", "shared-pod")
		require.Len(t, shared.ParentRefs, 2, "a Pod owned across the chunk boundary keeps both parents")
		assert.ElementsMatch(t, []string{replicaSetUID(0), replicaSetUID(parents - 1)},
			[]string{shared.ParentRefs[0].UID, shared.ParentRefs[1].UID})
	})

	t.Run("a group of exactly the per-query cap stays one query", func(t *testing.T) {
		rec := &gatewayRecorder{}
		cluster := replicaSetTree(protocol.MaxParentsPerQuery)
		svc := treeService(treeGateway(t, rec, cluster.match(t), nil), replicaSetPodConfig())

		runWalk(t, svc, deploymentRootObject(), deploymentRootStatus())

		podQueries := queriesForKind(rec, podChildKind)
		require.Equal(t, 1, len(podQueries), "the cap is inclusive; the agent rejects only what exceeds it")
		assert.Equal(t, podEdgeID(t, svc), podQueries[0].ID)
		assert.Equal(t, protocol.MaxParentsPerQuery, len(podQueries[0].Parents))
	})

	t.Run("a group far past the cap is split into full chunks and a remainder", func(t *testing.T) {
		const parents = protocol.DefaultMatchLimit

		rec := &gatewayRecorder{}
		cluster := replicaSetTree(parents)
		svc := treeService(treeGateway(t, rec, cluster.match(t), nil), replicaSetPodConfig())

		runWalk(t, svc, deploymentRootObject(), deploymentRootStatus())

		podQueries := queriesForKind(rec, podChildKind)
		require.Equal(t, 2, len(podQueries))
		assert.Equal(t, protocol.MaxParentsPerQuery, len(podQueries[0].Parents))
		assert.Equal(t, parents-protocol.MaxParentsPerQuery, len(podQueries[1].Parents))
	})

	t.Run("a failed chunk is reported only to the parents it carried", func(t *testing.T) {
		const parents = protocol.MaxParentsPerQuery + 1

		rec := &gatewayRecorder{}
		cluster := replicaSetTree(parents)
		gc := treeGateway(t, rec, func(req protocol.MatchRequest) stubResponse {
			return matchResponse(req, func(q protocol.MatchQuery) protocol.MatchResult {
				if q.Child.Kind == podChildKind && strings.HasSuffix(q.ID, "#1") {
					return protocol.MatchResult{Error: &protocol.MatchError{
						Code: protocol.CodeForbidden, Message: "pods is forbidden",
					}}
				}
				return protocol.MatchResult{Matches: cluster.answer(t, q)}
			})
		}, nil)
		svc := treeService(gc, replicaSetPodConfig())

		nodes := runWalk(t, svc, deploymentRootObject(), deploymentRootStatus())

		failed := findNode(t, nodes, "ReplicaSet", replicaSetName(parents-1))
		require.Len(t, failed.ChildrenStatus, 1, "the chunk's own parent must carry the failure")
		assert.Equal(t, discoveryStateForbidden, failed.ChildrenStatus[0].State)

		answered := findNode(t, nodes, "ReplicaSet", replicaSetName(0))
		assert.Empty(t, answered.ChildrenStatus, "a parent in a chunk that succeeded must not inherit the failure")
		findNode(t, nodes, "Pod", "first-pod")
	})

	t.Run("a hard error on a later batch fails the whole level", func(t *testing.T) {
		const edges = protocol.MaxQueriesPerRequest + 1

		rec := &gatewayRecorder{}
		cluster := widgetTree(edges)
		serve := cluster.match(t)
		// The batch number is read off the recorder, which the stub fills under its
		// own lock before this handler runs.
		gc := treeGateway(t, rec, func(req protocol.MatchRequest) stubResponse {
			if len(rec.matchRequests()) == 1 {
				return serve(req)
			}
			return stubResponse{status: http.StatusInternalServerError, raw: "boom"}
		}, func(*http.Request) stubResponse {
			t.Error("a hard discovery error must never fall back to the legacy list endpoints")
			return stubResponse{status: http.StatusOK, body: k8sList()}
		})
		svc := treeService(gc, widgetEdgesConfig(edges))

		nodes := runWalk(t, svc, deploymentRootObject(), deploymentRootStatus())

		require.Equal(t, 1, len(nodes), "a failed level emits nothing, not the batch that happened to answer first")
		require.Equal(t, edges, len(nodes[0].ChildrenStatus), "every edge of the level must report the failure")
		for _, status := range nodes[0].ChildrenStatus {
			assert.Equal(t, discoveryStateError, status.State)
			assert.NotEmpty(t, status.Message)
		}
		assert.Empty(t, rec.legacyURLs())
	})

	t.Run("a version-skew 404 on a later batch re-resolves the whole level", func(t *testing.T) {
		const edges = protocol.MaxQueriesPerRequest + 1

		rec := &gatewayRecorder{}
		cluster := widgetTree(edges)
		serve := cluster.match(t)
		gc := treeGateway(t, rec, func(req protocol.MatchRequest) stubResponse {
			if len(rec.matchRequests()) == 1 {
				return serve(req)
			}
			return unsupportedTarget(req)
		}, cluster.list(t))
		svc := treeService(gc, widgetEdgesConfig(edges))
		var logs bytes.Buffer
		svc.logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))

		nodes := runWalk(t, svc, deploymentRootObject(), deploymentRootStatus())

		require.Equal(t, 2, len(rec.matchRequests()), "the skew must be discovered by the batch that hit it")
		require.Equal(t, edges+1, len(nodes), "the whole level must be re-resolved")
		assert.Equal(t, edges, len(rec.legacyURLs()),
			"the fallback covers every edge of the level, not only the batch that reported the skew")
		assert.Equal(t, 1, strings.Count(logs.String(), "falling back to control-plane filtering"),
			"the version-skew warning is one line per release, whatever a level costs in batches")
	})
}

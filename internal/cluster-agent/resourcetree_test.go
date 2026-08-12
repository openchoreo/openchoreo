// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusteragent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
	"github.com/openchoreo/openchoreo/internal/resourcetree/protocol"
)

var (
	podsGVR        = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	secretsGVR     = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	replicaSetsGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
)

// testScheme registers the list kinds the fake dynamic client needs to serve
// LIST for pods, secrets and replicasets.
func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	for _, gvk := range []schema.GroupVersionKind{
		{Group: "", Version: "v1", Kind: "Pod"},
		{Group: "", Version: "v1", Kind: "Secret"},
		{Group: "apps", Version: "v1", Kind: "ReplicaSet"},
	} {
		s.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		listGVK := gvk
		listGVK.Kind += "List"
		s.AddKnownTypeWithName(listGVK, &unstructured.UnstructuredList{})
	}
	return s
}

type ownerRef struct {
	uid        string
	controller bool
}

func newObject(apiVersion, kind, namespace, name, uid string, owners ...ownerRef) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]any{
			"namespace": namespace,
			"name":      name,
			"uid":       uid,
		},
	}}
	if len(owners) > 0 {
		refs := make([]any, 0, len(owners))
		for _, o := range owners {
			ref := map[string]any{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"name":       "owner-" + o.uid,
				"uid":        o.uid,
			}
			if o.controller {
				ref["controller"] = true
			}
			refs = append(refs, ref)
		}
		meta, _ := obj.Object["metadata"].(map[string]any)
		meta["ownerReferences"] = refs
	}
	return obj
}

func newPod(namespace, name, uid string, owners ...ownerRef) *unstructured.Unstructured {
	return newObject("v1", "Pod", namespace, name, uid, owners...)
}

func newReplicaSet(namespace, name, uid string, owners ...ownerRef) *unstructured.Unstructured {
	return newObject("apps/v1", "ReplicaSet", namespace, name, uid, owners...)
}

// listCall captures one LIST issued against the fake dynamic client.
type listCall struct {
	resource      schema.GroupVersionResource
	namespace     string
	labelSelector string
	continueToken string
	limit         int64
}

// listRecorder records every LIST action. Its record reactor does not handle
// the action so the object tracker (or a later reactor) still serves it.
type listRecorder struct {
	mu    sync.Mutex
	calls []listCall
}

func (r *listRecorder) record(action k8stesting.Action) (bool, runtime.Object, error) {
	la, ok := action.(k8stesting.ListActionImpl)
	if !ok {
		return false, nil, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, listCall{
		resource:      la.Resource,
		namespace:     la.Namespace,
		labelSelector: la.ListOptions.LabelSelector,
		continueToken: la.ListOptions.Continue,
		limit:         la.ListOptions.Limit,
	})
	return false, nil, nil
}

func (r *listRecorder) snapshot() []listCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]listCall, len(r.calls))
	copy(out, r.calls)
	return out
}

func (r *listRecorder) namespaces() []string {
	calls := r.snapshot()
	out := make([]string, 0, len(calls))
	for _, c := range calls {
		out = append(out, c.namespace)
	}
	return out
}

func (r *listRecorder) selectors() []string {
	calls := r.snapshot()
	out := make([]string, 0, len(calls))
	for _, c := range calls {
		out = append(out, c.labelSelector)
	}
	return out
}

// resourceCalls returns the calls issued against one resource.
func (r *listRecorder) resourceCalls(resource string) []listCall {
	calls := r.snapshot()
	out := make([]listCall, 0, len(calls))
	for _, c := range calls {
		if c.resource.Resource == resource {
			out = append(out, c)
		}
	}
	return out
}

// newFakeDyn builds a fake dynamic client seeded with objects.
func newFakeDyn(t *testing.T, objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	t.Helper()
	return dynamicfake.NewSimpleDynamicClient(testScheme(t), objects...)
}

// recordLists attaches a list recorder. Reactors run in order, so this must be
// installed AFTER any canned reactor or the recorder never sees the action.
func recordLists(dyn *dynamicfake.FakeDynamicClient) *listRecorder {
	rec := &listRecorder{}
	dyn.PrependReactor("list", "*", rec.record)
	return rec
}

// selectorStampingReaction serves a canned list, stamping the requested
// selector's labels onto every returned item. The fake dynamic client
// re-applies the selector to whatever a reactor returns, so canned items must
// carry labels that satisfy it — stamping lets one fixture object answer
// several different per-parent selectors, which is exactly the shared-child
// case the dedupe path needs.
func selectorStampingReaction(items ...*unstructured.Unstructured) k8stesting.ReactionFunc {
	return func(action k8stesting.Action) (bool, runtime.Object, error) {
		la, ok := action.(k8stesting.ListActionImpl)
		if !ok {
			return false, nil, nil
		}
		set := labels.Set{}
		if la.ListOptions.LabelSelector != "" {
			parsed, err := labels.ConvertSelectorToLabelsMap(la.ListOptions.LabelSelector)
			if err != nil {
				return true, nil, err
			}
			set = parsed
		}
		list := &unstructured.UnstructuredList{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "PodList",
		}}
		for _, item := range items {
			cp := item.DeepCopy()
			merged := cp.GetLabels()
			if merged == nil {
				merged = map[string]string{}
			}
			for k, v := range set {
				merged[k] = v
			}
			cp.SetLabels(merged)
			list.Items = append(list.Items, *cp)
		}
		return true, list, nil
	}
}

func newTestHandler(t *testing.T, objects ...runtime.Object) (*resourceTreeHandler, *listRecorder) {
	t.Helper()
	dyn := newFakeDyn(t, objects...)
	rec := recordLists(dyn)
	return newResourceTreeHandler(dyn, testLogger()), rec
}

// newSelectorHandler serves every list from a canned, selector-stamped fixture
// so tests can assert on the server-side selector the handler sent.
func newSelectorHandler(t *testing.T, items ...*unstructured.Unstructured) (*resourceTreeHandler, *listRecorder) {
	t.Helper()
	dyn := newFakeDyn(t)
	dyn.PrependReactor("list", "pods", selectorStampingReaction(items...))
	rec := recordLists(dyn)
	return newResourceTreeHandler(dyn, testLogger()), rec
}

func tunnelRequest(t *testing.T, req protocol.MatchRequest) *messaging.HTTPTunnelRequest {
	t.Helper()
	body, err := json.Marshal(req)
	require.NoError(t, err)
	return &messaging.HTTPTunnelRequest{
		RequestID: "req-1",
		Target:    protocol.Target,
		Method:    http.MethodPost,
		Path:      protocol.PathMatches,
		Body:      body,
	}
}

func decodeMatchResponse(t *testing.T, resp *messaging.HTTPTunnelResponse) protocol.MatchResponse {
	t.Helper()
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, "unexpected tunnel error: %+v", resp.Error)
	var out protocol.MatchResponse
	require.NoError(t, json.Unmarshal(resp.Body, &out))
	return out
}

func resultsByID(t *testing.T, resp protocol.MatchResponse) map[string]protocol.MatchResult {
	t.Helper()
	out := map[string]protocol.MatchResult{}
	for _, r := range resp.Results {
		out[r.ID] = r
	}
	return out
}

// objectName reads metadata.name out of a marshaled match.
func objectName(t *testing.T, m protocol.MatchedObject) string {
	t.Helper()
	var obj map[string]any
	require.NoError(t, json.Unmarshal(m.Object, &obj))
	meta, _ := obj["metadata"].(map[string]any)
	name, _ := meta["name"].(string)
	return name
}

func ownerRefQuery(id string, gvr schema.GroupVersionResource, kind string, parents ...protocol.ParentRef) protocol.MatchQuery {
	return protocol.MatchQuery{
		ID:      id,
		Matcher: protocol.MatcherOwnerRef,
		Parents: parents,
		Child: protocol.ChildKind{
			Group:    gvr.Group,
			Version:  gvr.Version,
			Kind:     kind,
			Resource: gvr.Resource,
		},
	}
}

func labelSelectorQuery(t *testing.T, id string, criteria protocol.LabelSelectorCriteria, parents ...protocol.ParentRef) protocol.MatchQuery {
	t.Helper()
	raw, err := json.Marshal(criteria)
	require.NoError(t, err)
	q := ownerRefQuery(id, podsGVR, "Pod", parents...)
	q.Matcher = protocol.MatcherLabelSelector
	q.Criteria = raw
	return q
}

func TestHandle_OwnerRefMatch(t *testing.T) {
	owned := newReplicaSet("app-ns", "rs-owned", "rs-1", ownerRef{uid: "dep-1", controller: true})
	other := newReplicaSet("app-ns", "rs-other", "rs-2", ownerRef{uid: "dep-9", controller: true})

	h, _ := newTestHandler(t, owned, other)

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			ownerRefQuery("q1", replicaSetsGVR, "ReplicaSet", protocol.ParentRef{UID: "dep-1", Namespace: "app-ns", Name: "dep"}),
		},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	require.Nil(t, res.Error)
	require.Len(t, res.Matches, 1)
	assert.Equal(t, []string{"dep-1"}, res.Matches[0].ParentUIDs)
	assert.Equal(t, "rs-owned", objectName(t, res.Matches[0]))
}

// TestHandle_PaginatesListPages proves the handler follows the continue token.
// Dropping it would return only the first page — a silently partial tree with
// no Truncated flag, which is the failure this design exists to prevent.
func TestHandle_PaginatesListPages(t *testing.T) {
	firstPage := newPod("app-ns", "pod-a", "pod-1", ownerRef{uid: "rs-a"})
	secondPage := newPod("app-ns", "pod-b", "pod-2", ownerRef{uid: "rs-a"})

	dyn := newFakeDyn(t)
	dyn.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		la, ok := action.(k8stesting.ListActionImpl)
		if !ok {
			return false, nil, nil
		}
		list := &unstructured.UnstructuredList{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "PodList",
		}}
		if la.ListOptions.Continue == "" {
			list.Items = []unstructured.Unstructured{*firstPage.DeepCopy()}
			list.SetContinue("page-2")
		} else {
			list.Items = []unstructured.Unstructured{*secondPage.DeepCopy()}
		}
		return true, list, nil
	})
	rec := recordLists(dyn)
	h := newResourceTreeHandler(dyn, testLogger())

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			ownerRefQuery("q1", podsGVR, "Pod", protocol.ParentRef{UID: "rs-a", Namespace: "app-ns", Name: "rs-a"}),
		},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	require.Nil(t, res.Error)
	assert.False(t, res.Truncated)
	require.Len(t, res.Matches, 2, "both pages must be walked")

	names := []string{objectName(t, res.Matches[0]), objectName(t, res.Matches[1])}
	assert.ElementsMatch(t, []string{"pod-a", "pod-b"}, names)

	calls := rec.resourceCalls("pods")
	require.Len(t, calls, 2, "one list per page")
	assert.Equal(t, "", calls[0].continueToken)
	assert.Equal(t, "page-2", calls[1].continueToken, "the second list must carry the continue token")
	assert.Equal(t, int64(protocol.ListPageSize), calls[1].limit)
}

// TestHandle_PageCapStopsEndlessPagination pins the page budget. A server that
// keeps handing back a continue token must leave the query Truncated rather
// than paging on: Handle runs a batch sequentially under one context, so an
// unbounded walk would burn the deadline and take every later query with it.
//
// The reactor stops answering well past the cap so a regression fails this test
// instead of hanging the suite.
func TestHandle_PageCapStopsEndlessPagination(t *testing.T) {
	const reactorHardStop = maxContinuationPagesPerQuery * 4

	var listCount int
	dyn := newFakeDyn(t)
	dyn.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if _, ok := action.(k8stesting.ListActionImpl); !ok {
			return false, nil, nil
		}
		listCount++
		if listCount > reactorHardStop {
			return true, nil, fmt.Errorf("handler paged %d times without stopping; the page cap is not enforced", listCount)
		}
		// Always claim there is another page, and never return an object, so
		// nothing but the page budget can end this walk.
		list := &unstructured.UnstructuredList{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "PodList",
		}}
		list.SetContinue(fmt.Sprintf("page-%d", listCount+1))
		return true, list, nil
	})
	rec := recordLists(dyn)
	h := newResourceTreeHandler(dyn, testLogger())

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			ownerRefQuery("q1", podsGVR, "Pod", protocol.ParentRef{UID: "rs-a", Namespace: "app-ns", Name: "rs-a"}),
		},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	assert.True(t, res.Truncated)
	require.NotNil(t, res.Error)
	assert.Equal(t, protocol.CodeLimitExceeded, res.Error.Code)
	assert.Contains(t, res.Error.Message, "continuation page limit")

	// The free first page plus exactly the budgeted continuations.
	assert.Len(t, rec.resourceCalls("pods"), maxContinuationPagesPerQuery+1)
}

// TestHandle_PageBudgetIsSharedAcrossNamespaces proves the budget belongs to
// the query, not to each namespace walk: pages spent in one namespace are gone
// from the next, so a query cannot multiply the cap by its namespace count.
//
// The first namespace pages a few times and finishes normally; the second pages
// forever. A per-namespace budget would give the second the full cap, so the
// assertion is on what the second namespace got.
func TestHandle_PageBudgetIsSharedAcrossNamespaces(t *testing.T) {
	const (
		spentInFirstNamespace = 5
		reactorHardStop       = maxContinuationPagesPerQuery * 4
	)

	perNamespace := map[string]int{}
	dyn := newFakeDyn(t)
	dyn.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		la, ok := action.(k8stesting.ListActionImpl)
		if !ok {
			return false, nil, nil
		}
		perNamespace[la.Namespace]++
		count := perNamespace[la.Namespace]
		if count > reactorHardStop {
			return true, nil, fmt.Errorf("namespace %q paged %d times without stopping; the page budget is not shared",
				la.Namespace, count)
		}

		list := &unstructured.UnstructuredList{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "PodList",
		}}
		// ns-a stops on its own after spending part of the budget; ns-b never
		// stops, so only the budget can end it.
		if la.Namespace != "ns-a" || count <= spentInFirstNamespace {
			list.SetContinue(fmt.Sprintf("page-%d", count+1))
		}
		return true, list, nil
	})
	rec := recordLists(dyn)
	h := newResourceTreeHandler(dyn, testLogger())

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			ownerRefQuery("q1", podsGVR, "Pod",
				protocol.ParentRef{UID: "rs-a", Namespace: "ns-a", Name: "rs-a"},
				protocol.ParentRef{UID: "rs-b", Namespace: "ns-b", Name: "rs-b"},
			),
		},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	assert.True(t, res.Truncated)
	require.NotNil(t, res.Error)
	assert.Equal(t, protocol.CodeLimitExceeded, res.Error.Code)

	namespaceCalls := map[string]int{}
	for _, c := range rec.resourceCalls("pods") {
		namespaceCalls[c.namespace]++
	}

	// ns-a: its free first page plus the continuations it spent.
	assert.Equal(t, 1+spentInFirstNamespace, namespaceCalls["ns-a"])
	// ns-b: its own free first page plus only what ns-a left behind.
	assert.Equal(t, 1+maxContinuationPagesPerQuery-spentInFirstNamespace, namespaceCalls["ns-b"],
		"the second namespace must inherit the reduced budget, not a fresh one")
}

// TestHandle_FirstPagePerNamespaceIsFree proves a wide, shallow fan-out — the
// common shape, many namespaces of one page each — never trips the cap.
func TestHandle_FirstPagePerNamespaceIsFree(t *testing.T) {
	parents := make([]protocol.ParentRef, 0, maxContinuationPagesPerQuery+8)
	objs := make([]runtime.Object, 0, len(parents))
	for i := 0; i < maxContinuationPagesPerQuery+8; i++ {
		ns := fmt.Sprintf("ns-%d", i)
		uid := fmt.Sprintf("rs-%d", i)
		parents = append(parents, protocol.ParentRef{UID: uid, Namespace: ns, Name: uid})
		objs = append(objs, newPod(ns, fmt.Sprintf("pod-%d", i), fmt.Sprintf("pod-uid-%d", i), ownerRef{uid: uid}))
	}

	h, rec := newTestHandler(t, objs...)

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{ownerRefQuery("q1", podsGVR, "Pod", parents...)},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	require.Nil(t, res.Error, "single-page namespaces must not spend the page budget")
	assert.False(t, res.Truncated)
	assert.Len(t, res.Matches, len(parents))
	assert.Len(t, rec.resourceCalls("pods"), len(parents))
}

func TestHandle_ListCallBudgetTruncatesWideFanoutWithoutStarvingSibling(t *testing.T) {
	child := newPod("search-a", "child-a", "child-1")
	dyn := newFakeDyn(t)
	dyn.PrependReactor("list", "pods", selectorStampingReaction(child))
	rec := recordLists(dyn)
	h := newResourceTreeHandler(dyn, testLogger())
	h.maxListCalls = 3

	wide := labelSelectorQuery(t, "wide", protocol.LabelSelectorCriteria{
		MatchLabels: map[string]string{"owner": protocol.TokenParentName},
		Namespaces:  []string{"search-a", "search-b"},
	},
		protocol.ParentRef{UID: "parent-a", Name: "parent-a"},
		protocol.ParentRef{UID: "parent-b", Name: "parent-b"},
	)
	sibling := ownerRefQuery("sibling", podsGVR, "Pod",
		protocol.ParentRef{UID: "owner", Namespace: "sibling-ns", Name: "owner"})

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{wide, sibling},
	}))

	results := resultsByID(t, decodeMatchResponse(t, resp))
	assert.True(t, results["wide"].Truncated)
	require.NotNil(t, results["wide"].Error)
	assert.Equal(t, protocol.CodeLimitExceeded, results["wide"].Error.Code)
	assert.Contains(t, results["wide"].Error.Message, "list call limit")
	require.Nil(t, results["sibling"].Error)
	assert.False(t, results["sibling"].Truncated)
	assert.Len(t, rec.resourceCalls("pods"), 4,
		"wide query spends exactly three calls and the sibling gets its independent budget")
}

func TestHandle_MultipleParentsOneQuery(t *testing.T) {
	podA := newPod("app-ns", "pod-a", "pod-1", ownerRef{uid: "rs-a", controller: true})
	podB := newPod("app-ns", "pod-b", "pod-2", ownerRef{uid: "rs-b", controller: true})
	podC := newPod("app-ns", "pod-c", "pod-3", ownerRef{uid: "rs-a", controller: true})

	h, _ := newTestHandler(t, podA, podB, podC)

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			ownerRefQuery("q1", podsGVR, "Pod",
				protocol.ParentRef{UID: "rs-a", Namespace: "app-ns", Name: "rs-a"},
				protocol.ParentRef{UID: "rs-b", Namespace: "app-ns", Name: "rs-b"},
			),
		},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	require.Nil(t, res.Error)
	require.Len(t, res.Matches, 3)

	byName := map[string][]string{}
	for _, m := range res.Matches {
		byName[objectName(t, m)] = m.ParentUIDs
	}
	assert.Equal(t, []string{"rs-a"}, byName["pod-a"])
	assert.Equal(t, []string{"rs-b"}, byName["pod-b"])
	assert.Equal(t, []string{"rs-a"}, byName["pod-c"])
}

func TestHandle_NonControllerOwnerRefStillMatches(t *testing.T) {
	// Parity with the API's hasOwnerReference, which ignores `controller:`.
	pod := newPod("app-ns", "pod-a", "pod-1", ownerRef{uid: "rs-a", controller: false})

	h, _ := newTestHandler(t, pod)

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			ownerRefQuery("q1", podsGVR, "Pod", protocol.ParentRef{UID: "rs-a", Namespace: "app-ns", Name: "rs-a"}),
		},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	require.Nil(t, res.Error)
	require.Len(t, res.Matches, 1)
	assert.Equal(t, []string{"rs-a"}, res.Matches[0].ParentUIDs)
}

func TestHandle_MetadataOnlyStripsObject(t *testing.T) {
	secret := newObject("v1", "Secret", "app-ns", "sec-a", "sec-1", ownerRef{uid: "rel-1"})
	secret.Object["data"] = map[string]any{"password": "c3VwZXItc2VjcmV0"}
	meta, _ := secret.Object["metadata"].(map[string]any)
	meta["managedFields"] = []any{map[string]any{"manager": "kubectl"}}

	h, _ := newTestHandler(t, secret)

	q := ownerRefQuery("q1", secretsGVR, "Secret", protocol.ParentRef{UID: "rel-1", Namespace: "app-ns", Name: "rel"})
	q.MetadataOnly = true

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{q},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	require.Nil(t, res.Error)
	require.Len(t, res.Matches, 1)

	var obj map[string]any
	require.NoError(t, json.Unmarshal(res.Matches[0].Object, &obj))

	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{"apiVersion", "kind", "metadata"}, keys)
	assert.Equal(t, "v1", obj["apiVersion"])
	assert.Equal(t, "Secret", obj["kind"])

	gotMeta, ok := obj["metadata"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, gotMeta, "managedFields")
	assert.Equal(t, "sec-a", gotMeta["name"])
}

// TestHandle_StripsManagedFieldsOnFullObject covers the always-on strip on its
// own. The metadataOnly test cannot: its rebuild drops managedFields anyway and
// would mask a regression in the unconditional delete.
func TestHandle_StripsManagedFieldsOnFullObject(t *testing.T) {
	secret := newObject("v1", "Secret", "app-ns", "sec-a", "sec-1", ownerRef{uid: "rel-1"})
	secret.Object["data"] = map[string]any{"password": "c3VwZXItc2VjcmV0"}
	meta, _ := secret.Object["metadata"].(map[string]any)
	meta["managedFields"] = []any{map[string]any{"manager": "kubectl"}}

	h, _ := newTestHandler(t, secret)

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			ownerRefQuery("q1", secretsGVR, "Secret", protocol.ParentRef{UID: "rel-1", Namespace: "app-ns", Name: "rel"}),
		},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	require.Nil(t, res.Error)
	require.Len(t, res.Matches, 1)

	var obj map[string]any
	require.NoError(t, json.Unmarshal(res.Matches[0].Object, &obj))

	gotMeta, ok := obj["metadata"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, gotMeta, "managedFields", "managedFields must be stripped without metadataOnly too")
	assert.NotContains(t, obj, "data", "core Secret payloads are stripped even for full-object queries")
}

func TestHandle_FullSecretStripsSensitiveFieldsAndLastApplied(t *testing.T) {
	secret := newObject("v1", "Secret", "app-ns", "sec-a", "sec-1", ownerRef{uid: "rel-1"})
	secret.Object["type"] = "kubernetes.io/basic-auth"
	secret.Object["data"] = map[string]any{"password": "encoded-secret"}
	secret.Object["stringData"] = map[string]any{"username": "plain-secret"}
	secret.Object["immutable"] = true
	meta := secret.Object["metadata"].(map[string]any)
	meta["annotations"] = map[string]any{
		protocol.LastAppliedConfigAnnotation: `{"data":{"password":"encoded-secret"}}`,
		"openchoreo.dev/keep":                "kept",
	}

	nonSecret := newPod("app-ns", "pod-a", "pod-1", ownerRef{uid: "rel-1"})
	nonSecret.Object["data"] = map[string]any{"safe": "value"}
	h, _ := newTestHandler(t, secret, nonSecret)

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			ownerRefQuery("secret", secretsGVR, "Secret", protocol.ParentRef{UID: "rel-1", Namespace: "app-ns", Name: "rel"}),
			ownerRefQuery("pod", podsGVR, "Pod", protocol.ParentRef{UID: "rel-1", Namespace: "app-ns", Name: "rel"}),
		},
	}))

	results := resultsByID(t, decodeMatchResponse(t, resp))
	var gotSecret map[string]any
	require.NoError(t, json.Unmarshal(results["secret"].Matches[0].Object, &gotSecret))
	assert.NotContains(t, gotSecret, "data")
	assert.NotContains(t, gotSecret, "stringData")
	assert.Equal(t, "kubernetes.io/basic-auth", gotSecret["type"])
	assert.Equal(t, true, gotSecret["immutable"])
	annotations := gotSecret["metadata"].(map[string]any)["annotations"].(map[string]any)
	assert.NotContains(t, annotations, protocol.LastAppliedConfigAnnotation)
	assert.Equal(t, "kept", annotations["openchoreo.dev/keep"])
	assert.NotContains(t, string(results["secret"].Matches[0].Object), "encoded-secret")

	var gotPod map[string]any
	require.NoError(t, json.Unmarshal(results["pod"].Matches[0].Object, &gotPod))
	assert.Equal(t, map[string]any{"safe": "value"}, gotPod["data"],
		"data is sensitive only for core Secrets")
}

func TestHandle_FullSecretStripUsesQueriedGVRNotKindSpelling(t *testing.T) {
	secret := newObject("v1", "not-a-secret", "app-ns", "sec-a", "sec-1", ownerRef{uid: "rel-1"})
	secret.Object["data"] = map[string]any{"password": "encoded-secret"}

	got := shapeObject(secret, protocol.ChildKind{
		Version: "v1", Kind: "also-not-a-secret", Resource: "secrets",
	}, false)
	assert.NotContains(t, got, "data")
}

// TestHandle_StripsLastAppliedConfigAnnotation pins the other half of the
// metadataOnly guarantee. Rebuilding the object around its metadata map is not
// enough on its own: kubectl's last-applied annotation holds the whole
// serialized resource, so an applied Secret would ship its own data block inside
// the metadata the trim keeps.
func TestHandle_StripsLastAppliedConfigAnnotation(t *testing.T) {
	const lastApplied = `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"sec-a","namespace":"app-ns"},` +
		`"data":{"password":"c3VwZXItc2VjcmV0"}}`

	secret := newObject("v1", "Secret", "app-ns", "sec-a", "sec-1", ownerRef{uid: "rel-1"})
	secret.Object["data"] = map[string]any{"password": "c3VwZXItc2VjcmV0"}
	meta, _ := secret.Object["metadata"].(map[string]any)
	meta["annotations"] = map[string]any{
		protocol.LastAppliedConfigAnnotation: lastApplied,
		"openchoreo.dev/keep":                "displayed",
	}

	h, _ := newTestHandler(t, secret)

	q := ownerRefQuery("q1", secretsGVR, "Secret", protocol.ParentRef{UID: "rel-1", Namespace: "app-ns", Name: "rel"})
	q.MetadataOnly = true

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{q},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	require.Nil(t, res.Error)
	require.Len(t, res.Matches, 1)

	// The raw bytes are the real assertion: whatever the shape, the secret value
	// must not be anywhere in what crosses the tunnel.
	assert.NotContains(t, string(res.Matches[0].Object), "c3VwZXItc2VjcmV0",
		"the secret value must not survive inside an annotation")

	var obj map[string]any
	require.NoError(t, json.Unmarshal(res.Matches[0].Object, &obj))
	gotMeta, ok := obj["metadata"].(map[string]any)
	require.True(t, ok)
	annotations, ok := gotMeta["annotations"].(map[string]any)
	require.True(t, ok, "the annotations map itself must survive")
	assert.NotContains(t, annotations, protocol.LastAppliedConfigAnnotation)
	assert.Equal(t, "displayed", annotations["openchoreo.dev/keep"],
		"only the one annotation is dropped, not every annotation")
}

func TestHandle_ForbiddenClassified(t *testing.T) {
	pod := newPod("app-ns", "pod-a", "pod-1", ownerRef{uid: "rs-a"})

	dyn := newFakeDyn(t, pod)
	dyn.PrependReactor("list", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "", fmt.Errorf("nope"))
	})
	h := newResourceTreeHandler(dyn, testLogger())

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			ownerRefQuery("denied", secretsGVR, "Secret", protocol.ParentRef{UID: "rel-1", Namespace: "app-ns", Name: "rel"}),
			ownerRefQuery("ok", podsGVR, "Pod", protocol.ParentRef{UID: "rs-a", Namespace: "app-ns", Name: "rs-a"}),
		},
	}))

	results := resultsByID(t, decodeMatchResponse(t, resp))
	require.NotNil(t, results["denied"].Error)
	assert.Equal(t, protocol.CodeForbidden, results["denied"].Error.Code)
	assert.Empty(t, results["denied"].Matches)

	require.Nil(t, results["ok"].Error)
	assert.Len(t, results["ok"].Matches, 1)
}

func TestHandle_UnknownMatcherFailsClosed(t *testing.T) {
	pod := newPod("app-ns", "pod-a", "pod-1", ownerRef{uid: "rs-a"})
	h, rec := newTestHandler(t, pod)

	q := ownerRefQuery("q1", podsGVR, "Pod", protocol.ParentRef{UID: "rs-a", Namespace: "app-ns", Name: "rs-a"})
	q.Matcher = "objectRef"

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{q},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	require.NotNil(t, res.Error)
	assert.Equal(t, protocol.CodeUnsupportedMatcher, res.Error.Code)
	assert.Empty(t, res.Matches)
	assert.Empty(t, rec.snapshot(), "unknown matcher must not list anything")
}

func TestHandle_LabelSelectorMatch(t *testing.T) {
	child := newPod("app-ns", "child-a", "child-1")

	h, rec := newSelectorHandler(t, child)

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			labelSelectorQuery(t, "q1",
				protocol.LabelSelectorCriteria{MatchLabels: map[string]string{"owner": protocol.TokenParentName}},
				protocol.ParentRef{UID: "gw-uid", Namespace: "app-ns", Name: "gw-a"},
			),
		},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	require.Nil(t, res.Error)
	require.Len(t, res.Matches, 1)
	assert.Equal(t, []string{"gw-uid"}, res.Matches[0].ParentUIDs)

	calls := rec.resourceCalls("pods")
	require.Len(t, calls, 1)
	assert.Equal(t, "app-ns", calls[0].namespace)
	assert.Equal(t, "owner=gw-a", calls[0].labelSelector)
}

func TestHandle_LabelSelectorCrossNamespace(t *testing.T) {
	child := newPod("infra-ns", "child-a", "child-1")

	h, rec := newSelectorHandler(t, child)

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			labelSelectorQuery(t, "q1",
				protocol.LabelSelectorCriteria{
					MatchLabels: map[string]string{"owner": protocol.TokenParentName},
					Namespaces:  []string{"infra-ns"},
				},
				protocol.ParentRef{UID: "gw-uid", Namespace: "app-ns", Name: "gw-a"},
			),
		},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	require.Nil(t, res.Error)
	require.Len(t, res.Matches, 1)
	assert.Equal(t, []string{"infra-ns"}, rec.namespaces(), "parent's own namespace must not be listed")
}

func TestHandle_LabelSelectorMultipleParents(t *testing.T) {
	shared := newPod("app-ns", "shared", "shared-uid")

	h, rec := newSelectorHandler(t, shared)

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			labelSelectorQuery(t, "q1",
				protocol.LabelSelectorCriteria{MatchLabels: map[string]string{"owner": protocol.TokenParentName}},
				protocol.ParentRef{UID: "gw-a-uid", Namespace: "app-ns", Name: "gw-a"},
				protocol.ParentRef{UID: "gw-b-uid", Namespace: "app-ns", Name: "gw-b"},
			),
		},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	require.Nil(t, res.Error)
	require.Len(t, res.Matches, 1, "the shared child must collapse into one match")
	assert.Equal(t, []string{"gw-a-uid", "gw-b-uid"}, res.Matches[0].ParentUIDs)
	assert.ElementsMatch(t, []string{"owner=gw-a", "owner=gw-b"}, rec.selectors())
}

func TestHandle_LabelSelectorNamespaceToken(t *testing.T) {
	child := newPod("app-ns", "child-a", "child-1")

	h, rec := newSelectorHandler(t, child)

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			labelSelectorQuery(t, "q1",
				protocol.LabelSelectorCriteria{MatchLabels: map[string]string{"scope": protocol.TokenParentNamespace}},
				protocol.ParentRef{UID: "gw-uid", Namespace: "app-ns", Name: "gw-a"},
			),
		},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	require.Nil(t, res.Error)
	assert.Len(t, res.Matches, 1)
	assert.Equal(t, []string{"scope=app-ns"}, rec.selectors())
}

func TestHandle_LabelSelectorInvalidCriteria(t *testing.T) {
	tooManyNamespaces := make([]string, 0, protocol.MaxSelectorNamespaces+1)
	for i := 0; i <= protocol.MaxSelectorNamespaces; i++ {
		tooManyNamespaces = append(tooManyNamespaces, fmt.Sprintf("ns-%d", i))
	}

	tests := []struct {
		name     string
		criteria json.RawMessage
		// parents overrides the default single in-namespace parent.
		parents []protocol.ParentRef
		// wantMessage, when set, must appear in the error message.
		wantMessage string
	}{
		{name: "missing criteria", criteria: nil},
		{name: "undecodable criteria", criteria: json.RawMessage(`[1,2]`)},
		{name: "unknown criteria field", criteria: json.RawMessage(`{"matchLabels":{"owner":"${parent.metadata.name}"},"matchExpressions":[]}`), wantMessage: "unknown field"},
		{name: "empty matchLabels", criteria: mustJSON(t, protocol.LabelSelectorCriteria{MatchLabels: map[string]string{}})},
		{
			name:     "unknown token in value",
			criteria: mustJSON(t, protocol.LabelSelectorCriteria{MatchLabels: map[string]string{"owner": "${parent.spec.foo}"}}),
		},
		{
			name: "token in matchLabels key",
			criteria: mustJSON(t, protocol.LabelSelectorCriteria{
				MatchLabels: map[string]string{protocol.TokenParentName: "x"},
			}),
		},
		{
			name: "wildcard namespace",
			criteria: mustJSON(t, protocol.LabelSelectorCriteria{
				MatchLabels: map[string]string{"owner": protocol.TokenParentName},
				Namespaces:  []string{"*"},
			}),
		},
		{
			name: "empty namespace entry",
			criteria: mustJSON(t, protocol.LabelSelectorCriteria{
				MatchLabels: map[string]string{"owner": protocol.TokenParentName},
				Namespaces:  []string{""},
			}),
		},
		{
			name: "token in namespace entry",
			criteria: mustJSON(t, protocol.LabelSelectorCriteria{
				MatchLabels: map[string]string{"owner": protocol.TokenParentName},
				Namespaces:  []string{protocol.TokenParentNamespace},
			}),
		},
		{
			name: "too many namespaces",
			criteria: mustJSON(t, protocol.LabelSelectorCriteria{
				MatchLabels: map[string]string{"owner": protocol.TokenParentName},
				Namespaces:  tooManyNamespaces,
			}),
		},
		{
			// A namespace-less parent with no criteria.namespaces would widen
			// the search to every namespace, so it is rejected rather than run.
			name: "namespace-less parent without criteria namespaces",
			criteria: mustJSON(t, protocol.LabelSelectorCriteria{
				MatchLabels: map[string]string{"owner": protocol.TokenParentName},
			}),
			parents: []protocol.ParentRef{{UID: "gw-uid", Name: "gw-a"}},
		},
		{
			name: "literal value that cannot be a label value",
			criteria: mustJSON(t, protocol.LabelSelectorCriteria{
				MatchLabels: map[string]string{"owner": "Bad Value!"},
			}),
		},
		{
			// Every value is static, so one fixed selector runs per parent and
			// attributes the same objects to all of them. Config validation
			// forbids it, but criteria arrive over the wire.
			name: "no value derived from the parent",
			criteria: mustJSON(t, protocol.LabelSelectorCriteria{
				MatchLabels: map[string]string{"app": "static"},
			}),
			wantMessage: "derive at least one value from the parent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sibling := newPod("app-ns", "pod-a", "pod-1", ownerRef{uid: "rs-a"})
			h, rec := newTestHandler(t, sibling)

			bad := labelSelectorQuery(t, "bad", protocol.LabelSelectorCriteria{})
			bad.Criteria = tt.criteria
			bad.Child = protocol.ChildKind{Version: "v1", Kind: "Secret", Resource: "secrets"}
			bad.Parents = tt.parents
			if len(bad.Parents) == 0 {
				bad.Parents = []protocol.ParentRef{{UID: "gw-uid", Namespace: "app-ns", Name: "gw-a"}}
			}

			resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
				Version: protocol.Version,
				Queries: []protocol.MatchQuery{
					bad,
					ownerRefQuery("ok", podsGVR, "Pod", protocol.ParentRef{UID: "rs-a", Namespace: "app-ns", Name: "rs-a"}),
				},
			}))

			results := resultsByID(t, decodeMatchResponse(t, resp))
			require.NotNil(t, results["bad"].Error)
			assert.Equal(t, protocol.CodeInvalidQuery, results["bad"].Error.Code)
			if tt.wantMessage != "" {
				assert.Contains(t, results["bad"].Error.Message, tt.wantMessage)
			}
			assert.Empty(t, results["bad"].Matches)
			assert.Empty(t, rec.resourceCalls("secrets"), "invalid criteria must not list")

			require.Nil(t, results["ok"].Error)
			assert.Len(t, results["ok"].Matches, 1, "sibling query must still succeed")
		})
	}
}

func TestHandle_LabelSelectorOversizeSubstitution(t *testing.T) {
	oversize := strings.Repeat("a", 70) // label values cap at 63 characters
	child := newPod("app-ns", "child-a", "child-1")

	h, rec := newSelectorHandler(t, child)

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			labelSelectorQuery(t, "q1",
				protocol.LabelSelectorCriteria{MatchLabels: map[string]string{"owner": protocol.TokenParentName}},
				protocol.ParentRef{UID: "big-uid", Namespace: "app-ns", Name: oversize},
				protocol.ParentRef{UID: "gw-uid", Namespace: "app-ns", Name: "gw-a"},
			),
		},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	require.Nil(t, res.Error, "an unmatchable parent is not an error")
	require.Len(t, res.Matches, 1)
	assert.Equal(t, []string{"gw-uid"}, res.Matches[0].ParentUIDs)
	assert.Equal(t, []string{"owner=gw-a"}, rec.selectors(), "no list for the oversize parent")
}

// TestHandle_LabelSelectorEmptyParentNameSkipped pins the other half of the
// parent-derived invariant. An empty name substitutes to "", a LEGAL empty
// label value, so the selector still runs and quietly claims every object whose
// label value is empty for that parent. Skipping is the true answer; the
// siblings still match.
func TestHandle_LabelSelectorEmptyParentNameSkipped(t *testing.T) {
	child := newPod("app-ns", "child-a", "child-1")

	h, rec := newSelectorHandler(t, child)

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			labelSelectorQuery(t, "q1",
				protocol.LabelSelectorCriteria{MatchLabels: map[string]string{"owner": protocol.TokenParentName}},
				protocol.ParentRef{UID: "real-uid", Namespace: "app-ns", Name: "real"},
				protocol.ParentRef{UID: "nameless-uid", Namespace: "app-ns"},
			),
		},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	require.Nil(t, res.Error, "a skipped parent is not an error")
	require.Len(t, res.Matches, 1)
	assert.Equal(t, []string{"real-uid"}, res.Matches[0].ParentUIDs,
		"the nameless parent must not be credited with the child")
	assert.Equal(t, []string{"owner=real"}, rec.selectors(), "no list for the nameless parent")
}

// TestHandle_LabelSelectorEmptyParentNamespaceSkipped is the namespace half of
// the same skip. criteria.namespaces makes a namespace-less parent legal, so
// this is reachable without a malformed query.
func TestHandle_LabelSelectorEmptyParentNamespaceSkipped(t *testing.T) {
	child := newPod("fixed-ns", "child-a", "child-1")

	h, rec := newSelectorHandler(t, child)

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			labelSelectorQuery(t, "q1",
				protocol.LabelSelectorCriteria{
					MatchLabels: map[string]string{"ns": protocol.TokenParentNamespace},
					Namespaces:  []string{"fixed-ns"},
				},
				protocol.ParentRef{UID: "gw-uid", Name: "gw-a"},
			),
		},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	require.Nil(t, res.Error, "a skipped parent is not an error")
	assert.Empty(t, res.Matches, "an empty namespace must not match objects whose ns label is empty")
	assert.Empty(t, rec.selectors(), "no list for the namespace-less parent")
}

func TestHandle_ClusterScopedParentWithExplicitNamespacesAndNameToken(t *testing.T) {
	child := newPod("fixed-ns", "child-a", "child-1")
	h, rec := newSelectorHandler(t, child)

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			labelSelectorQuery(t, "q1", protocol.LabelSelectorCriteria{
				MatchLabels: map[string]string{"owner": protocol.TokenParentName},
				Namespaces:  []string{"fixed-ns"},
			}, protocol.ParentRef{UID: "cluster-uid", Name: "cluster-parent"}),
		},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	require.Nil(t, res.Error)
	require.Len(t, res.Matches, 1)
	assert.Equal(t, []string{"cluster-uid"}, res.Matches[0].ParentUIDs)
	assert.Equal(t, []string{"fixed-ns"}, rec.namespaces())
	assert.Equal(t, []string{"owner=cluster-parent"}, rec.selectors())
}

func TestHandle_UnknownVersion(t *testing.T) {
	pod := newPod("app-ns", "pod-a", "pod-1", ownerRef{uid: "rs-a"})
	h, rec := newTestHandler(t, pod)

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: "v2",
		Queries: []protocol.MatchQuery{
			ownerRefQuery("q1", podsGVR, "Pod", protocol.ParentRef{UID: "rs-a", Namespace: "app-ns", Name: "rs-a"}),
			ownerRefQuery("q2", podsGVR, "Pod", protocol.ParentRef{UID: "rs-b", Namespace: "app-ns", Name: "rs-b"}),
		},
	}))

	decoded := decodeMatchResponse(t, resp)
	require.Len(t, decoded.Results, 2)
	for _, res := range decoded.Results {
		require.NotNil(t, res.Error)
		assert.Equal(t, protocol.CodeUnsupportedVersion, res.Error.Code)
	}
	assert.Empty(t, rec.snapshot(), "unknown version must not list anything")
}

func TestHandle_LimitTruncates(t *testing.T) {
	objs := []runtime.Object{
		newPod("app-ns", "pod-a", "pod-1", ownerRef{uid: "rs-a"}),
		newPod("app-ns", "pod-b", "pod-2", ownerRef{uid: "rs-a"}),
		newPod("app-ns", "pod-c", "pod-3", ownerRef{uid: "rs-a"}),
	}
	h, _ := newTestHandler(t, objs...)

	q := ownerRefQuery("q1", podsGVR, "Pod", protocol.ParentRef{UID: "rs-a", Namespace: "app-ns", Name: "rs-a"})
	q.Limit = 2

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{q},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	assert.Len(t, res.Matches, 2)
	assert.True(t, res.Truncated)
	require.NotNil(t, res.Error)
	assert.Equal(t, protocol.CodeLimitExceeded, res.Error.Code)
}

func TestHandle_ByteBudgetTruncates(t *testing.T) {
	objs := []runtime.Object{
		newPod("app-ns", "pod-a", "pod-1", ownerRef{uid: "rs-a"}),
		newPod("app-ns", "pod-b", "pod-2", ownerRef{uid: "rs-a"}),
		newPod("app-ns", "pod-c", "pod-3", ownerRef{uid: "rs-a"}),
	}
	h, _ := newTestHandler(t, objs...)
	h.maxResponseBytes = 300 // one small pod fits, the rest do not

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			ownerRefQuery("q1", podsGVR, "Pod", protocol.ParentRef{UID: "rs-a", Namespace: "app-ns", Name: "rs-a"}),
		},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	assert.Less(t, len(res.Matches), 3)
	assert.True(t, res.Truncated)
	require.NotNil(t, res.Error)
	assert.Equal(t, protocol.CodeLimitExceeded, res.Error.Code)
}

// TestHandle_ByteBudgetChargesParentUIDs proves the response budget counts the
// parent UID arrays, not just object bodies. One child matched by many parents
// accumulates one UID per parent through the dedupe merge; if those UIDs were
// free, a high-fan-in match could push the marshaled response past the gateway's
// hard limit while the byte budget looked untouched (finding I1).
func TestHandle_ByteBudgetChargesParentUIDs(t *testing.T) {
	child := newPod("app-ns", "shared", "shared-uid")
	h, _ := newSelectorHandler(t, child)

	const parents = 60
	refs := make([]protocol.ParentRef, 0, parents)
	for i := 0; i < parents; i++ {
		name := fmt.Sprintf("parent-%03d-with-a-long-enough-uid-value", i)
		refs = append(refs, protocol.ParentRef{UID: name, Name: name})
	}
	q := labelSelectorQuery(t, "q1", protocol.LabelSelectorCriteria{
		MatchLabels: map[string]string{"owner": protocol.TokenParentName},
		Namespaces:  []string{"app-ns"},
	}, refs...)
	q.MetadataOnly = true // shrink the body so the UID arrays dominate the budget

	h.maxResponseBytes = 700

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{q},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	require.Len(t, res.Matches, 1, "the one shared child is still returned")
	assert.Less(t, len(res.Matches[0].ParentUIDs), parents,
		"parent UIDs must be charged, so a high-fan-in match truncates before attaching them all")
	assert.True(t, res.Truncated)
	require.NotNil(t, res.Error)
	assert.Equal(t, protocol.CodeLimitExceeded, res.Error.Code)
}

// TestHandle_DeadlinePreservesMatchesAndShortCircuitsSiblings proves wall-clock
// exhaustion returns the matches already collected, marked truncated, instead of
// discarding them as an Internal error, and that a sibling query sharing the
// spent deadline reports truncated too rather than issuing a doomed LIST that
// fails as Internal (finding I5).
func TestHandle_DeadlinePreservesMatchesAndShortCircuitsSiblings(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	child := newPod("app-ns", "pod-a", "pod-1", ownerRef{uid: "owner-a", controller: true})
	dyn := newFakeDyn(t, child)
	var calls int
	dyn.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		calls++
		if calls == 1 {
			// First page collects a match, then the shared deadline is spent.
			list := &unstructured.UnstructuredList{Object: map[string]any{"apiVersion": "v1", "kind": "PodList"}}
			list.SetContinue("next-page")
			list.Items = append(list.Items, *child.DeepCopy())
			cancel()
			return true, list, nil
		}
		return true, nil, context.DeadlineExceeded
	})
	h := newResourceTreeHandler(dyn, testLogger())

	wide := ownerRefQuery("wide", podsGVR, "Pod",
		protocol.ParentRef{UID: "owner-a", Namespace: "app-ns", Name: "owner-a"})
	sibling := ownerRefQuery("sibling", podsGVR, "Pod",
		protocol.ParentRef{UID: "owner-b", Namespace: "other-ns", Name: "owner-b"})

	resp := h.Handle(ctx, tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{wide, sibling},
	}))

	results := resultsByID(t, decodeMatchResponse(t, resp))

	require.NotNil(t, results["wide"].Error)
	assert.Equal(t, protocol.CodeLimitExceeded, results["wide"].Error.Code)
	assert.True(t, results["wide"].Truncated)
	assert.Len(t, results["wide"].Matches, 1, "the collected match survives the deadline")

	require.NotNil(t, results["sibling"].Error)
	assert.Equal(t, protocol.CodeLimitExceeded, results["sibling"].Error.Code,
		"a sibling sharing the spent deadline truncates rather than failing Internal")
	assert.True(t, results["sibling"].Truncated)
	assert.Empty(t, results["sibling"].Matches)
}

func TestHandle_InvalidQueriesRejected(t *testing.T) {
	secretChild := protocol.ChildKind{Version: "v1", Kind: "Secret", Resource: "secrets"}
	validParent := protocol.ParentRef{UID: "rel-1", Namespace: "app-ns", Name: "rel"}

	tests := []struct {
		name    string
		queries []protocol.MatchQuery
		badIDs  []string
	}{
		{
			name: "duplicate query IDs",
			queries: []protocol.MatchQuery{
				{ID: "dup", Matcher: protocol.MatcherOwnerRef, Child: secretChild, Parents: []protocol.ParentRef{validParent}},
				{ID: "dup", Matcher: protocol.MatcherOwnerRef, Child: secretChild, Parents: []protocol.ParentRef{validParent}},
			},
			badIDs: []string{"dup"},
		},
		{
			name: "missing child resource",
			queries: []protocol.MatchQuery{
				{ID: "bad", Matcher: protocol.MatcherOwnerRef, Child: protocol.ChildKind{Version: "v1", Kind: "Secret"}, Parents: []protocol.ParentRef{validParent}},
			},
			badIDs: []string{"bad"},
		},
		{
			name: "missing child version",
			queries: []protocol.MatchQuery{
				{ID: "bad", Matcher: protocol.MatcherOwnerRef, Child: protocol.ChildKind{Kind: "Secret", Resource: "secrets"}, Parents: []protocol.ParentRef{validParent}},
			},
			badIDs: []string{"bad"},
		},
		{
			name: "missing child kind",
			queries: []protocol.MatchQuery{
				{ID: "bad", Matcher: protocol.MatcherOwnerRef, Child: protocol.ChildKind{Version: "v1", Resource: "secrets"}, Parents: []protocol.ParentRef{validParent}},
			},
			badIDs: []string{"bad"},
		},
		{
			// Results are correlated back by id, so an empty one is rejected
			// even though nothing collides with it.
			name: "empty query id",
			queries: []protocol.MatchQuery{
				{ID: "", Matcher: protocol.MatcherOwnerRef, Child: secretChild, Parents: []protocol.ParentRef{validParent}},
			},
			badIDs: []string{""},
		},
		{
			name: "empty parents",
			queries: []protocol.MatchQuery{
				{ID: "bad", Matcher: protocol.MatcherOwnerRef, Child: secretChild},
			},
			badIDs: []string{"bad"},
		},
		{
			name: "parent with empty UID",
			queries: []protocol.MatchQuery{
				{ID: "bad", Matcher: protocol.MatcherOwnerRef, Child: secretChild, Parents: []protocol.ParentRef{{Namespace: "app-ns", Name: "rel"}}},
			},
			badIDs: []string{"bad"},
		},
		{
			name: "too many parents",
			queries: []protocol.MatchQuery{
				{ID: "bad", Matcher: protocol.MatcherOwnerRef, Child: secretChild, Parents: manyParents(protocol.MaxParentsPerQuery + 1)},
			},
			badIDs: []string{"bad"},
		},
		{
			name: "ownerRef with criteria",
			queries: []protocol.MatchQuery{
				{
					ID:       "bad",
					Matcher:  protocol.MatcherOwnerRef,
					Child:    secretChild,
					Parents:  []protocol.ParentRef{validParent},
					Criteria: json.RawMessage(`{"matchLabels":{"a":"b"}}`),
				},
			},
			badIDs: []string{"bad"},
		},
		{
			// An empty parent namespace reaches Namespace("") and lists the whole
			// cluster. Unlike labelSelector, ownerRef has no criteria.namespaces
			// with which a caller could ask for that deliberately, so it is always
			// a bug. The control plane cannot currently send one, but the agent is
			// reachable through the gateway independently of the control plane.
			name: "ownerRef parent with empty namespace",
			queries: []protocol.MatchQuery{
				{
					ID:      "bad",
					Matcher: protocol.MatcherOwnerRef,
					Child:   secretChild,
					Parents: []protocol.ParentRef{{UID: "rel-uid", Name: "rel"}},
				},
			},
			badIDs: []string{"bad"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sibling := newPod("app-ns", "pod-a", "pod-1", ownerRef{uid: "rs-a"})
			h, rec := newTestHandler(t, sibling)

			queries := append([]protocol.MatchQuery{}, tt.queries...)
			queries = append(queries, ownerRefQuery("ok", podsGVR, "Pod", protocol.ParentRef{UID: "rs-a", Namespace: "app-ns", Name: "rs-a"}))

			resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
				Version: protocol.Version,
				Queries: queries,
			}))

			decoded := decodeMatchResponse(t, resp)
			require.Len(t, decoded.Results, len(queries))

			badSet := map[string]bool{}
			for _, id := range tt.badIDs {
				badSet[id] = true
			}
			for _, res := range decoded.Results {
				if badSet[res.ID] {
					require.NotNil(t, res.Error, "query %s should have failed", res.ID)
					assert.Equal(t, protocol.CodeInvalidQuery, res.Error.Code)
					assert.Empty(t, res.Matches)
				}
			}
			assert.Empty(t, rec.resourceCalls("secrets"), "invalid query must not list")

			ok := resultsByID(t, decoded)["ok"]
			require.Nil(t, ok.Error)
			assert.Len(t, ok.Matches, 1, "sibling query must still succeed")
		})
	}
}

// TestHandle_GVRSyntaxRejectedBeforeAnyList proves that a child GVR carrying
// path-cleaning segments or otherwise malformed syntax fails as InvalidQuery and
// reaches no API server. The tripwire reactor fails the test if any list fires,
// so a query that slipped past validation would be caught here even though the
// fake client has no scheme entry for the adversarial resource.
func TestHandle_GVRSyntaxRejectedBeforeAnyList(t *testing.T) {
	validParent := protocol.ParentRef{UID: "rel-1", Namespace: "app-ns", Name: "rel"}

	tests := []struct {
		name  string
		child protocol.ChildKind
	}{
		{
			// Scoped to a namespace this path-cleans to /api/v1/secrets —
			// cluster-wide core Secrets — and dodges the core-Secret strip, which
			// keys off the literal resource name "secrets".
			name:  "traversal resource",
			child: protocol.ChildKind{Version: "v1", Kind: "Secret", Resource: "../../../../api/v1/secrets"},
		},
		{
			name:  "malformed group",
			child: protocol.ChildKind{Group: "apps/", Version: "v1", Kind: "ReplicaSet", Resource: "replicasets"},
		},
		{
			name:  "uppercase version",
			child: protocol.ChildKind{Version: "V1", Kind: "Pod", Resource: "pods"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dyn := newFakeDyn(t)
			dyn.PrependReactor("list", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
				t.Errorf("no list must reach the API server for an invalid GVR, got %+v", action)
				return true, nil, fmt.Errorf("list must not be reached")
			})
			h := newResourceTreeHandler(dyn, testLogger())

			resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
				Version: protocol.Version,
				Queries: []protocol.MatchQuery{{
					ID:      "bad",
					Matcher: protocol.MatcherOwnerRef,
					Child:   tt.child,
					Parents: []protocol.ParentRef{validParent},
				}},
			}))

			res := resultsByID(t, decodeMatchResponse(t, resp))["bad"]
			require.NotNil(t, res.Error)
			assert.Equal(t, protocol.CodeInvalidQuery, res.Error.Code)
			assert.Empty(t, res.Matches)
		})
	}
}

// TestShapeObject_SecretStripReliesOnCleanResourceName documents why validateQuery
// must reject a malformed resource BEFORE shaping: the core-Secret strip matches
// the literal resource "secrets", so a traversal spelling would bypass it here.
// The upstream rejection (TestHandle_GVRSyntaxRejectedBeforeAnyList) is what makes
// this unreachable in practice. If shapeObject is ever hardened to strip
// regardless of spelling, delete this test — do not weaken the strip to satisfy it.
func TestShapeObject_SecretStripReliesOnCleanResourceName(t *testing.T) {
	secret := newObject("v1", "Secret", "app-ns", "sec-a", "sec-1", ownerRef{uid: "rel-1"})
	secret.Object["data"] = map[string]any{"password": "c3VwZXItc2VjcmV0"}

	got := shapeObject(secret, protocol.ChildKind{
		Version:  "v1",
		Kind:     "Secret",
		Resource: "../../../../api/v1/secrets",
	}, false)

	assert.Contains(t, got, "data",
		"a non-canonical resource spelling bypasses the strip at the shaping layer; validateQuery must reject it upstream")
}

func TestHandle_TooManyQueries(t *testing.T) {
	h, rec := newTestHandler(t)

	queries := make([]protocol.MatchQuery, 0, protocol.MaxQueriesPerRequest+1)
	for i := 0; i <= protocol.MaxQueriesPerRequest; i++ {
		queries = append(queries, ownerRefQuery(fmt.Sprintf("q%d", i), podsGVR, "Pod",
			protocol.ParentRef{UID: "rs-a", Namespace: "app-ns", Name: "rs-a"}))
	}

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: queries,
	}))

	require.NotNil(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NotNil(t, resp.Error)
	assert.Empty(t, rec.snapshot())
}

func TestHandle_UndecodableBody(t *testing.T) {
	h, rec := newTestHandler(t)

	resp := h.Handle(context.Background(), &messaging.HTTPTunnelRequest{
		RequestID: "req-1",
		Target:    protocol.Target,
		Method:    http.MethodPost,
		Path:      protocol.PathMatches,
		Body:      []byte("{not json"),
	})

	require.NotNil(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NotNil(t, resp.Error)
	assert.Empty(t, rec.snapshot())
}

func TestHandle_NamespacesDerivedFromParents(t *testing.T) {
	podA := newPod("ns-a", "pod-a", "pod-1", ownerRef{uid: "rs-a"})
	podB := newPod("ns-b", "pod-b", "pod-2", ownerRef{uid: "rs-b"})

	h, rec := newTestHandler(t, podA, podB)

	resp := h.Handle(context.Background(), tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			ownerRefQuery("q1", podsGVR, "Pod",
				protocol.ParentRef{UID: "rs-a", Namespace: "ns-a", Name: "rs-a"},
				protocol.ParentRef{UID: "rs-a2", Namespace: "ns-a", Name: "rs-a2"},
				protocol.ParentRef{UID: "rs-b", Namespace: "ns-b", Name: "rs-b"},
			),
		},
	}))

	res := resultsByID(t, decodeMatchResponse(t, resp))["q1"]
	require.Nil(t, res.Error)
	assert.Len(t, res.Matches, 2)
	assert.ElementsMatch(t, []string{"ns-a", "ns-b"}, rec.namespaces(), "one list per distinct parent namespace")
}

// TestRoute_UnknownTargetSentinel pins the exact 404 an old agent answers with
// when it has no resource-tree handler. Task 6's client keys version-skew
// detection off this reply, so the message and its empty body are contract.
func TestRoute_UnknownTargetSentinel(t *testing.T) {
	router := newTestRouter(t, map[string]*Route{
		"k8s": newMockRoute("k8s", "https://k8s.svc", nil),
	})

	resp := router.Route(&messaging.HTTPTunnelRequest{
		RequestID: "req-1",
		Target:    protocol.Target,
		Method:    http.MethodPost,
		Path:      protocol.PathMatches,
	})

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NotNil(t, resp.Error)
	assert.Equal(t, "unknown target: resource-tree", resp.Error.Message)
	assert.Equal(t, http.StatusNotFound, resp.Error.Code)
	assert.Empty(t, resp.Body)
}

// TestHandleHTTPTunnelRequest_ResourceTreeIntercepted proves the agent answers
// the resource-tree target itself instead of handing it to the router (which
// would 404).
func TestHandleHTTPTunnelRequest_ResourceTreeIntercepted(t *testing.T) {
	pod := newPod("app-ns", "pod-a", "pod-1", ownerRef{uid: "rs-a"})
	dyn := newFakeDyn(t, pod)

	cfg := &Config{ServerURL: "ws://localhost:8443", PlaneType: "dataplane", PlaneID: "test"}
	agent, err := New(cfg, nil, &rest.Config{Host: "https://kubernetes.default.svc"}, dyn, testLogger())
	require.NoError(t, err)

	mock := &mockConnection{}
	agent.conn = mock

	req := tunnelRequest(t, protocol.MatchRequest{
		Version: protocol.Version,
		Queries: []protocol.MatchQuery{
			ownerRefQuery("q1", podsGVR, "Pod", protocol.ParentRef{UID: "rs-a", Namespace: "app-ns", Name: "rs-a"}),
		},
	})
	agent.handleHTTPTunnelRequest(req)

	written := mock.getWrittenMessages()
	require.Len(t, written, 1)

	var resp messaging.HTTPTunnelResponse
	require.NoError(t, json.Unmarshal(written[0], &resp))
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var decoded protocol.MatchResponse
	require.NoError(t, json.Unmarshal(resp.Body, &decoded))
	require.Len(t, decoded.Results, 1)
	assert.Len(t, decoded.Results[0].Matches, 1)
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}

func manyParents(n int) []protocol.ParentRef {
	out := make([]protocol.ParentRef, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, protocol.ParentRef{
			UID:       fmt.Sprintf("uid-%d", i),
			Namespace: "app-ns",
			Name:      fmt.Sprintf("p-%d", i),
		})
	}
	return out
}

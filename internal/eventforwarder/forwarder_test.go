// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package eventforwarder

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"github.com/openchoreo/openchoreo/internal/eventforwarder/config"
	"github.com/openchoreo/openchoreo/internal/eventforwarder/dispatcher"
)

// projectGVR is the canonical GVR used in handler tests.
var projectGVR = schema.GroupVersionResource{
	Group:    "openchoreo.dev",
	Version:  "v1alpha1",
	Resource: "projects",
}

// newProject constructs a minimal *unstructured.Unstructured shaped like an
// OpenChoreo Project under the "default" namespace.
func newProject(name string) *unstructured.Unstructured {
	obj := map[string]interface{}{
		"apiVersion": "openchoreo.dev/v1alpha1",
		"kind":       "Project",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": "default",
			"labels": map[string]interface{}{
				"openchoreo.dev/managed": "true",
			},
			"annotations": map[string]interface{}{
				"openchoreo.dev/display-name": name,
			},
		},
		"spec": map[string]interface{}{
			"deploymentPipelineRef": map[string]interface{}{
				"name": "default",
			},
		},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":   "Ready",
					"status": "True",
				},
			},
		},
	}
	return &unstructured.Unstructured{Object: obj}
}

// =====================================================================
// Forwarder.handleEvent
// =====================================================================

// captureServer is an httptest.Server that records every webhook payload
// it receives onto a channel.
type captureServer struct {
	*httptest.Server
	delivered chan dispatcher.Event
	hits      *atomic.Int32
}

func newCaptureServer(t *testing.T) *captureServer {
	t.Helper()
	delivered := make(chan dispatcher.Event, 16)
	hits := &atomic.Int32{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, _ := io.ReadAll(r.Body)
		var ev dispatcher.Event
		_ = json.Unmarshal(body, &ev)
		delivered <- ev
		w.WriteHeader(http.StatusOK)
	}))
	return &captureServer{Server: ts, delivered: delivered, hits: hits}
}

// newForwarderWithCapture wires a Forwarder to a real Dispatcher pointed
// at an httptest.Server. Returns the forwarder, the capture server, and a
// cleanup func.
func newForwarderWithCapture(t *testing.T) (*Forwarder, *captureServer, func()) {
	t.Helper()
	cs := newCaptureServer(t)
	d := dispatcher.New(config.WebhooksConfig{
		Endpoints: []config.EndpointConfig{{URL: cs.URL}},
	}, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	d.Start(ctx)
	f := &Forwarder{
		dispatcher:  d,
		logger:      slog.Default(),
		lastEvent:   make(map[string]time.Time),
		dispatchCtx: ctx,
	}
	return f, cs, func() {
		cancel()
		cs.Close()
	}
}

func waitForEvent(t *testing.T, cs *captureServer) dispatcher.Event {
	t.Helper()
	select {
	case ev := <-cs.delivered:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for webhook delivery")
		return dispatcher.Event{}
	}
}

func TestHandleEvent_DispatchesObservedFields(t *testing.T) {
	f, cs, cleanup := newForwarderWithCapture(t)
	defer cleanup()

	obj := newProject("url-shortener")
	f.handleEvent(obj, "updated", projectGVR)

	got := waitForEvent(t, cs)
	assert.Equal(t, "Project", got.Kind, "kind should come from object metadata, not GVR")
	assert.Equal(t, "url-shortener", got.Name)
	assert.Equal(t, "default", got.Namespace)
	assert.Equal(t, "updated", got.Action)
}

func TestHandleEvent_DebounceCollapsesRapidDuplicates(t *testing.T) {
	f, cs, cleanup := newForwarderWithCapture(t)
	defer cleanup()

	obj := newProject("p1")

	// Two calls within the debounce window should produce exactly one
	// dispatch.
	f.handleEvent(obj, "updated", projectGVR)
	f.handleEvent(obj, "updated", projectGVR)

	// First call should land.
	first := waitForEvent(t, cs)
	assert.Equal(t, "p1", first.Name)

	// Wait briefly to confirm no second delivery arrives.
	select {
	case extra := <-cs.delivered:
		t.Fatalf("expected debounce to suppress second dispatch; got %+v", extra)
	case <-time.After(150 * time.Millisecond):
		// good — no second event arrived
	}

	assert.Equal(t, int32(1), cs.hits.Load(), "exactly one HTTP delivery")
}

func TestHandleEvent_DebounceIsPerKey(t *testing.T) {
	f, cs, cleanup := newForwarderWithCapture(t)
	defer cleanup()

	// Two different projects in the same window should both dispatch —
	// the debounce key includes name/namespace.
	f.handleEvent(newProject("p1"), "updated", projectGVR)
	f.handleEvent(newProject("p2"), "updated", projectGVR)

	got1 := waitForEvent(t, cs)
	got2 := waitForEvent(t, cs)

	names := []string{got1.Name, got2.Name}
	assert.ElementsMatch(t, []string{"p1", "p2"}, names,
		"both events should be dispatched because the debounce key differs")
}

func TestHandleEvent_CreatedActionBypassesDebounce(t *testing.T) {
	f, cs, cleanup := newForwarderWithCapture(t)
	defer cleanup()

	// Create-then-recreate of the same name (e.g. user deletes a CR
	// then immediately recreates it before the cleanup goroutine
	// evicts the debounce key, or two name collisions across a
	// namespace boundary): the second `created` must NOT be dropped.
	// Creates are non-fungible — collapsing them risks dropping a
	// fresh resource entirely.
	f.handleEvent(newProject("p1"), "created", projectGVR)
	first := waitForEvent(t, cs)
	assert.Equal(t, "created", first.Action)

	// Same key, well within the 1s debounce window. Bypass for
	// `created` must keep this dispatch alive.
	f.handleEvent(newProject("p1"), "created", projectGVR)
	second := waitForEvent(t, cs)
	assert.Equal(t, "created", second.Action)
	assert.Equal(t, int32(2), cs.hits.Load(),
		"both creates must be dispatched even within the debounce window")
}

func TestHandleEvent_DeletedActionBypassesDebounce(t *testing.T) {
	f, cs, cleanup := newForwarderWithCapture(t)
	defer cleanup()

	obj := newProject("p1")

	// Sequence emulates "create-then-delete-immediately" of a fresh
	// resource: the deletionTimestamp UPDATE lands first, finalizer
	// cleanup completes within the debounce window, and the trailing
	// DELETE arrives while the window is still active. The delete must
	// bypass the debounce — losing it would leave an orphan entity in
	// the consumer's catalog until the next periodic full sync.
	f.handleEvent(obj, "updated", projectGVR)
	first := waitForEvent(t, cs)
	assert.Equal(t, "updated", first.Action)

	// Same key, well within the 1s debounce window. Update would be
	// suppressed; delete must still go through.
	f.handleEvent(obj, "deleted", projectGVR)
	second := waitForEvent(t, cs)
	assert.Equal(t, "p1", second.Name)
	assert.Equal(t, "deleted", second.Action)
	assert.Equal(t, int32(2), cs.hits.Load(),
		"both update and following delete must be dispatched")
}

func TestHandleEvent_DebounceExpiresAfterWindow(t *testing.T) {
	f, cs, cleanup := newForwarderWithCapture(t)
	defer cleanup()

	obj := newProject("p1")

	// First call — dispatches normally.
	f.handleEvent(obj, "updated", projectGVR)
	first := waitForEvent(t, cs)
	assert.Equal(t, "p1", first.Name)

	// Force the lastEvent timestamp to be older than the window so the
	// next call is no longer debounced. Avoids real-time sleeps.
	key := projectGVR.Resource + "/default/p1"
	f.mu.Lock()
	f.lastEvent[key] = time.Now().Add(-2 * debounceWindow)
	f.mu.Unlock()

	// Second call — should dispatch because the window has passed.
	f.handleEvent(obj, "updated", projectGVR)
	second := waitForEvent(t, cs)
	assert.Equal(t, "p1", second.Name)
}

func TestHandleEvent_TombstoneIsUnwrapped(t *testing.T) {
	f, cs, cleanup := newForwarderWithCapture(t)
	defer cleanup()

	// On informer relist after a long disconnect, deletes can arrive
	// wrapped in cache.DeletedFinalStateUnknown. The handler must
	// unwrap and dispatch the inner object's identity.
	tombstone := cache.DeletedFinalStateUnknown{
		Key: "default/p-deleted",
		Obj: newProject("p-deleted"),
	}
	f.handleEvent(tombstone, "deleted", projectGVR)

	got := waitForEvent(t, cs)
	assert.Equal(t, "Project", got.Kind)
	assert.Equal(t, "p-deleted", got.Name)
	assert.Equal(t, "default", got.Namespace)
	assert.Equal(t, "deleted", got.Action)
}

func TestHandleEvent_UnknownObjectTypeIsSilentlyIgnored(t *testing.T) {
	f, cs, cleanup := newForwarderWithCapture(t)
	defer cleanup()

	// Pass something neither *unstructured.Unstructured nor a tombstone.
	// The handler must log and return without dispatching.
	f.handleEvent("a string", "updated", projectGVR)

	select {
	case ev := <-cs.delivered:
		t.Fatalf("expected no dispatch; got %+v", ev)
	case <-time.After(150 * time.Millisecond):
		// good — handler dropped the bad input without crashing
	}
	assert.Equal(t, int32(0), cs.hits.Load())
}

func TestHandleEvent_ClusterScopedHasEmptyNamespace(t *testing.T) {
	f, cs, cleanup := newForwarderWithCapture(t)
	defer cleanup()

	cct := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "openchoreo.dev/v1alpha1",
		"kind":       "ClusterComponentType",
		"metadata": map[string]interface{}{
			"name":              "service",
			"creationTimestamp": metav1.Now().Format(time.RFC3339),
		},
	}}
	f.handleEvent(cct, "created", schema.GroupVersionResource{
		Group: "openchoreo.dev", Version: "v1alpha1", Resource: "clustercomponenttypes",
	})

	got := waitForEvent(t, cs)
	assert.Equal(t, "ClusterComponentType", got.Kind)
	assert.Equal(t, "service", got.Name)
	assert.Equal(t, "", got.Namespace, "cluster-scoped resources have empty namespace")
}

// =====================================================================
// gvrList — basic sanity check (no behavioral test; the list is static)
// =====================================================================

func TestGVRList_AllOpenChoreoGroup(t *testing.T) {
	for _, gvr := range gvrList() {
		assert.Equal(t, "openchoreo.dev", gvr.Group, "every CRD watched should be in the openchoreo.dev API group")
		assert.Equal(t, "v1alpha1", gvr.Version)
		assert.NotEmpty(t, gvr.Resource)
	}
	require.NotEmpty(t, gvrList(), "watched-resource list must not be empty")
}

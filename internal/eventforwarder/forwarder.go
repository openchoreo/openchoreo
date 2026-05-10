// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package eventforwarder

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	"github.com/openchoreo/openchoreo/internal/eventforwarder/dispatcher"
)

// ocControlPlaneLabelSelector matches namespaces marked as OpenChoreo
// Organizations (control-plane namespaces). The OC API server itself
// uses the same label to distinguish OC-managed namespaces from system
// namespaces (kube-system, etc.) and unrelated namespaces.
const ocControlPlaneLabelSelector = "openchoreo.dev/control-plane=true"

// namespaceGVR identifies the cluster-scoped Kubernetes Namespace
// resource. Watched separately from the OC custom resources because
// (a) it lives in the core API group and (b) we apply a label selector
// to scope to OC Organizations only.
var namespaceGVR = schema.GroupVersionResource{
	Group:    "",
	Version:  "v1",
	Resource: "namespaces",
}

// debounceWindow is the duration to wait before dispatching an event
// for the same resource, to avoid flooding on rapid successive updates.
const debounceWindow = 1 * time.Second

// debounceCleanupInterval is how often we sweep stale entries out of the
// debounce map. Without this, a long-lived process that sees many distinct
// resources over time would accumulate keys forever.
const debounceCleanupInterval = 5 * time.Minute

// Forwarder watches OpenChoreo Kubernetes resources and forwards
// change-notification webhooks to configured subscribers (typically the
// Backstage events plugin). It uses Kubernetes informers internally —
// the K8s "watch" terminology refers to the informer mechanism, while
// this component itself is named "event-forwarder" to describe its
// outward role: turning K8s events into HTTP webhooks that drive
// downstream catalog updates.
type Forwarder struct {
	client     dynamic.Interface
	dispatcher *dispatcher.Dispatcher
	logger     *slog.Logger

	// dispatchCtx is captured from Start() and passed to Dispatch so that
	// in-flight HTTP retries and backoffs abort cleanly on shutdown.
	// Informer event-handler callbacks don't carry their own context, so
	// we hang on to the one from Start.
	dispatchCtx context.Context

	// debounce tracks the last dispatch time per resource key
	mu        sync.Mutex
	lastEvent map[string]time.Time
}

// New creates a new Forwarder.
func New(client dynamic.Interface, d *dispatcher.Dispatcher, logger *slog.Logger) *Forwarder {
	return &Forwarder{
		client:     client,
		dispatcher: d,
		logger:     logger,
		lastEvent:  make(map[string]time.Time),
	}
}

// gvrList returns the GroupVersionResources to watch.
func gvrList() []schema.GroupVersionResource {
	group := "openchoreo.dev"
	version := "v1alpha1"

	resources := []string{
		// Namespaced
		"projects",
		"components",
		"workloads",
		"environments",
		"dataplanes",
		"deploymentpipelines",
		"componenttypes",
		"traits",
		"workflows",
		"workflowplanes",
		"observabilityplanes",
		// Cluster-scoped
		"clustercomponenttypes",
		"clustertraits",
		"clusterworkflows",
		"clusterdataplanes",
		"clusterobservabilityplanes",
		"clusterworkflowplanes",
	}

	gvrs := make([]schema.GroupVersionResource, 0, len(resources))
	for _, r := range resources {
		gvrs = append(gvrs, schema.GroupVersionResource{
			Group:    group,
			Version:  version,
			Resource: r,
		})
	}
	return gvrs
}

// Start begins watching all OpenChoreo resources (and OC-labeled core
// Namespaces) and blocks until the context is canceled.
//
// `onReady`, if non-nil, is invoked exactly once after every informer
// cache has finished its initial list — the moment the forwarder will
// start delivering events. Callers use this to flip readiness probes
// to "ready" so a rolling-update doesn't route traffic to this pod
// before it can actually consume events.
func (f *Forwarder) Start(ctx context.Context, onReady func()) error {
	f.dispatchCtx = ctx

	// Resource informers — unfiltered. Each watched OC resource has its
	// own informer so we receive events for every Project, Component,
	// Workload, etc.
	crdFactory := dynamicinformer.NewDynamicSharedInformerFactory(f.client, 0)

	for _, gvr := range gvrList() {
		informer := crdFactory.ForResource(gvr).Informer()

		gvrCopy := gvr // capture for closures
		_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				f.handleEvent(obj, "created", gvrCopy)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				if isStatusOnlyChange(oldObj, newObj) {
					return
				}
				f.handleEvent(newObj, "updated", gvrCopy)
			},
			DeleteFunc: func(obj interface{}) {
				f.handleEvent(obj, "deleted", gvrCopy)
			},
		})
		if err != nil {
			return fmt.Errorf("adding event handler for %s: %w", gvrCopy.Resource, err)
		}

		f.logger.Info("Watching resource", "resource", gvr.Resource, "group", gvr.Group)
	}

	// Namespace informer — filtered to OC-managed namespaces only via a
	// label selector. The Kubernetes API server applies the selector
	// server-side, so the informer's cache holds only OC Organization
	// namespaces and we never receive events for kube-system,
	// cert-manager, dp-* data-plane namespaces, or any other ambient
	// cluster activity.
	nsFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		f.client, 0, metav1.NamespaceAll,
		func(opts *metav1.ListOptions) {
			opts.LabelSelector = ocControlPlaneLabelSelector
		},
	)
	nsInformer := nsFactory.ForResource(namespaceGVR).Informer()
	_, err := nsInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			f.handleEvent(obj, "created", namespaceGVR)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if isStatusOnlyChange(oldObj, newObj) {
				return
			}
			f.handleEvent(newObj, "updated", namespaceGVR)
		},
		DeleteFunc: func(obj interface{}) {
			f.handleEvent(obj, "deleted", namespaceGVR)
		},
	})
	if err != nil {
		return fmt.Errorf("adding event handler for %s: %w", namespaceGVR.Resource, err)
	}
	f.logger.Info("Watching Namespaces", "labelSelector", ocControlPlaneLabelSelector)

	crdFactory.Start(ctx.Done())
	nsFactory.Start(ctx.Done())
	for gvr, ok := range crdFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("informer cache failed to sync for %s", gvr.Resource)
		}
	}
	for gvr, ok := range nsFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("informer cache failed to sync for %s", gvr.Resource)
		}
	}

	f.logger.Info("All informers synced, event-forwarder is ready")
	if onReady != nil {
		onReady()
	}

	go f.cleanupDebounceLoop(ctx)

	// Block until context is canceled
	<-ctx.Done()
	return nil
}

// cleanupDebounceLoop periodically evicts entries from the debounce map
// whose last-event time is older than the debounce window — they can no
// longer suppress anything, so keeping them around just leaks memory.
func (f *Forwarder) cleanupDebounceLoop(ctx context.Context) {
	ticker := time.NewTicker(debounceCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			f.mu.Lock()
			for key, last := range f.lastEvent {
				if now.Sub(last) > debounceWindow {
					delete(f.lastEvent, key)
				}
			}
			f.mu.Unlock()
		}
	}
}

func (f *Forwarder) handleEvent(obj interface{}, action string, gvr schema.GroupVersionResource) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		// Handle DeletedFinalStateUnknown
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			f.logger.Warn("Unexpected object type in event handler")
			return
		}
		u, ok = tombstone.Obj.(*unstructured.Unstructured)
		if !ok {
			f.logger.Warn("Unexpected object type in tombstone")
			return
		}
	}

	name := u.GetName()
	namespace := u.GetNamespace()
	kind := u.GetKind()

	// Debounce only "updated" events. Updates are the chatty case — a
	// controller patching labels then annotations on the same CR within
	// a single reconcile produces a burst of meaningful changes, and
	// collapsing them into one dispatch saves the consumer redundant
	// re-fetches without losing useful information (the consumer
	// re-fetches on each event and gets the latest committed state
	// anyway).
	//
	// "created" and "deleted" events are non-fungible — you can't merge
	// a create with a later create, and dropping a delete leaves the
	// consumer with an orphan entity until the next periodic full sync.
	// The common bug this guards against is "create-then-delete-
	// immediately" of a fresh resource, where the trailing DELETE
	// arrives within 1s of an earlier UPDATE for the same key.
	if action == "updated" {
		key := gvr.Resource + "/" + namespace + "/" + name
		now := time.Now()
		f.mu.Lock()
		if last, exists := f.lastEvent[key]; exists && now.Sub(last) < debounceWindow {
			f.mu.Unlock()
			return
		}
		f.lastEvent[key] = now
		f.mu.Unlock()
	}

	f.logger.Debug("Resource event detected",
		"action", action,
		"kind", kind,
		"name", name,
		"namespace", namespace,
	)

	ctx := f.dispatchCtx
	if ctx == nil {
		// Defensive: if handleEvent fires before Start() captures the
		// context (shouldn't happen — informers are only started inside
		// Start), fall back to Background so we don't panic.
		ctx = context.Background()
	}
	f.dispatcher.Dispatch(ctx, dispatcher.Event{
		Kind:      kind,
		Name:      name,
		Namespace: namespace,
		Action:    action,
	})
}

// isStatusOnlyChange returns true when the only differences between the old
// and new objects are inside `status` (and metadata fields the catalog
// doesn't care about, like resourceVersion / generation timestamps). When
// spec, labels, and annotations are all unchanged, the catalog has no
// reason to refresh — typical sources are controller reconcile loops
// updating status conditions and agent-heartbeat timestamps.
func isStatusOnlyChange(oldObj, newObj interface{}) bool {
	oldU, ok1 := oldObj.(*unstructured.Unstructured)
	newU, ok2 := newObj.(*unstructured.Unstructured)
	if !ok1 || !ok2 {
		// If we can't compare, fall through and dispatch — safer than silently dropping.
		return false
	}

	oldSpec, _, _ := unstructured.NestedFieldCopy(oldU.UnstructuredContent(), "spec")
	newSpec, _, _ := unstructured.NestedFieldCopy(newU.UnstructuredContent(), "spec")
	if !reflect.DeepEqual(oldSpec, newSpec) {
		return false
	}
	if !reflect.DeepEqual(oldU.GetLabels(), newU.GetLabels()) {
		return false
	}
	if !reflect.DeepEqual(oldU.GetAnnotations(), newU.GetAnnotations()) {
		return false
	}
	// Treat finalizer / deletion timestamp changes as meaningful — the
	// catalog cares about the resource being on the way out. Use
	// metav1.Time.Equal which handles nil + time-value comparison
	// correctly; the raw `!=` would compare *metav1.Time pointers and
	// see every reconciler write of the resource as a "change."
	if !oldU.GetDeletionTimestamp().Equal(newU.GetDeletionTimestamp()) {
		return false
	}
	if !reflect.DeepEqual(oldU.GetFinalizers(), newU.GetFinalizers()) {
		return false
	}

	return true
}

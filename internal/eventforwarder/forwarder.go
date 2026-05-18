// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package eventforwarder

import (
	"context"
	"fmt"
	"log/slog"
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

// debounceWindow is the duration to wait before dispatching an event
// for the same resource, to avoid flooding on rapid successive updates.
const debounceWindow = 1 * time.Second

// debounceCleanupInterval is how often we sweep stale entries out of the
// debounce map. Without this, a long-lived process that sees many distinct
// resources over time would accumulate keys forever.
const debounceCleanupInterval = 5 * time.Minute

// WatchResource is a single entry in the configured watch list — a
// GVR plus an optional Kubernetes label selector. When LabelSelector
// is non-empty, the K8s API server filters list/watch responses
// server-side so the informer cache only holds matching objects. The
// canonical use is scoping the core Namespace informer to
// OpenChoreo-labeled namespaces only, but operators can apply the
// same filter to any watched resource.
type WatchResource struct {
	GVR           schema.GroupVersionResource
	LabelSelector string
}

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

	// watchResources is the list of resources the forwarder watches,
	// sourced from config (eventForwarder.config.watch.resources in
	// Helm values). Each entry carries an optional label selector
	// applied server-side — Namespace uses this to scope to
	// OC-managed namespaces only, and the same mechanism is available
	// to any other resource an operator wants to filter.
	watchResources []WatchResource

	// dispatchCtx is captured from Start() and passed to Dispatch so that
	// in-flight HTTP retries and backoffs abort cleanly on shutdown.
	// Informer event-handler callbacks don't carry their own context, so
	// we hang on to the one from Start.
	dispatchCtx context.Context

	// debounce tracks the last dispatch time per resource key
	mu        sync.Mutex
	lastEvent map[string]time.Time
}

// New creates a new Forwarder. `watchResources` is the list the
// forwarder will watch (one informer per entry). It comes from the
// `eventForwarder.config.watch.resources` Helm value, parsed by the
// config package. The list is authoritative — passing an empty slice
// means the forwarder watches nothing.
func New(
	client dynamic.Interface,
	d *dispatcher.Dispatcher,
	logger *slog.Logger,
	watchResources []WatchResource,
) *Forwarder {
	return &Forwarder{
		client:         client,
		dispatcher:     d,
		logger:         logger,
		watchResources: watchResources,
		lastEvent:      make(map[string]time.Time),
	}
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

	// One factory per watch entry so each carries its own (optional)
	// label-selector tweak. NewFilteredDynamicSharedInformerFactory
	// applies a single TweakListOptionsFunc to every resource it
	// serves, so resources with different selectors can't share a
	// factory. An empty selector means "no filter" — the tweak fn is
	// still wired up but leaves opts.LabelSelector untouched.
	factories := make([]dynamicinformer.DynamicSharedInformerFactory, 0, len(f.watchResources))
	for _, w := range f.watchResources {
		gvrCopy := w.GVR
		selector := w.LabelSelector
		factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
			f.client, 0, metav1.NamespaceAll,
			func(opts *metav1.ListOptions) {
				if selector != "" {
					opts.LabelSelector = selector
				}
			},
		)
		informer := factory.ForResource(gvrCopy).Informer()
		_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				f.handleEvent(obj, "created", gvrCopy)
			},
			UpdateFunc: func(_ interface{}, newObj interface{}) {
				f.handleEvent(newObj, "updated", gvrCopy)
			},
			DeleteFunc: func(obj interface{}) {
				f.handleEvent(obj, "deleted", gvrCopy)
			},
		})
		if err != nil {
			return fmt.Errorf("adding event handler for %s: %w", gvrCopy.Resource, err)
		}

		args := []any{"resource", gvrCopy.Resource, "group", gvrCopy.Group}
		if selector != "" {
			args = append(args, "labelSelector", selector)
		}
		f.logger.Info("Watching resource", args...)
		factories = append(factories, factory)
	}

	for _, factory := range factories {
		factory.Start(ctx.Done())
	}
	for _, factory := range factories {
		for gvr, ok := range factory.WaitForCacheSync(ctx.Done()) {
			if !ok {
				return fmt.Errorf("informer cache failed to sync for %s", gvr.Resource)
			}
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

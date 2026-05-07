// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package eventforwarder

import (
	"context"
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
// resource. Watched separately from the OC CRDs because (a) it lives in
// the core API group and (b) we apply a label selector to scope to OC
// Organizations only.
var namespaceGVR = schema.GroupVersionResource{
	Group:    "",
	Version:  "v1",
	Resource: "namespaces",
}

// debounceWindow is the duration to wait before dispatching an event
// for the same resource, to avoid flooding on rapid successive updates.
const debounceWindow = 1 * time.Second

// Forwarder watches OpenChoreo CRDs and forwards change-notification
// webhooks to configured subscribers (typically the Backstage events
// plugin). It uses Kubernetes informers internally — the K8s "watch"
// terminology refers to the informer mechanism, while this component
// itself is named "event-forwarder" to describe its outward role:
// turning K8s events into HTTP webhooks that drive downstream catalog
// updates.
type Forwarder struct {
	client     dynamic.Interface
	dispatcher *dispatcher.Dispatcher
	logger     *slog.Logger

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

// Start begins watching all OpenChoreo CRDs (and OC-labelled core
// Namespaces) and blocks until the context is cancelled.
func (f *Forwarder) Start(ctx context.Context) error {
	// CRD informers — unfiltered. Each OC CRD has its own informer so
	// we receive events for every Project, Component, Workload, etc.
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
			return err
		}

		f.logger.Info("Watching CRD", "resource", gvr.Resource, "group", gvr.Group)
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
		return err
	}
	f.logger.Info("Watching Namespaces", "labelSelector", ocControlPlaneLabelSelector)

	crdFactory.Start(ctx.Done())
	nsFactory.Start(ctx.Done())
	crdFactory.WaitForCacheSync(ctx.Done())
	nsFactory.WaitForCacheSync(ctx.Done())

	f.logger.Info("All informers synced, event-forwarder is ready")

	// Block until context is cancelled
	<-ctx.Done()
	return nil
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

	// Debounce: skip if we dispatched for this resource within the window
	key := gvr.Resource + "/" + namespace + "/" + name
	now := time.Now()
	f.mu.Lock()
	if last, exists := f.lastEvent[key]; exists && now.Sub(last) < debounceWindow {
		f.mu.Unlock()
		return
	}
	f.lastEvent[key] = now
	f.mu.Unlock()

	f.logger.Info("CRD event detected",
		"action", action,
		"kind", kind,
		"name", name,
		"namespace", namespace,
	)

	f.dispatcher.Dispatch(dispatcher.Event{
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
	// catalog cares about the resource being on the way out.
	if oldU.GetDeletionTimestamp() != newU.GetDeletionTimestamp() {
		return false
	}
	if !reflect.DeepEqual(oldU.GetFinalizers(), newU.GetFinalizers()) {
		return false
	}

	return true
}

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package labelresolver

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/openchoreo/openchoreo/internal/events-collector/labelcache"
)

const (
	// openchoreoLabelPrefix is the prefix for all OpenChoreo labels.
	openchoreoLabelPrefix = "openchoreo.dev/"
)

// Resolver fetches and caches labels for Kubernetes objects referenced in events.
type Resolver struct {
	dynamicClient dynamic.Interface
	cache         *labelcache.Cache
	logger        *slog.Logger
}

// New creates a new label resolver.
func New(dynamicClient dynamic.Interface, cache *labelcache.Cache, logger *slog.Logger) *Resolver {
	if logger == nil {
		logger = slog.Default()
	}

	return &Resolver{
		dynamicClient: dynamicClient,
		cache:         cache,
		logger:        logger,
	}
}

// Resolve returns the labels for the given involved object.
// It checks the cache first; on cache miss, it fetches from the Kubernetes API.
// Only labels with the "openchoreo.dev/" prefix are returned.
func (r *Resolver) Resolve(ctx context.Context, involvedObj corev1.ObjectReference) (map[string]string, error) {
	key := labelcache.Key(involvedObj.Namespace, involvedObj.Kind, involvedObj.Name)

	// Check cache first
	if labels, found := r.cache.Get(key); found {
		return labels, nil
	}

	// Cache miss - fetch from Kubernetes API
	labels, err := r.fetchLabels(ctx, involvedObj)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object no longer exists (common for transient ReplicaSets, old Pods, etc.)
			r.cache.SetNotFound(key)
			r.logger.Debug("involved object not found, caching as not-found",
				"kind", involvedObj.Kind,
				"name", involvedObj.Name,
				"namespace", involvedObj.Namespace,
			)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to fetch labels for %s/%s/%s: %w",
			involvedObj.Namespace, involvedObj.Kind, involvedObj.Name, err)
	}

	// Filter to only OpenChoreo labels
	filtered := filterOpenChoreoLabels(labels)

	// Cache the result
	r.cache.Set(key, filtered)

	return filtered, nil
}

// fetchLabels retrieves the labels of a Kubernetes object using the dynamic client.
func (r *Resolver) fetchLabels(ctx context.Context, obj corev1.ObjectReference) (map[string]string, error) {
	gvr, err := objectRefToGVR(obj)
	if err != nil {
		return nil, err
	}

	resourceInterface := r.dynamicClient.Resource(gvr)
	var resource interface {
		GetLabels() map[string]string
	}
	if obj.Namespace == "" {
		resource, err = resourceInterface.Get(ctx, obj.Name, metav1.GetOptions{})
	} else {
		resource, err = resourceInterface.Namespace(obj.Namespace).Get(ctx, obj.Name, metav1.GetOptions{})
	}

	if err != nil {
		return nil, err
	}

	return resource.GetLabels(), nil
}

// objectRefToGVR converts a corev1.ObjectReference to a GroupVersionResource.
func objectRefToGVR(obj corev1.ObjectReference) (schema.GroupVersionResource, error) {
	gv, err := schema.ParseGroupVersion(obj.APIVersion)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("failed to parse API version %q: %w", obj.APIVersion, err)
	}

	// Convert Kind to plural resource name (lowercase + "s")
	// This handles most standard Kubernetes resources.
	resource := kindToResource(obj.Kind)

	return schema.GroupVersionResource{
		Group:    gv.Group,
		Version:  gv.Version,
		Resource: resource,
	}, nil
}

// kindToResource converts a Kubernetes Kind to its plural resource name.
func kindToResource(kind string) string {
	lower := strings.ToLower(kind)
	// Handle common irregular plurals
	switch lower {
	case "ingress":
		return "ingresses"
	case "networkpolicy":
		return "networkpolicies"
	case "poddisruptionbudget":
		return "poddisruptionbudgets"
	default:
		return lower + "s"
	}
}

// filterOpenChoreoLabels returns only labels with the "openchoreo.dev/" prefix.
func filterOpenChoreoLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}

	filtered := make(map[string]string)
	for k, v := range labels {
		if strings.HasPrefix(k, openchoreoLabelPrefix) {
			filtered[k] = v
		}
	}

	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

func watchTestRelease() *openchoreov1alpha1.RenderedRelease {
	return &openchoreov1alpha1.RenderedRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "release",
			Namespace:       "default",
			ResourceVersion: "10",
			Generation:      2,
			Labels:          map[string]string{"openchoreo.dev/component": "c"},
			Annotations:     map[string]string{"example.com/note": "a"},
			Finalizers:      []string{DataPlaneCleanupFinalizer},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "openchoreo.dev/v1alpha1", Kind: "ReleaseBinding", Name: "binding", UID: "uid-1",
			}},
		},
	}
}

func TestIgnoreStatusOnlyUpdates(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name   string
		mutate func(n *openchoreov1alpha1.RenderedRelease)
		want   bool
	}{
		{
			name: "own status write is dropped",
			mutate: func(n *openchoreov1alpha1.RenderedRelease) {
				n.ResourceVersion = "11"
				n.Status.Resources = []openchoreov1alpha1.RenderedManifestStatus{{ID: "es", LastObservedTime: &now}}
			},
			want: false,
		},
		{
			name:   "resync with unchanged resourceVersion passes",
			mutate: func(n *openchoreov1alpha1.RenderedRelease) {},
			want:   true,
		},
		{
			name: "spec change passes",
			mutate: func(n *openchoreov1alpha1.RenderedRelease) {
				n.ResourceVersion = "11"
				n.Generation = 3
			},
			want: true,
		},
		{
			name: "restartedAt annotation passes",
			mutate: func(n *openchoreov1alpha1.RenderedRelease) {
				n.ResourceVersion = "11"
				n.Annotations[controller.AnnotationKeyRestartedAt] = "2026-11-05T10:00:00Z"
			},
			want: true,
		},
		{
			name: "label change passes",
			mutate: func(n *openchoreov1alpha1.RenderedRelease) {
				n.ResourceVersion = "11"
				n.Labels["openchoreo.dev/component"] = "d"
			},
			want: true,
		},
		{
			name: "finalizer change passes",
			mutate: func(n *openchoreov1alpha1.RenderedRelease) {
				n.ResourceVersion = "11"
				n.Finalizers = nil
			},
			want: true,
		},
		{
			name: "owner reference change passes",
			mutate: func(n *openchoreov1alpha1.RenderedRelease) {
				n.ResourceVersion = "11"
				n.OwnerReferences[0].UID = "uid-2"
			},
			want: true,
		},
		{
			// The API server also bumps generation here; the deletion timestamp is
			// checked on its own so the filter does not depend on that.
			name: "deletion start passes",
			mutate: func(n *openchoreov1alpha1.RenderedRelease) {
				n.ResourceVersion = "11"
				n.DeletionTimestamp = &now
			},
			want: true,
		},
	}

	p := ignoreStatusOnlyUpdates()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldObj, newObj := watchTestRelease(), watchTestRelease()
			tt.mutate(newObj)
			if got := p.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj}); got != tt.want {
				t.Errorf("Update() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("create, delete and generic events pass", func(t *testing.T) {
		obj := watchTestRelease()
		if !p.Create(event.CreateEvent{Object: obj}) || !p.Delete(event.DeleteEvent{Object: obj}) ||
			!p.Generic(event.GenericEvent{Object: obj}) {
			t.Error("expected create, delete and generic events to pass")
		}
	})
}

func TestTransitioningRequeuePriority(t *testing.T) {
	health := func(hs ...openchoreov1alpha1.HealthStatus) []openchoreov1alpha1.RenderedManifestStatus {
		out := make([]openchoreov1alpha1.RenderedManifestStatus, 0, len(hs))
		for _, h := range hs {
			out = append(out, openchoreov1alpha1.RenderedManifestStatus{HealthStatus: h})
		}
		return out
	}
	tests := []struct {
		name      string
		resources []openchoreov1alpha1.RenderedManifestStatus
		observed  []openchoreov1alpha1.RenderedManifestStatus
		want      int
	}{
		{"progressing is active", health(openchoreov1alpha1.HealthStatusProgressing), nil, 0},
		{"unknown is active", health(openchoreov1alpha1.HealthStatusUnknown), nil, 0},
		{"degraded only is a poll", health(openchoreov1alpha1.HealthStatusDegraded), nil, handler.LowPriority},
		{"degraded with healthy is a poll",
			health(openchoreov1alpha1.HealthStatusHealthy, openchoreov1alpha1.HealthStatusDegraded), nil, handler.LowPriority},
		{"degraded with progressing is active",
			health(openchoreov1alpha1.HealthStatusDegraded, openchoreov1alpha1.HealthStatusProgressing), nil, 0},
		{"observed progressing is active", nil, health(openchoreov1alpha1.HealthStatusProgressing), 0},
		// An observed resource was read directly, so Unknown means missing or
		// unreadable: it does not settle on its own and must not hold priority 0.
		{"observed unknown is a poll", nil, health(openchoreov1alpha1.HealthStatusUnknown), handler.LowPriority},
		{"observed degraded is a poll", nil, health(openchoreov1alpha1.HealthStatusDegraded), handler.LowPriority},
		{"applied unknown with observed degraded is active",
			health(openchoreov1alpha1.HealthStatusUnknown), health(openchoreov1alpha1.HealthStatusDegraded), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := transitioningRequeuePriority(tt.resources, tt.observed); got != tt.want {
				t.Errorf("transitioningRequeuePriority() = %d, want %d", got, tt.want)
			}
		})
	}
}

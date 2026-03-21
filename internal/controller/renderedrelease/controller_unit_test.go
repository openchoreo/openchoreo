// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"fmt"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// helpers

func int32Ptr(v int32) *int32 { return &v }
func boolPtr(v bool) *bool    { return &v }

// toUnstructured converts a typed object to *unstructured.Unstructured using the default converter.
func toUnstructured(t *testing.T, obj interface{}) *unstructured.Unstructured {
	t.Helper()
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		t.Fatalf("ToUnstructured: %v", err)
	}
	return &unstructured.Unstructured{Object: raw}
}

// ─────────────────────────────────────────────────────────────
// addJitter
// ─────────────────────────────────────────────────────────────

func TestAddJitter(t *testing.T) {
	base := 10 * time.Second

	t.Run("zero maxJitter returns base", func(t *testing.T) {
		result := addJitter(base, 0)
		if result != base {
			t.Errorf("expected %v, got %v", base, result)
		}
	})

	t.Run("negative maxJitter returns base", func(t *testing.T) {
		result := addJitter(base, -5*time.Second)
		if result != base {
			t.Errorf("expected %v, got %v", base, result)
		}
	})

	t.Run("positive maxJitter returns value in [base, base+maxJitter)", func(t *testing.T) {
		maxJitter := 2 * time.Second
		for i := 0; i < 100; i++ {
			result := addJitter(base, maxJitter)
			if result < base || result >= base+maxJitter {
				t.Errorf("result %v outside expected range [%v, %v)", result, base, base+maxJitter)
			}
		}
	})
}

// ─────────────────────────────────────────────────────────────
// getStableRequeueInterval
// ─────────────────────────────────────────────────────────────

func TestGetStableRequeueInterval(t *testing.T) {
	t.Run("nil interval defaults to 5m with jitter", func(t *testing.T) {
		release := &openchoreov1alpha1.RenderedRelease{}
		result := getStableRequeueInterval(release)
		if result < 5*time.Minute || result >= 6*time.Minute {
			t.Errorf("expected result in [5m, 6m), got %v", result)
		}
	})

	t.Run("interval set to 0 returns zero", func(t *testing.T) {
		release := &openchoreov1alpha1.RenderedRelease{
			Spec: openchoreov1alpha1.RenderedReleaseSpec{
				Interval: &metav1.Duration{Duration: 0},
			},
		}
		result := getStableRequeueInterval(release)
		if result != 0 {
			t.Errorf("expected 0, got %v", result)
		}
	})

	t.Run("custom interval uses it as base with 20pct jitter", func(t *testing.T) {
		release := &openchoreov1alpha1.RenderedRelease{
			Spec: openchoreov1alpha1.RenderedReleaseSpec{
				Interval: &metav1.Duration{Duration: 10 * time.Minute},
			},
		}
		result := getStableRequeueInterval(release)
		// 20% jitter: [10m, 12m)
		if result < 10*time.Minute || result >= 12*time.Minute {
			t.Errorf("expected result in [10m, 12m), got %v", result)
		}
	})
}

// ─────────────────────────────────────────────────────────────
// getProgressingRequeueInterval
// ─────────────────────────────────────────────────────────────

func TestGetProgressingRequeueInterval(t *testing.T) {
	t.Run("nil interval defaults to 10s with jitter", func(t *testing.T) {
		release := &openchoreov1alpha1.RenderedRelease{}
		result := getProgressingRequeueInterval(release)
		if result < 10*time.Second || result >= 12*time.Second {
			t.Errorf("expected result in [10s, 12s), got %v", result)
		}
	})

	t.Run("interval set to 0 returns zero", func(t *testing.T) {
		release := &openchoreov1alpha1.RenderedRelease{
			Spec: openchoreov1alpha1.RenderedReleaseSpec{
				ProgressingInterval: &metav1.Duration{Duration: 0},
			},
		}
		result := getProgressingRequeueInterval(release)
		if result != 0 {
			t.Errorf("expected 0, got %v", result)
		}
	})

	t.Run("custom interval uses it as base with 20pct jitter", func(t *testing.T) {
		release := &openchoreov1alpha1.RenderedRelease{
			Spec: openchoreov1alpha1.RenderedReleaseSpec{
				ProgressingInterval: &metav1.Duration{Duration: 30 * time.Second},
			},
		}
		result := getProgressingRequeueInterval(release)
		// 20% jitter: [30s, 36s)
		if result < 30*time.Second || result >= 36*time.Second {
			t.Errorf("expected result in [30s, 36s), got %v", result)
		}
	})
}

// ─────────────────────────────────────────────────────────────
// findStaleResources
// ─────────────────────────────────────────────────────────────

func TestFindStaleResources(t *testing.T) {
	r := &Reconciler{}

	makeObj := func(resourceID string) *unstructured.Unstructured {
		obj := &unstructured.Unstructured{}
		obj.SetLabels(map[string]string{labels.LabelKeyRenderedReleaseResourceID: resourceID})
		return obj
	}

	t.Run("empty live resources returns empty", func(t *testing.T) {
		result := r.findStaleResources(nil, nil)
		if len(result) != 0 {
			t.Errorf("expected 0, got %d", len(result))
		}
	})

	t.Run("live resource not in desired is stale", func(t *testing.T) {
		live := []*unstructured.Unstructured{makeObj("res-1")}
		desired := []*unstructured.Unstructured{makeObj("res-2")}
		stale := r.findStaleResources(live, desired)
		if len(stale) != 1 {
			t.Fatalf("expected 1 stale, got %d", len(stale))
		}
		if stale[0].GetLabels()[labels.LabelKeyRenderedReleaseResourceID] != "res-1" {
			t.Error("wrong stale resource returned")
		}
	})

	t.Run("live resource in desired is not stale", func(t *testing.T) {
		live := []*unstructured.Unstructured{makeObj("res-1")}
		desired := []*unstructured.Unstructured{makeObj("res-1")}
		stale := r.findStaleResources(live, desired)
		if len(stale) != 0 {
			t.Errorf("expected 0 stale, got %d", len(stale))
		}
	})

	t.Run("live resource with no ID label is not stale", func(t *testing.T) {
		noIDObj := &unstructured.Unstructured{}
		noIDObj.SetLabels(map[string]string{"other": "label"})
		stale := r.findStaleResources([]*unstructured.Unstructured{noIDObj}, nil)
		if len(stale) != 0 {
			t.Errorf("expected 0 (no ID label skipped), got %d", len(stale))
		}
	})

	t.Run("mixed: some stale, some current", func(t *testing.T) {
		live := []*unstructured.Unstructured{makeObj("keep"), makeObj("remove")}
		desired := []*unstructured.Unstructured{makeObj("keep")}
		stale := r.findStaleResources(live, desired)
		if len(stale) != 1 {
			t.Fatalf("expected 1 stale, got %d", len(stale))
		}
		if stale[0].GetLabels()[labels.LabelKeyRenderedReleaseResourceID] != "remove" {
			t.Error("wrong stale resource")
		}
	})
}

// ─────────────────────────────────────────────────────────────
// findAllKnownGVKs
// ─────────────────────────────────────────────────────────────

func TestFindAllKnownGVKs(t *testing.T) {
	const wellKnownCount = 19 // number of well-known GVKs defined in the function (Secret excluded)

	containsGVK := func(gvks []schema.GroupVersionKind, group, kind string) bool {
		for _, gvk := range gvks {
			if gvk.Group == group && gvk.Kind == kind {
				return true
			}
		}
		return false
	}

	t.Run("empty inputs returns all well-known types", func(t *testing.T) {
		gvks := findAllKnownGVKs(nil, nil)
		if len(gvks) != wellKnownCount {
			t.Errorf("expected %d well-known GVKs, got %d", wellKnownCount, len(gvks))
		}
	})

	t.Run("well-known types include common Kubernetes resources", func(t *testing.T) {
		gvks := findAllKnownGVKs(nil, nil)
		for _, check := range []struct{ group, kind string }{
			{"apps", "Deployment"},
			{"apps", "StatefulSet"},
			{"", "Service"},
			{"", "ConfigMap"},
			{"batch", "CronJob"},
			{"networking.k8s.io", "NetworkPolicy"},
		} {
			if !containsGVK(gvks, check.group, check.kind) {
				t.Errorf("expected well-known GVK %s/%s to be present", check.group, check.kind)
			}
		}
	})

	t.Run("custom desired resource GVK is included alongside well-known", func(t *testing.T) {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "custom.io", Version: "v1", Kind: "Widget"})
		gvks := findAllKnownGVKs([]*unstructured.Unstructured{obj}, nil)
		if len(gvks) != wellKnownCount+1 {
			t.Errorf("expected %d, got %d", wellKnownCount+1, len(gvks))
		}
		if !containsGVK(gvks, "custom.io", "Widget") {
			t.Error("custom GVK not found in result")
		}
	})

	t.Run("desired resource matching a well-known GVK is not duplicated", func(t *testing.T) {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
		gvks := findAllKnownGVKs([]*unstructured.Unstructured{obj}, nil)
		if len(gvks) != wellKnownCount {
			t.Errorf("expected %d (no duplication), got %d", wellKnownCount, len(gvks))
		}
	})

	t.Run("applied resource status GVK is included alongside well-known", func(t *testing.T) {
		applied := []openchoreov1alpha1.ResourceStatus{
			{Group: "legacy.io", Version: "v1", Kind: "OldThing"},
		}
		gvks := findAllKnownGVKs(nil, applied)
		if len(gvks) != wellKnownCount+1 {
			t.Errorf("expected %d, got %d", wellKnownCount+1, len(gvks))
		}
		if !containsGVK(gvks, "legacy.io", "OldThing") {
			t.Error("applied resource GVK not found in result")
		}
	})

	t.Run("desired and applied GVKs are deduplicated against each other", func(t *testing.T) {
		desiredObj := &unstructured.Unstructured{}
		desiredObj.SetGroupVersionKind(schema.GroupVersionKind{Group: "a.io", Version: "v1", Kind: "A"})
		applied := []openchoreov1alpha1.ResourceStatus{
			{Group: "a.io", Version: "v1", Kind: "A"}, // duplicate of desired
			{Group: "b.io", Version: "v1", Kind: "B"},
		}
		gvks := findAllKnownGVKs([]*unstructured.Unstructured{desiredObj}, applied)
		if len(gvks) != wellKnownCount+2 {
			t.Errorf("expected %d, got %d", wellKnownCount+2, len(gvks))
		}
	})
}

// ─────────────────────────────────────────────────────────────
// hasTransitioningResources
// ─────────────────────────────────────────────────────────────

func TestHasTransitioningResources(t *testing.T) {
	r := &Reconciler{}

	tests := []struct {
		name      string
		resources []openchoreov1alpha1.ResourceStatus
		want      bool
	}{
		{
			name:      "empty resources returns false",
			resources: nil,
			want:      false,
		},
		{
			name:      "healthy resource returns false",
			resources: []openchoreov1alpha1.ResourceStatus{{HealthStatus: openchoreov1alpha1.HealthStatusHealthy}},
			want:      false,
		},
		{
			name:      "suspended resource returns false",
			resources: []openchoreov1alpha1.ResourceStatus{{HealthStatus: openchoreov1alpha1.HealthStatusSuspended}},
			want:      false,
		},
		{
			name:      "progressing resource returns true",
			resources: []openchoreov1alpha1.ResourceStatus{{HealthStatus: openchoreov1alpha1.HealthStatusProgressing}},
			want:      true,
		},
		{
			name:      "unknown resource returns true",
			resources: []openchoreov1alpha1.ResourceStatus{{HealthStatus: openchoreov1alpha1.HealthStatusUnknown}},
			want:      true,
		},
		{
			name:      "degraded resource returns true",
			resources: []openchoreov1alpha1.ResourceStatus{{HealthStatus: openchoreov1alpha1.HealthStatusDegraded}},
			want:      true,
		},
		{
			name: "mix of healthy and progressing returns true",
			resources: []openchoreov1alpha1.ResourceStatus{
				{HealthStatus: openchoreov1alpha1.HealthStatusHealthy},
				{HealthStatus: openchoreov1alpha1.HealthStatusProgressing},
			},
			want: true,
		},
		{
			name: "all healthy returns false",
			resources: []openchoreov1alpha1.ResourceStatus{
				{HealthStatus: openchoreov1alpha1.HealthStatusHealthy},
				{HealthStatus: openchoreov1alpha1.HealthStatusSuspended},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.hasTransitioningResources(tt.resources)
			if got != tt.want {
				t.Errorf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────
// GetHealthCheckFunc
// ─────────────────────────────────────────────────────────────

func TestGetHealthCheckFunc(t *testing.T) {
	tests := []struct {
		name        string
		gvk         schema.GroupVersionKind
		wantNonNil  bool
		wantUnknown bool // if true, the result fn should be getUnknownResourceHealth
	}{
		{
			name:       "apps/Deployment",
			gvk:        schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			wantNonNil: true,
		},
		{
			name:       "apps/StatefulSet",
			gvk:        schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"},
			wantNonNil: true,
		},
		{
			name:       "core/Pod",
			gvk:        schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			wantNonNil: true,
		},
		{
			name:       "batch/CronJob",
			gvk:        schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"},
			wantNonNil: true,
		},
		{
			name:        "unknown resource returns non-nil health function",
			gvk:         schema.GroupVersionKind{Group: "custom.io", Version: "v1", Kind: "Widget"},
			wantNonNil:  true,
			wantUnknown: true,
		},
		{
			name:        "core/Service uses unknown health function",
			gvk:         schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"},
			wantNonNil:  true,
			wantUnknown: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := GetHealthCheckFunc(tt.gvk)
			if tt.wantNonNil && fn == nil {
				t.Error("expected non-nil health check function")
			}

			if tt.wantUnknown && fn != nil {
				// Unknown health function should return Healthy for any object
				obj := &unstructured.Unstructured{}
				obj.SetGroupVersionKind(tt.gvk)
				health, err := fn(obj)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if health != openchoreov1alpha1.HealthStatusHealthy {
					t.Errorf("unknown resource health: expected Healthy, got %s", health)
				}
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────
// getDeploymentHealth
// ─────────────────────────────────────────────────────────────

func TestGetDeploymentHealth(t *testing.T) {
	makeDeployment := func(d appsv1.Deployment) *unstructured.Unstructured {
		return toUnstructured(t, &d)
	}

	tests := []struct {
		name       string
		deployment appsv1.Deployment
		want       openchoreov1alpha1.HealthStatus
		wantErr    bool
	}{
		{
			name: "paused deployment is Suspended",
			deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       appsv1.DeploymentSpec{Paused: true, Replicas: int32Ptr(1)},
				Status:     appsv1.DeploymentStatus{ObservedGeneration: 1},
			},
			want: openchoreov1alpha1.HealthStatusSuspended,
		},
		{
			name: "zero replicas is Suspended",
			deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(0)},
				Status:     appsv1.DeploymentStatus{ObservedGeneration: 1},
			},
			want: openchoreov1alpha1.HealthStatusSuspended,
		},
		{
			name: "new deployment (ObservedGeneration==0) is Progressing",
			deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(1)},
				Status:     appsv1.DeploymentStatus{ObservedGeneration: 0},
			},
			want: openchoreov1alpha1.HealthStatusProgressing,
		},
		{
			name: "generation ahead of observed is Progressing",
			deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 3},
				Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(1)},
				Status:     appsv1.DeploymentStatus{ObservedGeneration: 2},
			},
			want: openchoreov1alpha1.HealthStatusProgressing,
		},
		{
			name: "ProgressDeadlineExceeded is Degraded",
			deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(1)},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 1,
					Conditions: []appsv1.DeploymentCondition{
						{Type: appsv1.DeploymentProgressing, Reason: "ProgressDeadlineExceeded"},
					},
				},
			},
			want: openchoreov1alpha1.HealthStatusDegraded,
		},
		{
			name: "ReplicaFailure is Degraded",
			deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(1)},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 1,
					Conditions: []appsv1.DeploymentCondition{
						{Type: appsv1.DeploymentReplicaFailure, Status: corev1.ConditionTrue},
					},
				},
			},
			want: openchoreov1alpha1.HealthStatusDegraded,
		},
		{
			name: "all replicas match and none unavailable is Healthy",
			deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(3)},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration:  1,
					UpdatedReplicas:     3,
					ReadyReplicas:       3,
					AvailableReplicas:   3,
					UnavailableReplicas: 0,
				},
			},
			want: openchoreov1alpha1.HealthStatusHealthy,
		},
		{
			name: "not all replicas ready is Progressing",
			deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(3)},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration:  1,
					UpdatedReplicas:     3,
					ReadyReplicas:       1,
					AvailableReplicas:   1,
					UnavailableReplicas: 2,
				},
			},
			want: openchoreov1alpha1.HealthStatusProgressing,
		},
		{
			name: "all pods on new revision but none available, progressing condition true is Progressing",
			deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(2)},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 1,
					UpdatedReplicas:    2,
					AvailableReplicas:  0,
					Conditions: []appsv1.DeploymentCondition{
						{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue},
					},
				},
			},
			want: openchoreov1alpha1.HealthStatusProgressing,
		},
		{
			name: "rollout complete (NewReplicaSetAvailable) but no pods available is Degraded",
			deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(2)},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 1,
					UpdatedReplicas:    2,
					AvailableReplicas:  0,
					Conditions: []appsv1.DeploymentCondition{
						{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "NewReplicaSetAvailable"},
					},
				},
			},
			want: openchoreov1alpha1.HealthStatusDegraded,
		},
		{
			name: "replica set still updating (ReplicaSetUpdated) with no pods available is Progressing",
			deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(2)},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 1,
					UpdatedReplicas:    2,
					AvailableReplicas:  0,
					Conditions: []appsv1.DeploymentCondition{
						{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "ReplicaSetUpdated"},
					},
				},
			},
			want: openchoreov1alpha1.HealthStatusProgressing,
		},
		{
			name: "all pods on new revision but none available, progressing condition false is Degraded",
			deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(2)},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 1,
					UpdatedReplicas:    2,
					AvailableReplicas:  0,
					Conditions: []appsv1.DeploymentCondition{
						{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse},
					},
				},
			},
			want: openchoreov1alpha1.HealthStatusDegraded,
		},
		{
			name: "all pods on new revision but none available, progressing condition unknown is Degraded",
			deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(2)},
				Status: appsv1.DeploymentStatus{
					ObservedGeneration: 1,
					UpdatedReplicas:    2,
					AvailableReplicas:  0,
					Conditions: []appsv1.DeploymentCondition{
						{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionUnknown},
					},
				},
			},
			want: openchoreov1alpha1.HealthStatusDegraded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := makeDeployment(tt.deployment)
			got, err := getDeploymentHealth(obj)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────
// getStatefulSetHealth
// ─────────────────────────────────────────────────────────────

func TestGetStatefulSetHealth(t *testing.T) {
	makeSTS := func(s appsv1.StatefulSet) *unstructured.Unstructured {
		return toUnstructured(t, &s)
	}

	tests := []struct {
		name        string
		statefulSet appsv1.StatefulSet
		want        openchoreov1alpha1.HealthStatus
	}{
		{
			name: "new statefulset (ObservedGeneration==0) is Progressing",
			statefulSet: appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       appsv1.StatefulSetSpec{Replicas: int32Ptr(2)},
				Status:     appsv1.StatefulSetStatus{ObservedGeneration: 0},
			},
			want: openchoreov1alpha1.HealthStatusProgressing,
		},
		{
			name: "generation ahead of observed is Progressing",
			statefulSet: appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 3},
				Spec:       appsv1.StatefulSetSpec{Replicas: int32Ptr(1)},
				Status:     appsv1.StatefulSetStatus{ObservedGeneration: 2},
			},
			want: openchoreov1alpha1.HealthStatusProgressing,
		},
		{
			name: "update in progress (revision mismatch) is Progressing",
			statefulSet: appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       appsv1.StatefulSetSpec{Replicas: int32Ptr(2)},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 1,
					CurrentRevision:    "rev-1",
					UpdateRevision:     "rev-2",
				},
			},
			want: openchoreov1alpha1.HealthStatusProgressing,
		},
		{
			name: "all replicas ready and updated is Healthy",
			statefulSet: appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       appsv1.StatefulSetSpec{Replicas: int32Ptr(3)},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 1,
					CurrentRevision:    "rev-1",
					UpdateRevision:     "rev-1",
					ReadyReplicas:      3,
					AvailableReplicas:  3,
					UpdatedReplicas:    3,
				},
			},
			want: openchoreov1alpha1.HealthStatusHealthy,
		},
		{
			name: "partially ready is Progressing",
			statefulSet: appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Spec:       appsv1.StatefulSetSpec{Replicas: int32Ptr(3)},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 1,
					CurrentRevision:    "rev-1",
					UpdateRevision:     "rev-1",
					ReadyReplicas:      1,
					AvailableReplicas:  1,
					UpdatedReplicas:    3,
				},
			},
			want: openchoreov1alpha1.HealthStatusProgressing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := makeSTS(tt.statefulSet)
			got, err := getStatefulSetHealth(obj)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────
// getPodHealth
// ─────────────────────────────────────────────────────────────

func TestGetPodHealth(t *testing.T) {
	makePod := func(p corev1.Pod) *unstructured.Unstructured {
		return toUnstructured(t, &p)
	}

	tests := []struct {
		name string
		pod  corev1.Pod
		want openchoreov1alpha1.HealthStatus
	}{
		{
			name: "pending pod is Progressing",
			pod:  corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodPending}},
			want: openchoreov1alpha1.HealthStatusProgressing,
		},
		{
			name: "running pod with all containers ready is Healthy",
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{Ready: true},
						{Ready: true},
					},
				},
			},
			want: openchoreov1alpha1.HealthStatusHealthy,
		},
		{
			name: "running pod with not-ready container is Progressing",
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{Ready: true},
						{Ready: false},
					},
				},
			},
			want: openchoreov1alpha1.HealthStatusProgressing,
		},
		{
			name: "running pod with CrashLoopBackOff is Degraded",
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
							},
						},
					},
				},
			},
			want: openchoreov1alpha1.HealthStatusDegraded,
		},
		{
			name: "running pod with ImagePullBackOff is Degraded",
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"},
							},
						},
					},
				},
			},
			want: openchoreov1alpha1.HealthStatusDegraded,
		},
		{
			name: "running pod with ErrImagePull is Degraded",
			pod: corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{Reason: "ErrImagePull"},
							},
						},
					},
				},
			},
			want: openchoreov1alpha1.HealthStatusDegraded,
		},
		{
			name: "succeeded pod is Healthy",
			pod:  corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodSucceeded}},
			want: openchoreov1alpha1.HealthStatusHealthy,
		},
		{
			name: "failed pod is Degraded",
			pod:  corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodFailed}},
			want: openchoreov1alpha1.HealthStatusDegraded,
		},
		{
			name: "unknown pod phase is Unknown",
			pod:  corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodUnknown}},
			want: openchoreov1alpha1.HealthStatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := makePod(tt.pod)
			got, err := getPodHealth(obj)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────
// getCronJobHealth
// ─────────────────────────────────────────────────────────────

func TestGetCronJobHealth(t *testing.T) {
	now := metav1.Now()
	makeCJ := func(cj batchv1.CronJob) *unstructured.Unstructured {
		return toUnstructured(t, &cj)
	}

	tests := []struct {
		name    string
		cronJob batchv1.CronJob
		want    openchoreov1alpha1.HealthStatus
	}{
		{
			name: "suspended cronjob is Suspended",
			cronJob: batchv1.CronJob{
				Spec: batchv1.CronJobSpec{Suspend: boolPtr(true)},
			},
			want: openchoreov1alpha1.HealthStatusSuspended,
		},
		{
			name: "active jobs means Progressing",
			cronJob: batchv1.CronJob{
				Spec: batchv1.CronJobSpec{Suspend: boolPtr(false)},
				Status: batchv1.CronJobStatus{
					Active: []corev1.ObjectReference{{Name: "job-1"}},
				},
			},
			want: openchoreov1alpha1.HealthStatusProgressing,
		},
		{
			name: "has last schedule time with no active jobs is Healthy",
			cronJob: batchv1.CronJob{
				Status: batchv1.CronJobStatus{
					LastScheduleTime: &now,
				},
			},
			want: openchoreov1alpha1.HealthStatusHealthy,
		},
		{
			name: "never run and not suspended is Progressing",
			cronJob: batchv1.CronJob{
				Spec:   batchv1.CronJobSpec{},
				Status: batchv1.CronJobStatus{},
			},
			want: openchoreov1alpha1.HealthStatusProgressing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := makeCJ(tt.cronJob)
			got, err := getCronJobHealth(obj)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────
// getUnknownResourceHealth
// ─────────────────────────────────────────────────────────────

func TestGetUnknownResourceHealth(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"})
	obj.SetName("test")

	health, err := getUnknownResourceHealth(obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if health != openchoreov1alpha1.HealthStatusHealthy {
		t.Errorf("expected Healthy, got %s", health)
	}
}

// ─────────────────────────────────────────────────────────────
// makeDesiredResources
// ─────────────────────────────────────────────────────────────

func TestMakeDesiredResources(t *testing.T) {
	r := &Reconciler{}

	t.Run("empty resources returns empty slice", func(t *testing.T) {
		release := &openchoreov1alpha1.RenderedRelease{
			ObjectMeta: metav1.ObjectMeta{Name: "r1", Namespace: "default", UID: "uid-1"},
		}
		result, err := r.makeDesiredResources(release)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected 0, got %d", len(result))
		}
	})

	t.Run("resource with valid JSON gets tracking labels", func(t *testing.T) {
		release := &openchoreov1alpha1.RenderedRelease{
			ObjectMeta: metav1.ObjectMeta{Name: "my-release", Namespace: "cp-ns", UID: "release-uid-abc"},
			Spec: openchoreov1alpha1.RenderedReleaseSpec{
				Resources: []openchoreov1alpha1.Resource{
					{
						ID:     "res-configmap",
						Object: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm1","namespace":"dp-ns"}}`)},
					},
				},
			},
		}
		result, err := r.makeDesiredResources(release)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1, got %d", len(result))
		}

		lbls := result[0].GetLabels()
		checks := map[string]string{
			labels.LabelKeyManagedBy:                 ControllerName,
			labels.LabelKeyRenderedReleaseResourceID: "res-configmap",
			labels.LabelKeyRenderedReleaseUID:        "release-uid-abc",
			labels.LabelKeyRenderedReleaseName:       "my-release",
			labels.LabelKeyRenderedReleaseNamespace:  "cp-ns",
		}
		for key, want := range checks {
			if got := lbls[key]; got != want {
				t.Errorf("label %s: expected %q, got %q", key, want, got)
			}
		}
	})

	t.Run("existing labels in resource are preserved", func(t *testing.T) {
		release := &openchoreov1alpha1.RenderedRelease{
			ObjectMeta: metav1.ObjectMeta{Name: "r2", Namespace: "ns", UID: "uid-2"},
			Spec: openchoreov1alpha1.RenderedReleaseSpec{
				Resources: []openchoreov1alpha1.Resource{
					{
						ID:     "res-x",
						Object: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm","labels":{"app":"myapp"}}}`)},
					},
				},
			},
		}
		result, err := r.makeDesiredResources(release)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result[0].GetLabels()["app"] != "myapp" {
			t.Error("existing label not preserved")
		}
		if result[0].GetLabels()[labels.LabelKeyManagedBy] != ControllerName {
			t.Error("tracking label not added")
		}
	})

	t.Run("resource with invalid JSON returns error", func(t *testing.T) {
		release := &openchoreov1alpha1.RenderedRelease{
			Spec: openchoreov1alpha1.RenderedReleaseSpec{
				Resources: []openchoreov1alpha1.Resource{
					{
						ID:     "bad",
						Object: &runtime.RawExtension{Raw: []byte(`not json`)},
					},
				},
			},
		}
		_, err := r.makeDesiredResources(release)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("multiple resources all get tracking labels", func(t *testing.T) {
		release := &openchoreov1alpha1.RenderedRelease{
			ObjectMeta: metav1.ObjectMeta{Name: "r3", Namespace: "ns3", UID: "uid-3"},
			Spec: openchoreov1alpha1.RenderedReleaseSpec{
				Resources: []openchoreov1alpha1.Resource{
					{ID: "a", Object: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"a"}}`)}},
					{ID: "b", Object: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"b"}}`)}},
				},
			},
		}
		result, err := r.makeDesiredResources(release)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("expected 2, got %d", len(result))
		}
		for _, obj := range result {
			if obj.GetLabels()[labels.LabelKeyManagedBy] != ControllerName {
				t.Errorf("tracking label missing on %s", obj.GetName())
			}
		}
	})
}

// ─────────────────────────────────────────────────────────────
// makeDesiredNamespaces
// ─────────────────────────────────────────────────────────────

func TestMakeDesiredNamespaces(t *testing.T) {
	r := &Reconciler{}
	release := &openchoreov1alpha1.RenderedRelease{
		ObjectMeta: metav1.ObjectMeta{Name: "my-release", Namespace: "cp-ns", UID: "uid-xyz"},
		Spec: openchoreov1alpha1.RenderedReleaseSpec{
			EnvironmentName: "prod-env",
			Owner:           openchoreov1alpha1.RenderedReleaseOwner{ProjectName: "proj-alpha"},
		},
	}

	t.Run("cluster-scoped resources produce no namespaces", func(t *testing.T) {
		obj := &unstructured.Unstructured{}
		obj.SetNamespace("") // cluster-scoped
		result := r.makeDesiredNamespaces(release, []*unstructured.Unstructured{obj})
		if len(result) != 0 {
			t.Errorf("expected 0 namespaces, got %d", len(result))
		}
	})

	t.Run("empty resources returns empty", func(t *testing.T) {
		result := r.makeDesiredNamespaces(release, nil)
		if len(result) != 0 {
			t.Errorf("expected 0, got %d", len(result))
		}
	})

	t.Run("namespaced resource produces namespace with correct labels", func(t *testing.T) {
		obj := &unstructured.Unstructured{}
		obj.SetNamespace("dp-target-ns")
		result := r.makeDesiredNamespaces(release, []*unstructured.Unstructured{obj})
		if len(result) != 1 {
			t.Fatalf("expected 1 namespace, got %d", len(result))
		}
		ns := result[0]
		if ns.Name != "dp-target-ns" {
			t.Errorf("expected namespace name dp-target-ns, got %s", ns.Name)
		}
		checkLabels := map[string]string{
			labels.LabelKeyCreatedBy:                ControllerName,
			labels.LabelKeyRenderedReleaseName:      "my-release",
			labels.LabelKeyRenderedReleaseNamespace: "cp-ns",
			labels.LabelKeyRenderedReleaseUID:       "uid-xyz",
			labels.LabelKeyNamespaceName:            "cp-ns",
			labels.LabelKeyEnvironmentName:          "prod-env",
			labels.LabelKeyProjectName:              "proj-alpha",
		}
		for key, want := range checkLabels {
			if got := ns.Labels[key]; got != want {
				t.Errorf("namespace label %s: expected %q, got %q", key, want, got)
			}
		}
	})

	t.Run("multiple resources in same namespace produces one namespace", func(t *testing.T) {
		obj1 := &unstructured.Unstructured{}
		obj1.SetNamespace("shared-ns")
		obj2 := &unstructured.Unstructured{}
		obj2.SetNamespace("shared-ns")
		result := r.makeDesiredNamespaces(release, []*unstructured.Unstructured{obj1, obj2})
		if len(result) != 1 {
			t.Errorf("expected 1 unique namespace, got %d", len(result))
		}
	})

	t.Run("resources in different namespaces produce separate namespace objects", func(t *testing.T) {
		obj1 := &unstructured.Unstructured{}
		obj1.SetNamespace("ns-1")
		obj2 := &unstructured.Unstructured{}
		obj2.SetNamespace("ns-2")
		result := r.makeDesiredNamespaces(release, []*unstructured.Unstructured{obj1, obj2})
		if len(result) != 2 {
			t.Errorf("expected 2 namespaces, got %d", len(result))
		}
	})
}

// ─────────────────────────────────────────────────────────────
// NewRenderedReleaseFinalizingCondition / NewRenderedReleaseCleanupFailedCondition
// ─────────────────────────────────────────────────────────────

func TestNewRenderedReleaseFinalizingCondition(t *testing.T) {
	cond := NewRenderedReleaseFinalizingCondition(42)

	if cond.Type != string(ConditionFinalizing) {
		t.Errorf("expected type %s, got %s", ConditionFinalizing, cond.Type)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected status True, got %s", cond.Status)
	}
	if cond.Reason != string(ReasonCleanupInProgress) {
		t.Errorf("expected reason %s, got %s", ReasonCleanupInProgress, cond.Reason)
	}
	if cond.ObservedGeneration != 42 {
		t.Errorf("expected generation 42, got %d", cond.ObservedGeneration)
	}
}

func TestNewRenderedReleaseCleanupFailedCondition(t *testing.T) {
	testErr := fmt.Errorf("connection refused")
	cond := NewRenderedReleaseCleanupFailedCondition(7, testErr)

	if cond.Type != string(ConditionFinalizing) {
		t.Errorf("expected type %s, got %s", ConditionFinalizing, cond.Type)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected status True, got %s", cond.Status)
	}
	if cond.Reason != string(ReasonCleanupFailed) {
		t.Errorf("expected reason %s, got %s", ReasonCleanupFailed, cond.Reason)
	}
	if cond.Message != testErr.Error() {
		t.Errorf("expected message %q, got %q", testErr.Error(), cond.Message)
	}
	if cond.ObservedGeneration != 7 {
		t.Errorf("expected generation 7, got %d", cond.ObservedGeneration)
	}
}

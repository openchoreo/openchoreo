// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func predicateTestPlane() *openchoreov1alpha1.DataPlane {
	return &openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{
		Name: "dp", Namespace: "ns", Generation: 1, ResourceVersion: "1",
		Finalizers: []string{"openchoreo.dev/dataplane-cleanup"},
	}}
}

func TestPlaneChangedPredicate_Update(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name   string
		mutate func(*openchoreov1alpha1.DataPlane)
		want   bool
	}{
		{
			// The reason the predicate exists: a plane controller writes agent
			// connection status on every refresh, and reconciling its own write
			// doubles the work for every plane CR.
			name: "status-only write is dropped",
			mutate: func(dp *openchoreov1alpha1.DataPlane) {
				dp.ResourceVersion = "2"
				dp.Status.AgentConnection = &openchoreov1alpha1.AgentConnectionStatus{Connected: true, ConnectedAgents: 2}
			},
			want: false,
		},
		{
			// A changed client CA must reach the gateway without waiting for
			// the next refresh.
			name:   "spec change passes",
			mutate: func(dp *openchoreov1alpha1.DataPlane) { dp.Generation = 2 },
			want:   true,
		},
		{
			name:   "label change passes",
			mutate: func(dp *openchoreov1alpha1.DataPlane) { dp.Labels = map[string]string{"team": "a"} },
			want:   true,
		},
		{
			name:   "annotation change passes",
			mutate: func(dp *openchoreov1alpha1.DataPlane) { dp.Annotations = map[string]string{"note": "x"} },
			want:   true,
		},
		{
			// Neither of the next two moves metadata.generation. Dropping them
			// would leave a deleted plane holding its finalizer, and its
			// namespace terminating, until a periodic refresh came round.
			name:   "marking for deletion passes",
			mutate: func(dp *openchoreov1alpha1.DataPlane) { dp.DeletionTimestamp = &now },
			want:   true,
		},
		{
			name:   "finalizer change passes",
			mutate: func(dp *openchoreov1alpha1.DataPlane) { dp.Finalizers = nil },
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated := predicateTestPlane()
			tt.mutate(updated)
			got := PlaneChangedPredicate().Update(event.UpdateEvent{ObjectOld: predicateTestPlane(), ObjectNew: updated})
			assert.Equal(t, tt.want, got)
		})
	}
}

// A new plane CR is notified to the gateway by its first reconcile, and a
// removed one is cleaned up by its last, so the filter must only ever apply to
// updates.
func TestPlaneChangedPredicate_CreateAndDeletePass(t *testing.T) {
	p := PlaneChangedPredicate()
	assert.True(t, p.Create(event.CreateEvent{Object: predicateTestPlane()}))
	assert.True(t, p.Delete(event.DeleteEvent{Object: predicateTestPlane()}))
}

// Plane CRs created together are requeued together. Unless each requeue lands
// somewhere different inside the window, they refresh in the same instant every
// interval for as long as they exist.
func TestStatusRequeueAfter_SpreadsWithinJitter(t *testing.T) {
	seen := map[time.Duration]struct{}{}
	for range 200 {
		d := StatusRequeueAfter()
		require.GreaterOrEqual(t, d, StatusUpdateInterval, "a refresh must not come sooner than the interval")
		require.Less(t, d, StatusUpdateInterval+StatusUpdateJitter)
		seen[d] = struct{}{}
	}
	assert.Greater(t, len(seen), 1, "every requeue got the same delay")
}

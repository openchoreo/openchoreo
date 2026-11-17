// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestGetDefaultNotificationChannelName(t *testing.T) {
	const ns, env = "default", "development"
	t0 := metav1.NewTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	t1 := metav1.NewTime(t0.Add(time.Hour))

	channel := func(name, environment string, isDefault bool, created metav1.Time) *openchoreov1alpha1.ObservabilityAlertsNotificationChannel {
		return &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, CreationTimestamp: created},
			Spec: openchoreov1alpha1.ObservabilityAlertsNotificationChannelSpec{
				Environment:  environment,
				IsEnvDefault: isDefault,
			},
		}
	}
	deleting := func(ch *openchoreov1alpha1.ObservabilityAlertsNotificationChannel) *openchoreov1alpha1.ObservabilityAlertsNotificationChannel {
		ch.DeletionTimestamp = &t1
		ch.Finalizers = []string{"test/hold"}
		return ch
	}

	cases := []struct {
		name     string
		channels []client.Object
		want     string
	}{
		{"no_channels", nil, ""},
		{"none_marked", []client.Object{channel("a", env, false, t0)}, ""},
		{"one_marked", []client.Object{channel("a", env, false, t0), channel("b", env, true, t1)}, "b"},
		{"several_marked_newest_wins", []client.Object{channel("z-old", env, true, t0), channel("a-new", env, true, t1)}, "a-new"},
		{"several_marked_newest_wins_regardless_of_name", []client.Object{channel("a-old", env, true, t0), channel("z-new", env, true, t1)}, "z-new"},
		{"same_creation_time_name_breaks_tie", []client.Object{channel("b", env, true, t0), channel("a", env, true, t0)}, "a"},
		{"marked_but_deleting_is_skipped", []client.Object{channel("old", env, true, t0), deleting(channel("new", env, true, t1))}, "old"},
		{"marked_in_another_environment_is_ignored", []client.Object{channel("other", "staging", true, t1)}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))
			r := &Reconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(tc.channels...).Build()}

			got, err := r.getDefaultNotificationChannelName(context.Background(), ns, env)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

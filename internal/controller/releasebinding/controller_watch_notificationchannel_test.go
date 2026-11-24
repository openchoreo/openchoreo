// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func notificationChannel(ns, name, environment string, marked, deleting bool) *openchoreov1alpha1.ObservabilityAlertsNotificationChannel {
	ch := &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec: openchoreov1alpha1.ObservabilityAlertsNotificationChannelSpec{
			Environment:  environment,
			IsEnvDefault: marked,
		},
	}
	if deleting {
		now := metav1.NewTime(time.Now())
		ch.DeletionTimestamp = &now
	}
	return ch
}

// TestNotificationChannelDefaultChangedPredicate: only events that can change which channel is
// the environment's default re-render the bindings.
func TestNotificationChannelDefaultChangedPredicate(t *testing.T) {
	p := notificationChannelDefaultChangedPredicate()
	ch := func(marked, deleting bool) *openchoreov1alpha1.ObservabilityAlertsNotificationChannel {
		return notificationChannel("org", "ch", "dev", marked, deleting)
	}

	assert.True(t, p.Create(event.CreateEvent{Object: ch(true, false)}), "creating a marked channel should pass")
	assert.False(t, p.Create(event.CreateEvent{Object: ch(false, false)}), "creating an unmarked channel should be ignored")
	assert.True(t, p.Delete(event.DeleteEvent{Object: ch(true, true)}), "deleting a marked channel should pass")
	assert.False(t, p.Delete(event.DeleteEvent{Object: ch(false, true)}), "deleting an unmarked channel should be ignored")
	assert.False(t, p.Generic(event.GenericEvent{Object: ch(true, false)}), "generic should be ignored")

	cases := []struct {
		name     string
		old, new *openchoreov1alpha1.ObservabilityAlertsNotificationChannel
		want     bool
	}{
		{"marked", ch(false, false), ch(true, false), true},
		{"unmarked", ch(true, false), ch(false, false), true},
		{"marked_channel_deletion_started", ch(true, false), ch(true, true), true},
		{"unmarked_channel_deletion_started", ch(false, false), ch(false, true), false},
		{"marked_channel_other_update", ch(true, false), ch(true, false), false},
		{"unmarked_channel_other_update", ch(false, false), ch(false, false), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, p.Update(event.UpdateEvent{ObjectOld: tc.old, ObjectNew: tc.new}))
		})
	}
}

// TestFindReleaseBindingsForNotificationChannel: a channel change enqueues the bindings of the
// channel's environment in the channel's namespace, and no others.
func TestFindReleaseBindingsForNotificationChannel(t *testing.T) {
	r := newReconcilerWith(t,
		bindingFor("org", "rb-dev-1", "dev"),
		bindingFor("org", "rb-dev-2", "dev"),
		bindingFor("org", "rb-prod", "prod"),
		bindingFor("other-org", "rb-other-dev", "dev"),
	)

	reqs := r.findReleaseBindingsForNotificationChannel(context.Background(),
		notificationChannel("org", "ch", "dev", true, false))
	assert.ElementsMatch(t, []string{"rb-dev-1", "rb-dev-2"}, requestNames(reqs))

	reqs = r.findReleaseBindingsForNotificationChannel(context.Background(),
		notificationChannel("org", "ch", "staging", true, false))
	assert.Empty(t, reqs, "an environment without bindings yields nothing")
}

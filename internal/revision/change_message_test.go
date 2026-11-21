// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package revision

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestChangeMessage(t *testing.T) {
	key := openchoreov1alpha1.AnnotationKeyChangeMessage
	assert.Empty(t, ChangeMessage(nil))
	assert.Empty(t, ChangeMessage(map[string]string{key: "   "}))
	assert.Equal(t, "Fix checkout timeout", ChangeMessage(map[string]string{key: "  Fix checkout timeout\n"}))

	long := strings.Repeat("é", MaxChangeMessageLength+10)
	got := ChangeMessage(map[string]string{key: long})
	assert.Equal(t, MaxChangeMessageLength, len([]rune(got)), "cut to the limit in characters, not bytes")
}

func TestReleaseBindingSnapshotter_ChangeMessageIsRecordedAndRemoved(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	binding.Annotations = map[string]string{
		openchoreov1alpha1.AnnotationKeyChangeMessage: "  First deploy of the new checkout  ",
		"example.com/keep":                            "yes",
	}
	c := newTestClient(t, binding.DeepCopy())
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(binding), binding))
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{})

	_, err := s.Record(ctx, binding)
	require.NoError(t, err)
	assert.Equal(t, "First deploy of the new checkout", getRBR(t, c, "frontend-development-r1").Spec.Message)

	stored := &openchoreov1alpha1.ReleaseBinding{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(binding), stored))
	assert.Equal(t, map[string]string{"example.com/keep": "yes"}, stored.Annotations,
		"the message is removed once recorded; other annotations stay")

	// The next change without a message records none: the old one is not reused.
	bump(binding, newRelease)
	_, err = s.Record(ctx, binding)
	require.NoError(t, err)
	assert.Empty(t, getRBR(t, c, "frontend-development-r2").Spec.Message)
}

func TestReleaseBindingSnapshotter_ReactivationPutsTheMessageOnTheEvent(t *testing.T) {
	ctx := context.Background()
	s, binding, c, recorder := reactivationFixture(t)
	bump(binding, func(spec *openchoreov1alpha1.ReleaseBindingSpec) { spec.ReleaseName = firstRelease })
	binding.Annotations = map[string]string{openchoreov1alpha1.AnnotationKeyChangeMessage: "Payments broken in r2"}
	require.NoError(t, c.Update(ctx, binding))

	res, err := s.Record(ctx, binding)
	require.NoError(t, err)
	assert.True(t, res.Reactivated)
	assert.Equal(t, `Normal RevisionReactivated Revision 1 (frontend-development-r1) is active again: `+
		`the binding spec matches the one it recorded. Message: "Payments broken in r2"`, <-recorder.Events)
	assert.Empty(t, getRBR(t, c, "frontend-development-r1").Spec.Message, "an existing revision is never changed")
	assert.NotContains(t, binding.Annotations, openchoreov1alpha1.AnnotationKeyChangeMessage)
}

func TestReleaseBindingSnapshotter_StaleBindingKeepsTheChangeMessage(t *testing.T) {
	ctx := context.Background()
	binding := newBinding("frontend-development")
	c := newTestClient(t, binding.DeepCopy())
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(binding), binding))
	s := NewReleaseBindingSnapshotter(c, newTestScheme(t), Options{})
	_, err := s.Record(ctx, binding)
	require.NoError(t, err)

	// A cached read older than the active revision, whose spec is not recorded:
	// it records nothing, so the message must stay for the reconcile that does.
	stale := binding.DeepCopy()
	stale.Spec.ReleaseName = secondRelease
	stale.Annotations = map[string]string{openchoreov1alpha1.AnnotationKeyChangeMessage: "Upgrade"}
	_, err = s.Record(ctx, stale)
	require.NoError(t, err)
	assert.Contains(t, stale.Annotations, openchoreov1alpha1.AnnotationKeyChangeMessage)
}

func TestResourceReleaseBindingSnapshotter_ChangeMessage(t *testing.T) {
	ctx := context.Background()
	dev := newResourceBinding("db-development", "development", "db-abc123")
	dev.Annotations = map[string]string{openchoreov1alpha1.AnnotationKeyChangeMessage: "Bigger instance"}
	c := newTestClient(t, dev)
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(dev), dev))

	_, err := NewResourceReleaseBindingSnapshotter(c, newTestScheme(t), Options{}).Record(ctx, dev)
	require.NoError(t, err)
	r1 := &openchoreov1alpha1.ResourceReleaseBindingRevision{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "db-development-r1"}, r1))
	assert.Equal(t, "Bigger instance", r1.Spec.Message)
	assert.NotContains(t, dev.Annotations, openchoreov1alpha1.AnnotationKeyChangeMessage)
}

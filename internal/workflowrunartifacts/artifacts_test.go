// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrunartifacts

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func validArtifact() v1alpha1.WorkflowRunInputArtifact {
	return v1alpha1.WorkflowRunInputArtifact{
		Name: "unified-diff", GCS: v1alpha1.WorkflowRunInputArtifactGCS{
			Bucket: "workflow-inputs", Key: "unified-diff/delivery-123/" + digest + ".diff",
		},
		MediaType: "text/x-diff", SizeBytes: 42, SHA256: digest, ExpiresAt: metav1.NewTime(time.Now().Add(time.Hour).UTC()),
	}
}

func TestValidate(t *testing.T) {
	t.Run("accepts immutable trusted artifact", func(t *testing.T) {
		require.NoError(t, Validate([]v1alpha1.WorkflowRunInputArtifact{validArtifact()}, time.Now()))
	})
	for name, mutate := range map[string]func(*v1alpha1.WorkflowRunInputArtifact){
		"wrong artifact name": func(a *v1alpha1.WorkflowRunInputArtifact) { a.Name = "other" },
		"wrong media type":    func(a *v1alpha1.WorkflowRunInputArtifact) { a.MediaType = "application/octet-stream" },
		"checksum mismatch": func(a *v1alpha1.WorkflowRunInputArtifact) {
			a.GCS.Key = "unified-diff/delivery-123/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.diff"
		},
		"invalid SHA": func(a *v1alpha1.WorkflowRunInputArtifact) { a.SHA256 = "not-a-sha" },
		"expired":     func(a *v1alpha1.WorkflowRunInputArtifact) { a.ExpiresAt = metav1.NewTime(time.Now().Add(-time.Minute)) },
		"size exceeded": func(a *v1alpha1.WorkflowRunInputArtifact) {
			a.SizeBytes = v1alpha1.WorkflowRunInputArtifactMaxSizeBytes + 1
		},
		"expiry exceeds seven days": func(a *v1alpha1.WorkflowRunInputArtifact) {
			a.ExpiresAt = metav1.NewTime(time.Now().Add(7*24*time.Hour + time.Minute))
		},
		"non-UTC expiry": func(a *v1alpha1.WorkflowRunInputArtifact) {
			a.ExpiresAt = metav1.NewTime(time.Now().In(time.FixedZone("-03", -3*60*60)).Add(time.Hour))
		},
		"invalid bucket": func(a *v1alpha1.WorkflowRunInputArtifact) {
			a.GCS.Bucket = "invalid/bucket"
		},
	} {
		t.Run(name, func(t *testing.T) {
			a := validArtifact()
			mutate(&a)
			require.Error(t, Validate([]v1alpha1.WorkflowRunInputArtifact{a}, time.Now()))
		})
	}
	t.Run("duplicate name", func(t *testing.T) {
		a := validArtifact()
		require.ErrorContains(t, Validate([]v1alpha1.WorkflowRunInputArtifact{a, a}, time.Now()), "duplicated")
	})
	t.Run("absence is backward compatible", func(t *testing.T) {
		require.NoError(t, Validate(nil, time.Now()))
	})
	t.Run("GCS URI fragments are rejected", func(t *testing.T) {
		a := validArtifact()
		a.GCS.Key += "#1"
		require.Error(t, Validate([]v1alpha1.WorkflowRunInputArtifact{a}, time.Now()))
	})
}

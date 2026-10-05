// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrunartifacts

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func validArtifact() v1alpha1.WorkflowRunInputArtifact {
	return v1alpha1.WorkflowRunInputArtifact{Name: "unified-diff", URI: "s3://openchoreo-workflow-inputs/sha256/" + digest, MediaType: "text/x-diff", SizeBytes: 42, SHA256: digest, ExpiresAt: metav1.NewTime(time.Now().Add(time.Hour))}
}

func TestValidate(t *testing.T) {
	t.Run("accepts immutable trusted artifact", func(t *testing.T) {
		require.NoError(t, Validate([]v1alpha1.WorkflowRunInputArtifact{validArtifact()}, time.Now()))
	})
	for name, mutate := range map[string]func(*v1alpha1.WorkflowRunInputArtifact){
		"untrusted URI": func(a *v1alpha1.WorkflowRunInputArtifact) { a.URI = "https://attacker.example/diff" },
		"checksum mismatch": func(a *v1alpha1.WorkflowRunInputArtifact) {
			a.URI = "s3://openchoreo-workflow-inputs/sha256/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		},
		"expired": func(a *v1alpha1.WorkflowRunInputArtifact) { a.ExpiresAt = metav1.NewTime(time.Now().Add(-time.Minute)) },
		"size exceeded": func(a *v1alpha1.WorkflowRunInputArtifact) {
			a.SizeBytes = v1alpha1.WorkflowRunInputArtifactMaxSizeBytes + 1
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
	t.Run("credentials and tokens are rejected", func(t *testing.T) {
		a := validArtifact()
		a.URI = strings.Replace(a.URI, "s3://", "s3://token@", 1)
		require.Error(t, Validate([]v1alpha1.WorkflowRunInputArtifact{a}, time.Now()))
	})
}

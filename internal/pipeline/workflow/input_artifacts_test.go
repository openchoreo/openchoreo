// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowpipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const artifactDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestInjectInputArtifacts(t *testing.T) {
	resource := map[string]any{"apiVersion": "argoproj.io/v1alpha1", "kind": "Workflow", "spec": map[string]any{}}
	artifacts := []v1alpha1.WorkflowRunInputArtifact{{Name: "unified-diff", GCS: v1alpha1.WorkflowRunInputArtifactGCS{Bucket: "workflow-inputs", Key: "unified-diff/delivery-123/" + artifactDigest + ".diff"}, MediaType: "text/x-diff", SizeBytes: 99, SHA256: artifactDigest, ExpiresAt: metav1.NewTime(time.Now().Add(time.Hour))}}
	require.NoError(t, injectInputArtifacts(resource, artifacts))
	arguments := resource["spec"].(map[string]any)["arguments"].(map[string]any)
	got := arguments["artifacts"].([]any)[0].(map[string]any)
	assertGCS := got["gcs"].(map[string]any)
	require.Equal(t, "workflow-inputs", assertGCS["bucket"])
	require.Equal(t, "unified-diff/delivery-123/"+artifactDigest+".diff", assertGCS["key"])
	require.NotContains(t, got, "s3")
	require.NotContains(t, assertGCS, "serviceAccountKeySecret")
}

func TestInjectInputArtifactsRejectsTemplateArtifacts(t *testing.T) {
	resource := map[string]any{"spec": map[string]any{"arguments": map[string]any{"artifacts": []any{}}}}
	err := injectInputArtifacts(resource, []v1alpha1.WorkflowRunInputArtifact{{Name: "unified-diff", SHA256: artifactDigest}})
	require.ErrorContains(t, err, "spec.inputArtifacts exclusively")
}

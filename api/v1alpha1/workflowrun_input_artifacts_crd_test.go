// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkflowRunInputArtifactsCRDUsesNativeGCS(t *testing.T) {
	crd, err := os.ReadFile("../../config/crd/bases/openchoreo.dev_workflowruns.yaml")
	require.NoError(t, err)

	schema := string(crd)
	require.Contains(t, schema, "gcs:")
	require.Contains(t, schema, "- bucket")
	require.Contains(t, schema, "- key")
	require.Contains(t, schema, "maximum: 1048576")
	require.Contains(t, schema, "pattern: ^[a-f0-9]{64}$")
	require.Contains(t, schema, "format: date-time")
	require.Contains(t, schema, "- expiresAt")
	require.Contains(t, schema, "- mediaType")
	require.Contains(t, schema, "- sha256")
	require.Contains(t, schema, "- sizeBytes")
	require.Contains(t, schema, "- unified-diff")
	require.NotContains(t, strings.ToLower(schema), "s3:")
	require.NotContains(t, schema, "useSDKCreds")
}

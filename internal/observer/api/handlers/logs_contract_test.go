// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestLogsQueryContainerNameSchema(t *testing.T) {
	t.Parallel()

	_, testFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "failed to locate test source file")
	repoRoot := filepath.Join(filepath.Dir(testFile), "..", "..", "..", "..")

	for _, specFile := range []string{
		"observer-api.yaml",
		"observability-logs-adapter-api.yaml",
	} {
		t.Run(specFile, func(t *testing.T) {
			t.Parallel()

			loader := openapi3.NewLoader()
			spec, err := loader.LoadFromFile(filepath.Join(repoRoot, "openapi", specFile))
			require.NoError(t, err, "failed to load OpenAPI spec")

			requestRef := spec.Components.Schemas["LogsQueryRequest"]
			require.NotNil(t, requestRef, "spec must define LogsQueryRequest")
			request := requestRef.Value
			containerName := request.Properties["containerName"]
			require.NotNil(t, containerName, "LogsQueryRequest must define containerName")
			require.NotNil(t, containerName.Value)
			require.NotContains(t, request.Required, "containerName", "containerName must remain optional")
			require.NotNil(t, containerName.Value.MaxLength)
			require.EqualValues(t, 63, *containerName.Value.MaxLength)
			require.Equal(t, `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`, containerName.Value.Pattern)
		})
	}
}

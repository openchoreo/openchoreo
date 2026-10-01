// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	resourcepipeline "github.com/openchoreo/openchoreo/internal/pipeline/resource"
)

func TestMapResolvedOutputsPreservesEmptyValueSource(t *testing.T) {
	outputs := mapResolvedOutputs([]resourcepipeline.ResolvedOutput{
		{Name: "optionalSetting", Value: ""},
	})

	require.Len(t, outputs, 1)
	require.NotNil(t, outputs[0].Value)
	require.Equal(t, "", *outputs[0].Value)

	serialized, err := json.Marshal(outputs[0])
	require.NoError(t, err)
	require.JSONEq(t, `{"name":"optionalSetting","value":""}`, string(serialized))
}

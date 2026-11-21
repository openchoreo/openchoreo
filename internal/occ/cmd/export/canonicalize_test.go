// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalize_EquivalentDocumentsProduceIdenticalFiles(t *testing.T) {
	first := manifestResource(testComponentKind)
	second := manifestResource(testComponentKind)
	first.Object = json.RawMessage(`{
		"spec":{"parameters":{"z":true,"a":"123"},"owner":{"projectName":"store"}},
		"metadata":{"namespace":"acme","name":"sample"},
		"kind":"Component","apiVersion":"openchoreo.dev/v1alpha1"}`)
	second.Object = json.RawMessage(`{
		"apiVersion":"openchoreo.dev/v1alpha1","kind":"Component",
		"metadata":{"name":"sample","namespace":"acme"},
		"spec":{"owner":{"projectName":"store"},"parameters":{"a":"123","z":true}}}`)
	exports := make([][]SerializedFile, 0, 2)
	for _, resource := range []Resource{first, second} {
		canonical, err := Canonicalize(context.Background(), []Resource{resource})
		require.NoError(t, err)
		files, err := Serialize(context.Background(), canonical)
		require.NoError(t, err)
		exports = append(exports, files)
	}
	assert.Equal(t, exports[0], exports[1])
}

func TestCanonicalize_PreservesNumbersArraysAndOriginalDocuments(t *testing.T) {
	resource := manifestResource(testComponentKind)
	document := resource.Object.(map[string]any)
	document["spec"].(map[string]any)["parameters"] = json.RawMessage(
		`{"large":9007199254740993,"decimal":1.234567890123456789,"args":["sh","-c","./start.sh"],"empty":null}`)
	before, err := json.Marshal(document)
	require.NoError(t, err)
	canonical, err := Canonicalize(context.Background(), []Resource{resource})
	require.NoError(t, err)
	copyDocument := canonical[0].Object.(map[string]any)
	parameters := copyDocument["spec"].(map[string]any)["parameters"].(map[string]any)
	assert.Equal(t, json.Number("9007199254740993"), parameters["large"])
	assert.Equal(t, json.Number("1.234567890123456789"), parameters["decimal"])
	assert.Equal(t, []any{"sh", "-c", "./start.sh"}, parameters["args"])
	assert.Contains(t, parameters, "empty")
	assert.Nil(t, parameters["empty"])
	copyDocument["metadata"].(map[string]any)["name"] = "changed"
	after, err := json.Marshal(document)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestCanonicalize_ErrorsReturnNoPartialResult(t *testing.T) {
	for _, object := range []any{nil, []string{"not", "an", "object"}, make(chan int)} {
		resource := manifestResource(testComponentKind)
		resource.Object = object
		canonical, err := Canonicalize(context.Background(), []Resource{manifestResource("Project"), resource})
		require.Error(t, err)
		assert.Nil(t, canonical)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canonical, err := Canonicalize(ctx, nil)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, canonical)
}

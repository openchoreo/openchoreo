// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const filterFixtureJSON = `{
  "apiVersion":"openchoreo.dev/v1alpha1",
  "kind":"Component",
  "metadata":{
    "name":"backend","namespace":"acme",
    "uid":"server-uid","resourceVersion":"123","generation":7,
    "creationTimestamp":"2026-11-17T08:00:00Z",
    "deletionTimestamp":null,"deletionGracePeriodSeconds":30,
    "managedFields":[{"manager":"another-controller"}],
    "labels":{"team":"payments"},
    "annotations":{
      "kubectl.kubernetes.io/last-applied-configuration":"previous apply data",
      "openchoreo.dev/restartedAt":"2026-11-17T08:00:00Z",
      "openchoreo.dev/description":"Payments backend",
      "meta.helm.sh/release-name":"existing-release",
      "meta.helm.sh/release-namespace":"existing-namespace",
      "argocd.argoproj.io/tracking-id":"existing-application",
      "kustomize.toolkit.fluxcd.io/ssa":"Ignore"
    }
  },
  "spec":{
    "owner":{"projectName":"shop"},
    "componentType":{"kind":"ClusterComponentType","name":"deployment/service"},
    "autoDeploy":true,"autoBuild":true,
    "parameters":{
      "bigNumber":9007199254740993,"fraction":0.1234567890123456789,
      "status":"application-state","uid":"business-id",
      "metadata":{"uid":"nested-business-id","creationTimestamp":"user-value"},
      "annotations":{"openchoreo.dev/restartedAt":"keep-nested-value"},
      "enabled":false,"replicas":0,"emptyText":"","nothing":null,"emptyList":[]
    }
  },
  "status":{"observedGeneration":7}
}`

func TestFilter_ApprovedPolicyAndNumberPreservation(t *testing.T) {
	resource := Resource{
		Kind: componentKind, Name: "backend", Namespace: testExportNamespace,
		Object: json.RawMessage(filterFixtureJSON),
	}
	filtered, err := Filter(context.Background(), []Resource{resource})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, resource.Kind, filtered[0].Kind)
	assert.Equal(t, resource.Name, filtered[0].Name)
	assert.Equal(t, resource.Namespace, filtered[0].Namespace)
	data, err := json.Marshal(filtered[0].Object)
	require.NoError(t, err)
	var document map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &document))
	assert.NotContains(t, document, "status")
	assert.JSONEq(t, `{
      "name":"backend","namespace":"acme","labels":{"team":"payments"},
      "annotations":{
        "openchoreo.dev/description":"Payments backend",
        "meta.helm.sh/release-name":"existing-release",
        "meta.helm.sh/release-namespace":"existing-namespace",
        "argocd.argoproj.io/tracking-id":"existing-application",
        "kustomize.toolkit.fluxcd.io/ssa":"Ignore"
      }
    }`, string(document["metadata"]))
	var original map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(filterFixtureJSON), &original))
	wantSpec, err := json.Marshal(original["spec"])
	require.NoError(t, err)
	assert.JSONEq(t, string(wantSpec), string(document["spec"]), "preserve the entire spec, including numeric precision and nested fields")
	assert.Contains(t, string(document["spec"]), "9007199254740993")
	assert.Contains(t, string(document["spec"]), "0.1234567890123456789")
	assert.Equal(t, filterFixtureJSON, string(resource.Object.(json.RawMessage)))
}

func TestFilter_TypedInputsRemainUnchanged(t *testing.T) {
	var component gen.Component
	require.NoError(t, json.Unmarshal([]byte(filterFixtureJSON), &component))
	before, err := json.Marshal(component)
	require.NoError(t, err)
	resource := Resource{Kind: componentKind, Name: component.Metadata.Name, Namespace: testExportNamespace, Object: component}
	filtered, err := Filter(context.Background(), []Resource{resource})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	document, ok := filtered[0].Object.(resourceDocument)
	require.True(t, ok)
	// Editing the result must not change the typed input or its nested values.
	document["metadata"].(map[string]any)["name"] = "changed"
	document["spec"].(map[string]any)["parameters"].(map[string]any)["status"] = "edited"
	after, err := json.Marshal(resource.Object)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestFilter_PreservesNestedTemplatesAndSchemas(t *testing.T) {
	resource := manifestResource(workflowKind)
	object := resource.Object.(map[string]any)
	object["spec"] = map[string]any{
		"runTemplate": map[string]any{
			"metadata": map[string]any{"annotations": map[string]any{restartedAtAnnotation: "keep-template-value"}},
		},
		"parameters": map[string]any{"openAPIV3Schema": map[string]any{
			"properties": map[string]any{"status": map[string]any{"type": "string"}, "uid": map[string]any{"type": "string"}},
		}},
	}
	before, err := json.Marshal(object["spec"])
	require.NoError(t, err)
	filtered, err := Filter(context.Background(), []Resource{resource})
	require.NoError(t, err)
	after, err := json.Marshal(filtered[0].Object.(resourceDocument)["spec"])
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after))
}

func TestFilter_DeletionMarkers(t *testing.T) {
	for _, tc := range []struct {
		name, marker string
		want         int
	}{
		{name: "absent", want: 1},
		{name: "null", marker: `,"deletionTimestamp":null`, want: 1},
		{name: "deleting", marker: `,"deletionTimestamp":"2026-11-17T08:00:00Z"`, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resource := Resource{
				Kind: namespaceKind, Name: testExportNamespace,
				Object: json.RawMessage(`{"metadata":{"name":"acme"` + tc.marker + `}}`),
			}
			filtered, err := Filter(context.Background(), []Resource{resource})
			require.NoError(t, err)
			assert.Len(t, filtered, tc.want)
		})
	}
}

func TestFilter_AnnotationContainersAndNamespaceLabels(t *testing.T) {
	for _, annotations := range []string{"", `,"annotations":null`, `,"annotations":{}`, `,"annotations":{"openchoreo.dev/restartedAt":"old"}`} {
		resource := Resource{
			Kind: namespaceKind, Name: testExportNamespace,
			Object: json.RawMessage(`{"metadata":{"name":"acme","labels":{"openchoreo.dev/control-plane":"true"}` + annotations + `}}`),
		}
		filtered, err := Filter(context.Background(), []Resource{resource})
		require.NoError(t, err)
		require.Len(t, filtered, 1)
		metadata := filtered[0].Object.(resourceDocument)["metadata"].(map[string]any)
		assert.Equal(t, map[string]any{"openchoreo.dev/control-plane": "true"}, metadata["labels"])
		annotations, _ := metadata["annotations"].(map[string]any)
		assert.NotContains(t, annotations, restartedAtAnnotation)
	}
}

func TestFilter_RejectsInvalidDocumentsWithoutPartialResults(t *testing.T) {
	for _, tc := range []struct {
		name string
		data any
		want string
	}{
		{name: "nil object", want: "API object is required"},
		{name: "unsupported value", data: make(chan int), want: "copy API object"},
		{name: "invalid JSON", data: json.RawMessage(`{`), want: "copy API object"},
		{name: "array", data: json.RawMessage(`[]`), want: "read API object"},
		{name: "missing metadata", data: json.RawMessage(`{}`), want: "read metadata"},
		{name: "null metadata", data: json.RawMessage(`{"metadata":null}`), want: "metadata is required"},
		{name: "invalid metadata", data: json.RawMessage(`{"metadata":[]}`), want: "read metadata"},
		{name: "invalid annotations", data: json.RawMessage(`{"metadata":{"annotations":[]}}`), want: "read metadata.annotations"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resource := Resource{Kind: componentKind, Name: "invalid", Namespace: testExportNamespace, Object: tc.data}
			filtered, err := Filter(context.Background(), []Resource{manifestResource(environmentKind), resource})
			require.ErrorContains(t, err, tc.want)
			assert.Contains(t, err.Error(), componentKind)
			assert.Contains(t, err.Error(), resource.Name)
			assert.Contains(t, err.Error(), resource.Namespace)
			assert.Nil(t, filtered)
		})
	}
}

type cancelDuringFilter struct {
	cancel context.CancelFunc
}

func (c cancelDuringFilter) MarshalJSON() ([]byte, error) {
	c.cancel()
	return []byte(`{"metadata":{"name":"cancelled"}}`), nil
}

func TestFilter_Cancellation(t *testing.T) {
	for _, duringCopy := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		resource := manifestResource(componentKind)
		if duringCopy {
			resource.Object = cancelDuringFilter{cancel: cancel}
		} else {
			cancel()
		}
		filtered, err := Filter(ctx, []Resource{resource})
		cancel()
		require.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, filtered)
	}
}

func TestFilter_EmptyInventory(t *testing.T) {
	filtered, err := Filter(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, filtered)
}

func TestFilter_EquivalentDocumentsProduceIdenticalFiles(t *testing.T) {
	first := manifestResource(componentKind)
	second := manifestResource(componentKind)
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
		filtered, err := Filter(context.Background(), []Resource{resource})
		require.NoError(t, err)
		files, err := Serialize(context.Background(), filtered)
		require.NoError(t, err)
		exports = append(exports, files)
	}
	assert.Equal(t, exports[0], exports[1])
}

func TestFilter_PreservesNumbersArraysAndOriginalDocuments(t *testing.T) {
	resource := manifestResource(componentKind)
	document := resource.Object.(map[string]any)
	document["spec"].(map[string]any)["parameters"] = json.RawMessage(
		`{"large":9007199254740993,"decimal":1.234567890123456789,"args":["sh","-c","./start.sh"],"empty":null}`)
	before, err := json.Marshal(document)
	require.NoError(t, err)
	filtered, err := Filter(context.Background(), []Resource{resource})
	require.NoError(t, err)
	copyDocument := filtered[0].Object.(resourceDocument)
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

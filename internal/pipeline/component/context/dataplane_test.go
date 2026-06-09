// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

// readAnnotationExpr is the defensive read pattern templates use to pull a DataPlane
// annotation from the render context, defaulting when the key (or the whole annotations
// map) is absent. The generic feature is "any template can read any DataPlane annotation
// from CEL"; this expression exercises exactly that contract.
func readAnnotationExpr(key, def string) string {
	return fmt.Sprintf(`${has(dataplane.annotations) && %q in dataplane.annotations ? dataplane.annotations[%q] : %q}`, key, key, def)
}

// dataPlaneCELMap builds the CEL "dataplane" input the way the pipeline does: extract the
// DataPlaneData from the resource, then convert it to the map[string]any CEL evaluates against.
func dataPlaneCELMap(t *testing.T, dp *v1alpha1.DataPlane) map[string]any {
	t.Helper()
	m, err := structToMap(extractDataPlaneData(dp))
	require.NoError(t, err)
	return m
}

func TestExtractDataPlaneData_Annotations(t *testing.T) {
	t.Run("surfaces_all_annotations_verbatim", func(t *testing.T) {
		dp := &v1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"example.com/team":         "platform",
					"example.com/tier":         "gold",
					"plain-key":                "v",
					"example.com/empty":        "", // empty value must still be present
					"sub.domain.example.com/x": "dotted-and-slashed",
				},
			},
		}

		data := extractDataPlaneData(dp)
		require.NotNil(t, data.Annotations)
		assert.Len(t, data.Annotations, 5)
		assert.Equal(t, "platform", data.Annotations["example.com/team"])
		assert.Equal(t, "dotted-and-slashed", data.Annotations["sub.domain.example.com/x"])

		// All keys survive the struct->map conversion CEL evaluates against.
		m := dataPlaneCELMap(t, dp)
		ann, ok := m["annotations"].(map[string]any)
		require.True(t, ok, "annotations should be present in the CEL map")
		assert.Equal(t, "platform", ann["example.com/team"])
		assert.Equal(t, "gold", ann["example.com/tier"])
		assert.Equal(t, "", ann["example.com/empty"])
		assert.Equal(t, "dotted-and-slashed", ann["sub.domain.example.com/x"])
	})

	t.Run("omitted_when_no_annotations", func(t *testing.T) {
		data := extractDataPlaneData(&v1alpha1.DataPlane{})
		assert.Nil(t, data.Annotations)

		// omitempty drops the nil map, so the CEL "dataplane" object stays {} (no fixture
		// churn) and guarded reads still resolve without a missing-key error.
		m := dataPlaneCELMap(t, &v1alpha1.DataPlane{})
		_, present := m["annotations"]
		assert.False(t, present, "nil annotations should be omitted from the CEL map")
	})
}

// TestDataPlaneAnnotations_CELRead proves arbitrary annotations are readable from CEL through
// the real template engine, across the edge cases: present key, domain-prefixed key, empty
// value, absent key (map present), and entirely-absent annotations. None of these error.
func TestDataPlaneAnnotations_CELRead(t *testing.T) {
	engine := template.NewEngine()

	withAnnotations := &v1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
		"example.com/team":  "platform",
		"example.com/empty": "",
	}}}
	noAnnotations := &v1alpha1.DataPlane{}

	cases := []struct {
		name string
		dp   *v1alpha1.DataPlane
		key  string
		want string
	}{
		{name: "present_key", dp: withAnnotations, key: "example.com/team", want: "platform"},
		{name: "empty_value_is_present", dp: withAnnotations, key: "example.com/empty", want: ""},
		{name: "absent_key_with_map_present", dp: withAnnotations, key: "example.com/missing", want: "<default>"},
		{name: "absent_annotations_map", dp: noAnnotations, key: "example.com/team", want: "<default>"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inputs := map[string]any{"dataplane": dataPlaneCELMap(t, tc.dp)}
			got, err := engine.Render(readAnnotationExpr(tc.key, "<default>"), inputs)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("direct_index_of_present_key", func(t *testing.T) {
		inputs := map[string]any{"dataplane": dataPlaneCELMap(t, withAnnotations)}
		got, err := engine.Render(`${dataplane.annotations["example.com/team"]}`, inputs)
		require.NoError(t, err)
		assert.Equal(t, "platform", got)
	})
}

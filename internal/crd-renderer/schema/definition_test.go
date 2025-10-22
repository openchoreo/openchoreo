// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schema

import "testing"

func TestApplyDefaults_ArrayFieldBehaviour(t *testing.T) {
	def := Definition{
		Types: map[string]any{
			"Item": map[string]any{
				"name": "string | default=default-name",
			},
		},
		Schemas: []map[string]any{
			{
				"list": "[]Item",
			},
		},
	}

	structural, err := ToStructural(def)
	if err != nil {
		t.Fatalf("ToStructural returned error: %v", err)
	}

	defaults := ApplyDefaults(nil, structural)
	if _, ok := defaults["list"]; ok {
		t.Fatalf("expected no default array elements when only item defaults are present, got %v", defaults["list"])
	}

	defWithArrayDefault := Definition{
		Types: map[string]any{
			"Item": map[string]any{
				"name": "string | default=default-name",
			},
		},
		Schemas: []map[string]any{
			{
				"list": "[]Item | default=[{\"name\":\"custom\"}]",
			},
		},
	}

	structural, err = ToStructural(defWithArrayDefault)
	if err != nil {
		t.Fatalf("ToStructural returned error: %v", err)
	}

	defaults = ApplyDefaults(nil, structural)
	got, ok := defaults["list"].([]any)
	if !ok {
		t.Fatalf("expected slice default, got %T (%v)", defaults["list"], defaults["list"])
	}
	if len(got) != 1 || got[0].(map[string]any)["name"] != "custom" {
		t.Fatalf("unexpected array default: %v", got)
	}
}

func TestApplyDefaults_ArrayItems(t *testing.T) {
	def := Definition{
		Types: map[string]any{
			"MountConfig": map[string]any{
				"containerName": "string | required=true",
				"mountPath":     "string | required=true",
				"readOnly":      "boolean | default=true",
				"subPath":       "string | default=\"\"",
			},
		},
		Schemas: []map[string]any{
			{
				"volumeName": "string | required=true",
				"mounts":     "[]MountConfig",
			},
		},
	}

	structural, err := ToStructural(def)
	if err != nil {
		t.Fatalf("ToStructural returned error: %v", err)
	}

	values := map[string]any{
		"volumeName": "shared",
		"mounts": []any{
			map[string]any{
				"containerName": "app",
				"mountPath":     "/var/log/app",
			},
		},
	}

	ApplyDefaults(values, structural)

	mounts, ok := values["mounts"].([]any)
	if !ok || len(mounts) != 1 {
		t.Fatalf("expected one mount after defaulting, got %v", values["mounts"])
	}

	mount, ok := mounts[0].(map[string]any)
	if !ok {
		t.Fatalf("expected mount to be a map, got %T", mounts[0])
	}

	readOnly, ok := mount["readOnly"].(bool)
	if !ok {
		t.Fatalf("expected readOnly to be a bool, got %T", mount["readOnly"])
	}
	if !readOnly {
		t.Fatalf("expected readOnly default true, got %v", readOnly)
	}

	if _, ok := mount["subPath"].(string); !ok {
		t.Fatalf("expected subPath to be a string, got %T", mount["subPath"])
	}
}

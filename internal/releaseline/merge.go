// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package releaseline holds release line logic shared by the controllers and the API server.
package releaseline

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/runtime"
)

// MergeParameters deep-merges override onto base: maps merge key by key, scalars and lists replace.
// Neither input is modified.
func MergeParameters(base, override *runtime.RawExtension) (*runtime.RawExtension, error) {
	if override == nil || len(override.Raw) == 0 {
		return base.DeepCopy(), nil
	}
	if base == nil || len(base.Raw) == 0 {
		return override.DeepCopy(), nil
	}
	var b, o map[string]any
	if err := json.Unmarshal(base.Raw, &b); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(override.Raw, &o); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(deepMerge(b, o))
	if err != nil {
		return nil, err
	}
	return &runtime.RawExtension{Raw: raw}, nil
}

func deepMerge(base, override map[string]any) map[string]any {
	if base == nil {
		base = map[string]any{}
	}
	for k, ov := range override {
		if om, ok := ov.(map[string]any); ok {
			if bm, ok := base[k].(map[string]any); ok {
				base[k] = deepMerge(bm, om)
				continue
			}
		}
		base[k] = ov
	}
	return base
}

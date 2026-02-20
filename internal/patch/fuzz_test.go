// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package patch_test

import (
	"encoding/json"
	"testing"

	"github.com/openchoreo/openchoreo/internal/patch"
)

// FuzzApplyPatches exercises the patch engine with arbitrary JSON documents and
// patch operation lists. The fuzzer should never cause a panic; errors are acceptable.
func FuzzApplyPatches(f *testing.F) {
	// Seed corpus: minimal valid inputs
	f.Add([]byte(`{}`), []byte(`[]`))
	f.Add(
		[]byte(`{"spec":{"containers":[{"name":"app","image":"nginx"}]}}`),
		[]byte(`[{"op":"replace","path":"/spec/containers/0/image","value":"alpine"}]`),
	)
	f.Add(
		[]byte(`{"metadata":{"labels":{}}}`),
		[]byte(`[{"op":"add","path":"/metadata/labels/env","value":"prod"}]`),
	)
	f.Add(
		[]byte(`{"items":[1,2,3]}`),
		[]byte(`[{"op":"remove","path":"/items/0"}]`),
	)

	f.Fuzz(func(t *testing.T, docBytes []byte, opsBytes []byte) {
		// Unmarshal the document; skip if not valid JSON object
		var doc map[string]any
		if err := json.Unmarshal(docBytes, &doc); err != nil {
			return
		}

		// Unmarshal the operations; skip if not valid JSON array
		var ops []patch.JSONPatchOperation
		if err := json.Unmarshal(opsBytes, &ops); err != nil {
			return
		}

		// The fuzzer should never cause a panic — errors are acceptable
		_ = patch.ApplyPatches(doc, ops)
	})
}

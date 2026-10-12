// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestMergeParameters(t *testing.T) {
	base := &runtime.RawExtension{Raw: []byte(`{"repository":{"url":"u","revision":{"branch":"main"}},"tags":["a","b"],"replicas":1}`)}
	override := &runtime.RawExtension{Raw: []byte(`{"repository":{"revision":{"branch":"next"}},"tags":["c"]}`)}

	got, err := MergeParameters(base, override)
	require.NoError(t, err)
	assert.JSONEq(t, `{"repository":{"url":"u","revision":{"branch":"next"}},"tags":["c"],"replicas":1}`, string(got.Raw))
	assert.JSONEq(t, `{"repository":{"url":"u","revision":{"branch":"main"}},"tags":["a","b"],"replicas":1}`, string(base.Raw),
		"base is not modified")

	only, err := MergeParameters(nil, override)
	require.NoError(t, err)
	assert.JSONEq(t, string(override.Raw), string(only.Raw))

	same, err := MergeParameters(base, nil)
	require.NoError(t, err)
	assert.JSONEq(t, string(base.Raw), string(same.Raw))

	_, err = MergeParameters(base, &runtime.RawExtension{Raw: []byte(`[1]`)})
	assert.Error(t, err)
}

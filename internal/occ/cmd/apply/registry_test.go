// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSupportedKinds(t *testing.T) {
	kinds := supportedKinds()
	require.NotEmpty(t, kinds)
	assert.IsNonDecreasing(t, kinds)
}

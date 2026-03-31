// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewApplyCmd_Flags(t *testing.T) {
	cmd := NewApplyCmd()

	assert.Equal(t, "apply", cmd.Use)
	assert.NotNil(t, cmd.Flags().Lookup("file"), "expected flag --file")
	assert.Equal(t, "f", cmd.Flags().Lookup("file").Shorthand)
}

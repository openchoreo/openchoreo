// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewVersionCmd_Structure(t *testing.T) {
	cmd := NewVersionCmd()

	assert.Equal(t, "version", cmd.Use)
	assert.Empty(t, cmd.Commands(), "version should have no subcommands")
}

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package messaging

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorMessages(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		contains string
	}{
		{"ErrInvalidMessageID", ErrInvalidMessageID, "invalid or empty message ID"},
		{"ErrInvalidMessageType", ErrInvalidMessageType, "invalid message type"},
		{"ErrInvalidAction", ErrInvalidAction, "invalid action"},
		{"ErrMissingReplyTo", ErrMissingReplyTo, "missing replyTo"},
		{"ErrRequestTimeout", ErrRequestTimeout, "request timeout"},
		{"ErrNotConnected", ErrNotConnected, "not connected"},
		{"ErrAgentNotFound", ErrAgentNotFound, "agent not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotNil(t, tt.err)
			assert.Contains(t, tt.err.Error(), tt.contains)
		})
	}
}

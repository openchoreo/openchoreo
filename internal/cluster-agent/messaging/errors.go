// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package messaging

import "errors"

var (
	ErrInvalidMessageID = errors.New("invalid or empty message ID")

	ErrInvalidMessageType = errors.New("invalid message type")

	ErrInvalidAction = errors.New("invalid action")

	ErrMissingReplyTo = errors.New("response message missing replyTo field")

	ErrRequestTimeout = errors.New("request timeout")

	ErrNotConnected = errors.New("not connected to agent server")

	ErrAgentNotFound = errors.New("agent not found")
)

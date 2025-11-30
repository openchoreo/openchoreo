// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package messaging

import (
	"time"

	"github.com/google/uuid"
)

func GenerateMessageID() string {
	return uuid.New().String()
}

func NewRequestMessage(action Action, payload map[string]interface{}) *Message {
	return &Message{
		ID:        uuid.New().String(),
		Type:      TypeRequest,
		Action:    action,
		Payload:   payload,
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

func NewResponseMessage(replyTo string, success bool, payload map[string]interface{}, errorMsg string) *Message {
	msg := &Message{
		ID:        uuid.New().String(),
		Type:      TypeResponse,
		ReplyTo:   replyTo,
		Success:   success,
		Payload:   payload,
		Timestamp: time.Now().Format(time.RFC3339),
	}
	if errorMsg != "" {
		msg.Error = errorMsg
	}
	return msg
}

func NewSuccessResponse(replyTo string, payload map[string]interface{}) *Message {
	return NewResponseMessage(replyTo, true, payload, "")
}

func NewErrorResponse(replyTo string, errorMsg string) *Message {
	return NewResponseMessage(replyTo, false, nil, errorMsg)
}

func NewHeartbeatMessage(from string, sequence int) *Message {
	return &Message{
		ID:   uuid.New().String(),
		Type: TypeHeartbeat,
		From: from,
		Payload: map[string]interface{}{
			"sequence":  sequence,
			"timestamp": time.Now().Format(time.RFC3339),
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

func NewBroadcastMessage(payload map[string]interface{}) *Message {
	return &Message{
		ID:        uuid.New().String(),
		Type:      TypeBroadcast,
		Payload:   payload,
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

func (m *Message) Validate() error {
	if m.ID == "" {
		return ErrInvalidMessageID
	}

	if !m.Type.IsValid() {
		return ErrInvalidMessageType
	}

	if m.Type == TypeRequest && !m.Action.IsValid() {
		return ErrInvalidAction
	}

	if m.Type == TypeResponse && m.ReplyTo == "" {
		return ErrMissingReplyTo
	}

	return nil
}

func (m *Message) IsRequest() bool {
	return m.Type == TypeRequest
}

func (m *Message) IsResponse() bool {
	return m.Type == TypeResponse
}

func (m *Message) IsBroadcast() bool {
	return m.Type == TypeBroadcast
}

func (m *Message) IsHeartbeat() bool {
	return m.Type == TypeHeartbeat
}

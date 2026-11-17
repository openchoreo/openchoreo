// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package notifications

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
)

// Reason classifies why a notification was not delivered. The set is fixed so that it can be
// stored, logged and later used as a metric label. A reason never carries the error text, which
// can include a webhook URL that is itself a credential.
type Reason string

const (
	// ReasonTimeout means an attempt did not finish within its timeout.
	ReasonTimeout Reason = "timeout"
	// ReasonConnection means the channel's server could not be reached.
	ReasonConnection Reason = "connection"
	// ReasonHTTP5xx means the webhook server answered with a 5xx status.
	ReasonHTTP5xx Reason = "http_5xx"
	// ReasonHTTP429 means the webhook server rate limited the request.
	ReasonHTTP429 Reason = "http_429"
	// ReasonHTTP4xx means the webhook server rejected the request with another 4xx status.
	ReasonHTTP4xx Reason = "http_4xx"
	// ReasonSMTPTemporary means the mail server answered with a 4xx reply.
	ReasonSMTPTemporary Reason = "smtp_temporary"
	// ReasonSMTPPermanent means the mail server answered with a 5xx reply.
	ReasonSMTPPermanent Reason = "smtp_permanent"
	// ReasonChannelConfig means the channel's configuration is missing or invalid.
	ReasonChannelConfig Reason = "channel_config"
	// ReasonTemplate means the notification's subject, body or payload could not be rendered.
	ReasonTemplate Reason = "template"
	// ReasonInterrupted means the observer stopped before the notification was delivered.
	ReasonInterrupted Reason = "interrupted"
	// ReasonUnknown means the failure could not be classified.
	ReasonUnknown Reason = "unknown"
)

// Retryable reports whether a failure with this reason may succeed on another attempt.
func (r Reason) Retryable() bool {
	switch r {
	case ReasonTimeout, ReasonConnection, ReasonHTTP5xx, ReasonHTTP429, ReasonSMTPTemporary:
		return true
	default:
		return false
	}
}

// DeliveryError is a notification delivery failure with its classified reason.
type DeliveryError struct {
	Reason Reason
	// Detail is a short description that is safe to log, such as "HTTP 503". It never contains
	// a URL, header or message content.
	Detail string
	Err    error
}

func (e *DeliveryError) Error() string {
	if e.Err == nil {
		return string(e.Reason)
	}
	return fmt.Sprintf("%s: %v", e.Reason, e.Err)
}

func (e *DeliveryError) Unwrap() error { return e.Err }

// newDeliveryError wraps err with a reason and a detail that is safe to log.
func newDeliveryError(reason Reason, detail string, err error) *DeliveryError {
	return &DeliveryError{Reason: reason, Detail: detail, Err: err}
}

// ReasonOf classifies a delivery error. Errors that are not a DeliveryError are classified by
// their transport error.
func ReasonOf(err error) Reason {
	if err == nil {
		return ""
	}
	var de *DeliveryError
	if errors.As(err, &de) {
		return de.Reason
	}
	if errors.Is(err, context.Canceled) {
		return ReasonInterrupted
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ReasonTimeout
	}
	var tpErr *textproto.Error
	if errors.As(err, &tpErr) {
		if tpErr.Code >= 400 && tpErr.Code < 500 {
			return ReasonSMTPTemporary
		}
		return ReasonSMTPPermanent
	}
	// A server that closes the connection mid-exchange surfaces as EOF.
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return ReasonConnection
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return ReasonTimeout
		}
		return ReasonConnection
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return ReasonConnection
	}
	return ReasonUnknown
}

// DetailOf returns the detail of a delivery error that is safe to log, or an empty string.
func DetailOf(err error) string {
	var de *DeliveryError
	if errors.As(err, &de) {
		return de.Detail
	}
	var tpErr *textproto.Error
	if errors.As(err, &tpErr) {
		return fmt.Sprintf("SMTP %d", tpErr.Code)
	}
	return ""
}

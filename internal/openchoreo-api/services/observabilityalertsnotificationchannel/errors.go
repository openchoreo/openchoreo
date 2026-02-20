// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import "errors"

var (
	ErrObservabilityAlertsNotificationChannelNotFound      = errors.New("observability alerts notification channel not found")
	ErrObservabilityAlertsNotificationChannelAlreadyExists = errors.New("observability alerts notification channel already exists")
)

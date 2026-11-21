// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"os"
	"testing"

	"github.com/openchoreo/openchoreo/internal/observer/notifications"
	"github.com/openchoreo/openchoreo/internal/observer/notifications/destination"
)

func TestMain(m *testing.M) {
	// The notification tests send to webhook receivers that listen on loopback.
	notifications.SetDestinationPolicy(destination.Options{AllowLoopback: true})
	os.Exit(m.Run())
}

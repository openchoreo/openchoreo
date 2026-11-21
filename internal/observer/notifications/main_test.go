// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package notifications

import (
	"os"
	"testing"

	"github.com/openchoreo/openchoreo/internal/observer/notifications/destination"
)

func TestMain(m *testing.M) {
	// The test servers listen on loopback.
	SetDestinationPolicy(destination.Options{AllowLoopback: true})
	os.Exit(m.Run())
}

// withDestinationPolicy applies the given destination options for the rest of the test.
func withDestinationPolicy(t *testing.T, opts destination.Options) {
	t.Helper()
	origClient, origDial := httpClient, smtpDial
	t.Cleanup(func() { httpClient, smtpDial = origClient, origDial })
	SetDestinationPolicy(opts)
}

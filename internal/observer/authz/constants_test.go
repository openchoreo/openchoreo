// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"testing"

	"github.com/stretchr/testify/assert"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// TestActionConstantsMirrorCore pins the observer's action constants to the
// authoritative ones in internal/authz/core.
//
// The two sets are hand-mirrored. A drift is silent and expensive: the observer
// would check an action string the registry does not know, so no grant could ever
// match it and every caller would get a 403 with nothing to indicate the cause.
func TestActionConstantsMirrorCore(t *testing.T) {
	t.Parallel()

	mirrored := map[string]struct {
		observer Action
		core     string
	}{
		"ActionViewLogs":         {ActionViewLogs, authzcore.ActionViewLogs},
		"ActionViewEvents":       {ActionViewEvents, authzcore.ActionViewEvents},
		"ActionViewTraces":       {ActionViewTraces, authzcore.ActionViewTraces},
		"ActionViewMetrics":      {ActionViewMetrics, authzcore.ActionViewMetrics},
		"ActionViewAlerts":       {ActionViewAlerts, authzcore.ActionViewAlerts},
		"ActionViewIncidents":    {ActionViewIncidents, authzcore.ActionViewIncidents},
		"ActionUpdateIncidents":  {ActionUpdateIncidents, authzcore.ActionUpdateIncidents},
		"ActionViewFinOps":       {ActionViewFinOps, authzcore.ActionViewFinOps},
		"ActionViewPlatformLogs": {ActionViewPlatformLogs, authzcore.ActionViewPlatformLogs},
	}

	for name, pair := range mirrored {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, pair.core, string(pair.observer),
				"observer %s has drifted from internal/authz/core", name)
		})
	}
}

// TestActionConstantsAreRegistered pins that every action the observer checks is
// present in the core action registry. An unregistered action can never be granted,
// and nothing else in the codebase validates action strings.
func TestActionConstantsAreRegistered(t *testing.T) {
	t.Parallel()

	registered := make(map[string]bool)
	for _, a := range authzcore.PublicActions() {
		registered[a.Name] = true
	}

	for _, action := range []Action{
		ActionViewLogs, ActionViewEvents, ActionViewTraces, ActionViewMetrics,
		ActionViewAlerts, ActionViewIncidents, ActionUpdateIncidents,
		ActionViewFinOps, ActionViewPlatformLogs,
	} {
		assert.True(t, registered[string(action)],
			"%q is checked by the observer but not registered in internal/authz/core", action)
	}
}

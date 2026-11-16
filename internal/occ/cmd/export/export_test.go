// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestRun_ReturnsFetchedObjects(t *testing.T) {
	f := newFetchFixture(t, false)
	resources, err := Run(context.Background(), f.client)
	require.NoError(t, err)
	require.Len(t, resources, 58)
	var found bool
	for _, resource := range resources {
		if resource.Kind == testComponentKind && resource.Namespace == "acme" {
			component, ok := resource.Object.(gen.Component)
			require.True(t, ok)
			require.NotNil(t, component.Status)
			assert.EqualValues(t, 7, *component.Status.ObservedGeneration)
			found = true
		}
	}
	assert.True(t, found, "the coordinator must return fetched objects without applying future stages")
}

func TestRun_PropagatesCancellation(t *testing.T) {
	f := newFetchFixture(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resources, err := Run(ctx, f.client)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, resources)
	assert.Empty(t, f.calls)
}

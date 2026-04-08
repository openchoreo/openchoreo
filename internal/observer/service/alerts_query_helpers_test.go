// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
)

func TestStringPtrValue(t *testing.T) {
	t.Run("nil returns empty", func(t *testing.T) {
		assert.Equal(t, "", stringPtrValue(nil))
	})

	t.Run("non-nil returns trimmed", func(t *testing.T) {
		s := "  hello  "
		assert.Equal(t, "hello", stringPtrValue(&s))
	})

	t.Run("empty string returns empty", func(t *testing.T) {
		s := ""
		assert.Equal(t, "", stringPtrValue(&s))
	})
}

func TestAlertSortOrderOrDefault(t *testing.T) {
	t.Run("nil returns desc", func(t *testing.T) {
		assert.Equal(t, gen.AlertsQueryRequestSortOrderDesc, alertSortOrderOrDefault(nil))
	})

	t.Run("empty returns desc", func(t *testing.T) {
		empty := gen.AlertsQueryRequestSortOrder("")
		assert.Equal(t, gen.AlertsQueryRequestSortOrderDesc, alertSortOrderOrDefault(&empty))
	})

	t.Run("whitespace returns desc", func(t *testing.T) {
		ws := gen.AlertsQueryRequestSortOrder("  ")
		assert.Equal(t, gen.AlertsQueryRequestSortOrderDesc, alertSortOrderOrDefault(&ws))
	})

	t.Run("asc returns asc", func(t *testing.T) {
		asc := gen.AlertsQueryRequestSortOrderAsc
		assert.Equal(t, gen.AlertsQueryRequestSortOrderAsc, alertSortOrderOrDefault(&asc))
	})
}

func TestIncidentSortOrderOrDefault(t *testing.T) {
	t.Run("nil returns desc", func(t *testing.T) {
		assert.Equal(t, gen.IncidentsQueryRequestSortOrderDesc, incidentSortOrderOrDefault(nil))
	})

	t.Run("empty returns desc", func(t *testing.T) {
		empty := gen.IncidentsQueryRequestSortOrder("")
		assert.Equal(t, gen.IncidentsQueryRequestSortOrderDesc, incidentSortOrderOrDefault(&empty))
	})

	t.Run("asc returns asc", func(t *testing.T) {
		asc := gen.IncidentsQueryRequestSortOrderAsc
		assert.Equal(t, gen.IncidentsQueryRequestSortOrderAsc, incidentSortOrderOrDefault(&asc))
	})
}

func TestIntPtrValue(t *testing.T) {
	t.Run("nil returns default", func(t *testing.T) {
		assert.Equal(t, 50, intPtrValue(nil, 50))
	})

	t.Run("zero returns default", func(t *testing.T) {
		v := 0
		assert.Equal(t, 50, intPtrValue(&v, 50))
	})

	t.Run("negative returns default", func(t *testing.T) {
		v := -1
		assert.Equal(t, 50, intPtrValue(&v, 50))
	})

	t.Run("positive returns value", func(t *testing.T) {
		v := 100
		assert.Equal(t, 100, intPtrValue(&v, 50))
	})
}

func TestUuidStringPtr(t *testing.T) {
	t.Run("empty returns nil", func(t *testing.T) {
		assert.Nil(t, uuidStringPtr(""))
	})

	t.Run("invalid UUID returns nil", func(t *testing.T) {
		assert.Nil(t, uuidStringPtr("not-a-uuid"))
	})

	t.Run("valid UUID returns pointer", func(t *testing.T) {
		id := uuid.New().String()
		result := uuidStringPtr(id)
		require.NotNil(t, result)
		assert.Equal(t, id, *result)
	})

	t.Run("whitespace-only returns nil", func(t *testing.T) {
		assert.Nil(t, uuidStringPtr("   "))
	})
}

func TestParseTimePtr(t *testing.T) {
	t.Run("empty returns nil", func(t *testing.T) {
		assert.Nil(t, parseTimePtr(""))
	})

	t.Run("whitespace returns nil", func(t *testing.T) {
		assert.Nil(t, parseTimePtr("   "))
	})

	t.Run("invalid returns nil", func(t *testing.T) {
		assert.Nil(t, parseTimePtr("not-a-time"))
	})

	t.Run("RFC3339Nano parses", func(t *testing.T) {
		ts := "2026-01-15T10:30:00.123456789Z"
		result := parseTimePtr(ts)
		require.NotNil(t, result)
		assert.Equal(t, 2026, result.Year())
		assert.Equal(t, time.January, result.Month())
		assert.True(t, result.Location() == time.UTC)
	})

	t.Run("RFC3339 parses", func(t *testing.T) {
		ts := "2026-01-15T10:30:00Z"
		result := parseTimePtr(ts)
		require.NotNil(t, result)
		assert.Equal(t, 2026, result.Year())
	})

	t.Run("RFC3339 with offset parses", func(t *testing.T) {
		ts := "2026-01-15T10:30:00+05:30"
		result := parseTimePtr(ts)
		require.NotNil(t, result)
		assert.True(t, result.Location() == time.UTC) // Should be converted to UTC
	})
}

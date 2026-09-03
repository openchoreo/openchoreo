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

func TestUUIDPtr(t *testing.T) {
	t.Run("empty returns nil", func(t *testing.T) {
		assert.Nil(t, uuidPtr(""))
	})

	// Dropping an unparseable UID keeps a bad store value out of the response
	// rather than emitting the zero UUID for it.
	t.Run("invalid UUID returns nil", func(t *testing.T) {
		assert.Nil(t, uuidPtr("not-a-uuid"))
	})

	t.Run("valid UUID returns pointer", func(t *testing.T) {
		id := uuid.New().String()
		result := uuidPtr(id)
		require.NotNil(t, result)
		assert.Equal(t, id, result.String())
	})

	t.Run("whitespace-only returns nil", func(t *testing.T) {
		assert.Nil(t, uuidPtr("   "))
	})
}

func TestEnumPtr(t *testing.T) {
	t.Run("empty returns nil", func(t *testing.T) {
		assert.Nil(t, enumPtr[gen.IncidentStatus](""))
	})

	t.Run("whitespace-only returns nil", func(t *testing.T) {
		assert.Nil(t, enumPtr[gen.IncidentStatus]("   "))
	})

	t.Run("value is trimmed and converted", func(t *testing.T) {
		result := enumPtr[gen.IncidentStatus](" acknowledged ")
		require.NotNil(t, result)
		assert.Equal(t, gen.Acknowledged, *result)
	})

	// Store values are not validated against the enum's permitted set, so a
	// value the spec does not list still passes through.
	t.Run("unlisted value passes through", func(t *testing.T) {
		result := enumPtr[gen.IncidentStatus]("unknown")
		require.NotNil(t, result)
		assert.Equal(t, gen.IncidentStatus("unknown"), *result)
	})
}

func TestNotificationChannelsOrNil(t *testing.T) {
	// The field must stay absent rather than serialize as [], which is what the
	// pre-existing []string + omitempty did.
	t.Run("empty JSON array returns nil", func(t *testing.T) {
		assert.Nil(t, notificationChannelsOrNil("[]"))
	})

	t.Run("blank returns nil", func(t *testing.T) {
		assert.Nil(t, notificationChannelsOrNil(""))
	})

	t.Run("unparseable returns nil", func(t *testing.T) {
		assert.Nil(t, notificationChannelsOrNil("{not json"))
	})

	t.Run("channels are returned", func(t *testing.T) {
		result := notificationChannelsOrNil(`["email","slack"]`)
		require.NotNil(t, result)
		assert.Equal(t, []string{"email", "slack"}, *result)
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

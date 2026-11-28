// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointtraffic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func trips() []Backend {
	return []Backend{
		{Name: "trips-python", Weight: 50},
		{Name: "trips-stale", Weight: 50},
		{Name: "trips-go", Weight: 0},
		{Name: "trips-shadow", Copy: ptr(10)},
	}
}

func TestPercents(t *testing.T) {
	assert.Equal(t, []int32{50, 50, 0}, Percents([]int32{1, 1, 0}))
	assert.Equal(t, []int32{40, 40, 20}, Percents([]int32{4000, 4000, 2000}))
	assert.Equal(t, []int32{34, 33, 33}, Percents([]int32{1, 1, 1}))
	assert.Equal(t, []int32{0, 0}, Percents([]int32{0, 0}))
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name  string
		split map[string]int32
		err   string
	}{
		{"a full split", map[string]int32{"trips-python": 45, "trips-stale": 50, "trips-go": 5}, ""},
		{"only a copy", map[string]int32{"trips-shadow": 25}, ""},
		{"nothing", map[string]int32{}, ""},
		{"an unknown backend", map[string]int32{"trips-java": 10}, "no backend named trips-java"},
		{"a share left out", map[string]int32{"trips-python": 50, "trips-go": 50}, "give a share for trips-stale too"},
		{"more than 100", map[string]int32{"trips-python": 50, "trips-stale": 50, "trips-go": 10}, "add up to 110, not 100"},
		{"out of range", map[string]int32{"trips-shadow": 120}, "from 0 to 100, not 120"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(trips(), tt.split)
			if tt.err == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.err)
		})
	}
}

func TestValidateNeedsAShare(t *testing.T) {
	backends := []Backend{{Name: "a", Weight: 1}, {Name: "b", Weight: 0}}
	require.NoError(t, Validate(backends, map[string]int32{"a": 0, "b": 100}))
	err := Validate(backends, map[string]int32{"a": 0, "b": 0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "add up to 0, not 100")
}

func TestOverrides(t *testing.T) {
	t.Run("a share that differs stores every weight", func(t *testing.T) {
		got := Overrides(trips(), map[string]int32{"trips-python": 45, "trips-stale": 50, "trips-go": 5})
		assert.Equal(t, []Override{
			{Name: "trips-python", Weight: ptr(45)},
			{Name: "trips-stale", Weight: ptr(50)},
			{Name: "trips-go", Weight: ptr(5)},
		}, got)
	})

	t.Run("the release's ratio stores nothing", func(t *testing.T) {
		assert.Empty(t, Overrides(trips(), map[string]int32{"trips-python": 50, "trips-stale": 50, "trips-go": 0}))
		scaled := []Backend{{Name: "a", Weight: 4000}, {Name: "b", Weight: 1000}}
		assert.Empty(t, Overrides(scaled, map[string]int32{"a": 80, "b": 20}))
	})

	t.Run("a copy is stored only when it differs", func(t *testing.T) {
		assert.Equal(t, []Override{{Name: "trips-shadow", Copy: ptr(25)}},
			Overrides(trips(), map[string]int32{"trips-shadow": 25}))
		assert.Empty(t, Overrides(trips(), map[string]int32{"trips-shadow": 10}))
	})
}

func TestHeededAndEffective(t *testing.T) {
	overrides := []Override{
		{Name: "trips-python", Weight: ptr(0)},
		{Name: "trips-go", Weight: ptr(100)},
		{Name: "trips-java", Weight: ptr(10)},
		{Name: "trips-shadow", Copy: ptr(40)},
	}
	heeded := Heeded(overrides, trips())
	require.Len(t, heeded, 3)

	got := Effective(trips(), heeded)
	assert.Equal(t, int32(0), got[0].Weight)
	assert.Equal(t, int32(0), got[1].Weight, "a backend the own split doesn't name gets no share")
	assert.Equal(t, int32(100), got[2].Weight)
	assert.Equal(t, int32(40), *got[3].Copy)
	assert.Equal(t, int32(10), *trips()[3].Copy, "the release is left as it was")

	got = Effective(trips(), []Override{{Name: "trips-shadow", Copy: ptr(40)}})
	assert.Equal(t, int32(50), got[1].Weight, "without an own split the release's weights apply")
}

func TestSameRatio(t *testing.T) {
	assert.True(t, SameRatio([]int32{1, 1}, []int32{50, 50}))
	assert.False(t, SameRatio([]int32{1, 1}, []int32{45, 55}))
	assert.True(t, SameRatio([]int32{0, 0}, []int32{0, 0}))
	assert.False(t, SameRatio([]int32{1}, []int32{1, 0}))
}

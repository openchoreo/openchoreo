// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestReleaseCutoff(t *testing.T) {
	tests := []struct {
		name string
		now  string
		all  bool
		want string
	}{
		{name: "default three calendar months", now: "2026-11-05T10:30:00+05:30", want: "2026-08-05T05:00:00Z"},
		{name: "year boundary", now: "2026-01-05T10:30:00Z", want: "2025-10-05T10:30:00Z"},
		{name: "short month", now: "2026-05-31T10:30:00Z", want: "2026-02-28T10:30:00Z"},
		{name: "leap year", now: "2024-05-31T10:30:00Z", want: "2024-02-29T10:30:00Z"},
		{name: "all releases", all: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var now time.Time
			if tt.now != "" {
				var err error
				now, err = time.Parse(time.RFC3339, tt.now)
				require.NoError(t, err)
			}
			filter := newReleaseFilter(tt.all, now)
			if tt.all {
				assert.Nil(t, filter.since)
			} else {
				require.NotNil(t, filter.since)
				assert.Equal(t, tt.want, filter.since.Format(time.RFC3339Nano))
			}
		})
	}
}

func TestCollectReleases_FiltersPagesAndPreservesReferences(t *testing.T) {
	for _, all := range []bool{false, true} {
		name := "recent and referenced"
		if all {
			name = "all releases"
		}
		t.Run(name, func(t *testing.T) {
			cutoff := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
			old := cutoff.Add(-time.Nanosecond)
			recent := cutoff.Add(time.Hour)
			zero := time.Time{}
			filter := &releaseFilter{since: &cutoff}
			if all {
				filter.since = nil
			}
			spec := &gen.ProjectReleaseSpec{}
			spec.Owner.ProjectName = "shop"
			var cursors []string
			fetch := func(limit int, cursor string) ([]gen.ProjectRelease, string, error) {
				require.Positive(t, limit)
				cursors = append(cursors, cursor)
				switch cursor {
				case "":
					return []gen.ProjectRelease{{Metadata: gen.ObjectMeta{Name: "old-unbound", CreationTimestamp: &old}, Spec: spec}}, "empty", nil
				case "empty":
					return nil, "remaining", nil
				case "remaining":
					return []gen.ProjectRelease{
						{Metadata: gen.ObjectMeta{Name: "old-bound", CreationTimestamp: &old}, Spec: spec},
						{Metadata: gen.ObjectMeta{Name: "boundary", CreationTimestamp: &cutoff}, Spec: spec},
						{Metadata: gen.ObjectMeta{Name: "recent", CreationTimestamp: &recent}, Spec: spec},
						{Metadata: gen.ObjectMeta{Name: "undated"}, Spec: spec},
						{Metadata: gen.ObjectMeta{Name: "zero-timestamp", CreationTimestamp: &zero}, Spec: spec},
					}, "", nil
				default:
					t.Fatalf("unexpected cursor %q", cursor)
					return nil, "", nil
				}
			}
			binding := resource{
				kind: KindProjectReleaseBinding, name: "shop-production", namespace: "acme",
				object: map[string]any{"spec": map[string]any{"projectRelease": "old-bound"}},
			}
			bindings := []resource{binding, binding, {object: map[string]any{"spec": map[string]any{}}}}
			releases, err := collectReleases(KindProjectRelease, "acme", "shop", map[string]string{"projectName": "shop"},
				bindings, "projectRelease", filter, fetch, func(v gen.ProjectRelease) gen.ObjectMeta { return v.Metadata },
				func(name string) string { return path.Join("releases", name+".yaml") })
			require.NoError(t, err)
			assert.Equal(t, []string{"", "empty", "remaining"}, cursors)
			var names []string
			for _, item := range releases {
				names = append(names, item.name)
				assert.Equal(t, path.Join("releases", item.name+".yaml"), item.path)
				assert.NotContains(t, item.object["metadata"], "creationTimestamp")
			}
			want := []string{"old-bound", "boundary", "recent", "undated", "zero-timestamp"}
			if all {
				want = append([]string{"old-unbound"}, want...)
				assert.Zero(t, filter.skipped)
				assert.Zero(t, filter.olderReferenced)
				assert.Zero(t, filter.undated)
			} else {
				assert.Equal(t, 1, filter.skipped)
				assert.Equal(t, 1, filter.olderReferenced)
				assert.Equal(t, 2, filter.undated)
			}
			assert.Equal(t, want, names)
		})
	}
}

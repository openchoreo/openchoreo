// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"fmt"
	"time"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const defaultReleaseMonths = 3

type releaseFilter struct {
	since           *time.Time
	skipped         int
	olderReferenced int
	undated         int
}

// newReleaseFilter selects the three-month history window or all releases.
func newReleaseFilter(all bool, now time.Time) *releaseFilter {
	if all {
		return &releaseFilter{}
	}
	// Clamp month-end dates so May 31 minus three months falls in February.
	now = now.UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, now.Hour(), now.Minute(), now.Second(), now.Nanosecond(), time.UTC).AddDate(0, -defaultReleaseMonths, 0)
	lastDay := monthStart.AddDate(0, 1, -1).Day()
	cutoff := monthStart.AddDate(0, 0, min(now.Day(), lastDay)-1)
	return &releaseFilter{since: &cutoff}
}

// include checks a release's age and binding references and updates selection counts.
func (f *releaseFilter) include(metadata gen.ObjectMeta, referenced map[string]bool) bool {
	if f.since == nil {
		return true
	}
	// Keep releases whose age cannot be determined.
	if metadata.CreationTimestamp == nil || metadata.CreationTimestamp.IsZero() {
		f.undated++
		return true
	}
	if !metadata.CreationTimestamp.Before(*f.since) {
		return true
	}
	if referenced[metadata.Name] {
		f.olderReferenced++
		return true
	}
	f.skipped++
	return false
}

// collectReleases filters release pages and validates binding references.
func collectReleases[T any](kind, namespace, project string, owner map[string]string, bindings []resource, referenceField string, filter *releaseFilter, fetch resourcePage[T], metadataOf func(T) gen.ObjectMeta, pathOf func(string) string) ([]resource, error) {
	referenced := map[string]bool{}
	for _, binding := range bindings {
		spec := binding.object["spec"].(map[string]any)
		name, _ := spec[referenceField].(string)
		if name != "" {
			if err := validName("referenced release", name); err != nil {
				return nil, err
			}
			referenced[name] = true
		}
	}
	// Continue through filtered pages; releases are not ordered by creation time.
	selectedPage := func(limit int, cursor string) ([]T, string, error) {
		items, next, err := fetch(limit, cursor)
		if err != nil {
			return nil, "", err
		}
		selected := make([]T, 0, len(items))
		for _, item := range items {
			if filter.include(metadataOf(item), referenced) {
				selected = append(selected, item)
			}
		}
		return selected, next, nil
	}
	releases, err := collectOwnedResources(kind, namespace, project, owner, selectedPage,
		func(item T) string { return metadataOf(item).Name }, pathOf)
	if err != nil {
		return nil, err
	}
	if err := validateReleaseReferences(bindings, releases, referenceField); err != nil {
		return nil, err
	}
	return releases, nil
}

// validateReleaseReferences ensures every pinned release is included under the same owner.
func validateReleaseReferences(bindings, releases []resource, field string) error {
	names := make(map[string]bool, len(releases))
	for _, release := range releases {
		names[release.name] = true
	}
	for _, binding := range bindings {
		spec := binding.object["spec"].(map[string]any)
		name, _ := spec[field].(string)
		if name == "" {
			continue
		}
		if err := validName("referenced release", name); err != nil {
			return err
		}
		if !names[name] {
			return fmt.Errorf("%s %q in namespace %q references release %q that was not exported for its owner", binding.kind, binding.name, binding.namespace, name)
		}
	}
	return nil
}

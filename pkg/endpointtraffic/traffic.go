// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package endpointtraffic turns the shares a person picks for an Endpoint's
// backends into weights, and an environment's own split into the
// backendOverrides of its EndpointReleaseBinding. Every client uses it, so a
// split saved from occ, MCP or the portal is stored the same way.
package endpointtraffic

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Backend is one backend of the release an environment runs. Weight is its
// share of the requests no condition takes; a mirror has Copy instead.
type Backend struct {
	Name   string
	Weight int32
	Copy   *int32
}

// Mirror reports whether the backend only gets copies.
func (b Backend) Mirror() bool { return b.Copy != nil }

// Override is one backendOverrides entry: a weight, or a mirror's copy share.
type Override struct {
	Name   string
	Weight *int32
	Copy   *int32
}

// Percents gives each weight as a whole percentage of their total, adding up
// to exactly 100 (largest remainder). When every weight is 0, every share is 0.
func Percents(weights []int32) []int32 {
	out := make([]int32, len(weights))
	var total int64
	for _, w := range weights {
		total += int64(w)
	}
	if total <= 0 {
		return out
	}
	type remainder struct {
		index int
		rest  int64
	}
	rests := make([]remainder, len(weights))
	left := int64(100)
	for i, w := range weights {
		share := int64(w) * 100
		out[i] = int32(share / total) //nolint:gosec // a share of 100 is at most 100
		rests[i] = remainder{index: i, rest: share % total}
		left -= int64(out[i])
	}
	sort.SliceStable(rests, func(a, b int) bool { return rests[a].rest > rests[b].rest })
	for _, r := range rests {
		if left == 0 {
			break
		}
		out[r.index]++
		left--
	}
	return out
}

// Validate checks a split against the backends: every name is a backend, a
// value is a whole percentage, and the shares cover every backend that takes
// one and add up to 100, so requests no condition takes always have somewhere
// to go. A mirror's value is its copy share.
func Validate(backends []Backend, split map[string]int32) error {
	var unknown []string
	for name := range split {
		if !slices.ContainsFunc(backends, func(b Backend) bool { return b.Name == name }) {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		slices.Sort(unknown)
		return fmt.Errorf("no backend named %s", strings.Join(unknown, ", "))
	}
	var total, given int32
	var missing []string
	for _, b := range backends {
		v, ok := split[b.Name]
		if ok && (v < 0 || v > 100) {
			return fmt.Errorf("%s: give a whole percentage from 0 to 100, not %d", b.Name, v)
		}
		if b.Mirror() {
			continue
		}
		if !ok {
			missing = append(missing, b.Name)
			continue
		}
		given++
		total += v
	}
	if given == 0 {
		return nil
	}
	// Shares that add up to 100 always leave one above 0.
	switch {
	case len(missing) > 0:
		return fmt.Errorf("give a share for %s too; the shares add up to 100", strings.Join(missing, ", "))
	case total != 100:
		return fmt.Errorf("the shares add up to %d, not 100", total)
	}
	return nil
}

// Overrides turns a validated split into an environment's backendOverrides.
// Shares that keep the release's ratio and copies that match the release
// store nothing, so an environment differs from the release only where it
// really does. Backends left out of split keep the release's value.
func Overrides(backends []Backend, split map[string]int32) []Override {
	var release, picked []int32
	for _, b := range backends {
		if b.Mirror() {
			continue
		}
		release = append(release, b.Weight)
		if v, ok := split[b.Name]; ok {
			picked = append(picked, v)
		} else {
			picked = append(picked, b.Weight)
		}
	}
	sameShares := SameRatio(release, picked)
	var out []Override
	for _, b := range backends {
		v, ok := split[b.Name]
		switch {
		case b.Mirror():
			if ok && v != *b.Copy {
				out = append(out, Override{Name: b.Name, Copy: ptr(v)})
			}
		case !sameShares:
			if !ok {
				v = b.Weight
			}
			out = append(out, Override{Name: b.Name, Weight: ptr(v)})
		}
	}
	return out
}

// SameRatio reports whether two lists of weights split traffic the same way.
func SameRatio(a, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	var sumA, sumB int64
	for i := range a {
		sumA += int64(a[i])
		sumB += int64(b[i])
	}
	if sumA == 0 || sumB == 0 {
		return sumA == sumB
	}
	for i := range a {
		if int64(a[i])*sumB != int64(b[i])*sumA {
			return false
		}
	}
	return true
}

// Heeded keeps the overrides that name a backend of the release; the
// controller ignores the rest, so clients don't show them either.
func Heeded(overrides []Override, backends []Backend) []Override {
	var out []Override
	for _, o := range overrides {
		if slices.ContainsFunc(backends, func(b Backend) bool { return b.Name == o.Name }) {
			out = append(out, o)
		}
	}
	return out
}

// Effective applies an environment's overrides to the release's backends.
// When the environment has its own split, a backend it doesn't name gets 0,
// as the controller applies it.
func Effective(backends []Backend, overrides []Override) []Backend {
	ownSplit := slices.ContainsFunc(overrides, func(o Override) bool {
		return o.Weight != nil && slices.ContainsFunc(backends, func(b Backend) bool {
			return b.Name == o.Name && !b.Mirror()
		})
	})
	out := make([]Backend, len(backends))
	for i, b := range backends {
		out[i] = b
		if ownSplit && !b.Mirror() {
			out[i].Weight = 0
		}
		for _, o := range overrides {
			if o.Name != b.Name {
				continue
			}
			if o.Weight != nil && !b.Mirror() {
				out[i].Weight = *o.Weight
			}
			if o.Copy != nil && b.Mirror() {
				out[i].Copy = ptr(*o.Copy)
			}
		}
	}
	return out
}

func ptr(v int32) *int32 { return &v }

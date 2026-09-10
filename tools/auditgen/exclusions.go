// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

// excludedOperationIDs turns a service's exemption table into the set
// BuildDefinitions consults; the reasons are for readers, not for generation.
func excludedOperationIDs(exemptions map[string]string) map[string]bool {
	excluded := make(map[string]bool, len(exemptions))
	for id := range exemptions {
		excluded[id] = true
	}
	return excluded
}

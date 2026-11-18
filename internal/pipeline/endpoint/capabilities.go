// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointpipeline

import (
	"fmt"
	"slices"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// CapabilityProblems reports the routing features the backends use that their
// Endpoint's type does not declare. A type that declares no capabilities is
// not checked.
func CapabilityProblems(declared []openchoreov1alpha1.EndpointCapability, backendRefs []openchoreov1alpha1.EndpointBackendRef) []string {
	if len(declared) == 0 {
		return nil
	}
	var problems []string
	// need reports whether the type declares c; a refinement of a feature
	// the type lacks is not reported on its own.
	need := func(c openchoreov1alpha1.EndpointCapability, ref openchoreov1alpha1.EndpointBackendRef, what string) bool {
		if slices.Contains(declared, c) {
			return true
		}
		problems = append(problems, fmt.Sprintf("backend %q: %s needs the %s capability", ref.Name, what, c))
		return false
	}
	for _, ref := range backendRefs {
		if ref.Match != nil && need(openchoreov1alpha1.EndpointCapabilityMatch, ref, "a match") {
			// A cookie is matched by a regular expression on the Cookie header.
			if len(ref.Match.Cookies) > 0 {
				need(openchoreov1alpha1.EndpointCapabilityRegularExpression, ref, "a cookie match")
			} else if slices.ContainsFunc(ref.Match.Headers, func(h openchoreov1alpha1.EndpointHeaderMatch) bool {
				return h.Type == openchoreov1alpha1.EndpointHeaderMatchRegularExpression
			}) {
				need(openchoreov1alpha1.EndpointCapabilityRegularExpression, ref, "a regular expression header match")
			}
		}
		if ref.Mirror != nil && need(openchoreov1alpha1.EndpointCapabilityMirror, ref, "a mirror") &&
			ref.Mirror.Percent > 0 && ref.Mirror.Percent < 100 {
			need(openchoreov1alpha1.EndpointCapabilityMirrorPercent, ref, "mirroring a share of requests")
		}
	}
	return problems
}

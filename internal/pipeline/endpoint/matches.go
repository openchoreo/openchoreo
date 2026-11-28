// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointpipeline

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// MatchProblems reports backend matches the data planes cannot express the
// same way, before a release is cut. CEL on the CRD covers the shape; these
// need Go:
//
//   - Gateway API keeps only the first condition on a header, Istio keys
//     conditions by header name and Envoy ANDs them, so a header (names are
//     case-insensitive) may appear once per match, and a cookie, which
//     renders on the Cookie header, cannot sit next to a Cookie header
//     condition.
//   - Header patterns must compile as RE2, the engine Envoy, Istio and Cilium
//     all use; Go's regexp implements it.
//   - A path must mean the same on every proxy once joined to the Endpoint's
//     own path, so it has no empty, . or .. segments.
//   - Two backends with the same match are refused: which of them gets the
//     request would depend on the data plane.
func MatchProblems(backendRefs []openchoreov1alpha1.EndpointBackendRef) []string {
	var problems []string
	add := func(ref openchoreov1alpha1.EndpointBackendRef, format string, args ...any) {
		problems = append(problems, fmt.Sprintf("backend %q: match: ", ref.Name)+fmt.Sprintf(format, args...))
	}
	owner := map[string]string{}
	for _, ref := range backendRefs {
		m := ref.Match
		if m == nil {
			continue
		}
		if m.Path != nil {
			if msg := pathProblem(m.Path.Value); msg != "" {
				add(ref, "path %q %s", m.Path.Value, msg)
			}
		}
		seen := map[string]bool{}
		for _, h := range m.Headers {
			name := strings.ToLower(h.Name)
			if seen[name] {
				add(ref, "header %q appears more than once", h.Name)
			}
			seen[name] = true
			if name == "cookie" && len(m.Cookies) > 0 {
				add(ref, "match the cookie with cookies, or the Cookie header with headers, not both")
			}
			if h.Type != openchoreov1alpha1.EndpointHeaderMatchRegularExpression {
				continue
			}
			if _, err := regexp.Compile(h.Value); err != nil {
				add(ref, "header %q value is not a regular expression the data plane can run (RE2): %v", h.Name, err)
			}
		}
		key := matchKey(m)
		if first, dup := owner[key]; dup {
			add(ref, "backend %q has the same match; give each backend a different match", first)
			continue
		}
		owner[key] = ref.Name
	}
	return problems
}

// pathProblem explains why a match path is not accepted, or returns "".
func pathProblem(p string) string {
	for _, seg := range strings.Split(p, "/")[1:] {
		switch seg {
		case ".", "..":
			return "must not contain . or .. segments"
		}
	}
	if strings.Contains(p, "//") {
		return "must not contain //"
	}
	return ""
}

// matchKey identifies a match by its conditions, whatever their order and
// the case of header names.
func matchKey(m *openchoreov1alpha1.EndpointMatch) string {
	var parts []string
	if m.Path != nil {
		t := m.Path.Type
		if t == "" {
			t = openchoreov1alpha1.EndpointPathMatchPathPrefix
		}
		v := m.Path.Value
		// /next and /next/ select the same requests as a prefix, and the "/"
		// prefix selects them all, as no path does.
		if t == openchoreov1alpha1.EndpointPathMatchPathPrefix && len(v) > 1 {
			v = strings.TrimSuffix(v, "/")
		}
		if t != openchoreov1alpha1.EndpointPathMatchPathPrefix || v != "/" {
			parts = append(parts, "path\x00"+string(t)+"\x00"+v)
		}
	}
	for _, h := range m.Headers {
		t := h.Type
		if t == "" {
			t = openchoreov1alpha1.EndpointHeaderMatchExact
		}
		parts = append(parts, "header\x00"+strings.ToLower(h.Name)+"\x00"+string(t)+"\x00"+h.Value)
	}
	for _, c := range m.Cookies {
		parts = append(parts, "cookie\x00"+c.Name+"\x00"+c.Value)
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

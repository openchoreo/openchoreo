// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointpipeline

import (
	"regexp"
	"strings"
)

// Match types, as in the Endpoint API.
const (
	HeaderMatchExact             = "Exact"
	HeaderMatchRegularExpression = "RegularExpression"
	PathMatchPathPrefix          = "PathPrefix"
	PathMatchExact               = "Exact"
)

// PathRegex is the regular expression that matches the same request paths as
// a path condition: the path itself for Exact, and the path and every path
// below it for PathPrefix, segment by segment, so /next does not match
// /nextpage. The path is escaped.
func PathRegex(matchType, path string) string {
	if matchType == PathMatchExact {
		return regexp.QuoteMeta(path)
	}
	trimmed := strings.TrimSuffix(path, "/")
	if trimmed == "" {
		return "/.*"
	}
	return regexp.QuoteMeta(trimmed) + "(/.*)?"
}

// CookieRegex is the regular expression that matches a Cookie header
// carrying name=value, wherever it sits among the other cookies. Name and
// value are escaped, so a value such as 1.0 does not also match 1x0. Gateway
// API, Istio and Envoy all evaluate it with RE2 against the whole header.
func CookieRegex(name, value string) string {
	return `(.*;\s*)?` + regexp.QuoteMeta(name) + "=" + regexp.QuoteMeta(value) + `(;.*)?`
}

// NewMatch builds a Match with its Conditions: header names lowercased (HTTP
// header names are case-insensitive, and Istio requires lowercase keys) and
// each cookie turned into a condition on the cookie header. A path without a
// type is a PathPrefix, given without a trailing slash: Gateway API matches
// /next/ as /next, so types compare and rank prefixes by one spelling.
func NewMatch(path *PathMatch, headers []HeaderMatch, cookies []CookieMatch) Match {
	m := Match{
		Conditions: make([]HeaderMatch, 0, len(headers)+len(cookies)),
		Headers:    make([]HeaderMatch, 0, len(headers)),
		Cookies:    make([]CookieMatch, 0, len(cookies)),
	}
	if path != nil {
		p := PathMatch{Type: path.Type, Value: path.Value}
		if p.Type == "" {
			p.Type = PathMatchPathPrefix
		}
		if p.Type == PathMatchPathPrefix && len(p.Value) > 1 {
			p.Value = strings.TrimSuffix(p.Value, "/")
		}
		p.Regex = PathRegex(p.Type, p.Value)
		// A "/" prefix selects every request to the Endpoint, as no path does;
		// leaving it out renders and ranks the two the same way.
		if p.Type != PathMatchPathPrefix || p.Value != "/" {
			m.Path = &p
		}
	}
	for _, h := range headers {
		t := h.Type
		if t == "" {
			t = HeaderMatchExact
		}
		h = HeaderMatch{Name: strings.ToLower(h.Name), Type: t, Value: h.Value}
		m.Headers = append(m.Headers, h)
		m.Conditions = append(m.Conditions, h)
	}
	for _, c := range cookies {
		m.Cookies = append(m.Cookies, c)
		m.Conditions = append(m.Conditions, HeaderMatch{
			Name:  "cookie",
			Type:  HeaderMatchRegularExpression,
			Value: CookieRegex(c.Name, c.Value),
		})
	}
	return m
}

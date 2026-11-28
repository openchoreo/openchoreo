// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointpipeline

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

// fullMatch evaluates re against s the way Gateway API, Istio and Envoy do:
// RE2, anchored to the whole header value.
func fullMatch(re, s string) bool {
	return regexp.MustCompile("^(?:" + re + ")$").MatchString(s)
}

func TestCookieRegex(t *testing.T) {
	re := CookieRegex("exp_cart", "1.0")

	for _, header := range []string{
		"exp_cart=1.0",
		"session=abc; exp_cart=1.0",
		"exp_cart=1.0; theme=dark",
		"a=1;exp_cart=1.0;b=2",
	} {
		assert.True(t, fullMatch(re, header), header)
	}
	for _, header := range []string{
		"exp_cart=1x0",       // the value's dot is literal
		"exp_cart=1.01",      // no prefix match on the value
		"my_exp_cart=1.0",    // no suffix match on the name
		"session=exp_cart=1", // not a cookie of its own
	} {
		assert.False(t, fullMatch(re, header), header)
	}
}

func TestPathRegex(t *testing.T) {
	prefix := PathRegex(PathMatchPathPrefix, "/v1.2/")
	for _, p := range []string{"/v1.2", "/v1.2/", "/v1.2/orders"} {
		assert.True(t, fullMatch(prefix, p), p)
	}
	for _, p := range []string{"/v1x2", "/v1.2orders", "/"} {
		assert.False(t, fullMatch(prefix, p), p)
	}
	assert.True(t, fullMatch(PathRegex(PathMatchPathPrefix, "/"), "/anything"))
	assert.True(t, fullMatch(PathRegex(PathMatchExact, "/v1.2"), "/v1.2"))
	assert.False(t, fullMatch(PathRegex(PathMatchExact, "/v1.2"), "/v1.2/orders"))
}

func TestNewMatch(t *testing.T) {
	m := NewMatch(
		&PathMatch{Value: "/next"},
		[]HeaderMatch{
			{Name: "X-Client", Value: "ios"},
			{Name: "User-Agent", Type: HeaderMatchRegularExpression, Value: ".*Mobi.*"},
		},
		[]CookieMatch{{Name: "exp_cartrecs", Value: "b"}},
	)

	assert.Equal(t, []HeaderMatch{
		{Name: "x-client", Type: HeaderMatchExact, Value: "ios"},
		{Name: "user-agent", Type: HeaderMatchRegularExpression, Value: ".*Mobi.*"},
		{Name: "cookie", Type: HeaderMatchRegularExpression, Value: CookieRegex("exp_cartrecs", "b")},
	}, m.Conditions)
	assert.Equal(t, "x-client", m.Headers[0].Name)
	assert.Equal(t, []CookieMatch{{Name: "exp_cartrecs", Value: "b"}}, m.Cookies)
	assert.Equal(t, &PathMatch{Type: PathMatchPathPrefix, Value: "/next", Regex: `/next(/.*)?`}, m.Path)

	slash := NewMatch(&PathMatch{Value: "/next/"}, nil, nil)
	assert.Equal(t, &PathMatch{Type: PathMatchPathPrefix, Value: "/next", Regex: `/next(/.*)?`}, slash.Path,
		"a prefix is given without its trailing slash")
	assert.Nil(t, NewMatch(&PathMatch{Value: "/"}, nil, nil).Path, `a "/" prefix is no path condition`)
	assert.Equal(t, "/", NewMatch(&PathMatch{Type: PathMatchExact, Value: "/"}, nil, nil).Path.Value)
	assert.Equal(t, "/next/", NewMatch(&PathMatch{Type: PathMatchExact, Value: "/next/"}, nil, nil).Path.Value,
		"an exact path keeps it")

	empty := NewMatch(nil, nil, nil)
	assert.Nil(t, empty.Path)
	assert.NotNil(t, empty.Conditions)
	assert.NotNil(t, empty.Headers)
	assert.NotNil(t, empty.Cookies)
}

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointpipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSortBySpecificity(t *testing.T) {
	rule := func(name string, m Match) Rule { return Rule{Name: name, Matches: []Match{m}} }
	header := []HeaderMatch{{Name: "X-Preview", Value: "true"}}
	cookie := []CookieMatch{{Name: "group", Value: "b"}}
	rules := []Rule{
		rule("testers", NewMatch(nil, header, cookie)),
		rule("api", NewMatch(&PathMatch{Value: "/api"}, nil, nil)),
		rule("trips", NewMatch(&PathMatch{Type: PathMatchExact, Value: "/trips"}, nil, nil)),
		rule("api-v2", NewMatch(&PathMatch{Value: "/api/v2/"}, nil, nil)),
		rule("api-beta", NewMatch(&PathMatch{Value: "/api"}, header, nil)),
		rule("header", NewMatch(nil, header, nil)),
		rule("also-header", NewMatch(nil, []HeaderMatch{{Name: "x-beta", Value: "1"}}, nil)),
	}

	SortBySpecificity(rules)

	names := make([]string, len(rules))
	for i, r := range rules {
		names[i] = r.Name
	}
	assert.Equal(t, []string{
		"trips",    // an exact path first
		"api-v2",   // then the longest prefix; the trailing slash doesn't count
		"api-beta", // the same prefix with a header
		"api",
		"testers", // no path ranks below every path; a header and a cookie
		"header",  // one condition, listed before also-header
		"also-header",
	}, names)
}

func TestSortBySpecificity_TrailingSlashAndRootPath(t *testing.T) {
	rule := func(name string, m Match) Rule { return Rule{Name: name, Matches: []Match{m}} }
	header := []HeaderMatch{{Name: "x-beta", Value: "true"}}

	rules := []Rule{
		rule("plain", NewMatch(&PathMatch{Value: "/next/"}, nil, nil)),
		rule("with-header", NewMatch(&PathMatch{Value: "/next"}, header, nil)),
	}
	SortBySpecificity(rules)
	assert.Equal(t, "with-header", rules[0].Name, "/next/ ranks as /next, so the header decides")

	rules = []Rule{
		rule("root", NewMatch(&PathMatch{Value: "/"}, nil, nil)),
		rule("header-only", NewMatch(nil, header, nil)),
	}
	SortBySpecificity(rules)
	assert.Nil(t, rules[1].Matches[0].Path, `a "/" prefix is no path condition`)
	assert.Equal(t, "header-only", rules[0].Name, `"/" ranks as no path, so the header decides`)
}

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package localdevaddresses

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSingleEndpoint(t *testing.T) {
	got, err := Parse("database=outputs.host:outputs.port")
	require.NoError(t, err)
	require.Equal(t, []Declaration{{Name: "database", HostOutput: "host", PortOutput: "port"}}, got)
}

func TestParseMultipleEndpoints(t *testing.T) {
	got, err := Parse("database=outputs.host:outputs.port,adminEndpoint=outputs.host:outputs.adminPort")
	require.NoError(t, err)
	require.Equal(t, []Declaration{
		{Name: "database", HostOutput: "host", PortOutput: "port"},
		{Name: "adminEndpoint", HostOutput: "host", PortOutput: "adminPort"},
	}, got)
}

func TestParseIgnoresSurroundingWhitespace(t *testing.T) {
	got, err := Parse("  database = outputs.host : outputs.port ,\n  admin = outputs.host : outputs.adminPort  ")
	require.NoError(t, err)
	require.Equal(t, []Declaration{
		{Name: "database", HostOutput: "host", PortOutput: "port"},
		{Name: "admin", HostOutput: "host", PortOutput: "adminPort"},
	}, got)
}

// A blank annotation is a resource type with nothing to dial.
func TestParseEmptyIsNotAnError(t *testing.T) {
	for _, value := range []string{"", "   ", "\n"} {
		got, err := Parse(value)
		require.NoError(t, err)
		require.Empty(t, got)
	}
}

func TestFromAnnotations(t *testing.T) {
	got, err := FromAnnotations(map[string]string{AnnotationKey: "database=outputs.host:outputs.port"})
	require.NoError(t, err)
	require.Len(t, got, 1)

	got, err = FromAnnotations(map[string]string{"openchoreo.dev/description": "unrelated"})
	require.NoError(t, err)
	require.Empty(t, got)

	got, err = FromAnnotations(nil)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestParseRejectsMalformed(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"no equals", "database", "expected"},
		{"no colon", "database=outputs.host", "must name both halves"},
		{"unqualified host", "database=host:outputs.port", `must reference an output as "outputs.<name>"`},
		{"unqualified port", "database=outputs.host:port", `must reference an output as "outputs.<name>"`},
		{"empty host output", "database=outputs.:outputs.port", `must reference an output as "outputs.<name>"`},
		{"empty port output", "database=outputs.host:outputs.", `must reference an output as "outputs.<name>"`},
		{"trailing comma", "database=outputs.host:outputs.port,", "empty address entry"},
		{"empty name", "=outputs.host:outputs.port", "may contain only alphanumerics"},
		{"name with slash", "a/b=outputs.host:outputs.port", "may contain only alphanumerics"},
		{"name too long", strings.Repeat("a", 64) + "=outputs.host:outputs.port", "longer than 63 characters"},
		{
			"duplicate name",
			"database=outputs.host:outputs.port,database=outputs.host:outputs.otherPort",
			`"database" is declared more than once`,
		},
		{
			"shared port output",
			"writer=outputs.wHost:outputs.port,reader=outputs.rHost:outputs.port",
			`already used by address "writer"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.value)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.want)
			require.Nil(t, got)
		})
	}
}

func TestParseRejectsTooManyEndpoints(t *testing.T) {
	entries := make([]string, 0, MaxAddresses+1)
	for i := 0; i <= MaxAddresses; i++ {
		entries = append(entries, string(rune('a'+i))+"=outputs.host:outputs.port")
	}
	_, err := Parse(strings.Join(entries, ","))
	require.Error(t, err)
	require.Contains(t, err.Error(), "at most 10 are allowed")
}

// Two addresses on one service share its host output.
func TestParseAllowsSharedHostOutput(t *testing.T) {
	got, err := Parse("database=outputs.host:outputs.port,adminEndpoint=outputs.host:outputs.adminPort")
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, got[0].HostOutput, got[1].HostOutput)
}

func TestResolve(t *testing.T) {
	decls, err := Parse("database=outputs.host:outputs.port,admin=outputs.host:outputs.adminPort")
	require.NoError(t, err)

	got := Resolve(decls, map[string]Output{
		"host":      {Value: "pg.ns.svc.cluster.local"},
		"port":      {Value: "5432"},
		"adminPort": {Value: "8080"},
	})
	require.Equal(t, []Address{
		{Name: "database", HostOutput: "host", PortOutput: "port", Host: "pg.ns.svc.cluster.local", Port: 5432},
		{Name: "admin", HostOutput: "host", PortOutput: "adminPort", Host: "pg.ns.svc.cluster.local", Port: 8080},
	}, got)
}

// An address that cannot be dialed comes back with a reason rather than being dropped,
// so a caller reports it against the address it belongs to.
func TestResolveReportsReasons(t *testing.T) {
	decls, err := Parse("database=outputs.host:outputs.port")
	require.NoError(t, err)

	for _, tt := range []struct {
		name    string
		outputs map[string]Output
		want    string
	}{
		{"reference-backed", map[string]Output{
			"host": {Reference: true}, "port": {Value: "5432"}}, "only through a Secret or ConfigMap"},
		{"host not declared", map[string]Output{
			"port": {Value: "5432"}}, `host output "host" is not declared`},
		{"port not declared", map[string]Output{
			"host": {Value: "h"}}, `port output "port" is not declared`},
		{"host empty", map[string]Output{
			"host": {Value: ""}, "port": {Value: "5432"}}, `output "host" has no value yet`},
		{"port not numeric", map[string]Output{
			"host": {Value: "h"}, "port": {Value: "nope"}}, `port "nope" is not numeric`},
		{"port out of range", map[string]Output{
			"host": {Value: "h"}, "port": {Value: "99999"}}, "out of range"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(decls, tt.outputs)
			require.Len(t, got, 1)
			require.Empty(t, got[0].Host)
			require.Zero(t, got[0].Port)
			require.Contains(t, got[0].Reason, tt.want)
			// The output mapping survives, so a consumer's bindings still map to it.
			require.Equal(t, "host", got[0].HostOutput)
			require.Equal(t, "port", got[0].PortOutput)
		})
	}
}

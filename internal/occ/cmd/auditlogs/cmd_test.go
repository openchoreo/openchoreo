// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auditlogs

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func errFactory(msg string) client.NewClientFunc {
	return func() (client.Interface, error) {
		return nil, fmt.Errorf("%s", msg)
	}
}

func TestNewAuditLogsCmd_Use(t *testing.T) {
	cmd := NewAuditLogsCmd(errFactory("unused"))
	assert.Equal(t, "auditlogs", cmd.Name())
	assert.Contains(t, cmd.Aliases, "audit-logs")
}

func TestAuditLogsCmd_Flags(t *testing.T) {
	cmd := NewAuditLogsCmd(errFactory("unused"))
	for _, name := range []string{
		"since", "start", "end", "limit", "sort", "output",
		"actor", "actor-type", "issuer", "session-id", "entitlement",
		"action", "category", "result", "surface", "producer", "operation-id",
		"request-id", "event-id", "source-ip", "user-agent",
		"resource-type", "resource-name", "namespace", "project", "component", "resource", "env",
		"search",
	} {
		assert.NotNil(t, cmd.Flags().Lookup(name), "missing flag --%s", name)
	}

	assert.Equal(t, "o", cmd.Flags().Lookup("output").Shorthand)
	assert.Equal(t, "n", cmd.Flags().Lookup("namespace").Shorthand)
	assert.Equal(t, "p", cmd.Flags().Lookup("project").Shorthand)
	assert.Equal(t, "c", cmd.Flags().Lookup("component").Shorthand)
}

func TestAuditLogsCmd_IgnoresContextDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	require.NoError(t, config.SaveStoredConfig(&config.StoredConfig{
		CurrentContext: "ctx1",
		Contexts: []config.Context{{
			Name:      "ctx1",
			Namespace: "ns1",
			Project:   "proj1",
			Component: "comp1",
			Resource:  "res1",
		}},
	}))

	cmd := NewAuditLogsCmd(errFactory("unused"))
	require.NoError(t, config.ApplyContextDefaults(cmd))

	params := queryParams(cmd)
	assert.Empty(t, params.Namespaces)
	assert.Empty(t, params.Projects)
	assert.Empty(t, params.Components)
	assert.Empty(t, params.Resources)
}

func TestAuditLogsCmd_RejectsArgs(t *testing.T) {
	cmd := NewAuditLogsCmd(errFactory("unused"))
	assert.Error(t, cmd.Args(cmd, []string{"extra"}))
}

func TestAuditLogsCmd_SinceAndStartAreExclusive(t *testing.T) {
	cmd := NewAuditLogsCmd(errFactory("unused"))
	require.NoError(t, cmd.Flags().Set("since", "1h"))
	require.NoError(t, cmd.Flags().Set("start", "2026-08-01T00:00:00Z"))
	assert.ErrorContains(t, cmd.ValidateFlagGroups(), "none of the others can be")
}

func TestAuditLogsCmd_FactoryError(t *testing.T) {
	cmd := NewAuditLogsCmd(errFactory("factory failed"))
	assert.EqualError(t, cmd.RunE(cmd, nil), "factory failed")
}

func TestQueryParams(t *testing.T) {
	cmd := NewAuditLogsCmd(errFactory("unused"))
	for flag, value := range map[string]string{
		"since":         "7d",
		"end":           "2026-09-01T00:00:00Z",
		"limit":         "50",
		"sort":          sortAsc,
		"output":        outputJSON,
		"actor":         "alice@example.com,bob@example.com",
		"actor-type":    "user",
		"issuer":        "https://idp.example.com",
		"session-id":    "sid-1",
		"entitlement":   "platform-engineer",
		"action":        "create_project",
		"category":      "management",
		"result":        "denied",
		"surface":       "mcp",
		"producer":      "openchoreo-api",
		"operation-id":  "CreateProject",
		"request-id":    "req-1",
		"event-id":      "evt-1",
		"source-ip":     "10.0.0.1",
		"user-agent":    "occ/1.2.0",
		"resource-type": "project",
		"resource-name": "online-store",
		"namespace":     "acme-corp",
		"project":       "online-store",
		"component":     "cart",
		"resource":      "analytics-db",
		"env":           "acme-corp/production",
		"search":        "store",
	} {
		require.NoError(t, cmd.Flags().Set(flag, value), flag)
	}

	params := queryParams(cmd)
	assert.Equal(t, QueryParams{
		Since:         "7d",
		End:           "2026-09-01T00:00:00Z",
		Limit:         50,
		SortOrder:     sortAsc,
		Output:        outputJSON,
		ActorIDs:      []string{"alice@example.com", "bob@example.com"},
		ActorTypes:    []string{"user"},
		Issuers:       []string{"https://idp.example.com"},
		SessionIDs:    []string{"sid-1"},
		Entitlements:  []string{"platform-engineer"},
		Actions:       []string{"create_project"},
		Categories:    []string{"management"},
		Results:       []string{"denied"},
		Surfaces:      []string{"mcp"},
		Producers:     []string{"openchoreo-api"},
		OperationIDs:  []string{"CreateProject"},
		RequestIDs:    []string{"req-1"},
		EventIDs:      []string{"evt-1"},
		SourceIPs:     []string{"10.0.0.1"},
		UserAgents:    []string{"occ/1.2.0"},
		ResourceTypes: []string{"project"},
		ResourceNames: []string{"online-store"},
		Namespaces:    []string{"acme-corp"},
		Projects:      []string{"online-store"},
		Components:    []string{"cart"},
		Resources:     []string{"analytics-db"},
		Environments:  []string{"acme-corp/production"},
		Search:        "store",
	}, params)
}

func TestQueryParams_Defaults(t *testing.T) {
	params := queryParams(NewAuditLogsCmd(errFactory("unused")))
	assert.Empty(t, params.Since)
	assert.Equal(t, defaultLimit, params.Limit)
	assert.Equal(t, sortDesc, params.SortOrder)
	assert.Equal(t, outputText, params.Output)
	assert.Empty(t, params.ActorIDs)
}

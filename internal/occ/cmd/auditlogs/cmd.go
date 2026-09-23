// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auditlogs

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

const longDesc = `Query the audit trail: who did what, from where, and whether it was allowed.

The observer that serves the trail is discovered from the control plane, so there is no
plane to name. Reading the trail needs the cluster-scoped 'auditlogs:view' permission.
The --namespace, --project, --component, --resource and --env filters narrow the result;
they do not widen what you are authorized to read.

Multi-value filters match any of their values, and different filters must all match.

The window defaults to the last 24 hours and may span at most 366 days. Records come back
newest first unless --sort asc is given, and at most --limit of them. When the window
holds more, the command says so on stderr and prints the --start/--end that fetch the
next page.`

const example = `  # Everything in the last 24 hours
  occ auditlogs

  # Denied requests in the last 7 days
  occ auditlogs --since 7d --result denied

  # What one user changed in a namespace, as JSON
  occ auditlogs --actor alice@example.com --namespace acme-corp --category management -o json

  # Activity in one environment over an absolute window
  occ auditlogs --env acme-corp/production --start 2026-08-01T00:00:00Z --end 2026-09-01T00:00:00Z

  # The audit record for a request seen in an access log
  occ auditlogs --since 30d --request-id 4f8c2e1a-...`

// NewAuditLogsCmd builds the top-level `auditlogs` command.
func NewAuditLogsCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "auditlogs",
		Aliases: []string{"audit-logs", "auditlog"},
		Short:   "Query the OpenChoreo audit trail",
		Long:    longDesc,
		Example: example,
		Args:    cobra.NoArgs,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Query(queryParams(cmd))
		},
	}
	addQueryFlags(cmd)
	return cmd
}

func addQueryFlags(cmd *cobra.Command) {
	fl := cmd.Flags()

	fl.String("since", "", "Only return records newer than a relative duration like 30m, 24h or 7d (default 24h)")
	fl.String("start", "", "Inclusive start of the window, in RFC 3339 (e.g. 2026-08-01T00:00:00Z)")
	fl.String("end", "", "Exclusive end of the window, in RFC 3339 (default now)")
	cmd.MarkFlagsMutuallyExclusive("since", "start")
	fl.Int("limit", defaultLimit, "Maximum number of records to return (1-1000)")
	fl.String("sort", sortDesc, "Order by event time: 'desc' or 'asc'")
	fl.StringP("output", "o", outputText, "Output format: 'text' or 'json' (json emits one record per line)")

	fl.StringSlice("actor", nil, "Actor IDs; the claim recorded depends on the subject's type, so pair with --actor-type (comma-separated)")
	fl.StringSlice("actor-type", nil, "Kinds of subject, e.g. user, service_account, anonymous (comma-separated)")
	fl.StringSlice("issuer", nil, "Token issuers; pair with --actor where more than one identity provider is configured (comma-separated)")
	fl.StringSlice("session-id", nil, "Identity provider session IDs, joining the actions of one login (comma-separated)")
	fl.StringSlice("entitlement", nil, "Entitlement values such as group names (comma-separated)")
	fl.StringSlice("action", nil, "Semantic action names, e.g. create_project (comma-separated)")
	fl.StringSlice("category", nil, "Event categories: management, authorization, access (comma-separated)")
	fl.StringSlice("result", nil, "Outcomes: success, failure, denied (comma-separated)")
	fl.StringSlice("surface", nil, "API surfaces the call arrived through: rest, mcp (comma-separated)")
	fl.StringSlice("producer", nil, "Emitting services, e.g. openchoreo-api (comma-separated)")
	fl.StringSlice("operation-id", nil, "Canonical operation identifiers, e.g. CreateProject (comma-separated)")
	fl.StringSlice("request-id", nil, "Request correlation IDs (comma-separated)")
	fl.StringSlice("event-id", nil, "Audit record IDs (comma-separated)")
	fl.StringSlice("source-ip", nil, "Client addresses, matched exactly (comma-separated)")
	fl.StringSlice("user-agent", nil, "Client identifications, matched exactly; --search suits partial matches (comma-separated)")

	fl.StringSlice("resource-type", nil, "Kinds of the target resource, e.g. project (comma-separated)")
	fl.StringSlice("resource-name", nil, "Names of the target resource (comma-separated)")
	fl.StringSliceP("namespace", "n", nil, "OpenChoreo namespaces (comma-separated)")
	fl.StringSliceP("project", "p", nil, "Projects (comma-separated)")
	fl.StringSliceP("component", "c", nil, "Components (comma-separated)")
	fl.StringSlice("resource", nil, "Resources, the hierarchy level beside components (comma-separated)")
	fl.StringSlice("env", nil,
		"Environments as <namespace>/<name>; a bare name is qualified with --namespace when exactly one is given (comma-separated)")
	fl.String("search", "", "Only return records containing this text")
}

func queryParams(cmd *cobra.Command) QueryParams {
	fl := cmd.Flags()
	str := func(name string) string {
		v, _ := fl.GetString(name)
		return v
	}
	slice := func(name string) []string {
		v, _ := fl.GetStringSlice(name)
		return v
	}
	limit, _ := fl.GetInt("limit")

	return QueryParams{
		Since:         str("since"),
		Start:         str("start"),
		End:           str("end"),
		Limit:         limit,
		SortOrder:     str("sort"),
		Output:        str("output"),
		ActorIDs:      slice("actor"),
		ActorTypes:    slice("actor-type"),
		Issuers:       slice("issuer"),
		SessionIDs:    slice("session-id"),
		Entitlements:  slice("entitlement"),
		Actions:       slice("action"),
		Categories:    slice("category"),
		Results:       slice("result"),
		Surfaces:      slice("surface"),
		Producers:     slice("producer"),
		OperationIDs:  slice("operation-id"),
		RequestIDs:    slice("request-id"),
		EventIDs:      slice("event-id"),
		SourceIPs:     slice("source-ip"),
		UserAgents:    slice("user-agent"),
		ResourceTypes: slice("resource-type"),
		ResourceNames: slice("resource-name"),
		Namespaces:    slice("namespace"),
		Projects:      slice("project"),
		Components:    slice("component"),
		Resources:     slice("resource"),
		Environments:  slice("env"),
		Search:        str("search"),
	}
}

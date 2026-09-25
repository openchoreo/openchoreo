// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auditlogs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	obsgen "github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/observerclient"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

const (
	defaultSince = "24h"
	defaultLimit = 100
	maxLimit     = 1000

	maxWindowDays = 366
	maxWindow     = maxWindowDays * 24 * time.Hour

	maxFilterValues = 20
	maxActorTypes   = 4

	outputText = "text"
	outputJSON = "json"

	sortAsc  = "asc"
	sortDesc = "desc"

	viewAction = "auditlogs:view"
)

var (
	validCategories = []string{"management", "authorization", "access"}
	validResults    = []string{"success", "failure", "denied"}
	validSurfaces   = []string{"rest", "mcp"}
)

var (
	errAuditDisabled = errors.New("audit logging is not enabled on this OpenChoreo installation")
	errNoObserver    = errors.New("audit logging is enabled, but the control plane advertises no observer serving it: " +
		"the observability plane configured for audit logs was not found")
)

type observerAPI interface {
	QueryAuditLogsWithResponse(ctx context.Context, body obsgen.QueryAuditLogsJSONRequestBody,
		reqEditors ...obsgen.RequestEditorFn) (*obsgen.QueryAuditLogsResp, error)
}

// AuditLogs queries the audit trail through the observer the control plane advertises.
type AuditLogs struct {
	client client.Interface
}

// New creates an AuditLogs backed by the given control plane client.
func New(c client.Interface) *AuditLogs {
	return &AuditLogs{client: c}
}

// Query discovers the audit observer, runs one query against it and prints the records.
func (a *AuditLogs) Query(params QueryParams) error {
	ctx := context.Background()

	body, err := buildRequest(params, time.Now())
	if err != nil {
		return err
	}

	observerURL, err := a.resolveObserverURL(ctx)
	if err != nil {
		return err
	}

	credential, err := config.GetCurrentCredential()
	if err != nil {
		return fmt.Errorf("failed to get credentials: %w", err)
	}
	if credential == nil {
		return fmt.Errorf("no current credential available")
	}

	api, err := observerclient.New(observerURL, credential.Token)
	if err != nil {
		return fmt.Errorf("failed to create observer client: %w", err)
	}

	resp, err := fetch(ctx, api, body)
	if err != nil {
		return err
	}

	if err := printRecords(resp.Records, params.Output); err != nil {
		return err
	}
	if len(resp.Records) == 0 {
		fmt.Fprintln(os.Stderr, "No audit records matched the query.")
	}
	if hint := nextPageHint(body, resp); hint != "" {
		fmt.Fprintln(os.Stderr, hint)
	}
	return nil
}

func (a *AuditLogs) resolveObserverURL(ctx context.Context) (string, error) {
	md, err := a.client.GetMetadata(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to discover the audit log observer: %w", err)
	}
	feature := md.Features.AuditLogs
	if !feature.Enabled {
		return "", errAuditDisabled
	}
	if feature.ObserverURL == nil || *feature.ObserverURL == "" {
		return "", errNoObserver
	}
	return *feature.ObserverURL, nil
}

func fetch(ctx context.Context, api observerAPI, body obsgen.AuditLogsQueryRequest) (*obsgen.AuditLogsResponse, error) {
	resp, err := api.QueryAuditLogsWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("audit log query failed: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, observerError(resp)
	}
	return resp.JSON200, nil
}

func buildRequest(params QueryParams, now time.Time) (obsgen.AuditLogsQueryRequest, error) {
	var body obsgen.AuditLogsQueryRequest

	if params.Output != outputText && params.Output != outputJSON {
		return body, fmt.Errorf("invalid --output %q: must be %q or %q", params.Output, outputText, outputJSON)
	}
	if params.SortOrder != sortAsc && params.SortOrder != sortDesc {
		return body, fmt.Errorf("invalid --sort %q: must be %q or %q", params.SortOrder, sortDesc, sortAsc)
	}
	if params.Limit < 1 || params.Limit > maxLimit {
		return body, fmt.Errorf("invalid --limit %d: must be between 1 and %d", params.Limit, maxLimit)
	}

	for _, values := range []*[]string{
		&params.ActorIDs, &params.ActorTypes, &params.Issuers, &params.SessionIDs, &params.Entitlements,
		&params.Actions, &params.Categories, &params.Results, &params.Surfaces, &params.Producers,
		&params.OperationIDs, &params.RequestIDs, &params.EventIDs, &params.SourceIPs, &params.UserAgents,
		&params.ResourceTypes, &params.ResourceNames, &params.Namespaces, &params.Projects,
		&params.Components, &params.Resources, &params.Environments,
	} {
		*values = clean(*values)
	}
	if err := checkCounts(params); err != nil {
		return body, err
	}

	start, end, err := resolveWindow(params, now)
	if err != nil {
		return body, err
	}

	categories, err := closedSet("category", params.Categories, validCategories)
	if err != nil {
		return body, err
	}
	results, err := closedSet("result", params.Results, validResults)
	if err != nil {
		return body, err
	}
	surfaces, err := closedSet("surface", params.Surfaces, validSurfaces)
	if err != nil {
		return body, err
	}
	environments, err := qualifyEnvironments(params.Environments, params.Namespaces)
	if err != nil {
		return body, err
	}

	limit := params.Limit
	sortOrder := obsgen.AuditLogsQueryRequestSortOrder(params.SortOrder)
	body = obsgen.AuditLogsQueryRequest{
		StartTime:   start.UTC(),
		EndTime:     end.UTC(),
		Limit:       &limit,
		SortOrder:   &sortOrder,
		Action:      optional(params.Actions),
		Producer:    optional(params.Producers),
		OperationId: optional(params.OperationIDs),
		RequestId:   optional(params.RequestIDs),
		EventId:     optional(params.EventIDs),
		SourceIp:    optional(params.SourceIPs),
		UserAgent:   optional(params.UserAgents),
		Category:    convert[obsgen.AuditLogsQueryRequestCategory](categories),
		Result:      convert[obsgen.AuditLogsQueryRequestResult](results),
		Surface:     convert[obsgen.AuditLogsQueryRequestSurface](surfaces),
	}
	if params.Search != "" {
		body.SearchPhrase = &params.Search
	}

	actor := obsgen.AuditLogsActorFilter{
		Id:           optional(params.ActorIDs),
		Type:         optional(params.ActorTypes),
		Issuer:       optional(params.Issuers),
		SessionId:    optional(params.SessionIDs),
		Entitlements: optional(params.Entitlements),
	}
	if actor != (obsgen.AuditLogsActorFilter{}) {
		body.Actor = &actor
	}

	resource := obsgen.AuditLogsResourceFilter{
		Type:        optional(params.ResourceTypes),
		Name:        optional(params.ResourceNames),
		Namespace:   optional(params.Namespaces),
		Project:     optional(params.Projects),
		Component:   optional(params.Components),
		Resource:    optional(params.Resources),
		Environment: optional(environments),
	}
	if resource != (obsgen.AuditLogsResourceFilter{}) {
		body.Resource = &resource
	}

	return body, nil
}

func resolveWindow(params QueryParams, now time.Time) (time.Time, time.Time, error) {
	var zero time.Time
	if params.Since != "" && params.Start != "" {
		return zero, zero, fmt.Errorf("--since and --start cannot be used together")
	}

	end := now
	if params.End != "" {
		parsed, err := time.Parse(time.RFC3339, params.End)
		if err != nil {
			return zero, zero, fmt.Errorf("invalid --end %q: must be RFC 3339, e.g. 2026-09-01T00:00:00Z", params.End)
		}
		end = parsed
	}

	var start time.Time
	if params.Start != "" {
		parsed, err := time.Parse(time.RFC3339, params.Start)
		if err != nil {
			return zero, zero, fmt.Errorf("invalid --start %q: must be RFC 3339, e.g. 2026-08-01T00:00:00Z", params.Start)
		}
		start = parsed
	} else {
		since := params.Since
		if since == "" {
			since = defaultSince
		}
		duration, err := parseSince(since)
		if err != nil {
			return zero, zero, err
		}
		start = end.Add(-duration)
	}

	if !end.After(start) {
		return zero, zero, fmt.Errorf("invalid window: --end must be later than --start")
	}
	if end.Sub(start) > maxWindow {
		return zero, zero, fmt.Errorf("invalid window: the observer accepts a window of at most 366 days")
	}
	return start, end, nil
}

func parseSince(since string) (time.Duration, error) {
	var duration time.Duration
	if days, ok := strings.CutSuffix(since, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil {
			return 0, fmt.Errorf("invalid --since %q: use a duration like 30m, 24h or 7d", since)
		}
		if n > maxWindowDays {
			return 0, fmt.Errorf("invalid --since %q: the observer accepts a window of at most %d days", since, maxWindowDays)
		}
		duration = time.Duration(n) * 24 * time.Hour
	} else {
		parsed, err := time.ParseDuration(since)
		if err != nil {
			return 0, fmt.Errorf("invalid --since %q: use a duration like 30m, 24h or 7d", since)
		}
		duration = parsed
	}
	if duration <= 0 {
		return 0, fmt.Errorf("invalid --since %q: duration must be positive", since)
	}
	return duration, nil
}

func qualifyEnvironments(environments, namespaces []string) ([]string, error) {
	qualified := make([]string, 0, len(environments))
	for _, env := range environments {
		if namespace, name, ok := strings.Cut(env, "/"); ok {
			if strings.TrimSpace(namespace) == "" || strings.TrimSpace(name) == "" || strings.Contains(name, "/") {
				return nil, fmt.Errorf("invalid --env %q: must be <namespace>/<name>", env)
			}
			qualified = append(qualified, strings.TrimSpace(namespace)+"/"+strings.TrimSpace(name))
			continue
		}
		if len(namespaces) != 1 {
			return nil, fmt.Errorf("invalid --env %q: environments are recorded as <namespace>/<name>; "+
				"qualify it, or give exactly one --namespace", env)
		}
		qualified = append(qualified, strings.TrimSpace(namespaces[0])+"/"+env)
	}
	return qualified, nil
}

func closedSet(flag string, values, valid []string) ([]string, error) {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		lower := strings.ToLower(value)
		if !slices.Contains(valid, lower) {
			return nil, fmt.Errorf("invalid --%s %q: must be one of %s", flag, value, strings.Join(valid, ", "))
		}
		if !slices.Contains(normalized, lower) {
			normalized = append(normalized, lower)
		}
	}
	return normalized, nil
}

func clean(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !slices.Contains(cleaned, value) {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

func checkCounts(params QueryParams) error {
	for _, f := range []struct {
		flag   string
		values []string
		max    int
	}{
		{"actor", params.ActorIDs, maxFilterValues},
		{"actor-type", params.ActorTypes, maxActorTypes},
		{"issuer", params.Issuers, maxFilterValues},
		{"session-id", params.SessionIDs, maxFilterValues},
		{"entitlement", params.Entitlements, maxFilterValues},
		{"action", params.Actions, maxFilterValues},
		{"producer", params.Producers, maxFilterValues},
		{"operation-id", params.OperationIDs, maxFilterValues},
		{"request-id", params.RequestIDs, maxFilterValues},
		{"event-id", params.EventIDs, maxFilterValues},
		{"source-ip", params.SourceIPs, maxFilterValues},
		{"user-agent", params.UserAgents, maxFilterValues},
		{"resource-type", params.ResourceTypes, maxFilterValues},
		{"resource-name", params.ResourceNames, maxFilterValues},
		{"namespace", params.Namespaces, maxFilterValues},
		{"project", params.Projects, maxFilterValues},
		{"component", params.Components, maxFilterValues},
		{"resource", params.Resources, maxFilterValues},
		{"env", params.Environments, maxFilterValues},
	} {
		if len(f.values) > f.max {
			return fmt.Errorf("invalid --%s: at most %d values are accepted, got %d", f.flag, f.max, len(f.values))
		}
	}
	return nil
}

func optional(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	return &values
}

func convert[T ~string](values []string) *[]T {
	if len(values) == 0 {
		return nil
	}
	converted := make([]T, len(values))
	for i, value := range values {
		converted[i] = T(value)
	}
	return &converted
}

func nextPageHint(body obsgen.AuditLogsQueryRequest, resp *obsgen.AuditLogsResponse) string {
	shown := int64(len(resp.Records))
	if shown == 0 || resp.Total <= shown {
		return ""
	}

	last := resp.Records[len(resp.Records)-1].EventTime.UTC()
	start, end := body.StartTime, body.EndTime
	caveat := ""
	if body.SortOrder != nil && *body.SortOrder == obsgen.AuditLogsQueryRequestSortOrderAsc {
		// Adapters store event times at different precisions, from milliseconds to
		// 100 ns ticks, and occ cannot tell which one it is reading. The step must be
		// finer than all of them: an adapter that truncates the bound to its own unit
		// then repeats records from that unit instead of skipping the rest of it.
		start = last.Add(time.Nanosecond)
		caveat = "A store keeping coarser timestamps than the records may repeat a few already shown. "
	} else {
		end = last
	}
	if !end.After(start) {
		return fmt.Sprintf("Showing %d of %d records. The rest share the last record's time and no narrower window "+
			"reaches them; raise --limit to include them.", shown, resp.Total)
	}
	return fmt.Sprintf("Showing %d of %d records. For the next page, run the same query with "+
		"--start %s --end %s in place of any --since, --start or --end. %s"+
		"Records at that boundary instant that did not fit on this page are skipped; raise --limit to include them.",
		shown, resp.Total, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano), caveat)
}

func observerError(resp *obsgen.QueryAuditLogsResp) error {
	status := 0
	if resp.HTTPResponse != nil {
		status = resp.HTTPResponse.StatusCode
	}

	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("the audit log observer rejected your credentials (HTTP 401): run 'occ login' and try again")
	case http.StatusNotImplemented:
		return fmt.Errorf("the audit log observer does not serve audit logs: its logs adapter has not implemented them, " +
			"or the installation forwards the trail elsewhere without keeping a queryable copy")
	case http.StatusForbidden:
		return fmt.Errorf("not authorized to read audit logs: this needs the cluster-scoped %s permission", viewAction)
	}

	if msg := observerMessage(resp.Body); msg != "" {
		return fmt.Errorf("audit log query failed (HTTP %d): %s", status, msg)
	}
	if len(resp.Body) > 0 {
		return fmt.Errorf("audit log query failed (HTTP %d): %s", status, string(resp.Body))
	}
	return fmt.Errorf("audit log query failed (HTTP %d)", status)
}

func observerMessage(body []byte) string {
	var errResp obsgen.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Message != nil {
		return *errResp.Message
	}
	return ""
}

func printRecords(records []obsgen.AuditLogRecord, output string) error {
	if output == outputJSON {
		for _, record := range records {
			encoded, err := json.Marshal(record)
			if err != nil {
				return fmt.Errorf("failed to encode audit record: %w", err)
			}
			fmt.Println(string(encoded))
		}
		return nil
	}

	if len(records) == 0 {
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "TIME\tRESULT\tACTOR\tACTION\tNAMESPACE\tRESOURCE")
	for _, record := range records {
		namespace := ""
		if record.Resource != nil {
			namespace = deref(record.Resource.Namespace)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			record.EventTime.UTC().Format(time.RFC3339),
			record.Result,
			orDash(record.Actor.Id),
			record.Action,
			orDash(namespace),
			orDash(resourceLabel(record.Resource)))
	}
	return w.Flush()
}

func resourceLabel(resource *obsgen.AuditLogResource) string {
	if resource == nil {
		return ""
	}
	typ, name := deref(resource.Type), deref(resource.Name)
	switch {
	case typ != "" && name != "":
		return typ + "/" + name
	case name != "":
		return name
	default:
		return typ
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auditlogs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	obsgen "github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const observerTestURL = "http://observer.test"

func setupConfig(t *testing.T) {
	t.Helper()
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, config.StoredConfig{
		CurrentContext: "test",
		ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: "http://mock-api"}},
		Credentials:    []config.Credential{{Name: "cred", Token: testutil.NonExpiredJWT}},
		Contexts:       []config.Context{{Name: "test", ControlPlane: "cp", Credentials: "cred"}},
	})
}

func mockMetadata(t *testing.T, feature gen.AuditLogsFeature) *mocks.MockInterface {
	t.Helper()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetMetadata(mock.Anything).Return(
		&gen.MetadataResponse{Features: gen.MetadataFeatures{AuditLogs: feature}}, nil)
	return mc
}

func enabledMetadata(t *testing.T) *mocks.MockInterface {
	t.Helper()
	observerURL := observerTestURL
	return mockMetadata(t, gen.AuditLogsFeature{Enabled: true, ObserverURL: &observerURL})
}

func defaultParams() QueryParams {
	return QueryParams{Limit: defaultLimit, SortOrder: sortDesc, Output: outputText}
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	require.NoError(t, err)
	return parsed
}

func strPtr(s string) *string { return &s }

func sampleRecord(t *testing.T, eventTime string) obsgen.AuditLogRecord {
	t.Helper()
	return obsgen.AuditLogRecord{
		SchemaVersion: "1.0",
		EventId:       "evt-" + eventTime,
		EventTime:     mustTime(t, eventTime),
		Actor:         obsgen.AuditLogActor{Type: "user", Id: "alice@example.com"},
		Action:        "create_project",
		Category:      "management",
		Result:        "success",
		Resource: &obsgen.AuditLogResource{
			Type:      strPtr("project"),
			Name:      strPtr("online-store"),
			Namespace: strPtr("acme-corp"),
		},
	}
}

func TestQuery_SendsFiltersAndPrintsTable(t *testing.T) {
	setupConfig(t)
	mc := enabledMetadata(t)

	var got map[string]any
	testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "observer.test", r.URL.Host)
		assert.Equal(t, "/api/v1alpha1/audit-logs/query", r.URL.Path)
		assert.Equal(t, "Bearer "+testutil.NonExpiredJWT, r.Header.Get("Authorization"))
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &got))
		return testutil.JSONResp(http.StatusOK, obsgen.AuditLogsResponse{
			Records: []obsgen.AuditLogRecord{sampleRecord(t, "2026-08-14T16:30:00Z")},
			Total:   1,
		}), nil
	}))

	params := defaultParams()
	params.Start = "2026-08-14T00:00:00Z"
	params.End = "2026-08-15T00:00:00Z"
	params.ActorIDs = []string{"alice@example.com"}
	params.Categories = []string{"Management"}
	params.Namespaces = []string{"acme-corp"}
	params.Environments = []string{"production"}
	params.Search = "store"

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Query(params))
	})

	assert.Equal(t, "2026-08-14T00:00:00Z", got["startTime"])
	assert.Equal(t, "2026-08-15T00:00:00Z", got["endTime"])
	assert.EqualValues(t, defaultLimit, got["limit"])
	assert.Equal(t, sortDesc, got["sortOrder"])
	assert.Equal(t, "store", got["searchPhrase"])
	assert.Equal(t, []any{"management"}, got["category"])
	assert.Equal(t, map[string]any{"id": []any{"alice@example.com"}}, got["actor"])
	assert.Equal(t, map[string]any{
		"namespace":   []any{"acme-corp"},
		"environment": []any{"acme-corp/production"},
	}, got["resource"])
	assert.NotContains(t, got, "result")

	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, []string{"TIME", "RESULT", "ACTOR", "ACTION", "NAMESPACE", "RESOURCE"}, strings.Fields(lines[0]))
	assert.Equal(t, []string{
		"2026-08-14T16:30:00Z", "success", "alice@example.com", "create_project", "acme-corp", "project/online-store",
	}, strings.Fields(lines[1]))
}

func TestQuery_JSONOutputIsOneRecordPerLine(t *testing.T) {
	setupConfig(t)
	mc := enabledMetadata(t)

	testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return testutil.JSONResp(http.StatusOK, obsgen.AuditLogsResponse{
			Records: []obsgen.AuditLogRecord{
				sampleRecord(t, "2026-08-14T16:31:00Z"),
				sampleRecord(t, "2026-08-14T16:30:00Z"),
			},
			Total: 2,
		}), nil
	}))

	params := defaultParams()
	params.Output = outputJSON
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Query(params))
	})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 2)
	var record obsgen.AuditLogRecord
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &record))
	assert.Equal(t, "evt-2026-08-14T16:31:00Z", record.EventId)
}

func TestQuery_RejectsBadFlagsBeforeCallingTheAPI(t *testing.T) {
	params := defaultParams()
	params.Results = []string{"allowed"}
	err := New(mocks.NewMockInterface(t)).Query(params)
	assert.ErrorContains(t, err, `invalid --result "allowed"`)
}

func TestQuery_Discovery(t *testing.T) {
	empty := ""
	tests := []struct {
		name    string
		feature gen.AuditLogsFeature
		wantErr error
	}{
		{name: "disabled", feature: gen.AuditLogsFeature{Enabled: false}, wantErr: errAuditDisabled},
		{name: "enabled without observer", feature: gen.AuditLogsFeature{Enabled: true}, wantErr: errNoObserver},
		{name: "enabled with empty observer", feature: gen.AuditLogsFeature{Enabled: true, ObserverURL: &empty}, wantErr: errNoObserver},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(mockMetadata(t, tt.feature)).Query(defaultParams())
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestQuery_MetadataError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetMetadata(mock.Anything).Return(nil, errors.New("unauthorized"))

	err := New(mc).Query(defaultParams())
	assert.ErrorContains(t, err, "failed to discover the audit log observer: unauthorized")
}

func TestQuery_ObserverErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    any
		wantErr string
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, body: map[string]string{"message": "invalid token"},
			wantErr: "rejected your credentials (HTTP 401): run 'occ login'"},
		{name: "forbidden", status: http.StatusForbidden, body: map[string]string{"message": "forbidden"},
			wantErr: "needs the cluster-scoped auditlogs:view permission"},
		{name: "not implemented", status: http.StatusNotImplemented, body: map[string]string{"message": "nope"},
			wantErr: "does not serve audit logs"},
		{name: "bad request", status: http.StatusBadRequest, body: map[string]string{"message": "window too wide"},
			wantErr: "audit log query failed (HTTP 400): window too wide"},
		{name: "unstructured", status: http.StatusBadGateway, body: "upstream down",
			wantErr: "audit log query failed (HTTP 502)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupConfig(t)
			mc := enabledMetadata(t)
			testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				return testutil.JSONResp(tt.status, tt.body), nil
			}))

			err := New(mc).Query(defaultParams())
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestBuildRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*QueryParams)
		wantErr string
	}{
		{name: "output", modify: func(p *QueryParams) { p.Output = "yaml" }, wantErr: `invalid --output "yaml"`},
		{name: "sort", modify: func(p *QueryParams) { p.SortOrder = "newest" }, wantErr: `invalid --sort "newest"`},
		{name: "limit too low", modify: func(p *QueryParams) { p.Limit = 0 }, wantErr: "invalid --limit 0"},
		{name: "limit too high", modify: func(p *QueryParams) { p.Limit = 1001 }, wantErr: "invalid --limit 1001"},
		{name: "category", modify: func(p *QueryParams) { p.Categories = []string{"read"} }, wantErr: `invalid --category "read"`},
		{name: "surface", modify: func(p *QueryParams) { p.Surfaces = []string{"api"} }, wantErr: `invalid --surface "api"`},
		{name: "bare env without namespace", modify: func(p *QueryParams) { p.Environments = []string{"dev"} },
			wantErr: `invalid --env "dev"`},
		{name: "bare env with two namespaces", modify: func(p *QueryParams) {
			p.Environments = []string{"dev"}
			p.Namespaces = []string{"a", "b"}
		}, wantErr: `invalid --env "dev"`},
		{name: "since", modify: func(p *QueryParams) { p.Since = "yesterday" }, wantErr: `invalid --since "yesterday"`},
		{name: "negative since", modify: func(p *QueryParams) { p.Since = "-1h" }, wantErr: "duration must be positive"},
		{name: "start", modify: func(p *QueryParams) { p.Start = "2026-08-01" }, wantErr: `invalid --start "2026-08-01"`},
		{name: "end", modify: func(p *QueryParams) { p.End = "now" }, wantErr: `invalid --end "now"`},
		{name: "since with start", modify: func(p *QueryParams) {
			p.Since = "1h"
			p.Start = "2026-08-01T00:00:00Z"
		}, wantErr: "cannot be used together"},
		{name: "end before start", modify: func(p *QueryParams) {
			p.Start = "2026-08-02T00:00:00Z"
			p.End = "2026-08-01T00:00:00Z"
		}, wantErr: "--end must be later than --start"},
		{name: "window too wide", modify: func(p *QueryParams) { p.Since = "367d" }, wantErr: "at most 366 days"},
		{name: "day count that overflows", modify: func(p *QueryParams) { p.Since = "106752d" }, wantErr: "at most 366 days"},
		{name: "env without namespace part", modify: func(p *QueryParams) { p.Environments = []string{"/prod"} },
			wantErr: `invalid --env "/prod": must be <namespace>/<name>`},
		{name: "env without name part", modify: func(p *QueryParams) { p.Environments = []string{"acme/"} },
			wantErr: `invalid --env "acme/": must be <namespace>/<name>`},
		{name: "env with extra segment", modify: func(p *QueryParams) { p.Environments = []string{"acme/prod/x"} },
			wantErr: `invalid --env "acme/prod/x": must be <namespace>/<name>`},
		{name: "too many actors", modify: func(p *QueryParams) { p.ActorIDs = distinct(21) },
			wantErr: "invalid --actor: at most 20 values are accepted, got 21"},
		{name: "too many actor types", modify: func(p *QueryParams) { p.ActorTypes = distinct(5) },
			wantErr: "invalid --actor-type: at most 4 values are accepted, got 5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := defaultParams()
			tt.modify(&params)
			_, err := buildRequest(params, time.Now())
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func distinct(n int) []string {
	values := make([]string, n)
	for i := range values {
		values[i] = fmt.Sprintf("v%d", i)
	}
	return values
}

func TestBuildRequest_CleansValues(t *testing.T) {
	params := defaultParams()
	params.ActorIDs = []string{"alice", " bob", "", "alice"}
	params.Results = []string{" Denied", "denied"}
	params.Namespaces = []string{" acme-corp "}
	params.Environments = []string{" production", "acme-corp/ staging "}
	params.Actions = append(distinct(20), "v0", " v1 ")

	body, err := buildRequest(params, time.Now())
	require.NoError(t, err)

	assert.Equal(t, []string{"alice", "bob"}, *body.Actor.Id)
	assert.Equal(t, []obsgen.AuditLogsQueryRequestResult{"denied"}, *body.Result)
	assert.Equal(t, []string{"acme-corp"}, *body.Resource.Namespace)
	assert.Equal(t, []string{"acme-corp/production", "acme-corp/staging"}, *body.Resource.Environment)
	assert.Len(t, *body.Action, 20)
}

func TestBuildRequest_NoFiltersLeavesGroupsAbsent(t *testing.T) {
	now := mustTime(t, "2026-09-16T12:00:00Z")
	body, err := buildRequest(defaultParams(), now)
	require.NoError(t, err)

	assert.Equal(t, now, body.EndTime)
	assert.Equal(t, now.Add(-24*time.Hour), body.StartTime)
	assert.Nil(t, body.Actor)
	assert.Nil(t, body.Resource)
	assert.Nil(t, body.Category)
	assert.Nil(t, body.SearchPhrase)
}

func TestResolveWindow(t *testing.T) {
	now := mustTime(t, "2026-09-16T12:00:00Z")
	tests := []struct {
		name      string
		params    QueryParams
		wantStart string
		wantEnd   string
	}{
		{name: "default", wantStart: "2026-09-15T12:00:00Z", wantEnd: "2026-09-16T12:00:00Z"},
		{name: "days", params: QueryParams{Since: "7d"}, wantStart: "2026-09-09T12:00:00Z", wantEnd: "2026-09-16T12:00:00Z"},
		{name: "since before end", params: QueryParams{Since: "30m", End: "2026-09-01T00:00:00Z"},
			wantStart: "2026-08-31T23:30:00Z", wantEnd: "2026-09-01T00:00:00Z"},
		{name: "start only", params: QueryParams{Start: "2026-09-16T00:00:00Z"},
			wantStart: "2026-09-16T00:00:00Z", wantEnd: "2026-09-16T12:00:00Z"},
		{name: "full year", params: QueryParams{Since: "366d"}, wantStart: "2025-09-15T12:00:00Z", wantEnd: "2026-09-16T12:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := resolveWindow(tt.params, now)
			require.NoError(t, err)
			assert.Equal(t, mustTime(t, tt.wantStart), start.UTC())
			assert.Equal(t, mustTime(t, tt.wantEnd), end.UTC())
		})
	}
}

var hintWindow = regexp.MustCompile(`--start (\S+) --end (\S+) in place of any --since, --start or --end`)

func hintedParams(t *testing.T, previous QueryParams, hint string) QueryParams {
	t.Helper()
	match := hintWindow.FindStringSubmatch(hint)
	require.NotNil(t, match, "hint does not carry a window: %q", hint)
	next := previous
	next.Since, next.Start, next.End = "", match[1], match[2]
	return next
}

func pageOf(total int64, times ...time.Time) *obsgen.AuditLogsResponse {
	records := make([]obsgen.AuditLogRecord, len(times))
	for i, eventTime := range times {
		records[i] = obsgen.AuditLogRecord{EventId: eventTime.String(), EventTime: eventTime}
	}
	return &obsgen.AuditLogsResponse{Records: records, Total: total}
}

func TestNextPageHint_Descending(t *testing.T) {
	now := mustTime(t, "2026-08-15T00:00:00Z")
	params := defaultParams()
	params.Since = "7d"
	body, err := buildRequest(params, now)
	require.NoError(t, err)

	last := mustTime(t, "2026-08-14T16:30:00Z")
	hint := nextPageHint(body, pageOf(5, mustTime(t, "2026-08-14T18:00:00Z"), last))
	assert.Contains(t, hint, "Showing 2 of 5 records.")
	assert.Contains(t, hint, "did not fit on this page are skipped")

	next, err := buildRequest(hintedParams(t, params, hint), now)
	require.NoError(t, err)
	assert.Equal(t, body.StartTime, next.StartTime)
	assert.Equal(t, last, next.EndTime)
}

func TestNextPageHint_AscendingAdvancesByANanosecond(t *testing.T) {
	now := mustTime(t, "2026-08-15T00:00:00Z")
	params := defaultParams()
	params.SortOrder = sortAsc
	params.Since = "24h"
	body, err := buildRequest(params, now)
	require.NoError(t, err)

	// A 100 ns tick, as Azure Log Analytics stores it. Records later in the same
	// millisecond must stay inside the next window.
	last := mustTime(t, "2026-08-14T16:30:00.1234567Z")
	hint := nextPageHint(body, pageOf(5, mustTime(t, "2026-08-14T01:00:00Z"), last))
	assert.Contains(t, hint, "may repeat a few already shown")

	next, err := buildRequest(hintedParams(t, params, hint), now)
	require.NoError(t, err)
	assert.Equal(t, last.Add(time.Nanosecond), next.StartTime)
	assert.True(t, next.StartTime.Before(mustTime(t, "2026-08-14T16:30:00.1234568Z")),
		"next start %s passes the following tick", next.StartTime.Format(time.RFC3339Nano))
	assert.Equal(t, body.EndTime, next.EndTime)
}

func TestNextPageHint_NoWindowWhenNextPageWouldBeEmpty(t *testing.T) {
	now := mustTime(t, "2026-08-15T00:00:00Z")
	tests := []struct {
		name      string
		sortOrder string
		last      func(body obsgen.AuditLogsQueryRequest) time.Time
	}{
		{name: "descending, last record on the start", sortOrder: sortDesc,
			last: func(body obsgen.AuditLogsQueryRequest) time.Time { return body.StartTime }},
		{name: "ascending, last record in the final nanosecond", sortOrder: sortAsc,
			last: func(body obsgen.AuditLogsQueryRequest) time.Time { return body.EndTime.Add(-time.Nanosecond) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := defaultParams()
			params.SortOrder = tt.sortOrder
			body, err := buildRequest(params, now)
			require.NoError(t, err)

			hint := nextPageHint(body, pageOf(5, tt.last(body)))
			assert.Equal(t, "Showing 1 of 5 records. The rest share the last record's time and no narrower window "+
				"reaches them; raise --limit to include them.", hint)
			assert.NotContains(t, hint, "--start")
		})
	}
}

func TestNextPageHint_NoneWhenWindowIsComplete(t *testing.T) {
	body := obsgen.AuditLogsQueryRequest{}
	assert.Empty(t, nextPageHint(body, pageOf(2, time.Now(), time.Now())))
	assert.Empty(t, nextPageHint(body, &obsgen.AuditLogsResponse{Total: 3}))
}

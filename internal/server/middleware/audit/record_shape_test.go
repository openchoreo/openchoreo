// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// fixedTime is the EventTime most cases use, so the rendered bytes don't
// depend on the clock.
var fixedTime = time.Date(2026, 9, 7, 12, 30, 45, 0, time.UTC)

// logLinePrefix is what slog's JSONHandler puts before the event's own fields.
const logLinePrefix = `{"level":"INFO","msg":"AUDIT-LOG",`

// stripTimeAttr removes slog's handler-stamped "time" attr, the wall clock at
// emission, which would defeat a byte comparison; the event's own pinned
// "event_time" is the one asserted. Stripping here rather than with a
// ReplaceAttr hook keeps NewLogger free of a test-only option.
func stripTimeAttr(line string) string {
	const timeKey = `{"time":"`
	if !strings.HasPrefix(line, timeKey) {
		return line
	}
	end := strings.Index(line, `","level":`)
	if end < 0 {
		return line
	}
	return "{" + line[end+2:]
}

type recordCase struct {
	name  string
	event *Event
	want  string
}

// recordCases covers the branches the render takes: every field populated, a
// nil Resource carrying only a hierarchy, a rejection that resolved no
// operation, a sub-second timestamp, both metadata groups, and both presence
// and absence of the http group.
func recordCases() []recordCase {
	return []recordCase{
		{
			name: "full event",
			event: &Event{
				EventID:   "01920000-0000-7000-8000-000000000001",
				EventTime: fixedTime,
				Actor: Actor{
					Type:         "user",
					ID:           "user@example.com",
					Issuer:       "https://idp.example.com/oauth2/token",
					SessionID:    "b3f1c2d4",
					Entitlements: map[string][]string{"groups": {"dev", "ops"}},
				},
				Action:       "update_release_binding",
				Category:     CategoryManagement,
				Surface:      SurfaceREST,
				OperationID:  "UpdateReleaseBinding",
				HTTP:         &HTTPInfo{Method: "PUT", Path: "/api/v1/namespaces/ns-1/release-bindings/rb-1"},
				ResourceType: "releasebindings",
				Resource:     &Resource{Namespace: "ns-1", UID: "uid-1", Name: "rb-1"},
				Hierarchy:    Hierarchy{Namespace: "ns-1", Environment: "ns-1/production", Project: "p1", Component: "c1"},
				Result:       ResultSuccess,
				RequestID:    "11111111-1111-4111-8111-111111111111",
				SourceIP:     "10.0.0.1",
				Producer:     "openchoreo-api",
				Metadata:     map[string]any{"note": "n1"},
			},
			want: logLinePrefix + `"schema_version":"1.0",` +
				`"event_id":"01920000-0000-7000-8000-000000000001",` +
				`"event_time":"2026-09-07T12:30:45Z",` +
				`"actor":{"type":"user","id":"user@example.com",` +
				`"issuer":"https://idp.example.com/oauth2/token","session_id":"b3f1c2d4",` +
				`"entitlements":{"groups":["dev","ops"]}},` +
				`"action":"update_release_binding","category":"management","result":"success",` +
				`"request_id":"11111111-1111-4111-8111-111111111111","source_ip":"10.0.0.1",` +
				`"user_agent":"",` +
				`"producer":"openchoreo-api","surface":"rest","operation_id":"UpdateReleaseBinding",` +
				`"http":{"method":"PUT","path":"/api/v1/namespaces/ns-1/release-bindings/rb-1"},` +
				`"resource":{"type":"releasebindings","namespace":"ns-1","environment":"ns-1/production",` +
				`"project":"p1","component":"c1","uid":"uid-1","name":"rb-1"},` +
				`"metadata":{"note":"n1"}}` + "\n",
		},
		{
			name: "mcp tool call omits the http group",
			event: &Event{
				EventID:      "01920000-0000-7000-8000-000000000002",
				EventTime:    fixedTime,
				Actor:        Actor{Type: "user", ID: "u1"},
				Action:       "update_project",
				Category:     CategoryManagement,
				Surface:      SurfaceMCP,
				OperationID:  "UpdateProject",
				HTTP:         nil,
				ResourceType: "projects",
				Resource:     nil,
				Hierarchy:    Hierarchy{Namespace: "ns-1", Project: "p1"},
				Result:       ResultDenied,
				RequestID:    "22222222-2222-4222-8222-222222222222",
				SourceIP:     "10.0.0.2",
				Producer:     "openchoreo-api",
			},
			want: logLinePrefix + `"schema_version":"1.0",` +
				`"event_id":"01920000-0000-7000-8000-000000000002",` +
				`"event_time":"2026-09-07T12:30:45Z",` +
				`"actor":{"type":"user","id":"u1"},` +
				`"action":"update_project","category":"management","result":"denied",` +
				`"request_id":"22222222-2222-4222-8222-222222222222","source_ip":"10.0.0.2",` +
				`"user_agent":"",` +
				`"producer":"openchoreo-api","surface":"mcp","operation_id":"UpdateProject",` +
				`"resource":{"type":"projects","namespace":"ns-1","project":"p1"}}` + "\n",
		},
		{
			name: "unauthenticated with no operation",
			event: &Event{
				EventID:   "01920000-0000-7000-8000-000000000003",
				EventTime: fixedTime,
				Actor:     Actor{Type: "anonymous", ID: "anonymous"},
				Result:    ResultUnauthenticated,
				RequestID: "33333333-3333-4333-8333-333333333333",
				SourceIP:  "10.0.0.3",
				Producer:  "openchoreo-api",
				// http is present where operation_id is not: the rejection
				// resolved no operation but did have a request line.
				HTTP: &HTTPInfo{Method: "POST", Path: "/api/v1/namespaces/ns-1/projects"},
			},
			// No "resource" at all: nothing resource-shaped was resolved.
			want: logLinePrefix + `"schema_version":"1.0",` +
				`"event_id":"01920000-0000-7000-8000-000000000003",` +
				`"event_time":"2026-09-07T12:30:45Z",` +
				`"actor":{"type":"anonymous","id":"anonymous"},` +
				`"action":"","category":"","result":"unauthenticated",` +
				`"request_id":"33333333-3333-4333-8333-333333333333","source_ip":"10.0.0.3",` +
				`"user_agent":"",` +
				`"producer":"openchoreo-api",` +
				`"http":{"method":"POST","path":"/api/v1/namespaces/ns-1/projects"}}` + "\n",
		},
		{
			// Paired with "mcp tool call omits the http group": together they
			// pin that http's presence is not derivable from origin.
			name: "mcp rejection at the http boundary carries the http group",
			event: &Event{
				EventID:   "01920000-0000-7000-8000-000000000008",
				EventTime: fixedTime,
				Actor:     Actor{Type: "anonymous", ID: "anonymous"},
				Surface:   SurfaceMCP,
				Result:    ResultUnauthenticated,
				RequestID: "88888888-8888-4888-8888-888888888888",
				SourceIP:  "10.0.0.8",
				Producer:  "openchoreo-api",
				HTTP:      &HTTPInfo{Method: "POST", Path: "/mcp"},
			},
			want: logLinePrefix + `"schema_version":"1.0",` +
				`"event_id":"01920000-0000-7000-8000-000000000008",` +
				`"event_time":"2026-09-07T12:30:45Z",` +
				`"actor":{"type":"anonymous","id":"anonymous"},` +
				`"action":"","category":"","result":"unauthenticated",` +
				`"request_id":"88888888-8888-4888-8888-888888888888","source_ip":"10.0.0.8",` +
				`"user_agent":"",` +
				`"producer":"openchoreo-api","surface":"mcp",` +
				`"http":{"method":"POST","path":"/mcp"}}` + "\n",
		},
		{
			name: "sub-second timestamp",
			event: &Event{
				EventID:   "01920000-0000-7000-8000-000000000005",
				EventTime: time.Date(2026, 9, 7, 12, 30, 45, 123456789, time.UTC),
				Actor:     Actor{Type: "user", ID: "u1"},
				Action:    "delete_project",
				Category:  CategoryManagement,
				Result:    ResultSuccess,
				RequestID: "55555555-5555-4555-8555-555555555555",
				SourceIP:  "10.0.0.5",
				Producer:  "openchoreo-api",
			},
			want: logLinePrefix + `"schema_version":"1.0",` +
				`"event_id":"01920000-0000-7000-8000-000000000005",` +
				`"event_time":"2026-09-07T12:30:45.123456789Z",` +
				`"actor":{"type":"user","id":"u1"},` +
				`"action":"delete_project","category":"management","result":"success",` +
				`"request_id":"55555555-5555-4555-8555-555555555555","source_ip":"10.0.0.5",` +
				`"user_agent":"",` +
				`"producer":"openchoreo-api"}` + "\n",
		},
		{
			name: "resource metadata group",
			event: &Event{
				EventID:      "01920000-0000-7000-8000-000000000004",
				EventTime:    fixedTime,
				Actor:        Actor{Type: "service_account", ID: "sa-1"},
				Action:       "create_secret",
				Category:     CategoryAuthorization,
				Surface:      SurfaceREST,
				OperationID:  "CreateSecret",
				ResourceType: "secrets",
				Resource: &Resource{
					Namespace: "ns-1",
					Name:      "s1",
					Metadata:  map[string]any{"kind": "opaque"},
				},
				Result:    ResultFailure,
				RequestID: "44444444-4444-4444-8444-444444444444",
				SourceIP:  "10.0.0.4",
				Producer:  "observer",
			},
			want: logLinePrefix + `"schema_version":"1.0",` +
				`"event_id":"01920000-0000-7000-8000-000000000004",` +
				`"event_time":"2026-09-07T12:30:45Z",` +
				`"actor":{"type":"service_account","id":"sa-1"},` +
				`"action":"create_secret","category":"authorization","result":"failure",` +
				`"request_id":"44444444-4444-4444-8444-444444444444","source_ip":"10.0.0.4",` +
				`"user_agent":"",` +
				`"producer":"observer","surface":"rest","operation_id":"CreateSecret",` +
				`"resource":{"type":"secrets","namespace":"ns-1","name":"s1",` +
				`"metadata":{"kind":"opaque"}}}` + "\n",
		},
		{
			name: "empty nested object survives",
			event: &Event{
				EventID:   "01920000-0000-7000-8000-000000000007",
				EventTime: fixedTime,
				Actor:     Actor{Type: "user", ID: "u1"},
				Action:    "create_project",
				Category:  CategoryManagement,
				Result:    ResultSuccess,
				RequestID: "77777777-7777-4777-8777-777777777777",
				SourceIP:  "10.0.0.7",
				Producer:  "openchoreo-api",
				// slog.JSONHandler omits an empty group, so a nested {} has to
				// be rendered as a value to stay in the record.
				Metadata: map[string]any{"empty": map[string]any{}, "keep": "v"},
			},
			want: logLinePrefix + `"schema_version":"1.0",` +
				`"event_id":"01920000-0000-7000-8000-000000000007",` +
				`"event_time":"2026-09-07T12:30:45Z",` +
				`"actor":{"type":"user","id":"u1"},` +
				`"action":"create_project","category":"management","result":"success",` +
				`"request_id":"77777777-7777-4777-8777-777777777777","source_ip":"10.0.0.7",` +
				`"user_agent":"",` +
				`"producer":"openchoreo-api","metadata":{"empty":{},"keep":"v"}}` + "\n",
		},
		{
			name: "multi-key maps are ordered",
			event: &Event{
				EventID:   "01920000-0000-7000-8000-000000000006",
				EventTime: fixedTime,
				Actor: Actor{
					Type: "user",
					ID:   "u1",
					// json.Marshal sorts map keys, so a multi-key map has a
					// stable published order.
					Entitlements: map[string][]string{"roles": {"admin"}, "groups": {"dev"}},
				},
				Action:    "create_project",
				Category:  CategoryManagement,
				Result:    ResultSuccess,
				RequestID: "66666666-6666-4666-8666-666666666666",
				SourceIP:  "10.0.0.6",
				Producer:  "openchoreo-api",
				Metadata:  map[string]any{"z": 1, "a": 2},
			},
			want: logLinePrefix + `"schema_version":"1.0",` +
				`"event_id":"01920000-0000-7000-8000-000000000006",` +
				`"event_time":"2026-09-07T12:30:45Z",` +
				`"actor":{"type":"user","id":"u1","entitlements":{"groups":["dev"],"roles":["admin"]}},` +
				`"action":"create_project","category":"management","result":"success",` +
				`"request_id":"66666666-6666-4666-8666-666666666666","source_ip":"10.0.0.6",` +
				`"user_agent":"",` +
				`"producer":"openchoreo-api","metadata":{"a":2,"z":1}}` + "\n",
		},
	}
}

// TestPublishedRecordShape pins the exact bytes Logger.LogEvent publishes. A
// diff here is a change to the audit record downstream consumers read as a
// contract. Compared as bytes, not as an unmarshalled map, because field order
// is part of what this guards.
func TestPublishedRecordShape(t *testing.T) {
	for _, tc := range recordCases() {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			NewLogger(&buf).LogEvent(tc.event)
			if got := stripTimeAttr(buf.String()); got != tc.want {
				t.Errorf("published record changed:\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

// TestPublishedRecordShape_MarshalJSONAgrees asserts json.Marshal(event)
// publishes the same fields in the same order as the log stream, so a sink
// that marshals an *Event and the log stream stay one wire shape.
func TestPublishedRecordShape_MarshalJSONAgrees(t *testing.T) {
	for _, tc := range recordCases() {
		t.Run(tc.name, func(t *testing.T) {
			marshaled, err := json.Marshal(tc.event)
			if err != nil {
				t.Fatalf("json.Marshal(event) failed: %v", err)
			}
			// The log line is the marshaled object with slog's level/msg
			// spliced in after the opening brace.
			want := strings.TrimSuffix(strings.TrimPrefix(tc.want, logLinePrefix), "\n")
			if got := strings.TrimPrefix(string(marshaled), "{"); got != want {
				t.Errorf("marshaled form differs from the log stream:\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

// TestLogEvent_RenderFailureIsReported guards that an event whose body cannot
// be marshaled still produces a record rather than vanishing.
func TestLogEvent_RenderFailureIsReported(t *testing.T) {
	var buf bytes.Buffer
	NewLogger(&buf).LogEvent(&Event{
		EventID:  "01920000-0000-7000-8000-00000000000f",
		Action:   "create_project",
		Result:   ResultSuccess,
		Metadata: map[string]any{"bad": func() {}},
	})

	got := buf.String()
	if !strings.Contains(got, "AUDIT-LOG-RENDER-FAILED") {
		t.Errorf("expected a render-failure record, got: %s", got)
	}
	if !strings.Contains(got, "01920000-0000-7000-8000-00000000000f") {
		t.Errorf("render-failure record must identify the event, got: %s", got)
	}
}

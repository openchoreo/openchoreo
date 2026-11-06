// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// The builders below are the request.parameters allowlist of each audited
// read. limit and sortOrder are left out: returned.count says how much came
// back, and order doesn't change what was disclosed.

func auditLogsQueryParameters(req *types.AuditLogsQueryRequest) map[string]any {
	if req == nil {
		return nil
	}
	p := map[string]any{"startTime": req.StartTime, "endTime": req.EndTime}

	actor := map[string]any{}
	putStrings(actor, "id", req.Actor.IDs)
	putStrings(actor, "type", req.Actor.Types)
	putStrings(actor, "issuer", req.Actor.Issuers)
	putStrings(actor, "session_id", req.Actor.SessionIDs)
	putStrings(actor, "entitlements", req.Actor.Entitlements)
	putGroup(p, "actor", actor)

	resource := map[string]any{}
	putStrings(resource, "type", req.Resource.Types)
	putStrings(resource, "namespace", req.Resource.Namespaces)
	putStrings(resource, "environment", req.Resource.Environments)
	putStrings(resource, "project", req.Resource.Projects)
	putStrings(resource, "component", req.Resource.Components)
	putStrings(resource, "resource", req.Resource.Resources)
	putStrings(resource, "name", req.Resource.Names)
	putGroup(p, "resource", resource)

	items := map[string]any{}
	putStrings(items, "name", req.Returned.Items.Names)
	if len(items) > 0 {
		p["returned"] = map[string]any{"items": items}
	}

	putStrings(p, "action", req.Actions)
	putStrings(p, "category", req.Categories)
	putStrings(p, "result", req.Results)
	putStrings(p, "producer", req.Producers)
	putStrings(p, "surface", req.Surfaces)
	putStrings(p, "operation_id", req.OperationIDs)
	putStrings(p, "request_id", req.RequestIDs)
	putStrings(p, "event_id", req.EventIDs)
	putStrings(p, "source_ip", req.SourceIPs)
	putStrings(p, "user_agent", req.UserAgents)
	putString(p, "searchPhrase", req.SearchPhrase)
	return p
}

func logsQueryParameters(req *types.LogsQueryRequest) map[string]any {
	if req == nil {
		return nil
	}
	p := map[string]any{"startTime": req.StartTime, "endTime": req.EndTime}
	if req.SearchScope != nil {
		scope := map[string]any{}
		if c := req.SearchScope.Component; c != nil {
			putString(scope, "namespace", c.Namespace)
			putString(scope, "project", c.Project)
			putString(scope, "component", c.Component)
			putString(scope, "environment", c.Environment)
		}
		if w := req.SearchScope.Workflow; w != nil {
			putString(scope, "namespace", w.Namespace)
			putString(scope, "workflowRunName", w.WorkflowRunName)
			putString(scope, "taskName", w.TaskName)
		}
		putGroup(p, "searchScope", scope)
	}
	putString(p, "searchPhrase", req.SearchPhrase)
	putStrings(p, "logLevels", req.LogLevels)
	return p
}

func platformLogsQueryParameters(req *types.PlatformLogsQueryRequest) map[string]any {
	if req == nil {
		return nil
	}
	p := map[string]any{"startTime": req.StartTime, "endTime": req.EndTime}
	putStrings(p, "clusterInstance", req.ClusterInstances)
	putStrings(p, "namespace", req.Namespaces)
	putStrings(p, "podName", req.PodNames)
	putStrings(p, "containerName", req.ContainerNames)
	putString(p, "labels", labelSelector(req.Labels))
	putString(p, "searchPhrase", req.SearchPhrase)
	putStrings(p, "logLevels", req.LogLevels)
	return p
}

// includeAttributes is recorded even when false: it is what says whether span
// attributes were returned.
func spansQueryParameters(req *types.TracesQueryRequest) map[string]any {
	if req == nil {
		return nil
	}
	p := map[string]any{
		"startTime":         formatTime(req.StartTime),
		"endTime":           formatTime(req.EndTime),
		"includeAttributes": req.IncludeAttributes,
	}
	scope := map[string]any{}
	putString(scope, "namespace", req.SearchScope.Namespace)
	putString(scope, "project", req.SearchScope.Project)
	putString(scope, "component", req.SearchScope.Component)
	putString(scope, "environment", req.SearchScope.Environment)
	putGroup(p, "searchScope", scope)
	return p
}

// rawTracesQueryParameters records the raw request: its media type, its
// size, the SHA-256 of the body as received, and the start of the query as
// text (a JSON request as compact JSON, a text request as written).
//
// Only the first audit.MaxParameterLength runes of the query are kept, so
// queryTruncated says when the recorded query is not the whole request, and
// querySha256 identifies the whole request.
func rawTracesQueryParameters(req *types.RawTracesQueryRequest) map[string]any {
	if req == nil {
		return nil
	}
	sum := sha256.Sum256(req.Body)
	query, truncated := rawQueryText(req.ContentType, req.Body, audit.MaxParameterLength)
	p := map[string]any{
		"queryBytes":     len(req.Body),
		"querySha256":    hex.EncodeToString(sum[:]),
		"queryTruncated": truncated,
	}
	putString(p, "contentType", req.ContentType)
	putString(p, "query", query)
	return p
}

// rawQueryText returns the first maxRunes runes of the query, and whether the
// query is longer. A JSON body is compacted as it is read, so only the kept
// prefix is ever copied, however large the body is.
func rawQueryText(contentType string, body []byte, maxRunes int) (string, bool) {
	isJSON := strings.HasPrefix(contentType, "application/json")
	out := make([]byte, 0, min(len(body), maxRunes*utf8.UTFMax))
	runes, inString, escaped := 0, false, false
	for _, b := range body {
		if isJSON {
			switch {
			case inString && escaped:
				escaped = false
			case inString && b == '\\':
				escaped = true
			case b == '"':
				inString = !inString
			case !inString && (b == ' ' || b == '\t' || b == '\n' || b == '\r'):
				continue // insignificant whitespace
			}
		}
		if utf8.RuneStart(b) {
			if runes == maxRunes {
				return string(out), true
			}
			runes++
		}
		out = append(out, b)
	}
	return string(out), false
}

// spanId is a path parameter, but resource.name holds the trace and an MCP
// call has no http.path to recover the span from.
func spanDetailsParameters(spanID string) map[string]any {
	return map[string]any{"spanId": spanID}
}

// labelSelector records labels as the API's selector string rather than a
// map, which a store that flattens keys would turn into a column per label.
func labelSelector(labels map[string]string) string {
	terms := make([]string, 0, len(labels))
	for k, v := range labels {
		terms = append(terms, k+"="+v)
	}
	slices.Sort(terms)
	return strings.Join(terms, ",")
}

func putString(m map[string]any, key, value string) {
	if value != "" {
		m[key] = value
	}
}

func putStrings(m map[string]any, key string, values []string) {
	if len(values) > 0 {
		m[key] = values
	}
}

func putGroup(m map[string]any, key string, group map[string]any) {
	if len(group) > 0 {
		m[key] = group
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

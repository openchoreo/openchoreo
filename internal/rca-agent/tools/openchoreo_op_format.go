// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
	"text/template"
	"time"

	obstypes "github.com/openchoreo/openchoreo/internal/observer/types"
)

//go:embed templates/logs.tmpl
var logsTmpl string

//go:embed templates/metrics.tmpl
var metricsTmpl string

//go:embed templates/traces.tmpl
var tracesTmpl string

//go:embed templates/trace_spans.tmpl
var traceSpansTmpl string

var templateFuncs = template.FuncMap{
	"toMB": func(v float64) float64 { return v / (1024 * 1024) },
	"mul":  func(a, b float64) float64 { return a * b },
	"escapeMD": func(s string) string {
		s = strings.ReplaceAll(s, "|", "\\|")
		s = strings.ReplaceAll(s, "\n", " ")
		return s
	},
	"hasKey": func(m map[string]float64, key string) bool {
		_, ok := m[key]
		return ok
	},
}

var (
	logsTpl      = template.Must(template.New("logs").Funcs(templateFuncs).Parse(logsTmpl))
	metricsTpl   = template.Must(template.New("metrics").Funcs(templateFuncs).Parse(metricsTmpl))
	tracesTpl    = template.Must(template.New("traces").Funcs(templateFuncs).Parse(tracesTmpl))
	traceSpanTpl = template.Must(template.New("trace_spans").Funcs(templateFuncs).Parse(traceSpansTmpl))
)

type logComponent struct {
	ComponentName string
	Logs          []obstypes.LogEntry
}

type logContext struct {
	NamespaceName   string
	ProjectName     string
	EnvironmentName string
	Components      []logComponent
}

func formatLogs(raw string) string {
	var data obstypes.LogsQueryResponse
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return raw
	}

	if len(data.Logs) == 0 {
		return "No logs found"
	}

	groups := make(map[string]*logComponent)
	var order []string
	for _, entry := range data.Logs {
		name := "unknown"
		if entry.Metadata != nil && entry.Metadata.ComponentName != "" {
			name = entry.Metadata.ComponentName
		}
		if _, exists := groups[name]; !exists {
			groups[name] = &logComponent{ComponentName: name}
			order = append(order, name)
		}
		groups[name].Logs = append(groups[name].Logs, entry)
	}

	components := make([]logComponent, 0, len(order))
	for _, name := range order {
		components = append(components, *groups[name])
	}

	ctx := logContext{Components: components}
	if first := data.Logs[0].Metadata; first != nil {
		ctx.NamespaceName = firstNonEmpty(first.NamespaceName, "N/A")
		ctx.ProjectName = firstNonEmpty(first.ProjectName, "N/A")
		ctx.EnvironmentName = firstNonEmpty(first.EnvironmentName, "N/A")
	}

	return renderTemplate(logsTpl, ctx, raw)
}

type metricsContext struct {
	Stats          map[string]*MetricStats
	Anomalies      map[string]*AnomalyInfo
	ConfigValues   map[string]float64
	CPUPressure    *ResourcePressure
	MemoryPressure *ResourcePressure
	Correlations   map[string]float64
}

func formatMetrics(raw string) string {
	var data obstypes.ResourceMetricsQueryResponse
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return raw
	}

	extractValues := func(items []obstypes.MetricsTimeSeriesItem) ([]float64, []string) {
		values := make([]float64, len(items))
		timestamps := make([]string, len(items))
		for i, item := range items {
			values[i] = item.Value
			timestamps[i] = item.Timestamp.UTC().Format(time.RFC3339)
		}
		return values, timestamps
	}

	cpuUsage, cpuTS := extractValues(data.CPUUsage)
	memUsage, memTS := extractValues(data.MemoryUsage)
	cpuReqs, _ := extractValues(data.CPURequests)
	cpuLims, _ := extractValues(data.CPULimits)
	memReqs, _ := extractValues(data.MemoryRequests)
	memLims, _ := extractValues(data.MemoryLimits)

	if len(cpuUsage) == 0 && len(memUsage) == 0 {
		return "No metrics data available"
	}

	ctx := metricsContext{
		Stats:        make(map[string]*MetricStats),
		Anomalies:    make(map[string]*AnomalyInfo),
		ConfigValues: make(map[string]float64),
		Correlations: make(map[string]float64),
	}

	if len(cpuUsage) > 0 {
		ctx.Stats["CPUUsage"] = calculateStats(cpuUsage, cpuTS)
		ctx.Anomalies["CPUUsage"] = detectAnomalies(cpuUsage)
	}
	if len(memUsage) > 0 {
		ctx.Stats["MemoryUsage"] = calculateStats(memUsage, memTS)
		ctx.Anomalies["MemoryUsage"] = detectAnomalies(memUsage)
	}

	if len(cpuReqs) > 0 {
		ctx.ConfigValues["CPURequests"] = cpuReqs[0]
	}
	if len(cpuLims) > 0 {
		ctx.ConfigValues["CPULimits"] = cpuLims[0]
	}
	if len(memReqs) > 0 {
		ctx.ConfigValues["MemoryRequests"] = memReqs[0]
	}
	if len(memLims) > 0 {
		ctx.ConfigValues["MemoryLimits"] = memLims[0]
	}

	ctx.CPUPressure = calculateResourcePressure(cpuUsage, cpuReqs, cpuLims)
	ctx.MemoryPressure = calculateResourcePressure(memUsage, memReqs, memLims)

	if len(cpuUsage) > 0 && len(memUsage) > 0 {
		ctx.Correlations["CPUMemory"] = correlation(cpuUsage, memUsage)
	}

	return renderTemplate(metricsTpl, ctx, raw)
}

type traceRow struct {
	TraceID    string
	TraceName  string
	SpanCount  int
	DurationMs float64
	StartTime  string
}

type tracesContext struct {
	Traces []traceRow
	Total  int
}

func formatTraces(raw string) string {
	var data obstypes.TracesQueryResponse
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return raw
	}

	if len(data.Traces) == 0 {
		return "No traces found"
	}

	rows := make([]traceRow, len(data.Traces))
	for i, t := range data.Traces {
		startTime := ""
		if t.StartTime != nil {
			startTime = t.StartTime.UTC().Format("2006-01-02T15:04:05Z")
		}
		rows[i] = traceRow{
			TraceID:    t.TraceID,
			TraceName:  t.TraceName,
			SpanCount:  t.SpanCount,
			DurationMs: float64(t.DurationNs) / 1e6,
			StartTime:  startTime,
		}
	}

	total := data.Total
	if total == 0 {
		total = len(data.Traces)
	}

	return renderTemplate(tracesTpl, tracesContext{Traces: rows, Total: total}, raw)
}

type spanRow struct {
	SpanName    string
	ServiceName string
	Component   string
	Project     string
	Namespace   string
	DurationMs  float64
	Depth       int
	Indent      string
	Attributes  map[string]interface{}
}

type spansContext struct {
	Spans []spanRow
	Total int
}

func formatTraceSpans(raw string) string {
	var data obstypes.SpansQueryResponse
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return raw
	}

	if len(data.Spans) == 0 {
		return "No spans found"
	}

	tree := buildSpanTree(data.Spans)
	total := data.Total
	if total == 0 {
		total = len(data.Spans)
	}

	return renderTemplate(traceSpanTpl, spansContext{Spans: tree, Total: total}, raw)
}

func buildSpanTree(spans []obstypes.SpanInfo) []spanRow {
	spanMap := make(map[string]*obstypes.SpanInfo, len(spans))
	children := make(map[string][]string)

	for i := range spans {
		spanMap[spans[i].SpanID] = &spans[i]
	}

	var rootIDs []string
	for i := range spans {
		s := &spans[i]
		if s.ParentSpanID == "" || spanMap[s.ParentSpanID] == nil {
			rootIDs = append(rootIDs, s.SpanID)
		}
		children[s.ParentSpanID] = append(children[s.ParentSpanID], s.SpanID)
	}

	sortByStartTime := func(ids []string) {
		sort.SliceStable(ids, func(i, j int) bool {
			si, sj := spanMap[ids[i]], spanMap[ids[j]]
			ti, tj := si.StartTime, sj.StartTime
			switch {
			case ti == nil && tj == nil:
				return ids[i] < ids[j]
			case ti == nil:
				return false // nil sorts after non-nil
			case tj == nil:
				return true
			case ti.Equal(*tj):
				return ids[i] < ids[j]
			default:
				return ti.Before(*tj)
			}
		})
	}

	sortByStartTime(rootIDs)

	var result []spanRow
	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		s, ok := spanMap[id]
		if !ok {
			return
		}

		resAttrs := s.ResourceAttributes
		serviceName := attrStr(resAttrs, "service.name", "unknown")
		component := attrStr(resAttrs, "openchoreo.dev/component", "")
		project := attrStr(resAttrs, "openchoreo.dev/project", "")
		namespace := attrStr(resAttrs, "openchoreo.dev/namespace", "")

		filtered := make(map[string]interface{})
		for k, v := range s.Attributes {
			if !strings.HasPrefix(k, "data_stream") {
				filtered[k] = v
			}
		}

		result = append(result, spanRow{
			SpanName:    s.SpanName,
			ServiceName: serviceName,
			Component:   component,
			Project:     project,
			Namespace:   namespace,
			DurationMs:  float64(s.DurationNs) / 1e6,
			Depth:       depth,
			Indent:      strings.Repeat("  ", depth),
			Attributes:  filtered,
		})

		childIDs := children[id]
		sortByStartTime(childIDs)
		for _, cid := range childIDs {
			walk(cid, depth+1)
		}
	}

	for _, rid := range rootIDs {
		walk(rid, 0)
	}

	return result
}

func renderTemplate(tmpl *template.Template, data any, raw string) string {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return raw
	}
	return buf.String()
}

func attrStr(attrs map[string]interface{}, key, fallback string) string {
	if v, ok := attrs[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

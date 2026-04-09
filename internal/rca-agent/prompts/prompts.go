// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package prompts

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"strings"
	"text/template"
)

//go:embed rca_prompt.tmpl
var rcaPromptTmpl string

//go:embed chat_prompt.tmpl
var chatPromptTmpl string

//go:embed rca_request.tmpl
var rcaRequestTmpl string

//go:embed remed_prompt.tmpl
var remedPromptTmpl string

var funcMap = template.FuncMap{
	"joinToolNames": func(tools []ToolInfo) string {
		names := make([]string, len(tools))
		for i, t := range tools {
			names[i] = t.Name
		}
		return strings.Join(names, "`, `")
	},
	"hasToolName": func(tools []ToolInfo, name string) bool {
		for _, t := range tools {
			if t.Name == name {
				return true
			}
		}
		return false
	},
	"toJSON": func(v any) string {
		b, _ := json.MarshalIndent(v, "", "  ")
		return string(b)
	},
}

var (
	rcaPrompt   = template.Must(template.New("rca_prompt").Funcs(funcMap).Parse(rcaPromptTmpl))
	chatPrompt  = template.Must(template.New("chat_prompt").Funcs(funcMap).Parse(chatPromptTmpl))
	rcaRequest  = template.Must(template.New("rca_request").Funcs(funcMap).Parse(rcaRequestTmpl))
	remedPrompt = template.Must(template.New("remed_prompt").Funcs(funcMap).Parse(remedPromptTmpl))
)

// ToolInfo describes a tool available to the agent, used in prompt rendering.
type ToolInfo struct {
	Name string
}

// Scope identifies the namespace/project/environment/component for the analysis.
type Scope struct {
	Namespace   string
	Environment string
	Project     string
	Component   string
}

// AlertData describes the alert that triggered the RCA.
type AlertData struct {
	ID        string
	Value     float64
	Timestamp string
	Rule      AlertRuleData
}

// AlertRuleData describes the alert rule configuration.
type AlertRuleData struct {
	Name        string
	Description string
	Severity    string
	Source      *AlertSourceData
	Condition   *AlertConditionData
}

// AlertSourceData describes the alert source (log query or metric).
type AlertSourceData struct {
	Type   string
	Query  string
	Metric string
}

// AlertConditionData describes the alert trigger condition.
type AlertConditionData struct {
	Window    string
	Interval  string
	Operator  string
	Threshold int
}

// RCAPromptData is the template context for the RCA agent system prompt.
type RCAPromptData struct {
	ObservabilityTools []ToolInfo
	OpenchoreoTools    []ToolInfo
}

// ChatPromptData is the template context for the chat agent system prompt.
type ChatPromptData struct {
	ObservabilityTools []ToolInfo
	OpenchoreoTools    []ToolInfo
	Scope              *Scope
	ReportContext      any
}

// RCARequestData is the template context for the analysis user message.
type RCARequestData struct {
	Scope *Scope
	Alert *AlertData
	Meta  any
}

// RemedPromptData is the template context for the remediation agent system prompt.
type RemedPromptData struct {
	Scope *Scope
}

// RenderRCAPrompt renders the RCA agent system prompt.
func RenderRCAPrompt(data *RCAPromptData) (string, error) {
	return render(rcaPrompt, data)
}

// RenderChatPrompt renders the chat agent system prompt.
func RenderChatPrompt(data *ChatPromptData) (string, error) {
	return render(chatPrompt, data)
}

// RenderRCARequest renders the analysis user message.
func RenderRCARequest(data *RCARequestData) (string, error) {
	return render(rcaRequest, data)
}

// RenderRemedPrompt renders the remediation agent system prompt.
func RenderRemedPrompt(data *RemedPromptData) (string, error) {
	return render(remedPrompt, data)
}

func render(tmpl *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

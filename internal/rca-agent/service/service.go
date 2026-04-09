// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/mozilla-ai/any-llm-go/providers"

	"github.com/openchoreo/openchoreo/internal/agent"
	"github.com/openchoreo/openchoreo/internal/rca-agent/config"
	rcamw "github.com/openchoreo/openchoreo/internal/rca-agent/middleware"
	"github.com/openchoreo/openchoreo/internal/rca-agent/models"
	"github.com/openchoreo/openchoreo/internal/rca-agent/prompts"
	"github.com/openchoreo/openchoreo/internal/rca-agent/store"
	rcatools "github.com/openchoreo/openchoreo/internal/rca-agent/tools"
)

// AnalysisParams holds the parameters for RunAnalysis.
// Scope must be resolved before calling RunAnalysis — the HTTP handler resolves
// scope and passes it here so UIDs are available for report storage.
type AnalysisParams struct {
	ReportID string
	AlertID  string
	Scope    *Scope
	Alert    AlertParams
	Meta     map[string]any
}

// AlertParams describes the alert that triggered the analysis.
type AlertParams struct {
	ID        string
	Value     float64
	Timestamp string
	Rule      AlertRuleParams
}

// AlertRuleParams describes the alert rule.
type AlertRuleParams struct {
	Name        string
	Description string
	Severity    string
	Source      *AlertSourceParams
	Condition   *AlertConditionParams
}

// AlertSourceParams describes the alert source.
type AlertSourceParams struct {
	Type   string
	Query  string
	Metric string
}

// AlertConditionParams describes the alert condition.
type AlertConditionParams struct {
	Window    string
	Interval  string
	Operator  string
	Threshold int
}

// ChatParams holds the parameters for StreamChat.
type ChatParams struct {
	Messages      []ChatMessage
	ReportContext any
	Scope         *prompts.Scope
}

// ChatMessage is a single message in a chat conversation.
type ChatMessage struct {
	Role    string
	Content string
}

// ChatEvent is a streaming event emitted by StreamChat.
type ChatEvent struct {
	Type       string `json:"type"`
	Content    string `json:"content,omitempty"`
	Tool       string `json:"tool,omitempty"`
	ActiveForm string `json:"activeForm,omitempty"`
	Args       string `json:"args,omitempty"`
	Actions    []any  `json:"actions,omitempty"`
	Message    string `json:"message,omitempty"`
}

// Service orchestrates RCA agent operations: analysis and chat.
type Service struct {
	ctx      context.Context
	provider providers.Provider
	config   *config.Config
	store    store.ReportStore
	logger   *slog.Logger
	sem      chan struct{}
}

// New creates a new agent service. The context controls the lifetime of
// background analyses — cancel it during shutdown to stop in-flight work.
func New(ctx context.Context, provider providers.Provider, cfg *config.Config, reportStore store.ReportStore, logger *slog.Logger) *Service {
	return &Service{
		ctx:      ctx,
		provider: provider,
		config:   cfg,
		store:    reportStore,
		logger:   logger,
		sem:      make(chan struct{}, cfg.Agent.MaxConcurrentAnalyses),
	}
}

// ValidateConnectivity tests LLM, OAuth2, and API connections at startup.
// Fails fast if any dependency is unreachable.
func (s *Service) ValidateConnectivity(ctx context.Context) error {
	// 1. Test OAuth2 token fetch.
	s.logger.Info("testing OAuth2 connectivity")
	if _, err := fetchOAuth2Token(ctx,
		s.config.Auth.OAuthTokenURL,
		s.config.Auth.OAuthClientID,
		s.config.Auth.OAuthClientSecret,
		s.config.Auth.TLSInsecureSkipVerify,
	); err != nil {
		return fmt.Errorf("OAuth2 connectivity check failed: %w", err)
	}
	s.logger.Info("OAuth2 connectivity OK")

	// 2. Test LLM connectivity.
	s.logger.Info("testing LLM connectivity", "model", s.config.LLM.ModelName)
	completion, err := s.provider.Completion(ctx, providers.CompletionParams{
		Model: s.config.LLM.ModelName,
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "Hello"},
		},
	})
	if err != nil {
		return fmt.Errorf("LLM connectivity check failed: %w", err)
	}
	if len(completion.Choices) > 0 {
		content := completion.Choices[0].Message.ContentString()
		if len(content) > 50 {
			content = content[:50]
		}
		s.logger.Info("LLM connectivity OK", "response_preview", content)
	}

	return nil
}

// ResolveComponentScope resolves namespace/project/component/environment names
// to UIDs via the OpenChoreo API. Called from the HTTP handler before creating
// the pending report so UIDs are always available.
func (s *Service) ResolveComponentScope(ctx context.Context, namespace, project, component, environment string) (*Scope, error) {
	token, err := fetchOAuth2Token(ctx,
		s.config.Auth.OAuthTokenURL,
		s.config.Auth.OAuthClientID,
		s.config.Auth.OAuthClientSecret,
		s.config.Auth.TLSInsecureSkipVerify,
	)
	if err != nil {
		return nil, fmt.Errorf("fetching OAuth2 token: %w", err)
	}

	httpClient := authedHTTPClient(token, s.config.Auth.TLSInsecureSkipVerify)
	return resolveComponentScope(ctx, s.config.API.OpenChoreoAPIURL, httpClient, namespace, project, component, environment)
}

// ResolveProjectScope resolves namespace/project/environment names to UIDs.
// Used by ListReports and Chat handlers.
func (s *Service) ResolveProjectScope(ctx context.Context, namespace, project, environment string) (*Scope, error) {
	token, err := fetchOAuth2Token(ctx,
		s.config.Auth.OAuthTokenURL,
		s.config.Auth.OAuthClientID,
		s.config.Auth.OAuthClientSecret,
		s.config.Auth.TLSInsecureSkipVerify,
	)
	if err != nil {
		return nil, fmt.Errorf("fetching OAuth2 token: %w", err)
	}

	httpClient := authedHTTPClient(token, s.config.Auth.TLSInsecureSkipVerify)
	return resolveProjectScope(ctx, s.config.API.OpenChoreoAPIURL, httpClient, namespace, project, environment)
}

// RunAnalysis runs the RCA agent. Intended to be called as a goroutine.
// It acquires a semaphore slot, creates an agent, runs analysis, and stores
// the result. On any failure the report is marked as "failed".
func (s *Service) RunAnalysis(params *AnalysisParams) {
	ctx := s.ctx
	logger := s.logger.With("report_id", params.ReportID, "alert_id", params.AlertID)

	defer func() {
		if r := recover(); r != nil {
			logger.Error("panic in RunAnalysis", "panic", r)
			s.markFailed(context.Background(), params, fmt.Sprintf("internal error (report_id: %s)", params.ReportID), logger)
		}
	}()

	// Acquire semaphore.
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		s.markFailed(ctx, params, "analysis cancelled while waiting for slot", logger)
		return
	}

	// Apply timeout.
	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.config.Agent.AnalysisTimeout)*time.Second)
	defer cancel()

	scope := params.Scope
	report, err := s.runRCAAgent(ctx, params, logger)
	if err != nil {
		summary := fmt.Sprintf("Analysis failed: %v", err)
		if ctx.Err() != nil {
			summary = fmt.Sprintf("Analysis timed out (report_id: %s)", params.ReportID)
		}
		logger.Error("analysis failed", "error", err)
		s.markFailed(context.Background(), params, summary, logger)
		return
	}

	// Run remediation agent if enabled and root cause was identified.
	report = s.maybeRunRemediation(ctx, report, params, logger)

	// Store completed report.
	reportJSON, err := json.Marshal(report)
	if err != nil {
		logger.Error("failed to marshal report", "error", err)
		s.markFailed(context.Background(), params, "failed to marshal report", logger)
		return
	}

	reportStr := string(reportJSON)
	summaryPtr := extractSummary(report)
	if err := s.store.UpsertReport(context.Background(), &store.ReportEntry{
		ReportID:        params.ReportID,
		AlertID:         params.AlertID,
		Status:          "completed",
		Summary:         summaryPtr,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		NamespaceName:   scope.Namespace,
		ProjectName:     scope.Project,
		EnvironmentName: scope.Environment,
		ComponentName:   scope.Component,
		EnvironmentUID:  scope.EnvironmentUID,
		ProjectUID:      scope.ProjectUID,
		Report:          &reportStr,
	}); err != nil {
		logger.Error("failed to store completed report", "error", err)
	}

	logger.Info("analysis completed")
}

// sendChatEvent sends a ChatEvent or returns false if ctx is cancelled.
func sendChatEvent(ctx context.Context, events chan<- ChatEvent, ev ChatEvent) bool {
	select {
	case events <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

// StreamChat streams chat agent events. The bearerToken is the user's JWT,
// used to authenticate API requests on behalf of the user.
func (s *Service) StreamChat(ctx context.Context, bearerToken string, params *ChatParams) <-chan ChatEvent {
	events := make(chan ChatEvent, 64)

	go func() {
		defer close(events)

		requestID := fmt.Sprintf("msg_%s", randomHex(6))

		if err := s.runChatAgent(ctx, bearerToken, params, events); err != nil {
			s.logger.Error("chat stream error", "request_id", requestID, "error", err)
			sendChatEvent(ctx, events, ChatEvent{
				Type:    "error",
				Message: fmt.Sprintf("An error occured (request_id: %s)", requestID), //nolint:misspell // intentional, matches existing client contract
			})
		}
	}()

	return events
}

func (s *Service) runRCAAgent(ctx context.Context, params *AnalysisParams, logger *slog.Logger) (json.RawMessage, error) {
	scope := params.Scope

	// Fetch fresh OAuth2 token for API connections.
	logger.Info("fetching OAuth2 token", "token_url", s.config.Auth.OAuthTokenURL)
	token, err := fetchOAuth2Token(ctx,
		s.config.Auth.OAuthTokenURL,
		s.config.Auth.OAuthClientID,
		s.config.Auth.OAuthClientSecret,
		s.config.Auth.TLSInsecureSkipVerify,
	)
	if err != nil {
		return nil, fmt.Errorf("fetching OAuth2 token: %w", err)
	}
	logger.Info("OAuth2 token acquired")

	httpClient := authedHTTPClient(token, s.config.Auth.TLSInsecureSkipVerify)

	logger.Info("loading API tools")
	opTools, cpTools, err := s.createTools(httpClient)
	if err != nil {
		return nil, fmt.Errorf("loading tools: %w", err)
	}
	logger.Info("tools loaded", "op_count", len(opTools), "cp_count", len(cpTools))

	obsTools := toolInfoList(opTools)
	ocTools := toolInfoList(cpTools)

	systemPrompt, err := prompts.RenderRCAPrompt(&prompts.RCAPromptData{
		ObservabilityTools: obsTools,
		OpenchoreoTools:    ocTools,
	})
	if err != nil {
		return nil, fmt.Errorf("rendering RCA prompt: %w", err)
	}

	promptScope := &prompts.Scope{
		Namespace:   scope.Namespace,
		Environment: scope.Environment,
		Project:     scope.Project,
		Component:   scope.Component,
	}
	userMessage, err := prompts.RenderRCARequest(&prompts.RCARequestData{
		Scope: promptScope,
		Alert: toPromptAlert(&params.Alert),
		Meta:  params.Meta,
	})
	if err != nil {
		return nil, fmt.Errorf("rendering RCA request: %w", err)
	}

	rcaSchema, err := models.RCAReportSchema()
	if err != nil {
		return nil, fmt.Errorf("loading RCA schema: %w", err)
	}

	allTools := make([]agent.Tool, 0, len(opTools)+len(cpTools))
	allTools = append(allTools, opTools...)
	allTools = append(allTools, cpTools...)
	rcaTools := filterTools(allTools, rcaAgentTools)
	rcaTools = append(rcaTools, agent.NewWriteTodosTool())
	logger.Info("creating RCA agent", "model", s.config.LLM.ModelName, "tools", len(rcaTools))
	rcaAgent, err := agent.CreateAgent(s.provider, s.config.LLM.ModelName,
		agent.WithSystemPrompt(systemPrompt),
		agent.WithTools(rcaTools...),
		agent.WithMiddleware(
			rcamw.NewToolErrorHandler(logger),
		),
		agent.WithStructuredOutput(&agent.StructuredOutput{
			Strategy: agent.OutputStrategyProvider,
			Name:     "rca_report",
			Schema:   rcaSchema,
		}),
		agent.WithMaxIterations(200),
		agent.WithLogger(logger),
	)
	if err != nil {
		return nil, fmt.Errorf("creating agent: %w", err)
	}

	logger.Info("running RCA agent")
	result, err := rcaAgent.Run(ctx, []providers.Message{
		{Role: providers.RoleUser, Content: userMessage},
	})
	if err != nil {
		return nil, fmt.Errorf("running agent: %w", err)
	}

	if result.StructuredResponse == nil {
		return nil, fmt.Errorf("agent did not produce structured output")
	}

	return result.StructuredResponse, nil
}

func (s *Service) runChatAgent(ctx context.Context, bearerToken string, params *ChatParams, events chan<- ChatEvent) error {
	// Chat uses the user's bearer token for API auth.
	httpClient := authedHTTPClient(bearerToken, s.config.Auth.TLSInsecureSkipVerify)

	opTools, cpTools, err := s.createTools(httpClient)
	if err != nil {
		return fmt.Errorf("loading tools: %w", err)
	}

	obsTools := toolInfoList(opTools)
	ocTools := toolInfoList(cpTools)

	systemPrompt, err := prompts.RenderChatPrompt(&prompts.ChatPromptData{
		ObservabilityTools: obsTools,
		OpenchoreoTools:    ocTools,
		Scope:              params.Scope,
		ReportContext:      params.ReportContext,
	})
	if err != nil {
		return fmt.Errorf("rendering chat prompt: %w", err)
	}

	chatSchema, err := models.ChatResponseSchema()
	if err != nil {
		return fmt.Errorf("loading chat schema: %w", err)
	}

	allTools := make([]agent.Tool, 0, len(opTools)+len(cpTools))
	allTools = append(allTools, opTools...)
	allTools = append(allTools, cpTools...)
	chatTools := filterTools(allTools, chatAgentTools)
	chatAgent, err := agent.CreateAgent(s.provider, s.config.LLM.ModelName,
		agent.WithSystemPrompt(systemPrompt),
		agent.WithTools(chatTools...),
		agent.WithMiddleware(
			rcamw.NewToolErrorHandler(s.logger),
		),
		agent.WithStructuredOutput(&agent.StructuredOutput{
			Strategy: agent.OutputStrategyProvider,
			Name:     "chat_response",
			Schema:   chatSchema,
		}),
		agent.WithMaxIterations(50),
		agent.WithLogger(s.logger),
	)
	if err != nil {
		return fmt.Errorf("creating chat agent: %w", err)
	}

	msgs := make([]providers.Message, len(params.Messages))
	for i, m := range params.Messages {
		msgs[i] = providers.Message{Role: m.Role, Content: m.Content}
	}

	agentEvents, agentErrs := chatAgent.Stream(ctx, msgs)

	parser := &chatResponseParser{}

	for ev := range agentEvents {
		switch ev.Type {
		case agent.StreamEventTextDelta:
			// Feed through parser for clean message deltas (not raw JSON).
			if delta := parser.push(ev.Delta); delta != "" {
				if !sendChatEvent(ctx, events, ChatEvent{Type: "message_chunk", Content: delta}) {
					return ctx.Err()
				}
			}
		case agent.StreamEventToolCallStart:
			if !sendChatEvent(ctx, events, ChatEvent{
				Type:       "tool_call",
				Tool:       ev.ToolName,
				ActiveForm: toolActiveForms[ev.ToolName],
				Args:       ev.Args,
			}) {
				return ctx.Err()
			}
		case agent.StreamEventToolResult:
			// Internal; not forwarded to client.
		case agent.StreamEventComplete:
			// Emit actions from parser (accumulated during streaming).
			if len(parser.actions) > 0 {
				if !sendChatEvent(ctx, events, ChatEvent{Type: "actions", Actions: parser.actions}) {
					return ctx.Err()
				}
			}
			// Emit done with parser's accumulated message.
			if parser.message != "" {
				sendChatEvent(ctx, events, ChatEvent{Type: "done", Message: parser.message})
			} else if ev.Result != nil && ev.Result.StructuredResponse != nil {
				// Fallback: use structured response if parser didn't capture.
				sendChatEvent(ctx, events, ChatEvent{Type: "done", Message: string(ev.Result.StructuredResponse)})
			} else {
				sendChatEvent(ctx, events, ChatEvent{Type: "done"})
			}
		}
	}

	return <-agentErrs
}

func (s *Service) createTools(httpClient *http.Client) (opTools []agent.Tool, cpTools []agent.Tool, err error) {
	opTools, err = rcatools.NewOPTools(s.config.API.ObserverAPIURL, httpClient)
	if err != nil {
		return nil, nil, fmt.Errorf("creating observer tools: %w", err)
	}
	cpTools, err = rcatools.NewCPTools(s.config.API.OpenChoreoAPIURL, httpClient)
	if err != nil {
		return nil, nil, fmt.Errorf("creating control-plane tools: %w", err)
	}
	return opTools, cpTools, nil
}

// maybeRunRemediation runs the remediation agent if enabled and root cause was identified.
// On failure, returns the original report unchanged (graceful degradation).
func (s *Service) maybeRunRemediation(ctx context.Context, report json.RawMessage, params *AnalysisParams, logger *slog.Logger) json.RawMessage {
	if !s.config.Agent.RemediationEnabled {
		return report
	}

	// Check if result type is "root_cause_identified".
	var parsed struct {
		Result struct {
			Type string `json:"type"`
		} `json:"result"`
	}
	if err := json.Unmarshal(report, &parsed); err != nil || parsed.Result.Type != "root_cause_identified" {
		return report
	}

	logger.Info("running remediation agent")

	// Exclude observability_recommendations from the message to remed agent.
	var reportMap map[string]any
	if err := json.Unmarshal(report, &reportMap); err != nil {
		logger.Error("failed to parse report for remediation", "error", err)
		return report
	}
	if result, ok := reportMap["result"].(map[string]any); ok {
		if recs, ok := result["recommendations"].(map[string]any); ok {
			delete(recs, "observability_recommendations")
		}
	}
	reportForRemed, _ := json.Marshal(reportMap)

	remedCtx, remedCancel := context.WithTimeout(ctx, time.Duration(s.config.Agent.AnalysisTimeout)*time.Second)
	defer remedCancel()

	remedResult, err := s.runRemedAgent(remedCtx, string(reportForRemed), params, logger)
	if err != nil {
		logger.Error("remediation agent failed, saving RCA report without it", "error", err)
		return report
	}

	// Merge: replace recommended_actions in the RCA report.
	var remedParsed struct {
		RecommendedActions json.RawMessage `json:"recommended_actions"`
	}
	if err := json.Unmarshal(remedResult, &remedParsed); err != nil {
		logger.Error("failed to parse remediation result", "error", err)
		return report
	}

	if result, ok := reportMap["result"].(map[string]any); ok {
		if recs, ok := result["recommendations"].(map[string]any); ok {
			var actions []any
			if err := json.Unmarshal(remedParsed.RecommendedActions, &actions); err == nil {
				recs["recommended_actions"] = actions
			}
		}
	}

	merged, err := json.Marshal(reportMap)
	if err != nil {
		logger.Error("failed to marshal merged report", "error", err)
		return report
	}

	logger.Info("remediation completed, merged into report")
	return merged
}

func (s *Service) runRemedAgent(ctx context.Context, rcaReportJSON string, params *AnalysisParams, logger *slog.Logger) (json.RawMessage, error) {
	token, err := fetchOAuth2Token(ctx,
		s.config.Auth.OAuthTokenURL,
		s.config.Auth.OAuthClientID,
		s.config.Auth.OAuthClientSecret,
		s.config.Auth.TLSInsecureSkipVerify,
	)
	if err != nil {
		return nil, fmt.Errorf("fetching OAuth2 token: %w", err)
	}

	httpClient := authedHTTPClient(token, s.config.Auth.TLSInsecureSkipVerify)

	cpTools, err := rcatools.NewCPTools(s.config.API.OpenChoreoAPIURL, httpClient)
	if err != nil {
		return nil, fmt.Errorf("creating remediation tools: %w", err)
	}
	remedTools := filterTools(cpTools, remedAgentTools)

	scope := params.Scope
	systemPrompt, err := prompts.RenderRemedPrompt(&prompts.RemedPromptData{
		Scope: &prompts.Scope{
			Namespace:   scope.Namespace,
			Environment: scope.Environment,
			Project:     scope.Project,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("rendering remed prompt: %w", err)
	}

	remedSchema, err := models.RemediationResultSchema()
	if err != nil {
		return nil, fmt.Errorf("loading remed schema: %w", err)
	}

	remedAgent, err := agent.CreateAgent(s.provider, s.config.LLM.ModelName,
		agent.WithSystemPrompt(systemPrompt),
		agent.WithTools(remedTools...),
		agent.WithMiddleware(
			rcamw.NewToolErrorHandler(logger),
		),
		agent.WithStructuredOutput(&agent.StructuredOutput{
			Strategy: agent.OutputStrategyProvider,
			Name:     "remediation_result",
			Schema:   remedSchema,
		}),
		agent.WithMaxIterations(50),
		agent.WithLogger(logger),
	)
	if err != nil {
		return nil, fmt.Errorf("creating remed agent: %w", err)
	}

	result, err := remedAgent.Run(ctx, []providers.Message{
		{Role: providers.RoleUser, Content: rcaReportJSON},
	})
	if err != nil {
		return nil, fmt.Errorf("running remed agent: %w", err)
	}

	if result.StructuredResponse == nil {
		return nil, fmt.Errorf("remed agent did not produce structured output")
	}

	return result.StructuredResponse, nil
}

func (s *Service) markFailed(ctx context.Context, params *AnalysisParams, summary string, logger *slog.Logger) {
	summaryPtr := &summary
	entry := &store.ReportEntry{
		ReportID:  params.ReportID,
		AlertID:   params.AlertID,
		Status:    "failed",
		Summary:   summaryPtr,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	if params.Scope != nil {
		entry.NamespaceName = params.Scope.Namespace
		entry.ProjectName = params.Scope.Project
		entry.EnvironmentName = params.Scope.Environment
		entry.ComponentName = params.Scope.Component
		entry.EnvironmentUID = params.Scope.EnvironmentUID
		entry.ProjectUID = params.Scope.ProjectUID
	}
	if err := s.store.UpsertReport(ctx, entry); err != nil {
		logger.Error("failed to mark report as failed", "error", err)
	}
}

// toolActiveForms maps tool names to friendly UI labels for streaming.
var toolActiveForms = map[string]string{
	"query_component_logs":         "Fetching component logs...",
	"query_workflow_logs":          "Fetching workflow logs...",
	"query_resource_metrics":       "Gathering resource metrics...",
	"query_http_metrics":           "Gathering HTTP metrics...",
	"query_traces":                 "Retrieving traces...",
	"query_trace_spans":            "Retrieving trace spans...",
	"get_span_details":             "Fetching span details...",
	"list_environments":            "Loading environments...",
	"list_namespaces":              "Loading namespaces...",
	"list_projects":                "Loading projects...",
	"list_components":              "Loading components...",
	"patch_releasebinding":         "Patching release binding...",
	"get_resource":                 "Fetching resource...",
	"get_component_release":        "Fetching component release...",
	"get_component_release_schema": "Fetching release schema...",
	"create_workload":              "Creating workload...",
	"get_component_workloads":      "Fetching component workloads...",
	"list_release_bindings":        "Loading release bindings...",
	"list_component_traits":        "Loading component traits...",
	"get_trait_schema":             "Fetching trait schema...",
}

// rcaAgentTools is the set of tools available to the RCA agent.
var rcaAgentTools = map[string]bool{
	"query_component_logs":    true,
	"query_resource_metrics":  true,
	"query_traces":            true,
	"query_trace_spans":       true,
	"list_components":         true,
	"list_component_releases": true,
}

// chatAgentTools is the set of tools available to the chat agent.
var chatAgentTools = map[string]bool{
	"query_component_logs":   true,
	"query_resource_metrics": true,
	"query_traces":           true,
	"query_trace_spans":      true,
	"list_components":        true,
}

// remedAgentTools is the set of tools available to the remediation agent.
var remedAgentTools = map[string]bool{
	"list_components":              true,
	"list_release_bindings":        true,
	"list_component_releases":      true,
	"get_component_workloads":      true,
	"get_component_release_schema": true,
	"list_component_traits":        true,
}

// filterTools returns only tools whose names are in the whitelist.
func filterTools(tools []agent.Tool, whitelist map[string]bool) []agent.Tool {
	var filtered []agent.Tool
	for _, t := range tools {
		if whitelist[t.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func toolInfoList(tools []agent.Tool) []prompts.ToolInfo {
	info := make([]prompts.ToolInfo, len(tools))
	for i, t := range tools {
		info[i] = prompts.ToolInfo{Name: t.Name}
	}
	return info
}

func toPromptAlert(a *AlertParams) *prompts.AlertData {
	data := &prompts.AlertData{
		ID:        a.ID,
		Value:     a.Value,
		Timestamp: a.Timestamp,
		Rule: prompts.AlertRuleData{
			Name:        a.Rule.Name,
			Description: a.Rule.Description,
			Severity:    a.Rule.Severity,
		},
	}
	if a.Rule.Source != nil {
		data.Rule.Source = &prompts.AlertSourceData{
			Type:   a.Rule.Source.Type,
			Query:  a.Rule.Source.Query,
			Metric: a.Rule.Source.Metric,
		}
	}
	if a.Rule.Condition != nil {
		data.Rule.Condition = &prompts.AlertConditionData{
			Window:    a.Rule.Condition.Window,
			Interval:  a.Rule.Condition.Interval,
			Operator:  a.Rule.Condition.Operator,
			Threshold: a.Rule.Condition.Threshold,
		}
	}
	return data
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func extractSummary(report json.RawMessage) *string {
	var parsed struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(report, &parsed); err == nil && parsed.Summary != "" {
		return &parsed.Summary
	}
	return nil
}

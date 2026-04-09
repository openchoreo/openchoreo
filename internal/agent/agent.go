// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	anyllmerrors "github.com/mozilla-ai/any-llm-go/errors"
	"github.com/mozilla-ai/any-llm-go/providers"
)

// newRunLogger creates a logger scoped to a single agent run.
func (a *Agent) newRunLogger() *slog.Logger {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return a.logger.With("run_id", hex.EncodeToString(b[:]))
}

const defaultMaxIterations = 100

// ErrMaxIterations is returned when the agent exhausts its iteration budget
// without reaching a natural exit condition.
var ErrMaxIterations = errors.New("agent: maximum iterations exceeded")

// Agent is a compiled ReAct agent ready for execution.
type Agent struct {
	provider         providers.Provider
	model            string
	systemPrompt     string
	tools            []Tool
	middlewares      []Middleware
	structuredOutput *StructuredOutput
	maxIterations    int
	logger           *slog.Logger

	// Precomputed middleware chains.
	modelCallChain ModelCallHandler
	toolCallChain  ToolCallHandler

	// Classified hooks.
	beforeAgentHooks []BeforeAgentHook
	afterAgentHooks  []AfterAgentHook
	beforeModelHooks []BeforeModelHook
	afterModelHooks  []AfterModelHook

	// toolsByName is a lookup map for all tools (user + middleware-provided).
	toolsByName map[string]*Tool

	// Stored for building streaming model call chains.
	modelCallMW []ModelCallMiddleware
}

// Option configures an Agent during creation.
type Option func(*Agent)

// WithSystemPrompt sets the system prompt prepended to every conversation.
func WithSystemPrompt(prompt string) Option {
	return func(a *Agent) { a.systemPrompt = prompt }
}

// WithTools sets the tools available to the agent.
func WithTools(tools ...Tool) Option {
	return func(a *Agent) { a.tools = tools }
}

// WithMiddleware sets the middleware chain. Middleware is applied in order:
// the first middleware is the outermost wrapper.
func WithMiddleware(mw ...Middleware) Option {
	return func(a *Agent) { a.middlewares = mw }
}

// WithStructuredOutput configures the agent to produce structured output
// conforming to the given schema.
func WithStructuredOutput(so *StructuredOutput) Option {
	return func(a *Agent) { a.structuredOutput = so }
}

// WithMaxIterations sets the maximum number of ReAct loop iterations.
// Defaults to 100.
func WithMaxIterations(n int) Option {
	return func(a *Agent) { a.maxIterations = n }
}

// WithLogger sets the logger. Defaults to slog.Default().
func WithLogger(logger *slog.Logger) Option {
	return func(a *Agent) { a.logger = logger }
}

// CreateAgent creates a new ReAct agent with the given LLM provider and model.
func CreateAgent(provider providers.Provider, model string, opts ...Option) (*Agent, error) {
	if provider == nil {
		return nil, errors.New("agent: provider must not be nil")
	}
	if model == "" {
		return nil, errors.New("agent: model must not be empty")
	}

	a := &Agent{
		provider:      provider,
		model:         model,
		maxIterations: defaultMaxIterations,
		logger:        slog.Default(),
	}
	for _, opt := range opts {
		opt(a)
	}

	if a.maxIterations <= 0 {
		return nil, errors.New("agent: maxIterations must be positive")
	}

	if a.structuredOutput != nil {
		if a.structuredOutput.Name == "" {
			return nil, errors.New("agent: structured output name must not be empty")
		}
		if a.structuredOutput.Schema == nil {
			return nil, errors.New("agent: structured output schema must not be nil")
		}
	}

	if err := a.init(); err != nil {
		return nil, err
	}
	return a, nil
}

// init classifies middleware, collects tools, validates, and builds handler chains.
func (a *Agent) init() error {
	// Validate no duplicate middleware names.
	seen := make(map[string]bool)
	for _, mw := range a.middlewares {
		if seen[mw.Name()] {
			return fmt.Errorf("agent: duplicate middleware name %q", mw.Name())
		}
		seen[mw.Name()] = true
	}

	// Collect middleware-contributed tools.
	var middlewareTools []Tool
	for _, mw := range a.middlewares {
		if tp, ok := mw.(ToolProvider); ok {
			middlewareTools = append(middlewareTools, tp.Tools()...)
		}
	}

	// Build tool lookup map (user tools + middleware tools).
	allTools := make([]Tool, 0, len(a.tools)+len(middlewareTools))
	allTools = append(allTools, a.tools...)
	allTools = append(allTools, middlewareTools...)
	a.toolsByName = make(map[string]*Tool, len(allTools))
	for i := range allTools {
		if _, exists := a.toolsByName[allTools[i].Name]; exists {
			return fmt.Errorf("agent: duplicate tool name %q", allTools[i].Name)
		}
		a.toolsByName[allTools[i].Name] = &allTools[i]
	}
	a.tools = allTools

	// Classify middleware by hook type.
	var modelCallMW []ModelCallMiddleware
	var toolCallMW []ToolCallMiddleware

	for _, mw := range a.middlewares {
		if m, ok := mw.(ModelCallMiddleware); ok {
			modelCallMW = append(modelCallMW, m)
		}
		if m, ok := mw.(ToolCallMiddleware); ok {
			toolCallMW = append(toolCallMW, m)
		}
		if m, ok := mw.(BeforeAgentHook); ok {
			a.beforeAgentHooks = append(a.beforeAgentHooks, m)
		}
		if m, ok := mw.(AfterAgentHook); ok {
			a.afterAgentHooks = append(a.afterAgentHooks, m)
		}
		if m, ok := mw.(BeforeModelHook); ok {
			a.beforeModelHooks = append(a.beforeModelHooks, m)
		}
		if m, ok := mw.(AfterModelHook); ok {
			a.afterModelHooks = append(a.afterModelHooks, m)
		}
	}

	a.modelCallMW = modelCallMW
	a.modelCallChain = chainModelCallMiddleware(modelCallMW, a.executeModelCall)
	a.toolCallChain = chainToolCallMiddleware(toolCallMW, a.executeToolCall)
	return nil
}

// Run executes the agent with the given input messages and returns the result.
// The agent runs a ReAct loop: model call -> tool execution -> model call -> ...
// until the model stops calling tools or structured output is obtained.
func (a *Agent) Run(ctx context.Context, messages []providers.Message) (*Result, error) {
	logger := a.newRunLogger()

	state := &State{
		Messages: make([]providers.Message, 0, len(messages)),
	}
	state.Messages = append(state.Messages, messages...)

	strategy := a.initialStrategy()

	// Before-agent hooks (forward order).
	for _, hook := range a.beforeAgentHooks {
		if err := hook.BeforeAgent(ctx, state); err != nil {
			return nil, fmt.Errorf("before_agent %q: %w", hook.Name(), err)
		}
	}

	// ReAct loop.
	finished := false
	for i := 0; i < a.maxIterations; i++ {
		// Before-model hooks (forward order). May set state.Done.
		for _, hook := range a.beforeModelHooks {
			if err := hook.BeforeModel(ctx, state); err != nil {
				return nil, fmt.Errorf("before_model %q: %w", hook.Name(), err)
			}
		}
		if state.Done {
			finished = true
			break
		}

		// Model call.
		req := a.buildModelRequest(state, strategy)
		modelStart := time.Now()
		resp, err := a.modelCallChain(ctx, req)

		// Auto-strategy fallback: provider -> tool.
		if err != nil && a.shouldFallbackToTool(strategy, err) {
			logger.InfoContext(ctx, "provider strategy unsupported, falling back to tool strategy")
			strategy = OutputStrategyTool
			req = a.buildModelRequest(state, strategy)
			modelStart = time.Now()
			resp, err = a.modelCallChain(ctx, req)
		}
		if err != nil {
			logger.ErrorContext(ctx, "model call failed", "iteration", i+1, "elapsed_s", time.Since(modelStart).Seconds(), "error", err)
			return nil, fmt.Errorf("model call (iteration %d): %w", i+1, err)
		}
		logger.InfoContext(ctx, "model call completed", "iteration", i+1, "elapsed_s", time.Since(modelStart).Seconds())

		// Append assistant message.
		state.Messages = append(state.Messages, resp.Message)

		// Provider strategy: structured response extracted from content
		// (only when model returned no tool calls).
		if resp.StructuredResponse != nil {
			state.StructuredResponse = resp.StructuredResponse
		}

		// After-model hooks (reverse order for cleanup/unwinding). May set state.Done.
		for j := len(a.afterModelHooks) - 1; j >= 0; j-- {
			if err := a.afterModelHooks[j].AfterModel(ctx, state); err != nil {
				return nil, fmt.Errorf("after_model %q: %w", a.afterModelHooks[j].Name(), err)
			}
		}
		if state.Done {
			finished = true
			break
		}

		// Exit: no tool calls.
		if len(resp.Message.ToolCalls) == 0 {
			finished = true
			break
		}

		// Process tool calls (parallel execution).
		returnDirect, err := a.processToolCalls(ctx, logger, state, resp.Message.ToolCalls, strategy, nil)
		if err != nil {
			return nil, err
		}

		// Exit: return_direct tool executed.
		if returnDirect {
			finished = true
			break
		}

		// Exit: structured response obtained.
		if state.StructuredResponse != nil {
			finished = true
			break
		}
	}

	if !finished {
		return nil, fmt.Errorf("%w: limit is %d", ErrMaxIterations, a.maxIterations)
	}

	// After-agent hooks (reverse order for cleanup/unwinding).
	for i := len(a.afterAgentHooks) - 1; i >= 0; i-- {
		if err := a.afterAgentHooks[i].AfterAgent(ctx, state); err != nil {
			return nil, fmt.Errorf("after_agent %q: %w", a.afterAgentHooks[i].Name(), err)
		}
	}

	return &Result{
		Messages:           state.Messages,
		StructuredResponse: state.StructuredResponse,
	}, nil
}

// toolResult holds the outcome of a single tool execution for parallel collection.
type toolResult struct {
	message      providers.Message
	toolName     string
	returnDirect bool
}

// processToolCalls executes tool calls in parallel, appending tool messages to state.
// Structured output tool calls are intercepted and parsed rather than executed.
// If events is non-nil, StreamEventToolResult events are emitted for each tool.
// Returns (returnDirect, error) where returnDirect is true if ALL executed tools
// have ReturnDirect=true (all executed tools have ReturnDirect=true).
func (a *Agent) processToolCalls(ctx context.Context, logger *slog.Logger, state *State, toolCalls []providers.ToolCall, strategy OutputStrategy, events chan<- StreamEvent) (bool, error) {
	isOutputTool := a.isStructuredOutputTool(strategy)

	// Separate structured output tool calls from regular ones.
	var structuredCalls []providers.ToolCall
	var regularCalls []providers.ToolCall
	for _, tc := range toolCalls {
		if isOutputTool(tc.Function.Name) {
			structuredCalls = append(structuredCalls, tc)
		} else {
			regularCalls = append(regularCalls, tc)
		}
	}

	// Handle structured output tool calls (synchronous, no execution needed).
	if err := a.handleStructuredToolCalls(state, structuredCalls); err != nil {
		return false, err
	}

	if len(regularCalls) == 0 {
		return false, nil
	}

	// Execute regular tool calls in parallel.
	// Each goroutine receives a shallow snapshot of the current state so that
	// ToolCallMiddleware can safely read state fields without racing against
	// other goroutines. The snapshot shares the same Messages slice header
	// (read-only during tool execution) but is a distinct State value.
	stateSnapshot := &State{
		Messages:           state.Messages,
		StructuredResponse: state.StructuredResponse,
		Done:               state.Done,
	}

	results := make([]toolResult, len(regularCalls))
	var wg sync.WaitGroup

	for i, tc := range regularCalls {
		wg.Add(1)
		go func(idx int, tc providers.ToolCall) {
			defer wg.Done()
			results[idx] = a.executeSingleToolCall(ctx, logger, tc, stateSnapshot)
		}(i, tc)
	}

	wg.Wait()

	// Append results to state in original order and check return_direct.
	allReturnDirect := true
	for _, r := range results {
		state.Messages = append(state.Messages, r.message)
		if !r.returnDirect {
			allReturnDirect = false
		}
		if events != nil && r.toolName != "" {
			_ = trySend(ctx, events, StreamEvent{
				Type:       StreamEventToolResult,
				ToolName:   r.toolName,
				ToolCallID: r.message.ToolCallID,
				Content:    r.message.ContentString(),
			})
		}
	}

	return allReturnDirect, nil
}

// executeSingleToolCall runs one tool call through the middleware chain.
// Returns a toolResult that is always valid (errors become error messages).
func (a *Agent) executeSingleToolCall(ctx context.Context, logger *slog.Logger, tc providers.ToolCall, state *State) toolResult {
	tool := a.toolsByName[tc.Function.Name]
	if tool == nil {
		return toolResult{
			message: providers.Message{
				Role:       providers.RoleTool,
				Content:    fmt.Sprintf("Error: unknown tool %q", tc.Function.Name),
				ToolCallID: tc.ID,
			},
			toolName: tc.Function.Name,
		}
	}

	logger.DebugContext(ctx, "tool call starting", "tool", tool.Name, "args", tc.Function.Arguments)

	toolReq := &ToolCallRequest{ToolCall: tc, Tool: tool, State: state}
	toolStart := time.Now()
	toolResp, err := a.toolCallChain(ctx, toolReq)
	elapsed := time.Since(toolStart).Seconds()

	if err != nil {
		logger.ErrorContext(ctx, "tool call failed", "tool", tool.Name, "elapsed_s", elapsed, "error", err)
		// On error, never honor ReturnDirect — let the model see the error
		// and decide how to recover rather than returning an error as the
		// final result.
		return toolResult{
			message: providers.Message{
				Role:       providers.RoleTool,
				Content:    fmt.Sprintf("Error executing tool %q: %v", tc.Function.Name, err),
				ToolCallID: tc.ID,
			},
			toolName: tool.Name,
		}
	}

	logger.InfoContext(ctx, "tool call completed", "tool", tool.Name, "elapsed_s", elapsed, "result_chars", len(toolResp.Content))

	return toolResult{
		message: providers.Message{
			Role:       providers.RoleTool,
			Content:    toolResp.Content,
			ToolCallID: tc.ID,
		},
		toolName:     tool.Name,
		returnDirect: tool.ReturnDirect,
	}
}

// isStructuredOutputTool returns a function that checks whether a tool name
// is the structured output tool. Returns a no-op function if not applicable.
func (a *Agent) isStructuredOutputTool(strategy OutputStrategy) func(string) bool {
	if strategy != OutputStrategyTool || a.structuredOutput == nil {
		return func(string) bool { return false }
	}
	name := a.structuredOutput.Name
	return func(n string) bool { return n == name }
}

// handleStructuredToolCalls processes structured output tool calls:
// - 0 calls: no-op
// - 1 call: parse arguments, set structured response
// - >1 calls: MultipleStructuredOutputsError, optionally retry
func (a *Agent) handleStructuredToolCalls(state *State, calls []providers.ToolCall) error {
	if len(calls) == 0 {
		return nil
	}

	// Multiple structured output tool calls.
	if len(calls) > 1 {
		names := make([]string, len(calls))
		for i, tc := range calls {
			names[i] = tc.Function.Name
		}
		mErr := &MultipleStructuredOutputsError{ToolNames: names}

		if a.structuredOutput.HandleErrors == nil {
			return mErr
		}

		// Retry: send error message for each call.
		errorMsg := a.structuredOutput.HandleErrors(mErr)
		for _, tc := range calls {
			state.Messages = append(state.Messages, providers.Message{
				Role:       providers.RoleTool,
				Content:    errorMsg,
				ToolCallID: tc.ID,
			})
		}
		return nil
	}

	// Single structured output tool call.
	tc := calls[0]
	raw := json.RawMessage(tc.Function.Arguments)

	if !json.Valid(raw) {
		parseErr := &StructuredOutputError{
			ToolName: tc.Function.Name,
			Err:      fmt.Errorf("invalid JSON: %s", tc.Function.Arguments),
		}

		if a.structuredOutput.HandleErrors == nil {
			return parseErr
		}

		// Retry: send error message to model.
		errorMsg := a.structuredOutput.HandleErrors(parseErr)
		state.Messages = append(state.Messages, providers.Message{
			Role:       providers.RoleTool,
			Content:    errorMsg,
			ToolCallID: tc.ID,
		})
		return nil
	}

	state.StructuredResponse = raw
	state.Messages = append(state.Messages, providers.Message{
		Role:       providers.RoleTool,
		Content:    a.structuredOutput.successMessage(raw),
		ToolCallID: tc.ID,
	})
	return nil
}

// initialStrategy resolves the starting output strategy.
func (a *Agent) initialStrategy() OutputStrategy {
	if a.structuredOutput == nil {
		return OutputStrategyAuto
	}
	switch a.structuredOutput.Strategy {
	case OutputStrategyProvider, OutputStrategyTool:
		return a.structuredOutput.Strategy
	default:
		// Auto: start with provider, fall back to tool on error.
		return OutputStrategyProvider
	}
}

// shouldFallbackToTool returns true if the error indicates the provider
// doesn't support native structured output and we should try tool strategy.
func (a *Agent) shouldFallbackToTool(currentStrategy OutputStrategy, err error) bool {
	if a.structuredOutput == nil || a.structuredOutput.Strategy != OutputStrategyAuto {
		return false
	}
	if currentStrategy != OutputStrategyProvider {
		return false
	}
	return errors.Is(err, anyllmerrors.ErrUnsupportedParam) ||
		errors.Is(err, anyllmerrors.ErrInvalidRequest)
}

// buildModelRequest constructs a ModelRequest for the current strategy.
// System message is kept separate from conversation messages.
func (a *Agent) buildModelRequest(state *State, strategy OutputStrategy) *ModelRequest {
	req := &ModelRequest{
		Provider: a.provider,
		Model:    a.model,
		Messages: state.Messages,
		Tools:    a.providerTools(),
		State:    state,
	}

	// System message is separate — prepended at invocation time in executeModelCall.
	if a.systemPrompt != "" {
		msg := providers.Message{
			Role:    providers.RoleSystem,
			Content: a.systemPrompt,
		}
		req.SystemMessage = &msg
	}

	if a.structuredOutput == nil {
		return req
	}

	switch strategy {
	case OutputStrategyProvider:
		req.ResponseFormat = a.structuredOutput.toProviderResponseFormat()

	case OutputStrategyTool:
		req.Tools = append(req.Tools, a.structuredOutput.toProviderTool())
		if len(a.tools) == 0 {
			// No regular tools: force the specific output tool.
			req.ToolChoice = providers.ToolChoice{
				Type:     "function",
				Function: &providers.ToolChoiceFunction{Name: a.structuredOutput.Name},
			}
		} else {
			// Regular tools exist alongside the output tool: force the model
			// to call at least one tool rather than responding with plain text.
			req.ToolChoice = "required"
		}
	}

	return req
}

// executeModelCall is the innermost handler that calls the LLM provider.
// System message is prepended to messages here (not stored in state).
func (a *Agent) executeModelCall(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
	// Prepend system message at invocation time.
	messages := req.Messages
	if req.SystemMessage != nil {
		messages = make([]providers.Message, 0, len(req.Messages)+1)
		messages = append(messages, *req.SystemMessage)
		messages = append(messages, req.Messages...)
	}

	params := providers.CompletionParams{
		Model:          req.Model,
		Messages:       messages,
		Tools:          req.Tools,
		ResponseFormat: req.ResponseFormat,
		ToolChoice:     req.ToolChoice,
	}

	completion, err := a.provider.Completion(ctx, params)
	if err != nil {
		return nil, err
	}

	if len(completion.Choices) == 0 {
		return nil, errors.New("model returned no choices")
	}

	msg := completion.Choices[0].Message

	// Handle structured response based on effective strategy.
	structured, err := a.handleModelOutput(req, msg)
	if err != nil {
		return nil, err
	}

	return &ModelResponse{
		Message:            msg,
		StructuredResponse: structured,
	}, nil
}

// handleModelOutput extracts structured response from model output per strategy.
func (a *Agent) handleModelOutput(req *ModelRequest, msg providers.Message) (json.RawMessage, error) {
	// Provider strategy: extract structured response from message content,
	// but ONLY when the model did NOT call any tools. If tools were called,
	// the model is still investigating.
	if req.ResponseFormat != nil && req.ResponseFormat.Type == "json_schema" {
		if len(msg.ToolCalls) > 0 {
			return nil, nil
		}

		content := msg.ContentString()
		if content == "" {
			return nil, nil
		}

		if !json.Valid([]byte(content)) {
			return nil, &StructuredOutputError{
				ToolName: req.ResponseFormat.JSONSchema.Name,
				Err:      fmt.Errorf("invalid JSON in model response: %.200s", content),
			}
		}

		return json.RawMessage(content), nil
	}

	// Tool strategy: structured response is handled in processToolCalls.
	return nil, nil
}

// executeToolCall is the innermost handler that executes a tool.
func (a *Agent) executeToolCall(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error) {
	result, err := req.Tool.Execute(ctx, json.RawMessage(req.ToolCall.Function.Arguments))
	if err != nil {
		return nil, err
	}
	return &ToolCallResponse{Content: result}, nil
}

// providerTools converts the agent's tools to provider tool definitions.
func (a *Agent) providerTools() []providers.Tool {
	if len(a.tools) == 0 {
		return nil
	}
	tools := make([]providers.Tool, len(a.tools))
	for i := range a.tools {
		tools[i] = a.tools[i].toProviderTool()
	}
	return tools
}

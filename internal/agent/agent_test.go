// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	anyllmerrors "github.com/mozilla-ai/any-llm-go/errors"
	"github.com/mozilla-ai/any-llm-go/providers"
)

type fakeProvider struct {
	completions []providers.ChatCompletion
	index       atomic.Int32
	calls       []providers.CompletionParams
}

func newFakeProvider(completions ...providers.ChatCompletion) *fakeProvider {
	return &fakeProvider{completions: completions}
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Completion(_ context.Context, params providers.CompletionParams) (*providers.ChatCompletion, error) {
	i := int(f.index.Add(1) - 1)
	f.calls = append(f.calls, params)
	if i >= len(f.completions) {
		return nil, fmt.Errorf("fakeProvider: no completion at index %d", i)
	}
	c := f.completions[i]
	return &c, nil
}

func (f *fakeProvider) CompletionStream(context.Context, providers.CompletionParams) (<-chan providers.ChatCompletionChunk, <-chan error) {
	panic("not implemented")
}

type errProvider struct {
	err error
}

func (e *errProvider) Name() string { return "err" }
func (e *errProvider) Completion(context.Context, providers.CompletionParams) (*providers.ChatCompletion, error) {
	return nil, e.err
}
func (e *errProvider) CompletionStream(context.Context, providers.CompletionParams) (<-chan providers.ChatCompletionChunk, <-chan error) {
	panic("not implemented")
}

type fallbackProvider struct {
	firstErr    error
	completions []providers.ChatCompletion
	callCount   atomic.Int32
}

func (f *fallbackProvider) Name() string { return "fallback" }
func (f *fallbackProvider) Completion(_ context.Context, _ providers.CompletionParams) (*providers.ChatCompletion, error) {
	i := int(f.callCount.Add(1) - 1)
	if i == 0 {
		return nil, f.firstErr
	}
	ci := i - 1
	if ci >= len(f.completions) {
		return nil, fmt.Errorf("fallbackProvider: no completion at index %d", ci)
	}
	return &f.completions[ci], nil
}
func (f *fallbackProvider) CompletionStream(context.Context, providers.CompletionParams) (<-chan providers.ChatCompletionChunk, <-chan error) {
	panic("not implemented")
}

func completion(content string, toolCalls ...providers.ToolCall) providers.ChatCompletion {
	return providers.ChatCompletion{
		Choices: []providers.Choice{{
			Message: providers.Message{
				Role:      providers.RoleAssistant,
				Content:   content,
				ToolCalls: toolCalls,
			},
		}},
	}
}

func toolCall(id, name, args string) providers.ToolCall {
	return providers.ToolCall{
		ID:   id,
		Type: "function",
		Function: providers.FunctionCall{
			Name:      name,
			Arguments: args,
		},
	}
}

func echoTool(name string) Tool {
	return Tool{
		Name:        name,
		Description: "echoes input",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(_ context.Context, args json.RawMessage) (string, error) {
			return string(args), nil
		},
	}
}

func userMsg(content string) providers.Message {
	return providers.Message{Role: providers.RoleUser, Content: content}
}

type callTracker struct {
	calls []string
}

func (t *callTracker) record(name string) {
	t.calls = append(t.calls, name)
}

type trackingMiddleware struct {
	name    string
	tracker *callTracker
}

func (m *trackingMiddleware) Name() string { return m.name }
func (m *trackingMiddleware) BeforeAgent(_ context.Context, _ *State) error {
	m.tracker.record(m.name + ".BeforeAgent")
	return nil
}
func (m *trackingMiddleware) AfterAgent(_ context.Context, _ *State) error {
	m.tracker.record(m.name + ".AfterAgent")
	return nil
}
func (m *trackingMiddleware) BeforeModel(_ context.Context, _ *State) error {
	m.tracker.record(m.name + ".BeforeModel")
	return nil
}
func (m *trackingMiddleware) AfterModel(_ context.Context, _ *State) error {
	m.tracker.record(m.name + ".AfterModel")
	return nil
}
func (m *trackingMiddleware) WrapModelCall(ctx context.Context, req *ModelRequest, next ModelCallHandler) (*ModelResponse, error) {
	m.tracker.record(m.name + ".WrapModelCall")
	return next(ctx, req)
}
func (m *trackingMiddleware) WrapToolCall(ctx context.Context, req *ToolCallRequest, next ToolCallHandler) (*ToolCallResponse, error) {
	m.tracker.record(m.name + ".WrapToolCall")
	return next(ctx, req)
}

var (
	_ Middleware          = (*trackingMiddleware)(nil)
	_ BeforeAgentHook     = (*trackingMiddleware)(nil)
	_ AfterAgentHook      = (*trackingMiddleware)(nil)
	_ BeforeModelHook     = (*trackingMiddleware)(nil)
	_ AfterModelHook      = (*trackingMiddleware)(nil)
	_ ModelCallMiddleware = (*trackingMiddleware)(nil)
	_ ToolCallMiddleware  = (*trackingMiddleware)(nil)
)

func TestCreateAgent_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func() (*Agent, error)
		wantErr string
	}{
		{
			name: "nil provider",
			setup: func() (*Agent, error) {
				return CreateAgent(nil, "model")
			},
			wantErr: "provider must not be nil",
		},
		{
			name: "empty model",
			setup: func() (*Agent, error) {
				return CreateAgent(newFakeProvider(), "")
			},
			wantErr: "model must not be empty",
		},
		{
			name: "structured output missing name",
			setup: func() (*Agent, error) {
				return CreateAgent(newFakeProvider(), "m", WithStructuredOutput(&StructuredOutput{
					Schema: map[string]any{"type": "object"},
				}))
			},
			wantErr: "structured output name must not be empty",
		},
		{
			name: "structured output missing schema",
			setup: func() (*Agent, error) {
				return CreateAgent(newFakeProvider(), "m", WithStructuredOutput(&StructuredOutput{
					Name: "out",
				}))
			},
			wantErr: "structured output schema must not be nil",
		},
		{
			name: "duplicate middleware names",
			setup: func() (*Agent, error) {
				tracker := &callTracker{}
				return CreateAgent(newFakeProvider(), "m",
					WithMiddleware(
						&trackingMiddleware{name: "dup", tracker: tracker},
						&trackingMiddleware{name: "dup", tracker: tracker},
					))
			},
			wantErr: `duplicate middleware name "dup"`,
		},
		{
			name: "duplicate tool names",
			setup: func() (*Agent, error) {
				return CreateAgent(newFakeProvider(), "m",
					WithTools(echoTool("t"), echoTool("t")))
			},
			wantErr: `duplicate tool name "t"`,
		},
		{
			name: "valid minimal agent",
			setup: func() (*Agent, error) {
				return CreateAgent(newFakeProvider(), "m")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a, err := tt.setup()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if got := err.Error(); !contains(got, tt.wantErr) {
					t.Fatalf("error %q does not contain %q", got, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if a == nil {
				t.Fatal("expected non-nil agent")
			}
		})
	}
}

func TestRun_SingleTurn_NoTools(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider(completion("hello back"))
	a, err := CreateAgent(fp, "test-model")
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.Run(context.Background(), []providers.Message{userMsg("hi")})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result.Messages))
	}
	if got := result.Messages[1].ContentString(); got != "hello back" {
		t.Fatalf("expected %q, got %q", "hello back", got)
	}
}

func TestRun_SystemPrompt(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider(completion("ok"))
	a, err := CreateAgent(fp, "m", WithSystemPrompt("You are helpful."))
	if err != nil {
		t.Fatal(err)
	}

	_, err = a.Run(context.Background(), []providers.Message{userMsg("hi")})
	if err != nil {
		t.Fatal(err)
	}

	if len(fp.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fp.calls))
	}
	msgs := fp.calls[0].Messages
	if len(msgs) < 2 {
		t.Fatal("expected system + user messages")
	}
	if msgs[0].Role != providers.RoleSystem {
		t.Fatalf("first message role should be system, got %q", msgs[0].Role)
	}
	if msgs[0].ContentString() != "You are helpful." {
		t.Fatalf("system prompt mismatch: %q", msgs[0].ContentString())
	}
}

func TestRun_MultiTurn_WithTools(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider(
		completion("", toolCall("tc1", "echo", `{"msg":"world"}`)),
		completion("final answer"),
	)

	a, err := CreateAgent(fp, "m", WithTools(echoTool("echo")))
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.Run(context.Background(), []providers.Message{userMsg("hello")})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(result.Messages))
	}
	if result.Messages[1].Role != providers.RoleAssistant {
		t.Fatal("message[1] should be assistant")
	}
	if result.Messages[2].Role != providers.RoleTool {
		t.Fatal("message[2] should be tool")
	}
	if result.Messages[2].ContentString() != `{"msg":"world"}` {
		t.Fatalf("tool result mismatch: %q", result.Messages[2].ContentString())
	}
	if result.Messages[3].ContentString() != "final answer" {
		t.Fatalf("final answer mismatch: %q", result.Messages[3].ContentString())
	}
}

func TestRun_UnknownTool(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider(
		completion("", toolCall("tc1", "nonexistent", `{}`)),
		completion("recovered"),
	)
	a, err := CreateAgent(fp, "m")
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.Run(context.Background(), []providers.Message{userMsg("go")})
	if err != nil {
		t.Fatal(err)
	}

	toolMsg := result.Messages[2]
	if toolMsg.Role != providers.RoleTool {
		t.Fatal("expected tool message")
	}
	if !contains(toolMsg.ContentString(), "unknown tool") {
		t.Fatalf("expected unknown tool error, got %q", toolMsg.ContentString())
	}
}

func TestRun_ToolExecutionError(t *testing.T) {
	t.Parallel()

	failTool := Tool{
		Name:        "fail",
		Description: "always fails",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(context.Context, json.RawMessage) (string, error) {
			return "", errors.New("boom")
		},
	}

	fp := newFakeProvider(
		completion("", toolCall("tc1", "fail", `{}`)),
		completion("handled"),
	)
	a, err := CreateAgent(fp, "m", WithTools(failTool))
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.Run(context.Background(), []providers.Message{userMsg("go")})
	if err != nil {
		t.Fatal(err)
	}

	toolMsg := result.Messages[2]
	if !contains(toolMsg.ContentString(), "Error executing tool") {
		t.Fatalf("expected error message, got %q", toolMsg.ContentString())
	}
}

func TestRun_ReturnDirect(t *testing.T) {
	t.Parallel()

	directTool := Tool{
		Name:         "direct",
		Description:  "returns directly",
		Parameters:   map[string]any{"type": "object"},
		ReturnDirect: true,
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "direct result", nil
		},
	}

	fp := newFakeProvider(
		completion("", toolCall("tc1", "direct", `{}`)),
		completion("should not reach"),
	)
	a, err := CreateAgent(fp, "m", WithTools(directTool))
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.Run(context.Background(), []providers.Message{userMsg("go")})
	if err != nil {
		t.Fatal(err)
	}

	if int(fp.index.Load()) != 1 {
		t.Fatalf("expected 1 provider call, got %d", fp.index.Load())
	}
	last := result.Messages[len(result.Messages)-1]
	if last.ContentString() != "direct result" {
		t.Fatalf("expected direct result, got %q", last.ContentString())
	}
}

func TestRun_MaxIterations(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider(
		completion("", toolCall("tc1", "echo", `{}`)),
		completion("", toolCall("tc2", "echo", `{}`)),
		completion("", toolCall("tc3", "echo", `{}`)),
	)

	a, err := CreateAgent(fp, "m",
		WithTools(echoTool("echo")),
		WithMaxIterations(3),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = a.Run(context.Background(), []providers.Message{userMsg("go")})
	if !errors.Is(err, ErrMaxIterations) {
		t.Fatalf("expected ErrMaxIterations, got %v", err)
	}
}

func TestRun_MiddlewareHookOrdering(t *testing.T) {
	t.Parallel()

	tracker := &callTracker{}
	mwA := &trackingMiddleware{name: "A", tracker: tracker}
	mwB := &trackingMiddleware{name: "B", tracker: tracker}

	fp := newFakeProvider(
		completion("", toolCall("tc1", "echo", `{"v":"1"}`)),
		completion("done"),
	)

	a, err := CreateAgent(fp, "m",
		WithTools(echoTool("echo")),
		WithMiddleware(mwA, mwB),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = a.Run(context.Background(), []providers.Message{userMsg("go")})
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"A.BeforeAgent", "B.BeforeAgent",
		"A.BeforeModel", "B.BeforeModel",
		"A.WrapModelCall", "B.WrapModelCall",
		"B.AfterModel", "A.AfterModel",
		"A.WrapToolCall", "B.WrapToolCall",
		"A.BeforeModel", "B.BeforeModel",
		"A.WrapModelCall", "B.WrapModelCall",
		"B.AfterModel", "A.AfterModel",
		"B.AfterAgent", "A.AfterAgent",
	}

	if len(tracker.calls) != len(expected) {
		t.Fatalf("expected %d calls, got %d:\n  got:  %v\n  want: %v",
			len(expected), len(tracker.calls), tracker.calls, expected)
	}
	for i := range expected {
		if tracker.calls[i] != expected[i] {
			t.Errorf("call[%d]: got %q, want %q", i, tracker.calls[i], expected[i])
		}
	}
}

func TestRun_MiddlewareBeforeModelSetsDone(t *testing.T) {
	t.Parallel()

	stopper := &beforeModelStopper{afterN: 1}

	fp := newFakeProvider(
		completion("first"),
		completion("unreachable"),
	)

	a, err := CreateAgent(fp, "m",
		WithTools(echoTool("echo")),
		WithMiddleware(stopper),
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.Run(context.Background(), []providers.Message{userMsg("go")})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Messages) != 2 {
		t.Fatalf("expected 2 messages (user + first assistant), got %d", len(result.Messages))
	}
}

type beforeModelStopper struct {
	afterN int
	count  int
}

func (s *beforeModelStopper) Name() string { return "stopper" }
func (s *beforeModelStopper) BeforeModel(_ context.Context, state *State) error {
	s.count++
	if s.count > s.afterN {
		state.Done = true
	}
	return nil
}

func TestInit_MiddlewareToolProvider(t *testing.T) {
	t.Parallel()

	mw := &toolProviderMiddleware{
		tools: []Tool{echoTool("mw_tool")},
	}

	a, err := CreateAgent(newFakeProvider(), "m",
		WithTools(echoTool("user_tool")),
		WithMiddleware(mw),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(a.tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(a.tools))
	}
	if _, ok := a.toolsByName["mw_tool"]; !ok {
		t.Fatal("middleware-provided tool not found")
	}
}

type toolProviderMiddleware struct {
	tools []Tool
}

func (m *toolProviderMiddleware) Name() string  { return "tool-provider" }
func (m *toolProviderMiddleware) Tools() []Tool { return m.tools }

func TestRun_StructuredOutput_ToolStrategy(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider(
		completion("", toolCall("tc1", "result", `{"answer":42}`)),
	)

	a, err := CreateAgent(fp, "m",
		WithStructuredOutput(&StructuredOutput{
			Strategy: OutputStrategyTool,
			Name:     "result",
			Schema:   map[string]any{"type": "object"},
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.Run(context.Background(), []providers.Message{userMsg("go")})
	if err != nil {
		t.Fatal(err)
	}

	if result.StructuredResponse == nil {
		t.Fatal("expected structured response")
	}
	if string(result.StructuredResponse) != `{"answer":42}` {
		t.Fatalf("unexpected structured response: %s", result.StructuredResponse)
	}
}

func TestRun_StructuredOutput_ProviderStrategy(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider(
		completion(`{"answer":42}`),
	)

	a, err := CreateAgent(fp, "m",
		WithStructuredOutput(&StructuredOutput{
			Strategy: OutputStrategyProvider,
			Name:     "result",
			Schema:   map[string]any{"type": "object"},
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.Run(context.Background(), []providers.Message{userMsg("go")})
	if err != nil {
		t.Fatal(err)
	}

	if result.StructuredResponse == nil {
		t.Fatal("expected structured response")
	}
	if string(result.StructuredResponse) != `{"answer":42}` {
		t.Fatalf("unexpected structured response: %s", result.StructuredResponse)
	}

	if fp.calls[0].ResponseFormat == nil {
		t.Fatal("expected ResponseFormat to be set")
	}
}

func TestRun_StructuredOutput_AutoFallback(t *testing.T) {
	t.Parallel()

	fp := &fallbackProvider{
		firstErr: fmt.Errorf("wrapped: %w", anyllmerrors.ErrUnsupportedParam),
		completions: []providers.ChatCompletion{
			completion("", toolCall("tc1", "out", `{"x":1}`)),
		},
	}

	a, err := CreateAgent(fp, "m",
		WithStructuredOutput(&StructuredOutput{
			Strategy: OutputStrategyAuto,
			Name:     "out",
			Schema:   map[string]any{"type": "object"},
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.Run(context.Background(), []providers.Message{userMsg("go")})
	if err != nil {
		t.Fatal(err)
	}

	if result.StructuredResponse == nil {
		t.Fatal("expected structured response after fallback")
	}
	if string(result.StructuredResponse) != `{"x":1}` {
		t.Fatalf("unexpected response: %s", result.StructuredResponse)
	}
}

func TestRun_StructuredOutput_HandleErrors(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider(
		completion("", toolCall("tc1", "out", `not json`)),
		completion("", toolCall("tc2", "out", `{"ok":true}`)),
	)

	a, err := CreateAgent(fp, "m",
		WithStructuredOutput(&StructuredOutput{
			Strategy: OutputStrategyTool,
			Name:     "out",
			Schema:   map[string]any{"type": "object"},
			HandleErrors: func(err error) string {
				return "Please provide valid JSON."
			},
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.Run(context.Background(), []providers.Message{userMsg("go")})
	if err != nil {
		t.Fatal(err)
	}

	if result.StructuredResponse == nil {
		t.Fatal("expected structured response after retry")
	}
	if string(result.StructuredResponse) != `{"ok":true}` {
		t.Fatalf("unexpected response: %s", result.StructuredResponse)
	}
}

func TestRun_StructuredOutput_MultipleCallsError(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider(
		completion("",
			toolCall("tc1", "out", `{"a":1}`),
			toolCall("tc2", "out", `{"b":2}`),
		),
	)

	a, err := CreateAgent(fp, "m",
		WithStructuredOutput(&StructuredOutput{
			Strategy: OutputStrategyTool,
			Name:     "out",
			Schema:   map[string]any{"type": "object"},
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = a.Run(context.Background(), []providers.Message{userMsg("go")})
	var mErr *MultipleStructuredOutputsError
	if !errors.As(err, &mErr) {
		t.Fatalf("expected MultipleStructuredOutputsError, got %v", err)
	}
}

func TestRun_ParallelToolExecution(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32
	started := make(chan struct{}, 3)
	release := make(chan struct{})

	countTool := Tool{
		Name:        "count",
		Description: "counts",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			callCount.Add(1)
			started <- struct{}{} // signal started
			<-release             // wait for all to be started
			return "ok", nil
		},
	}

	fp := newFakeProvider(
		completion("",
			toolCall("tc1", "count", `{}`),
			toolCall("tc2", "count", `{}`),
			toolCall("tc3", "count", `{}`),
		),
		completion("done"),
	)

	a, err := CreateAgent(fp, "m", WithTools(countTool))
	if err != nil {
		t.Fatal(err)
	}

	// Run in a goroutine so we can observe the started signals.
	type runResult struct {
		result *Result
		err    error
	}
	done := make(chan runResult, 1)
	go func() {
		r, err := a.Run(context.Background(), []providers.Message{userMsg("go")})
		done <- runResult{r, err}
	}()

	// Wait for all 3 tool calls to be concurrently started.
	for i := 0; i < 3; i++ {
		<-started
	}
	// All 3 are blocked on release — proving they run in parallel.
	close(release)

	rr := <-done
	if rr.err != nil {
		t.Fatal(rr.err)
	}

	if callCount.Load() != 3 {
		t.Fatalf("expected 3 tool calls, got %d", callCount.Load())
	}

	if len(rr.result.Messages) != 6 {
		t.Fatalf("expected 6 messages, got %d", len(rr.result.Messages))
	}
}

func TestChainModelCallMiddleware(t *testing.T) {
	t.Parallel()

	var order []string

	mw1 := &modelCallRecorder{name: "outer", order: &order}
	mw2 := &modelCallRecorder{name: "inner", order: &order}

	inner := func(_ context.Context, _ *ModelRequest) (*ModelResponse, error) {
		order = append(order, "handler")
		return &ModelResponse{}, nil
	}

	chain := chainModelCallMiddleware([]ModelCallMiddleware{mw1, mw2}, inner)
	_, err := chain(context.Background(), &ModelRequest{})
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"outer.before", "inner.before", "handler", "inner.after", "outer.after"}
	if len(order) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
	for i := range expected {
		if order[i] != expected[i] {
			t.Errorf("order[%d]: got %q, want %q", i, order[i], expected[i])
		}
	}
}

type modelCallRecorder struct {
	name  string
	order *[]string
}

func (m *modelCallRecorder) Name() string { return m.name }
func (m *modelCallRecorder) WrapModelCall(ctx context.Context, req *ModelRequest, next ModelCallHandler) (*ModelResponse, error) {
	*m.order = append(*m.order, m.name+".before")
	resp, err := next(ctx, req)
	*m.order = append(*m.order, m.name+".after")
	return resp, err
}

func TestChainToolCallMiddleware(t *testing.T) {
	t.Parallel()

	var order []string

	mw1 := &toolCallRecorder{name: "outer", order: &order}
	mw2 := &toolCallRecorder{name: "inner", order: &order}

	inner := func(_ context.Context, _ *ToolCallRequest) (*ToolCallResponse, error) {
		order = append(order, "handler")
		return &ToolCallResponse{Content: "ok"}, nil
	}

	chain := chainToolCallMiddleware([]ToolCallMiddleware{mw1, mw2}, inner)
	_, err := chain(context.Background(), &ToolCallRequest{})
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"outer.before", "inner.before", "handler", "inner.after", "outer.after"}
	if len(order) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
	for i := range expected {
		if order[i] != expected[i] {
			t.Errorf("order[%d]: got %q, want %q", i, order[i], expected[i])
		}
	}
}

type toolCallRecorder struct {
	name  string
	order *[]string
}

func (m *toolCallRecorder) Name() string { return m.name }
func (m *toolCallRecorder) WrapToolCall(ctx context.Context, req *ToolCallRequest, next ToolCallHandler) (*ToolCallResponse, error) {
	*m.order = append(*m.order, m.name+".before")
	resp, err := next(ctx, req)
	*m.order = append(*m.order, m.name+".after")
	return resp, err
}

func TestRun_ProviderError(t *testing.T) {
	t.Parallel()

	a, err := CreateAgent(&errProvider{err: errors.New("api down")}, "m")
	if err != nil {
		t.Fatal(err)
	}

	_, err = a.Run(context.Background(), []providers.Message{userMsg("go")})
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "api down") {
		t.Fatalf("expected 'api down' in error, got %q", err.Error())
	}
}

func TestRun_ToolsAndStructuredOutput(t *testing.T) {
	t.Parallel()

	fp := newFakeProvider(
		completion("",
			toolCall("tc1", "echo", `{"v":"hi"}`),
			toolCall("tc2", "result", `{"answer":"done"}`),
		),
	)

	a, err := CreateAgent(fp, "m",
		WithTools(echoTool("echo")),
		WithStructuredOutput(&StructuredOutput{
			Strategy: OutputStrategyTool,
			Name:     "result",
			Schema:   map[string]any{"type": "object"},
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.Run(context.Background(), []providers.Message{userMsg("go")})
	if err != nil {
		t.Fatal(err)
	}

	if result.StructuredResponse == nil {
		t.Fatal("expected structured response")
	}
	if string(result.StructuredResponse) != `{"answer":"done"}` {
		t.Fatalf("unexpected structured response: %s", result.StructuredResponse)
	}
}

func TestBuildModelRequest_ToolStrategy_ForcesToolChoice(t *testing.T) {
	t.Parallel()

	a, err := CreateAgent(newFakeProvider(), "m",
		WithStructuredOutput(&StructuredOutput{
			Strategy: OutputStrategyTool,
			Name:     "out",
			Schema:   map[string]any{"type": "object"},
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	req := a.buildModelRequest(&State{}, OutputStrategyTool)
	tc, ok := req.ToolChoice.(providers.ToolChoice)
	if !ok {
		t.Fatalf("expected providers.ToolChoice, got %T", req.ToolChoice)
	}
	if tc.Function == nil || tc.Function.Name != "out" {
		t.Fatal("expected forced tool choice for 'out'")
	}
}

func TestBuildModelRequest_ToolStrategy_WithRegularTools(t *testing.T) {
	t.Parallel()

	a, err := CreateAgent(newFakeProvider(), "m",
		WithTools(echoTool("t")),
		WithStructuredOutput(&StructuredOutput{
			Strategy: OutputStrategyTool,
			Name:     "out",
			Schema:   map[string]any{"type": "object"},
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	req := a.buildModelRequest(&State{}, OutputStrategyTool)
	if req.ToolChoice != "required" {
		t.Fatalf("expected 'required' tool choice, got %v", req.ToolChoice)
	}
	if len(req.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(req.Tools))
	}
}

func TestRun_MultiStepInvestigation(t *testing.T) {
	t.Parallel()

	// Simulate: model calls query_logs, sees errors, calls query_metrics, then answers.
	queryLogs := Tool{
		Name:        "query_logs",
		Description: "query logs",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return `{"logs":[{"level":"ERROR","message":"OOM killed"}]}`, nil
		},
	}
	queryMetrics := Tool{
		Name:        "query_metrics",
		Description: "query metrics",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return `{"memory_usage":0.95,"memory_limit":0.5}`, nil
		},
	}

	fp := newFakeProvider(
		// Iteration 1: model calls query_logs.
		completion("", toolCall("tc1", "query_logs", `{}`)),
		// Iteration 2: model sees log results, calls query_metrics.
		completion("", toolCall("tc2", "query_metrics", `{}`)),
		// Iteration 3: model produces final answer.
		completion("Root cause: OOM. Memory usage at 95% exceeds 50% limit."),
	)

	a, err := CreateAgent(fp, "m", WithTools(queryLogs, queryMetrics))
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.Run(context.Background(), []providers.Message{userMsg("analyze alert")})
	if err != nil {
		t.Fatal(err)
	}

	// 3 model calls.
	if int(fp.index.Load()) != 3 {
		t.Fatalf("expected 3 provider calls, got %d", fp.index.Load())
	}

	// Messages: user, assistant(tool_call), tool_result, assistant(tool_call), tool_result, assistant(final).
	if len(result.Messages) != 6 {
		t.Fatalf("expected 6 messages, got %d", len(result.Messages))
	}

	last := result.Messages[len(result.Messages)-1]
	if !contains(last.ContentString(), "OOM") {
		t.Fatalf("expected final answer about OOM, got %q", last.ContentString())
	}
}

func TestRun_ToolErrorRecovery(t *testing.T) {
	t.Parallel()

	callCount := 0
	flaky := Tool{
		Name:        "flaky_api",
		Description: "fails first, succeeds second",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			callCount++
			if callCount == 1 {
				return "", errors.New("connection timeout")
			}
			return `{"status":"healthy"}`, nil
		},
	}

	fp := newFakeProvider(
		// Iteration 1: model calls flaky_api → fails.
		completion("", toolCall("tc1", "flaky_api", `{}`)),
		// Iteration 2: model sees error, retries.
		completion("", toolCall("tc2", "flaky_api", `{}`)),
		// Iteration 3: model got result, answers.
		completion("Service is healthy."),
	)

	a, err := CreateAgent(fp, "m", WithTools(flaky))
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.Run(context.Background(), []providers.Message{userMsg("check")})
	if err != nil {
		t.Fatal(err)
	}

	if callCount != 2 {
		t.Fatalf("expected tool called 2 times, got %d", callCount)
	}

	// First tool result should be an error message.
	if !contains(result.Messages[2].ContentString(), "Error executing tool") {
		t.Fatalf("expected error in first tool result, got %q", result.Messages[2].ContentString())
	}

	// Second tool result should be the success response.
	if !contains(result.Messages[4].ContentString(), "healthy") {
		t.Fatalf("expected success in second tool result, got %q", result.Messages[4].ContentString())
	}

	last := result.Messages[len(result.Messages)-1]
	if !contains(last.ContentString(), "healthy") {
		t.Fatalf("expected final answer about healthy, got %q", last.ContentString())
	}
}

func TestRun_StructuredOutputAfterTools(t *testing.T) {
	t.Parallel()

	investigate := Tool{
		Name:        "investigate",
		Description: "gathers data",
		Parameters:  map[string]any{"type": "object"},
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return `{"finding":"high latency on /api/orders"}`, nil
		},
	}

	fp := newFakeProvider(
		// Iteration 1: model investigates.
		completion("", toolCall("tc1", "investigate", `{}`)),
		// Iteration 2: model produces structured output via tool strategy.
		completion("", toolCall("tc2", "report", `{"root_cause":"high latency","severity":"high"}`)),
	)

	a, err := CreateAgent(fp, "m",
		WithTools(investigate),
		WithStructuredOutput(&StructuredOutput{
			Strategy: OutputStrategyTool,
			Name:     "report",
			Schema:   map[string]any{"type": "object"},
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.Run(context.Background(), []providers.Message{userMsg("analyze")})
	if err != nil {
		t.Fatal(err)
	}

	// Model called twice.
	if int(fp.index.Load()) != 2 {
		t.Fatalf("expected 2 provider calls, got %d", fp.index.Load())
	}

	// Structured output captured.
	if result.StructuredResponse == nil {
		t.Fatal("expected structured response")
	}
	if !contains(string(result.StructuredResponse), "high latency") {
		t.Fatalf("unexpected structured response: %s", result.StructuredResponse)
	}

	// The investigate tool should have executed and its result should be in messages.
	foundToolResult := false
	for _, msg := range result.Messages {
		if msg.Role == providers.RoleTool && contains(msg.ContentString(), "high latency on /api/orders") {
			foundToolResult = true
		}
	}
	if !foundToolResult {
		t.Fatal("expected investigate tool result in messages")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstr(s, sub))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mozilla-ai/any-llm-go/providers"
)

// errStreamCancelled is returned when context is cancelled during event emission.
var errStreamCancelled = errors.New("stream cancelled")

// trySend sends an event or returns an error if context is done.
func trySend(ctx context.Context, events chan<- StreamEvent, ev StreamEvent) error {
	select {
	case events <- ev:
		return nil
	case <-ctx.Done():
		return errStreamCancelled
	}
}

// Stream executes the agent like Run but emits streaming events as the model
// generates output. Returns two channels: events (buffered) and errors (at most
// one). Both channels are closed when the agent finishes.
//
// The consumer should read from events until it is closed, then check errors:
//
//	events, errs := agent.Stream(ctx, messages)
//	for event := range events {
//	    switch event.Type {
//	    case agent.StreamEventTextDelta:
//	        fmt.Print(event.Delta)
//	    case agent.StreamEventToolCallStart:
//	        fmt.Printf("calling %s...\n", event.ToolName)
//	    case agent.StreamEventToolResult:
//	        fmt.Printf("tool %s done\n", event.ToolName)
//	    case agent.StreamEventComplete:
//	        // event.Result has the final Result
//	    }
//	}
//	if err := <-errs; err != nil {
//	    log.Fatal(err)
//	}
func (a *Agent) Stream(ctx context.Context, messages []providers.Message) (<-chan StreamEvent, <-chan error) {
	events := make(chan StreamEvent, 64)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		if err := a.runStream(ctx, messages, events); err != nil {
			errs <- err
		}
	}()

	return events, errs
}

// runStream is the streaming ReAct loop. It mirrors Run() but uses
// CompletionStream for model calls and emits events to the channel.
func (a *Agent) runStream(ctx context.Context, messages []providers.Message, events chan<- StreamEvent) error {
	logger := a.newRunLogger()

	state := &State{
		Messages: make([]providers.Message, 0, len(messages)),
	}
	state.Messages = append(state.Messages, messages...)

	strategy := a.initialStrategy()

	// Before-agent hooks (forward order).
	for _, hook := range a.beforeAgentHooks {
		if err := hook.BeforeAgent(ctx, state); err != nil {
			return fmt.Errorf("before_agent %q: %w", hook.Name(), err)
		}
	}

	// Build streaming model call chain: same middleware, streaming inner handler.
	streamModelCallChain := chainModelCallMiddleware(a.modelCallMW, func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
		return a.executeModelCallStreaming(ctx, req, events)
	})

	// ReAct loop.
	finished := false
	for i := 0; i < a.maxIterations; i++ {
		// Before-model hooks (forward order).
		for _, hook := range a.beforeModelHooks {
			if err := hook.BeforeModel(ctx, state); err != nil {
				return fmt.Errorf("before_model %q: %w", hook.Name(), err)
			}
		}
		if state.Done {
			finished = true
			break
		}

		// Streaming model call.
		req := a.buildModelRequest(state, strategy)
		modelStart := time.Now()
		resp, err := streamModelCallChain(ctx, req)

		// Auto-strategy fallback: provider -> tool.
		if err != nil && a.shouldFallbackToTool(strategy, err) {
			logger.InfoContext(ctx, "provider strategy unsupported, falling back to tool strategy")
			strategy = OutputStrategyTool
			req = a.buildModelRequest(state, strategy)
			modelStart = time.Now()
			resp, err = streamModelCallChain(ctx, req)
		}
		if err != nil {
			logger.ErrorContext(ctx, "model call failed", "iteration", i+1, "elapsed_s", time.Since(modelStart).Seconds(), "error", err)
			return fmt.Errorf("model call (iteration %d): %w", i+1, err)
		}
		logger.InfoContext(ctx, "model call completed", "iteration", i+1, "elapsed_s", time.Since(modelStart).Seconds())

		// Append assistant message.
		state.Messages = append(state.Messages, resp.Message)

		// Provider strategy: structured response from content.
		if resp.StructuredResponse != nil {
			state.StructuredResponse = resp.StructuredResponse
		}

		// After-model hooks (reverse order).
		for j := len(a.afterModelHooks) - 1; j >= 0; j-- {
			if err := a.afterModelHooks[j].AfterModel(ctx, state); err != nil {
				return fmt.Errorf("after_model %q: %w", a.afterModelHooks[j].Name(), err)
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

		// Process tool calls (parallel, with event emission).
		returnDirect, err := a.processToolCalls(ctx, logger, state, resp.Message.ToolCalls, strategy, events)
		if err != nil {
			return err
		}

		if returnDirect {
			finished = true
			break
		}

		if state.StructuredResponse != nil {
			finished = true
			break
		}
	}

	if !finished {
		return fmt.Errorf("%w: limit is %d", ErrMaxIterations, a.maxIterations)
	}

	// After-agent hooks (reverse order).
	for i := len(a.afterAgentHooks) - 1; i >= 0; i-- {
		if err := a.afterAgentHooks[i].AfterAgent(ctx, state); err != nil {
			return fmt.Errorf("after_agent %q: %w", a.afterAgentHooks[i].Name(), err)
		}
	}

	_ = trySend(ctx, events, StreamEvent{
		Type: StreamEventComplete,
		Result: &Result{
			Messages:           state.Messages,
			StructuredResponse: state.StructuredResponse,
		},
	})

	return nil
}

// executeModelCallStreaming uses CompletionStream to stream model output,
// emitting text delta and tool call events. Returns the accumulated
// ModelResponse when the stream completes (so middleware sees the full response).
func (a *Agent) executeModelCallStreaming(ctx context.Context, req *ModelRequest, events chan<- StreamEvent) (*ModelResponse, error) {
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
		Stream:         true,
	}

	chunks, errs := a.provider.CompletionStream(ctx, params)

	// Accumulate the full message from stream chunks.
	// On send failure (context cancelled), we keep draining chunks so the
	// provider goroutine can complete and channels are properly closed.
	var content strings.Builder
	var toolCalls []providers.ToolCall
	toolCallArgs := make(map[string]*strings.Builder) // keyed by tool call ID
	var sendErr error

	for chunk := range chunks {
		if sendErr != nil {
			continue // drain remaining chunks to unblock provider
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		// Text content.
		if delta.Content != "" {
			content.WriteString(delta.Content)
			if err := trySend(ctx, events, StreamEvent{
				Type:  StreamEventTextDelta,
				Delta: delta.Content,
			}); err != nil {
				sendErr = err
				continue
			}
		}

		// Tool calls (streamed incrementally).
		for _, tc := range delta.ToolCalls {
			if tc.ID != "" && tc.Function.Name != "" {
				// New tool call starting.
				toolCalls = append(toolCalls, providers.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: providers.FunctionCall{
						Name: tc.Function.Name,
					},
				})
				toolCallArgs[tc.ID] = &strings.Builder{}

				if err := trySend(ctx, events, StreamEvent{
					Type:       StreamEventToolCallStart,
					ToolName:   tc.Function.Name,
					ToolCallID: tc.ID,
					Args:       tc.Function.Arguments,
				}); err != nil {
					sendErr = err
					break
				}
			}

			// Accumulate arguments.
			if tc.Function.Arguments != "" {
				id := tc.ID
				if id == "" {
					if len(toolCalls) == 0 {
						continue
					}
					// No ID on delta — append to most recent tool call.
					id = toolCalls[len(toolCalls)-1].ID
				}
				if b, ok := toolCallArgs[id]; ok {
					b.WriteString(tc.Function.Arguments)
				}
			}
		}
	}

	// Always read the stream error to prevent goroutine leak.
	streamErr := <-errs
	if sendErr != nil {
		return nil, sendErr
	}
	if streamErr != nil {
		return nil, streamErr
	}

	// Build the accumulated message.
	for i := range toolCalls {
		if b, ok := toolCallArgs[toolCalls[i].ID]; ok {
			toolCalls[i].Function.Arguments = b.String()
		}
	}

	msg := providers.Message{
		Role:      providers.RoleAssistant,
		Content:   content.String(),
		ToolCalls: toolCalls,
	}

	// Handle structured output (same as non-streaming).
	structured, err := a.handleModelOutput(req, msg)
	if err != nil {
		return nil, err
	}

	return &ModelResponse{
		Message:            msg,
		StructuredResponse: structured,
	}, nil
}

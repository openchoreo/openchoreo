// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/mozilla-ai/any-llm-go/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// streamProvider implements providers.Provider with a scripted sequence of
// streaming responses. Each call to CompletionStream sends the next set of
// chunks, then closes the channels.
type streamProvider struct {
	// responses is a sequence of chunk slices, one per CompletionStream call.
	responses [][]providers.ChatCompletionChunk
	callIndex int
}

func (s *streamProvider) Name() string { return "stream-fake" }

func (s *streamProvider) Completion(context.Context, providers.CompletionParams) (*providers.ChatCompletion, error) {
	panic("streaming provider: use CompletionStream")
}

func (s *streamProvider) CompletionStream(_ context.Context, _ providers.CompletionParams) (<-chan providers.ChatCompletionChunk, <-chan error) {
	chunks := make(chan providers.ChatCompletionChunk, 64)
	errs := make(chan error, 1)

	i := s.callIndex
	s.callIndex++

	go func() {
		defer close(chunks)
		defer close(errs)
		if i < len(s.responses) {
			for _, c := range s.responses[i] {
				chunks <- c
			}
		}
	}()

	return chunks, errs
}

// textChunk creates a chunk with text content.
func textChunk(content string) providers.ChatCompletionChunk {
	return providers.ChatCompletionChunk{
		Choices: []providers.ChunkChoice{{
			Delta: providers.ChunkDelta{Content: content},
		}},
	}
}

// toolCallStartChunk creates a chunk that starts a new tool call.
func toolCallStartChunk(id, name, args string) providers.ChatCompletionChunk {
	return providers.ChatCompletionChunk{
		Choices: []providers.ChunkChoice{{
			Delta: providers.ChunkDelta{
				ToolCalls: []providers.ToolCall{{
					ID:   id,
					Type: "function",
					Function: providers.FunctionCall{
						Name:      name,
						Arguments: args,
					},
				}},
			},
		}},
	}
}

// toolCallArgsChunk creates a chunk that appends arguments to the most recent tool call.
func toolCallArgsChunk(args string) providers.ChatCompletionChunk {
	return providers.ChatCompletionChunk{
		Choices: []providers.ChunkChoice{{
			Delta: providers.ChunkDelta{
				ToolCalls: []providers.ToolCall{{
					Function: providers.FunctionCall{Arguments: args},
				}},
			},
		}},
	}
}

// collectEvents drains the events channel and returns all events.
func collectEvents(t *testing.T, events <-chan StreamEvent, errs <-chan error) []StreamEvent {
	t.Helper()
	var result []StreamEvent
	for ev := range events {
		result = append(result, ev)
	}
	if err := <-errs; err != nil {
		t.Fatalf("stream error: %v", err)
	}
	return result
}

func TestStream_TextOnly(t *testing.T) {
	t.Parallel()

	sp := &streamProvider{
		responses: [][]providers.ChatCompletionChunk{
			{textChunk("hello "), textChunk("world")},
		},
	}

	a, err := CreateAgent(sp, "m")
	require.NoError(t, err)

	events, errs := a.Stream(context.Background(), []providers.Message{userMsg("hi")})
	all := collectEvents(t, events, errs)

	var deltas []string
	for _, ev := range all {
		if ev.Type == StreamEventTextDelta {
			deltas = append(deltas, ev.Delta)
		}
	}
	assert.Equal(t, []string{"hello ", "world"}, deltas)

	// Last event should be complete.
	last := all[len(all)-1]
	assert.Equal(t, StreamEventComplete, last.Type)
	require.NotNil(t, last.Result)
	assert.Equal(t, "hello world", last.Result.Messages[len(last.Result.Messages)-1].ContentString())
}

func TestStream_WithToolCall(t *testing.T) {
	t.Parallel()

	sp := &streamProvider{
		responses: [][]providers.ChatCompletionChunk{
			// First call: model calls a tool.
			{
				toolCallStartChunk("tc1", "echo", `{"m`),
				toolCallArgsChunk(`sg":"hi"}`),
			},
			// Second call: model responds with text.
			{textChunk("done")},
		},
	}

	a, err := CreateAgent(sp, "m", WithTools(echoTool("echo")))
	require.NoError(t, err)

	events, errs := a.Stream(context.Background(), []providers.Message{userMsg("go")})
	all := collectEvents(t, events, errs)

	types := make([]StreamEventType, 0, len(all))
	for _, ev := range all {
		types = append(types, ev.Type)
	}

	assert.Contains(t, types, StreamEventToolCallStart)
	assert.Contains(t, types, StreamEventToolResult)
	assert.Contains(t, types, StreamEventTextDelta)
	assert.Contains(t, types, StreamEventComplete)

	// Verify tool call start event has correct name.
	for _, ev := range all {
		if ev.Type == StreamEventToolCallStart {
			assert.Equal(t, "echo", ev.ToolName)
			assert.Equal(t, "tc1", ev.ToolCallID)
		}
	}
}

func TestStream_ToolResult(t *testing.T) {
	t.Parallel()

	sp := &streamProvider{
		responses: [][]providers.ChatCompletionChunk{
			{toolCallStartChunk("tc1", "echo", `{"v":"hello"}`)},
			{textChunk("ok")},
		},
	}

	a, err := CreateAgent(sp, "m", WithTools(echoTool("echo")))
	require.NoError(t, err)

	events, errs := a.Stream(context.Background(), []providers.Message{userMsg("go")})
	all := collectEvents(t, events, errs)

	for _, ev := range all {
		if ev.Type == StreamEventToolResult {
			assert.Equal(t, "echo", ev.ToolName)
			assert.Equal(t, `{"v":"hello"}`, ev.Content)
			return
		}
	}
	t.Fatal("expected StreamEventToolResult")
}

func TestStream_UnknownToolEmitsEvent(t *testing.T) {
	t.Parallel()

	sp := &streamProvider{
		responses: [][]providers.ChatCompletionChunk{
			{toolCallStartChunk("tc1", "nonexistent", `{}`)},
			{textChunk("recovered")},
		},
	}

	a, err := CreateAgent(sp, "m")
	require.NoError(t, err)

	events, errs := a.Stream(context.Background(), []providers.Message{userMsg("go")})
	all := collectEvents(t, events, errs)

	// Unknown tool should still emit a ToolResult event (bug 4 fix).
	for _, ev := range all {
		if ev.Type == StreamEventToolResult {
			assert.Equal(t, "nonexistent", ev.ToolName)
			assert.Contains(t, ev.Content, "unknown tool")
			return
		}
	}
	t.Fatal("expected StreamEventToolResult for unknown tool")
}

func TestStream_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Provider that sends many chunks — enough to fill the event buffer.
	var chunks []providers.ChatCompletionChunk
	for i := 0; i < 200; i++ {
		chunks = append(chunks, textChunk("x"))
	}
	sp := &streamProvider{responses: [][]providers.ChatCompletionChunk{chunks}}

	a, err := CreateAgent(sp, "m")
	require.NoError(t, err)

	events, errs := a.Stream(ctx, []providers.Message{userMsg("go")})

	// Read a few events then cancel.
	count := 0
	for range events {
		count++
		if count == 5 {
			cancel()
			break
		}
	}

	// Drain remaining events.
	for range events {
	}

	// Should not hang — the drain fix ensures the provider goroutine completes.
	// Error channel may or may not have an error (stream cancelled).
	<-errs
}

func TestStream_CompleteEvent(t *testing.T) {
	t.Parallel()

	sp := &streamProvider{
		responses: [][]providers.ChatCompletionChunk{
			{textChunk("answer")},
		},
	}

	a, err := CreateAgent(sp, "m")
	require.NoError(t, err)

	events, errs := a.Stream(context.Background(), []providers.Message{userMsg("q")})
	all := collectEvents(t, events, errs)

	last := all[len(all)-1]
	assert.Equal(t, StreamEventComplete, last.Type)
	require.NotNil(t, last.Result)
	assert.Len(t, last.Result.Messages, 2) // user + assistant
}

func TestStream_MultipleToolCalls(t *testing.T) {
	t.Parallel()

	sp := &streamProvider{
		responses: [][]providers.ChatCompletionChunk{
			{
				toolCallStartChunk("tc1", "echo", `{"a":"1"}`),
				toolCallStartChunk("tc2", "echo", `{"a":"2"}`),
			},
			{textChunk("done")},
		},
	}

	a, err := CreateAgent(sp, "m", WithTools(echoTool("echo")))
	require.NoError(t, err)

	events, errs := a.Stream(context.Background(), []providers.Message{userMsg("go")})
	all := collectEvents(t, events, errs)

	var toolStarts int
	var toolResults int
	for _, ev := range all {
		switch ev.Type {
		case StreamEventToolCallStart:
			toolStarts++
		case StreamEventToolResult:
			toolResults++
		}
	}
	assert.Equal(t, 2, toolStarts)
	assert.Equal(t, 2, toolResults)
}

func TestStream_StructuredOutput(t *testing.T) {
	t.Parallel()

	sp := &streamProvider{
		responses: [][]providers.ChatCompletionChunk{
			{textChunk(`{"answer":42}`)},
		},
	}

	a, err := CreateAgent(sp, "m",
		WithStructuredOutput(&StructuredOutput{
			Strategy: OutputStrategyProvider,
			Name:     "result",
			Schema:   map[string]any{"type": "object"},
		}),
	)
	require.NoError(t, err)

	events, errs := a.Stream(context.Background(), []providers.Message{userMsg("go")})
	all := collectEvents(t, events, errs)

	last := all[len(all)-1]
	assert.Equal(t, StreamEventComplete, last.Type)
	require.NotNil(t, last.Result)
	assert.NotNil(t, last.Result.StructuredResponse)
	assert.Equal(t, `{"answer":42}`, string(last.Result.StructuredResponse))
}

// Ensure streaming works with StructuredOutput tool strategy.
func TestStream_StructuredOutput_ToolStrategy(t *testing.T) {
	t.Parallel()

	sp := &streamProvider{
		responses: [][]providers.ChatCompletionChunk{
			{toolCallStartChunk("tc1", "result", `{"answer":42}`)},
		},
	}

	a, err := CreateAgent(sp, "m",
		WithStructuredOutput(&StructuredOutput{
			Strategy: OutputStrategyTool,
			Name:     "result",
			Schema:   map[string]any{"type": "object"},
		}),
	)
	require.NoError(t, err)

	events, errs := a.Stream(context.Background(), []providers.Message{userMsg("go")})
	all := collectEvents(t, events, errs)

	last := all[len(all)-1]
	assert.Equal(t, StreamEventComplete, last.Type)
	require.NotNil(t, last.Result)
	assert.Equal(t, `{"answer":42}`, string(last.Result.StructuredResponse))
}

func TestStream_ProviderError(t *testing.T) {
	t.Parallel()

	sp := &errorStreamProvider{
		chunks: []providers.ChatCompletionChunk{textChunk("partial")},
		err:    fmt.Errorf("connection reset"),
	}

	a, err := CreateAgent(sp, "m")
	require.NoError(t, err)

	events, errs := a.Stream(context.Background(), []providers.Message{userMsg("go")})

	// Drain events.
	for range events {
	}

	err = <-errs
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection reset")
}

// errorStreamProvider sends chunks followed by an error on the errs channel.
type errorStreamProvider struct {
	chunks []providers.ChatCompletionChunk
	err    error
}

func (e *errorStreamProvider) Name() string { return "error-stream-fake" }

func (e *errorStreamProvider) Completion(context.Context, providers.CompletionParams) (*providers.ChatCompletion, error) {
	panic("use CompletionStream")
}

func (e *errorStreamProvider) CompletionStream(_ context.Context, _ providers.CompletionParams) (<-chan providers.ChatCompletionChunk, <-chan error) {
	chunks := make(chan providers.ChatCompletionChunk, 64)
	errs := make(chan error, 1)

	go func() {
		defer close(chunks)
		defer close(errs)
		for _, c := range e.chunks {
			chunks <- c
		}
		errs <- e.err
	}()

	return chunks, errs
}

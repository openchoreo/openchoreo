// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"encoding/json"
	"fmt"

	"github.com/mozilla-ai/any-llm-go/providers"
)

// OutputStrategy determines how structured output is obtained from the model.
type OutputStrategy int

const (
	// OutputStrategyAuto tries provider-native structured output first, falling
	// back to tool-based structured output if the provider returns an error
	// indicating it doesn't support the feature.
	OutputStrategyAuto OutputStrategy = iota

	// OutputStrategyProvider uses the model provider's native structured output
	// (e.g., OpenAI's response_format with json_schema type).
	OutputStrategyProvider

	// OutputStrategyTool creates a synthetic tool from the output schema and
	// extracts the structured response from the tool call arguments. This works
	// with any provider that supports tool calling.
	OutputStrategyTool
)

// HandleErrorsFunc is called when structured output parsing fails.
// It receives the error and returns a message to send to the model for retry.
type HandleErrorsFunc func(err error) string

// StructuredOutput configures structured output for an agent.
// The Schema is a standard JSON Schema and may contain oneOf, anyOf, or
// any other JSON Schema construct — the agent passes it through as-is.
type StructuredOutput struct {
	// Strategy controls how structured output is obtained.
	// Defaults to OutputStrategyAuto if zero value.
	Strategy OutputStrategy

	// Name is the schema/tool name (e.g., "rca_report").
	// Required.
	Name string

	// Description explains what the output represents. Used as the tool
	// description in tool strategy or schema description in provider strategy.
	Description string

	// Schema is the JSON Schema for the structured output.
	// May contain oneOf/anyOf for union types — handled natively by the
	// provider or the tool calling mechanism.
	// Required.
	Schema map[string]any

	// Strict enforces strict schema validation when supported by the provider.
	Strict *bool

	// HandleErrors controls retry behavior when structured output parsing fails.
	// If nil, parse errors are returned as-is (no retry).
	// If set, the function is called with the error, and its return string is
	// sent to the model as a ToolMessage so it can retry.
	HandleErrors HandleErrorsFunc

	// ToolMessageContent customizes the ToolMessage content sent back to the
	// model when structured output is successfully parsed via tool strategy.
	// If empty, defaults to "Returning structured response: <value>".
	ToolMessageContent string
}

// toProviderResponseFormat converts to a provider ResponseFormat for
// native structured output.
func (so *StructuredOutput) toProviderResponseFormat() *providers.ResponseFormat {
	return &providers.ResponseFormat{
		Type: "json_schema",
		JSONSchema: &providers.JSONSchema{
			Name:        so.Name,
			Description: so.Description,
			Schema:      so.Schema,
			Strict:      so.Strict,
		},
	}
}

// toProviderTool converts the output schema to a tool definition for
// tool-based structured output.
func (so *StructuredOutput) toProviderTool() providers.Tool {
	return providers.Tool{
		Type: "function",
		Function: providers.Function{
			Name:        so.Name,
			Description: so.Description,
			Parameters:  so.Schema,
		},
	}
}

// successMessage returns the ToolMessage content for a successful parse.
func (so *StructuredOutput) successMessage(value json.RawMessage) string {
	if so.ToolMessageContent != "" {
		return so.ToolMessageContent
	}
	return fmt.Sprintf("Returning structured response: %s", string(value))
}

// StructuredOutputError is returned when structured output parsing fails
// and HandleErrors is not configured (or is nil).
type StructuredOutputError struct {
	// ToolName is the name of the structured output tool/schema that failed.
	ToolName string
	// Err is the underlying parse error.
	Err error
}

func (e *StructuredOutputError) Error() string {
	return fmt.Sprintf("structured output %q: %v", e.ToolName, e.Err)
}

func (e *StructuredOutputError) Unwrap() error {
	return e.Err
}

// MultipleStructuredOutputsError is returned when the model calls the
// structured output tool more than once in a single response.
type MultipleStructuredOutputsError struct {
	// ToolNames is the list of structured output tool names that were called.
	ToolNames []string
}

func (e *MultipleStructuredOutputsError) Error() string {
	return fmt.Sprintf("model called structured output tool multiple times: %v", e.ToolNames)
}

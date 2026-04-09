// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"encoding/json"

	"github.com/mozilla-ai/any-llm-go/providers"
)

// Tool represents a callable tool available to the agent.
type Tool struct {
	// Name is the function name the model uses to invoke this tool.
	Name string
	// Description explains what the tool does, shown to the model.
	Description string
	// Parameters is the JSON Schema describing the tool's input arguments.
	Parameters map[string]any
	// Execute runs the tool with the given JSON arguments and returns
	// the result as a string. Errors are caught and sent to the model as
	// error messages so it can recover.
	Execute func(ctx context.Context, arguments json.RawMessage) (string, error)
	// ReturnDirect, when true, causes the agent to exit immediately after
	// this tool executes, returning the tool's output without sending it
	// back to the model for further processing.
	ReturnDirect bool
}

// toProviderTool converts a Tool to the anyllm-go provider tool format.
func (t *Tool) toProviderTool() providers.Tool {
	return providers.Tool{
		Type: "function",
		Function: providers.Function{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		},
	}
}

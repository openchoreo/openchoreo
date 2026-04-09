// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// NewWriteTodosTool creates a write_todos tool for agent task tracking.
func NewWriteTodosTool() Tool {
	return Tool{
		Name:        "write_todos",
		Description: "Track investigation tasks. Call this to update your todo list with current tasks and their statuses.",
		Parameters: map[string]any{
			"type":     "object",
			"required": []string{"todos"},
			"properties": map[string]any{
				"todos": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"content", "status"},
						"properties": map[string]any{
							"content": map[string]any{
								"type":        "string",
								"minLength":   1,
								"description": "What needs to be done",
							},
							"status": map[string]any{
								"type":        "string",
								"description": "Task status: pending, in_progress, or completed",
								"enum":        []string{"pending", "in_progress", "completed"},
							},
						},
					},
					"description": "The updated todo list",
				},
			},
		},
		Execute: executeTodos,
	}
}

func executeTodos(_ context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Todos []struct {
			Content string `json:"content"`
			Status  string `json:"status"`
		} `json:"todos"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid todos params: %w", err)
	}
	if params.Todos == nil {
		return "", fmt.Errorf("missing required field: todos")
	}

	for _, todo := range params.Todos {
		if strings.TrimSpace(todo.Content) == "" {
			return "", fmt.Errorf("todo content must not be empty")
		}
	}

	var pending, inProgress, completed int
	var sb strings.Builder
	sb.WriteString("Todo list updated.\n\nCurrent todos:\n")

	for _, todo := range params.Todos {
		var icon string
		switch todo.Status {
		case "pending":
			icon = "[ ]"
			pending++
		case "in_progress":
			icon = "[~]"
			inProgress++
		case "completed":
			icon = "[x]"
			completed++
		default:
			return "", fmt.Errorf("invalid status %q for todo %q", todo.Status, todo.Content)
		}
		fmt.Fprintf(&sb, "  %s %s\n", icon, todo.Content)
	}

	fmt.Fprintf(&sb, "\nStatus: %d pending, %d in progress, %d completed",
		pending, inProgress, completed)

	return sb.String(), nil
}

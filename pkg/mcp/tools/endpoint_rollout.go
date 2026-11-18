// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

var defaultRolloutSteps = []int32{5, 25, 50, 100}

func dryRunProperty() map[string]any {
	return map[string]any{
		"type":        "boolean",
		"description": "Optional: return the change without writing it",
	}
}

func rolloutEnvironmentProperty() map[string]any {
	return stringProperty("Environment whose binding the call changes, e.g. development")
}

func (t *Toolsets) RegisterGetEndpointRollout(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "get_endpoint_rollout"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionViewEndpoint}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Get an endpoint's rollout: the plan in spec.rollout (strategy, stable, candidate, steps) " +
			"and, per environment, the step it is at, each backend's share and whether its routes are ready.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Endpoint name. Use list_endpoints to discover valid names"),
		}, []string{"namespace_name", "name"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		result, err := t.EndpointToolset.GetEndpointRollout(ctx, args.NamespaceName, args.Name)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterStartEndpointRollout(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "start_endpoint_rollout"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateEndpoint}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Plan a rollout on an endpoint: move traffic from its stable backend to one candidate. " +
			"canary shifts a growing share in steps, blueGreen previews the candidate by a header then " +
			"switches, abTest sends the requests carrying a cookie, and mirror copies a growing share. The " +
			"candidate starts at weight 0 in every environment; step_endpoint_rollout moves each environment.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Endpoint name. Use list_endpoints to discover valid names"),
			"strategy": map[string]any{
				"type": "string",
				"enum": []string{"canary", "blueGreen", "abTest", "mirror"},
			},
			"stable": stringProperty("Name of the backend serving now. It keeps the rest of the traffic"),
			"candidate": map[string]any{
				"type": "object",
				"description": "The backend being rolled out. Name one the endpoint has, or also give component " +
					"and endpoint to add it. Its match is how testers (a header) or an abTest group (a cookie) " +
					"reach it; an abTest needs a cookie match.",
				"properties": map[string]any{
					"name":      stringProperty("Backend name (DNS label)"),
					"component": stringProperty("Optional: component that serves a new backend"),
					"endpoint":  stringProperty("Optional: workload endpoint on that component"),
					"match":     endpointMatchProperty(),
				},
				"required": []string{"name"},
			},
			"steps": map[string]any{
				"type": "array",
				"description": fmt.Sprintf("Optional, canary and mirror: the candidate's percent at each step "+
					"(default %v)", defaultRolloutSteps),
				"items": map[string]any{"type": "integer"},
			},
			"dry_run": dryRunProperty(),
		}, []string{"namespace_name", "name", "strategy", "stable", "candidate"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string             `json:"namespace_name"`
		Name          string             `json:"name"`
		Strategy      string             `json:"strategy"`
		Stable        string             `json:"stable"`
		Candidate     endpointBackendArg `json:"candidate"`
		Steps         []int32            `json:"steps"`
		DryRun        bool               `json:"dry_run"`
	}) (*mcp.CallToolResult, any, error) {
		body, err := rolloutStartRequest(args.Strategy, args.Stable, args.Candidate, args.Steps)
		if err != nil {
			return nil, nil, err
		}
		body.DryRun = &args.DryRun
		result, err := t.EndpointToolset.StartEndpointRollout(ctx, args.NamespaceName, args.Name, body)
		return handleToolResult(result, err)
	})
}

// rolloutStartRequest builds the plan for one candidate: weight 0, a mirror
// for a mirror rollout, and the same percent for it at each step.
func rolloutStartRequest(
	strategy, stable string, candidate endpointBackendArg, percents []int32,
) (*gen.EndpointRolloutStartRequest, error) {
	s := gen.EndpointRolloutStrategy(strategy)
	switch s {
	case gen.EndpointRolloutStrategyCanary, gen.EndpointRolloutStrategyBlueGreen,
		gen.EndpointRolloutStrategyAbTest, gen.EndpointRolloutStrategyMirror:
	default:
		return nil, fmt.Errorf("strategy %q must be one of: canary, blueGreen, abTest, mirror", strategy)
	}
	if candidate.Name == "" {
		return nil, fmt.Errorf("candidate.name is required")
	}
	if (candidate.Component == "") != (candidate.Endpoint == "") {
		return nil, fmt.Errorf("give both candidate.component and candidate.endpoint to add a backend, or neither")
	}
	if s == gen.EndpointRolloutStrategyAbTest && (candidate.Match == nil || candidate.Match.Cookies == nil) {
		return nil, fmt.Errorf("an abTest needs candidate.match.cookies: the group that gets the candidate")
	}

	zero := int32(0)
	candidate.Weight = &zero
	candidate.Mirror = nil
	if s == gen.EndpointRolloutStrategyMirror {
		candidate.Match, candidate.Mirror = nil, &gen.EndpointMirror{Percent: 0}
	}
	ref := toEndpointBackendRefs([]endpointBackendArg{candidate})[0]
	if candidate.Component == "" {
		ref.Component = nil
	}

	ro := gen.EndpointRollout{Strategy: s, Stable: stable}
	ro.Candidates = append(ro.Candidates, struct {
		Name string `json:"name"`
	}{Name: candidate.Name})
	if s == gen.EndpointRolloutStrategyCanary || s == gen.EndpointRolloutStrategyMirror {
		if len(percents) == 0 {
			percents = defaultRolloutSteps
		}
		steps := &gen.EndpointRolloutSteps{}
		for _, p := range percents {
			steps.Steps = append(steps.Steps, gen.EndpointRolloutStep{Backends: []struct {
				Name    string `json:"name"`
				Percent int32  `json:"percent"`
			}{{Name: candidate.Name, Percent: p}}})
		}
		if s == gen.EndpointRolloutStrategyCanary {
			ro.Canary = steps
		} else {
			ro.Mirror = steps
		}
	}
	backends := []gen.EndpointBackendRef{ref}
	return &gen.EndpointRolloutStartRequest{Rollout: ro, Backends: &backends}, nil
}

func (t *Toolsets) RegisterStepEndpointRollout(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "step_endpoint_rollout"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateEndpointReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Move one environment of an endpoint's rollout to its next step, or to step 'to'. It " +
			"writes the step's weights on that environment's EndpointReleaseBinding, with no new release. " +
			"Pass expected_step, the step get_endpoint_rollout reported, so a retried call can't skip a step.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Endpoint name. Use list_endpoints to discover valid names"),
			"environment":    rolloutEnvironmentProperty(),
			"to": intProperty("Optional: the step to move to instead of the next one; " +
				"0 takes the rollout's weighted traffic back to stable"),
			"expected_step": intProperty("Optional: refuse unless the environment is at this step"),
			"dry_run":       dryRunProperty(),
		}, []string{"namespace_name", "name", "environment"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
		Environment   string `json:"environment"`
		To            *int   `json:"to"`
		ExpectedStep  *int   `json:"expected_step"`
		DryRun        bool   `json:"dry_run"`
	}) (*mcp.CallToolResult, any, error) {
		body := &gen.EndpointRolloutStepRequest{
			Environment:  args.Environment,
			To:           args.To,
			ExpectedStep: args.ExpectedStep,
			DryRun:       &args.DryRun,
		}
		result, err := t.EndpointToolset.StepEndpointRollout(ctx, args.NamespaceName, args.Name, body)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterAbortEndpointRollout(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "abort_endpoint_rollout"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateEndpointReleaseBinding}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "Move one environment back to step 0: weighted traffic goes to the stable backend, " +
			"while requests a candidate's match selects (testers' header, preview path, A/B cookie) still " +
			"reach it. The plan stays, so the rollout can be stepped again.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Endpoint name. Use list_endpoints to discover valid names"),
			"environment":    rolloutEnvironmentProperty(),
			"dry_run":        dryRunProperty(),
		}, []string{"namespace_name", "name", "environment"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
		Environment   string `json:"environment"`
		DryRun        bool   `json:"dry_run"`
	}) (*mcp.CallToolResult, any, error) {
		body := &gen.EndpointRolloutAbortRequest{Environment: args.Environment, DryRun: &args.DryRun}
		result, err := t.EndpointToolset.AbortEndpointRollout(ctx, args.NamespaceName, args.Name, body)
		return handleToolResult(result, err)
	})
}

func (t *Toolsets) RegisterFinishEndpointRollout(s *mcp.Server, perms map[string]ToolPermission) {
	const name = "finish_endpoint_rollout"
	perms[name] = ToolPermission{ToolName: name, Action: authzcore.ActionUpdateEndpoint}
	mcp.AddTool(s, &mcp.Tool{
		Name: name,
		Description: "End an endpoint's rollout keeping one backend: the stable one to drop the candidate, " +
			"or the candidate to replace stable. The other backend and spec.rollout are removed, which cuts " +
			"a new EndpointRelease. Refused while the backend leaving still gets weighted traffic somewhere: " +
			"step or abort those environments first. An A/B candidate may leave while its cookie's requests reach it.",
		InputSchema: createSchema(map[string]any{
			"namespace_name": defaultStringProperty(),
			"name":           stringProperty("Endpoint name. Use list_endpoints to discover valid names"),
			"keep":           stringProperty("Name of the backend to keep"),
			"dry_run":        dryRunProperty(),
		}, []string{"namespace_name", "name", "keep"}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		NamespaceName string `json:"namespace_name"`
		Name          string `json:"name"`
		Keep          string `json:"keep"`
		DryRun        bool   `json:"dry_run"`
	}) (*mcp.CallToolResult, any, error) {
		body := &gen.EndpointRolloutFinishRequest{Keep: args.Keep, DryRun: &args.DryRun}
		result, err := t.EndpointToolset.FinishEndpointRollout(ctx, args.NamespaceName, args.Name, body)
		return handleToolResult(result, err)
	})
}

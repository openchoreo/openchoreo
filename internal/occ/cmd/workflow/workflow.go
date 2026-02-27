// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/tidwall/sjson"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// Workflow implements workflow operations
type Workflow struct{}

// New creates a new workflow implementation
func New() *Workflow {
	return &Workflow{}
}

// componentWorkflowAnnotation is the annotation key that identifies component workflows.
const componentWorkflowAnnotation = "openchoreo.dev/component-workflow-parameters"

// List lists all workflows in a namespace, excluding component workflows.
func (w *Workflow) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceWorkflow, params); err != nil {
		return err
	}

	items, err := fetchWorkflows(params.Namespace)
	if err != nil {
		return err
	}

	// Exclude component workflows (those with the component-workflow-parameters annotation)
	filtered := filterWorkflows(items, false)
	return printList(filtered)
}

// fetchWorkflows fetches all workflows from a namespace.
func fetchWorkflows(namespace string) ([]gen.Workflow, error) {
	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	return pagination.FetchAll(func(limit int, cursor string) ([]gen.Workflow, string, error) {
		p := &gen.ListWorkflowsParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := c.ListWorkflows(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		next := ""
		if result.Pagination.NextCursor != nil {
			next = *result.Pagination.NextCursor
		}
		return result.Items, next, nil
	})
}

// filterWorkflows filters workflows based on the component-workflow-parameters annotation.
// If componentOnly is true, returns only workflows with the annotation.
// If componentOnly is false, returns only workflows without the annotation.
func filterWorkflows(items []gen.Workflow, componentOnly bool) []gen.Workflow {
	var filtered []gen.Workflow
	for _, wf := range items {
		hasAnnotation := hasComponentWorkflowAnnotation(wf)
		if hasAnnotation == componentOnly {
			filtered = append(filtered, wf)
		}
	}
	return filtered
}

// hasComponentWorkflowAnnotation checks if a workflow has the component-workflow-parameters annotation.
func hasComponentWorkflowAnnotation(wf gen.Workflow) bool {
	if wf.Metadata.Annotations == nil {
		return false
	}
	_, ok := (*wf.Metadata.Annotations)[componentWorkflowAnnotation]
	return ok
}

// Get retrieves a single workflow and outputs it as YAML
func (w *Workflow) Get(params GetParams) error {
	if params.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if params.WorkflowName == "" {
		return fmt.Errorf("workflow name is required")
	}

	ctx := context.Background()
	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.GetWorkflow(ctx, params.Namespace, params.WorkflowName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// StartRun starts a workflow run
func (w *Workflow) StartRun(params StartRunParams) error {
	if params.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if params.WorkflowName == "" {
		return fmt.Errorf("workflow name is required")
	}

	ns := params.Namespace
	runName := params.RunName
	if runName == "" {
		runName = fmt.Sprintf("%s-%d", params.WorkflowName, time.Now().Unix())
	}
	// K8s names are limited to 253 characters
	if len(runName) > 253 {
		runName = runName[:253]
	}
	var baseParams *map[string]interface{}
	if len(params.Parameters) > 0 {
		baseParams = &params.Parameters
	}
	req := gen.WorkflowRun{
		Metadata: gen.ObjectMeta{
			Name:      runName,
			Namespace: &ns,
		},
		Spec: &gen.WorkflowRunSpec{
			Workflow: gen.WorkflowRunConfig{
				Name:       params.WorkflowName,
				Parameters: baseParams,
			},
		},
	}

	req, err := applySetOverrides(req, params.WorkflowName, params.Set)
	if err != nil {
		return err
	}

	ctx := context.Background()
	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	workflowRun, err := c.CreateWorkflowRun(ctx, params.Namespace, req)
	if err != nil {
		return err
	}

	workflowName := ""
	if workflowRun.Spec != nil {
		workflowName = workflowRun.Spec.Workflow.Name
	}
	runNs := ""
	if workflowRun.Metadata.Namespace != nil {
		runNs = *workflowRun.Metadata.Namespace
	}
	fmt.Printf("Successfully started workflow run: %s\n", workflowRun.Metadata.Name)
	fmt.Printf("  Workflow: %s\n", workflowName)
	fmt.Printf("  Namespace: %s\n", runNs)

	return nil
}

// applySetOverrides applies --set values as JSON paths to the full WorkflowRun object.
// The workflow name is enforced and cannot be overridden.
func applySetOverrides(req gen.WorkflowRun, workflowName string, setValues []string) (gen.WorkflowRun, error) {
	if len(setValues) == 0 {
		return req, nil
	}

	reqJSON, err := json.Marshal(req)
	if err != nil {
		return req, fmt.Errorf("failed to marshal request: %w", err)
	}

	jsonStr := string(reqJSON)
	for _, s := range setValues {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			return req, fmt.Errorf("invalid --set format %q, expected key=value", s)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return req, fmt.Errorf("empty key in --set flag")
		}
		jsonStr, err = sjson.SetRaw(jsonStr, key, toJSONLiteral(value))
		if err != nil {
			return req, fmt.Errorf("failed to set value for key %q: %w", key, err)
		}
	}

	var result gen.WorkflowRun
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return req, fmt.Errorf("failed to unmarshal request: %w", err)
	}

	// Enforce workflow name cannot be changed
	if result.Spec != nil {
		result.Spec.Workflow.Name = workflowName
	}

	return result, nil
}

// toJSONLiteral converts a CLI string value to its raw JSON representation.
func toJSONLiteral(s string) string {
	if s == "true" || s == "false" || s == "null" {
		return s
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
		return s
	}
	b, _ := json.Marshal(s)
	return string(b)
}

func printList(items []gen.Workflow) error {
	if len(items) == 0 {
		fmt.Println("No workflows found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, wf := range items {
		age := "<unknown>"
		if wf.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*wf.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			wf.Metadata.Name,
			age,
		)
	}

	return w.Flush()
}

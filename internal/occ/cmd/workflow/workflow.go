// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/setoverride"
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

// workflowScopeAnnotation is the annotation key that identifies workflow scope (component or generic).
const workflowScopeAnnotation = "openchoreo.dev/workflow-scope"

// List lists all workflows in a namespace.
func (w *Workflow) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceWorkflow, params); err != nil {
		return err
	}

	items, err := fetchWorkflows(params.Namespace)
	if err != nil {
		return err
	}

	// Show all workflows (no filtering)
	return printList(items)
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
	var labels *map[string]string
	if len(params.Labels) > 0 {
		labels = &params.Labels
	}
	req := gen.WorkflowRun{
		Metadata: gen.ObjectMeta{
			Name:      runName,
			Namespace: &ns,
			Labels:    labels,
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

	jsonStr, err := setoverride.Apply(string(reqJSON), setValues)
	if err != nil {
		return req, err
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

func printList(items []gen.Workflow) error {
	if len(items) == 0 {
		fmt.Println("No workflows found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tAGE")

	for _, wf := range items {
		age := "<unknown>"
		if wf.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*wf.Metadata.CreationTimestamp)
		}
		workflowType := "generic"
		if isComponentWorkflow(wf) {
			workflowType = "ci"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			wf.Metadata.Name,
			workflowType,
			age,
		)
	}

	return w.Flush()
}

// isComponentWorkflow checks if a workflow is a component workflow by checking the workflow-scope annotation.
func isComponentWorkflow(wf gen.Workflow) bool {
	if wf.Metadata.Annotations == nil {
		return false
	}
	scope, ok := (*wf.Metadata.Annotations)[workflowScopeAnnotation]
	if !ok {
		return false
	}
	return scope == "component"
}

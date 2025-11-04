// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	workflowpipeline "github.com/openchoreo/openchoreo/internal/crd-renderer/workflow-pipeline"
)

// Reconciler reconciles a Workflow object.
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=workflows,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workflows/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workflows/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workflowdefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=components,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componenttypedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=argoproj.io,resources=workflows,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop.
//
// Workflow:
//  1. Fetch the Workflow CR
//  2. Fetch the referenced WorkflowDefinition
//  3. Optionally fetch the Component to get ComponentTypeDefinition reference
//  4. Fetch ComponentTypeDefinition for parameter overrides (if component exists and has one)
//  5. Render the workflow resource using the pipeline
//  6. Apply the rendered resource to the cluster (typically to the build plane namespace)
//  7. Update the Workflow status
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. Fetch the Workflow CR
	workflow := &openchoreodevv1alpha1.Workflow{}
	if err := r.Get(ctx, req.NamespacedName, workflow); err != nil {
		if errors.IsNotFound(err) {
			// Workflow deleted - nothing to do
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Workflow")
		return ctrl.Result{}, err
	}

	// 2. Fetch the referenced WorkflowDefinition
	workflowDefNamespace := workflow.Namespace
	workflowDef := &openchoreodevv1alpha1.WorkflowDefinition{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      workflow.Spec.WorkflowDefinitionRef,
		Namespace: workflowDefNamespace,
	}, workflowDef); err != nil {
		logger.Error(err, "Failed to get WorkflowDefinition",
			"name", workflow.Spec.WorkflowDefinitionRef,
			"namespace", workflowDefNamespace)
		return r.updateStatus(ctx, workflow, "Error",
			fmt.Sprintf("Failed to get WorkflowDefinition: %v", err))
	}

	// 3. Optionally fetch the Component to get ComponentTypeDefinition reference
	// This is optional - workflows can work without a Component
	var componentTypeDef *openchoreodevv1alpha1.ComponentTypeDefinition
	component := &openchoreodevv1alpha1.Component{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      workflow.Spec.Owner.ComponentName,
		Namespace: workflow.Namespace,
	}, component)

	if err != nil {
		if !errors.IsNotFound(err) {
			// Non-NotFound errors should be logged but not fail the reconciliation
			logger.Info("Failed to get Component, proceeding without ComponentTypeDefinition",
				"name", workflow.Spec.Owner.ComponentName,
				"namespace", workflow.Namespace,
				"error", err.Error())
		}
		// Component not found or error - proceed without ComponentTypeDefinition
		component = nil
	}

	// 4. Fetch ComponentTypeDefinition for parameter overrides (if component exists and has one)
	if component != nil && component.Spec.ComponentType != "" {
		// Parse componentType format: {workloadType}/{componentTypeDefinitionName}
		// Example: "deployment/service"
		componentTypeDefName, err := parseComponentTypeDefinitionName(component.Spec.ComponentType)
		if err != nil {
			logger.Error(err, "Failed to parse component type",
				"componentType", component.Spec.ComponentType)
			return r.updateStatus(ctx, workflow, "Error",
				fmt.Sprintf("Failed to parse component type: %v", err))
		}

		componentTypeDef = &openchoreodevv1alpha1.ComponentTypeDefinition{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      componentTypeDefName,
			Namespace: workflow.Namespace,
		}, componentTypeDef); err != nil {
			if errors.IsNotFound(err) {
				logger.Info("ComponentTypeDefinition not found, proceeding without overrides",
					"name", componentTypeDefName)
				componentTypeDef = nil
			} else {
				logger.Error(err, "Failed to get ComponentTypeDefinition",
					"name", componentTypeDefName,
					"namespace", workflow.Namespace)
				return r.updateStatus(ctx, workflow, "Error",
					fmt.Sprintf("Failed to get ComponentTypeDefinition: %v", err))
			}
		}
	}

	// 5. Build context and render the workflow resource
	pipeline := workflowpipeline.NewPipeline()
	renderInput := &workflowpipeline.RenderInput{
		Workflow:                workflow,
		WorkflowDefinition:      workflowDef,
		ComponentTypeDefinition: componentTypeDef,
		Context: workflowpipeline.WorkflowContext{
			OrgName:       workflow.Namespace, // Use namespace as org name
			ProjectName:   workflow.Spec.Owner.ProjectName,
			ComponentName: workflow.Spec.Owner.ComponentName,
			WorkflowName:  workflow.Name,
			// Timestamp and UUID will be auto-generated by pipeline
		},
	}

	output, err := pipeline.Render(renderInput)
	if err != nil {
		logger.Error(err, "Failed to render workflow resource")
		return r.updateStatus(ctx, workflow, "Error",
			fmt.Sprintf("Failed to render workflow: %v", err))
	}

	// Generate YAML for verification and write to test.yaml file (for testing only)
	renderedYAML, yamlErr := r.generateYAMLFromResource(output.Resource)
	if yamlErr != nil {
		logger.Error(yamlErr, "Failed to generate YAML from rendered resource (non-fatal)")
	} else {
		logger.Info("Successfully rendered workflow resource",
			"workflow", workflow.Name,
			"namespace", workflow.Namespace,
			"yaml", "\n"+renderedYAML)

		// Write to test.yaml file for easy testing
		if writeErr := r.writeYAMLToFile(renderedYAML, workflow.Name); writeErr != nil {
			logger.Error(writeErr, "Failed to write YAML to test file (non-fatal)")
		} else {
			logger.Info("Wrote rendered YAML to test.yaml file",
				"workflow", workflow.Name)
		}
	}

	// 6. Apply the rendered resource directly to the cluster
	renderedResource := output.Resource
	if err := r.applyRenderedResource(ctx, workflow, renderedResource); err != nil {
		logger.Error(err, "Failed to apply rendered resource")
		return r.updateStatus(ctx, workflow, "Error",
			fmt.Sprintf("Failed to apply resource: %v", err))
	}

	return r.updateStatus(ctx, workflow, "Running",
		"Workflow resource has been created")
}

// applyRenderedResource applies the rendered workflow resource to the cluster.
func (r *Reconciler) applyRenderedResource(
	ctx context.Context,
	workflow *openchoreodevv1alpha1.Workflow,
	resource map[string]any,
) error {
	logger := log.FromContext(ctx)

	// Convert all Argo Workflow parameter values to strings
	// Argo Workflows require all parameter values to be strings
	resource = convertParameterValuesToStrings(resource)

	// Convert map to unstructured for Kubernetes API
	unstructuredResource := &unstructured.Unstructured{
		Object: resource,
	}

	// Only set owner reference if in the same namespace
	// Cross-namespace owner references are not allowed in Kubernetes
	resourceNamespace := unstructuredResource.GetNamespace()
	if resourceNamespace == workflow.Namespace || resourceNamespace == "" {
		// Set owner reference to the Workflow CR for garbage collection
		if err := ctrl.SetControllerReference(workflow, unstructuredResource, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}
	} else {
		// For cross-namespace resources, add labels for tracking instead
		labels := unstructuredResource.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels["openchoreo.dev/workflow"] = workflow.Name
		labels["openchoreo.dev/workflow-namespace"] = workflow.Namespace
		unstructuredResource.SetLabels(labels)

		logger.Info("Skipping owner reference for cross-namespace resource, added tracking labels instead",
			"workflow-namespace", workflow.Namespace,
			"resource-namespace", resourceNamespace)
	}

	// Check if resource already exists
	existingResource := &unstructured.Unstructured{}
	existingResource.SetGroupVersionKind(unstructuredResource.GroupVersionKind())

	namespace := unstructuredResource.GetNamespace()
	name := unstructuredResource.GetName()

	err := r.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, existingResource)

	if err != nil {
		if errors.IsNotFound(err) {
			// Resource doesn't exist - create it
			logger.Info("Creating rendered resource",
				"kind", unstructuredResource.GetKind(),
				"name", name,
				"namespace", namespace)
			if err := r.Create(ctx, unstructuredResource); err != nil {
				return fmt.Errorf("failed to create resource: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to get existing resource: %w", err)
	}

	// Resource exists - update it
	logger.Info("Updating rendered resource",
		"kind", unstructuredResource.GetKind(),
		"name", name,
		"namespace", namespace)

	// Preserve resourceVersion for update
	unstructuredResource.SetResourceVersion(existingResource.GetResourceVersion())

	if err := r.Update(ctx, unstructuredResource); err != nil {
		return fmt.Errorf("failed to update resource: %w", err)
	}

	return nil
}

// updateStatus updates the Workflow status.
func (r *Reconciler) updateStatus(
	ctx context.Context,
	workflow *openchoreodevv1alpha1.Workflow,
	phase string,
	message string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	workflow.Status.Phase = phase
	workflow.Status.Message = message

	if err := r.Status().Update(ctx, workflow); err != nil {
		logger.Error(err, "Failed to update Workflow status")
		return ctrl.Result{}, err
	}

	logger.Info("Updated Workflow status",
		"phase", phase,
		"message", message)

	return ctrl.Result{}, nil
}

// parseComponentTypeDefinitionName extracts the ComponentTypeDefinition name from the componentType string.
// Format: {workloadType}/{componentTypeDefinitionName}
// Example: "deployment/service" -> "service"
func parseComponentTypeDefinitionName(componentType string) (string, error) {
	parts := strings.Split(componentType, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid componentType format: expected 'workloadType/name', got '%s'", componentType)
	}
	return parts[1], nil
}

// generateYAMLFromResource converts a resource map to YAML string for logging.
func (r *Reconciler) generateYAMLFromResource(resource map[string]any) (string, error) {
	yamlBytes, err := yaml.Marshal(resource)
	if err != nil {
		return "", fmt.Errorf("failed to marshal resource to YAML: %w", err)
	}
	return string(yamlBytes), nil
}

// writeYAMLToFile writes the rendered YAML to a test.yaml file in the workflow controller directory.
func (r *Reconciler) writeYAMLToFile(yamlContent string, workflowName string) error {
	// Get the current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Construct the path to the workflow controller directory
	// This assumes the controller is running from the project root
	controllerDir := filepath.Join(cwd, "internal", "controller", "workflow")

	// Create the directory if it doesn't exist
	if err := os.MkdirAll(controllerDir, 0755); err != nil {
		return fmt.Errorf("failed to create controller directory: %w", err)
	}

	// Post-process YAML to convert block-style arrays to flow style
	processedYAML := convertBlockArraysToFlowStyle(yamlContent)

	// Write the YAML content to test.yaml
	testFilePath := filepath.Join(controllerDir, "test.yaml")
	if err := os.WriteFile(testFilePath, []byte(processedYAML), 0644); err != nil {
		return fmt.Errorf("failed to write test.yaml: %w", err)
	}

	return nil
}

// convertBlockArraysToFlowStyle converts YAML block-style arrays under "value:" to flow style.
// Example transformation:
//
//	value:          =>    value: [item1, item2, item3]
//	- item1
//	- item2
//	- item3
func convertBlockArraysToFlowStyle(yamlContent string) string {
	lines := strings.Split(yamlContent, "\n")
	var result []string
	i := 0

	for i < len(lines) {
		line := lines[i]

		// Check if this line contains "value:" and the next line starts with "-"
		trimmed := strings.TrimSpace(line)
		if strings.HasSuffix(trimmed, "value:") {
			// Check if next line is a block array item
			if i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "- ") {
				// Collect all array items
				var items []string
				baseIndent := getIndent(line)
				arrayIndent := getIndent(lines[i+1])

				j := i + 1
				for j < len(lines) {
					itemLine := lines[j]
					if getIndent(itemLine) == arrayIndent && strings.HasPrefix(strings.TrimSpace(itemLine), "- ") {
						// Extract the item value
						item := strings.TrimSpace(itemLine[strings.Index(itemLine, "-")+1:])
						items = append(items, item)
						j++
					} else {
						break
					}
				}

				// Format as flow style
				flowArray := formatFlowStyleArray(items)
				result = append(result, fmt.Sprintf("%svalue: %s", baseIndent, flowArray))
				i = j
				continue
			}
		}

		result = append(result, line)
		i++
	}

	return strings.Join(result, "\n")
}

// getIndent returns the leading whitespace of a line.
func getIndent(line string) string {
	for i, ch := range line {
		if ch != ' ' && ch != '\t' {
			return line[:i]
		}
	}
	return line
}

// formatFlowStyleArray formats an array of items as a flow-style YAML array.
func formatFlowStyleArray(items []string) string {
	if len(items) == 0 {
		return "[]"
	}

	var formattedItems []string
	for _, item := range items {
		// Determine if item needs quoting
		if needsQuoting(item) {
			formattedItems = append(formattedItems, fmt.Sprintf("\"%s\"", escapeString(item)))
		} else {
			formattedItems = append(formattedItems, item)
		}
	}

	return "[" + strings.Join(formattedItems, ", ") + "]"
}

// needsQuoting determines if a string value needs to be quoted in YAML.
func needsQuoting(s string) bool {
	// If it's already a number or boolean, don't quote
	if s == "true" || s == "false" {
		return false
	}
	if _, err := fmt.Sscanf(s, "%f", new(float64)); err == nil {
		return false
	}

	// If it contains special characters or spaces, it needs quoting
	return strings.ContainsAny(s, " -/:")
}

// escapeString escapes special characters for YAML string values.
func escapeString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

// convertParameterValuesToStrings converts all Argo Workflow parameter values to strings.
// Argo Workflows require all parameter values to be strings, regardless of their original type.
// This function traverses the resource and converts:
// - integers/floats to string (e.g., 42 -> "42")
// - booleans to string (e.g., true -> "true")
// - arrays to JSON string (e.g., [1,2,3] -> "[1,2,3]")
// - objects to JSON string
func convertParameterValuesToStrings(resource map[string]any) map[string]any {
	result := make(map[string]any)

	for key, value := range resource {
		if key == "spec" {
			// Navigate into spec.arguments.parameters
			if spec, ok := value.(map[string]any); ok {
				result[key] = convertSpecParametersToStrings(spec)
			} else {
				result[key] = value
			}
		} else {
			result[key] = value
		}
	}

	return result
}

// convertSpecParametersToStrings converts parameters inside spec.arguments.parameters
func convertSpecParametersToStrings(spec map[string]any) map[string]any {
	result := make(map[string]any)

	for key, value := range spec {
		if key == "arguments" {
			if args, ok := value.(map[string]any); ok {
				result[key] = convertArgumentsParametersToStrings(args)
			} else {
				result[key] = value
			}
		} else {
			result[key] = value
		}
	}

	return result
}

// convertArgumentsParametersToStrings converts parameters inside arguments
func convertArgumentsParametersToStrings(args map[string]any) map[string]any {
	result := make(map[string]any)

	for key, value := range args {
		if key == "parameters" {
			if params, ok := value.([]any); ok {
				result[key] = convertParameterListToStrings(params)
			} else {
				result[key] = value
			}
		} else {
			result[key] = value
		}
	}

	return result
}

// convertParameterListToStrings converts each parameter's value field to string
func convertParameterListToStrings(params []any) []any {
	result := make([]any, len(params))

	for i, param := range params {
		if paramMap, ok := param.(map[string]any); ok {
			convertedParam := make(map[string]any)
			for k, v := range paramMap {
				if k == "value" {
					// Convert value to string
					convertedParam[k] = convertToString(v)
				} else {
					convertedParam[k] = v
				}
			}
			result[i] = convertedParam
		} else {
			result[i] = param
		}
	}

	return result
}

// convertToString converts any value to its string representation
func convertToString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case int, int32, int64:
		return fmt.Sprintf("%d", v)
	case float32, float64:
		return fmt.Sprintf("%v", v)
	case bool:
		return fmt.Sprintf("%t", v)
	case []any, map[string]any:
		// Convert arrays and objects to JSON strings
		if jsonBytes, err := yaml.Marshal(v); err == nil {
			// Convert YAML to JSON-like string
			jsonStr := strings.TrimSpace(string(jsonBytes))
			// For arrays, convert to flow style
			if _, isArray := v.([]any); isArray {
				return convertYAMLArrayToJSONString(jsonStr)
			}
			return jsonStr
		}
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// convertYAMLArrayToJSONString converts YAML block array to JSON array string
func convertYAMLArrayToJSONString(yamlArray string) string {
	lines := strings.Split(yamlArray, "\n")
	var items []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimSpace(trimmed[2:])
			items = append(items, item)
		}
	}

	if len(items) == 0 {
		return "[]"
	}

	// Format as JSON array
	var formattedItems []string
	for _, item := range items {
		// Check if item needs quotes
		if needsQuoting(item) {
			formattedItems = append(formattedItems, fmt.Sprintf("\"%s\"", escapeString(item)))
		} else {
			formattedItems = append(formattedItems, item)
		}
	}

	return "[" + strings.Join(formattedItems, ",") + "]"
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreodevv1alpha1.Workflow{}).
		Named("workflow").
		Complete(r)
}

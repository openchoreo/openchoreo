// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package argo

import (
	"context"
	"fmt"
	"github.com/openchoreo/openchoreo/internal/controller/build/engines"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller/build/utils"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
)

// Engine implements BuildEngine interface for Argo Workflows
type Engine struct {
	logger logr.Logger
}

// NewEngine creates a new Argo build engine
func NewEngine() *Engine {
	return &Engine{
		logger: log.Log.WithName("argo-build-engine"),
	}
}

// GetName returns the name of the build engine
func (e *Engine) GetName() string {
	return "argo"
}

// EnsurePrerequisites creates namespace, service account, role, and role binding
func (e *Engine) EnsurePrerequisites(ctx context.Context, client client.Client, build *openchoreov1alpha1.Build) error {
	logger := e.logger.WithValues("build", build.Name)

	// Create namespace
	namespace := utils.MakeNamespace(build)
	if err := engines.EnsureResource(ctx, client, namespace, "Namespace", logger); err != nil {
		return fmt.Errorf("failed to ensure namespace: %w", err)
	}

	// Create service account
	serviceAccount := e.makeServiceAccount(build)
	if err := engines.EnsureResource(ctx, client, serviceAccount, "ServiceAccount", logger); err != nil {
		return fmt.Errorf("failed to ensure service account: %w", err)
	}

	// Create role
	role := e.makeRole(build)
	if err := engines.EnsureResource(ctx, client, role, "Role", logger); err != nil {
		return fmt.Errorf("failed to ensure role: %w", err)
	}

	// Create role binding
	roleBinding := e.makeRoleBinding(build)
	if err := engines.EnsureResource(ctx, client, roleBinding, "RoleBinding", logger); err != nil {
		return fmt.Errorf("failed to ensure role binding: %w", err)
	}

	return nil
}

// CreateBuild creates an Argo Workflow for the build
func (e *Engine) CreateBuild(ctx context.Context, client client.Client, build *openchoreov1alpha1.Build) (engines.BuildCreationResponse, error) {
	workflow := e.makeArgoWorkflow(build)

	err := client.Create(ctx, workflow)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return engines.BuildCreationResponse{
				ID:      workflow.Name,
				Created: false,
			}, nil
		}
		return engines.BuildCreationResponse{}, fmt.Errorf("failed to create workflow: %w", err)
	}

	return engines.BuildCreationResponse{
		ID:      workflow.Name,
		Created: true,
	}, nil
}

// GetBuildStatus retrieves the current status of the Argo Workflow
func (e *Engine) GetBuildStatus(ctx context.Context, client client.Client, build *openchoreov1alpha1.Build) (engines.BuildStatus, error) {
	workflow := &argoproj.Workflow{}
	err := client.Get(ctx,
		types.NamespacedName{
			Name:      utils.MakeWorkflowName(build),
			Namespace: utils.MakeNamespaceName(build),
		},
		workflow,
	)
	if err != nil {
		return engines.BuildStatus{}, fmt.Errorf("failed to get workflow: %w", err)
	}

	return engines.BuildStatus{
		Phase:   e.convertArgoPhase(workflow.Status.Phase),
		Message: workflow.Status.Message,
	}, nil
}

// ExtractBuildArtifacts extracts image and workload CR from completed workflow
func (e *Engine) ExtractBuildArtifacts(ctx context.Context, client client.Client, build *openchoreov1alpha1.Build) (*engines.BuildArtifacts, error) {
	workflow := &argoproj.Workflow{}
	err := client.Get(ctx,
		types.NamespacedName{
			Name:      utils.MakeWorkflowName(build),
			Namespace: utils.MakeNamespaceName(build),
		},
		workflow,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	artifacts := &engines.BuildArtifacts{}

	// Extract image from push-step
	if pushStep := e.getStepByTemplateName(workflow.Status.Nodes, engines.StepPush); pushStep != nil {
		if image := e.getImageNameFromWorkflow(*pushStep.Outputs); image != "" {
			artifacts.Image = string(image)
		}
	}

	// Extract workload CR from workload-create-step
	if workloadStep := e.getStepByTemplateName(workflow.Status.Nodes, engines.StepWorkloadCreate); workloadStep != nil {
		if workloadStep.Phase == argoproj.NodeSucceeded {
			artifacts.WorkloadCR = e.getWorkloadCRFromWorkflow(*workloadStep.Outputs)
		}
	}

	return artifacts, nil
}

// convertArgoPhase converts Argo workflow phase to generic build phase
func (e *Engine) convertArgoPhase(phase argoproj.WorkflowPhase) engines.BuildPhase {
	switch phase {
	case argoproj.WorkflowRunning:
		return engines.BuildPhaseRunning
	case argoproj.WorkflowSucceeded:
		return engines.BuildPhaseSucceeded
	case argoproj.WorkflowFailed, argoproj.WorkflowError:
		return engines.BuildPhaseFailed
	default:
		return engines.BuildPhaseUnknown
	}
}

func (e *Engine) getStepByTemplateName(nodes argoproj.Nodes, step string) *argoproj.NodeStatus {
	for _, node := range nodes {
		if node.TemplateName == step {
			return &node
		}
	}
	return nil
}

func (e *Engine) getImageNameFromWorkflow(output argoproj.Outputs) argoproj.AnyString {
	for _, param := range output.Parameters {
		if param.Name == "image" {
			return *param.Value
		}
	}
	return ""
}

func (e *Engine) getWorkloadCRFromWorkflow(output argoproj.Outputs) string {
	for _, param := range output.Parameters {
		if param.Name == "workload-cr" {
			return string(*param.Value)
		}
	}
	return ""
}

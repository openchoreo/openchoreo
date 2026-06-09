// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kinds

import (
	"fmt"
	"os"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/occ/resources"
	synth "github.com/openchoreo/openchoreo/internal/occ/resources/workload"
)

// WorkloadResource provides operations for Workload CRs.
type WorkloadResource struct {
	*resources.ResourceBase
}

// NewWorkloadResource constructs a WorkloadResource with CRDConfig and optionally sets namespace.
func NewWorkloadResource(cfg resources.CRDConfig, namespace string) (*WorkloadResource, error) {
	options := []resources.ResourceBaseOption{
		resources.WithResourceConfig(cfg),
	}

	if namespace != "" {
		options = append(options, resources.WithResourceNamespace(namespace))
	}

	return &WorkloadResource{
		ResourceBase: resources.NewResourceBase(options...),
	}, nil
}

// CreateWorkload creates a Workload CR from a descriptor file or basic parameters.
func (w *WorkloadResource) CreateWorkload(params synth.CreateWorkloadParams) error {
	if params.NamespaceName == "" {
		return fmt.Errorf("namespace name is required (--namespace)")
	}
	if params.ProjectName == "" {
		return fmt.Errorf("project name is required (--project)")
	}
	if params.ComponentName == "" {
		return fmt.Errorf("component name is required (--component)")
	}
	if params.ImageURL == "" {
		return fmt.Errorf("image URL is required (--image)")
	}

	var workloadCR *openchoreov1alpha1.Workload
	var err error

	if params.FilePath != "" {
		workloadCR, err = synth.ConvertWorkloadDescriptorToWorkloadCR(params.FilePath, params)
		if err != nil {
			return fmt.Errorf("failed to convert workload descriptor: %w", err)
		}
	} else {
		workloadCR, err = synth.CreateBasicWorkload(params)
		if err != nil {
			return fmt.Errorf("failed to create basic workload CR: %w", err)
		}
	}

	yamlBytes, err := synth.ConvertWorkloadCRToYAML(workloadCR)
	if err != nil {
		return fmt.Errorf("failed to convert Workload CR to YAML: %w", err)
	}

	if params.OutputPath != "" {
		if err := os.WriteFile(params.OutputPath, yamlBytes, 0644); err != nil { //nolint:gosec // Generated YAML files are meant to be readable
			return fmt.Errorf("failed to write output file %s: %w", params.OutputPath, err)
		}
		fmt.Printf("Workload CR written to %s\n", params.OutputPath)
	} else {
		fmt.Print(string(yamlBytes))
	}

	return nil
}

// GenerateWorkloadCR generates a Workload CR without writing it (used in file-system mode).
func (w *WorkloadResource) GenerateWorkloadCR(params synth.CreateWorkloadParams) (*openchoreov1alpha1.Workload, error) {
	if params.NamespaceName == "" {
		return nil, fmt.Errorf("namespace name is required (--namespace)")
	}
	if params.ProjectName == "" {
		return nil, fmt.Errorf("project name is required (--project)")
	}
	if params.ComponentName == "" {
		return nil, fmt.Errorf("component name is required (--component)")
	}
	if params.ImageURL == "" {
		return nil, fmt.Errorf("image URL is required (--image)")
	}

	var workloadCR *openchoreov1alpha1.Workload
	var err error

	if params.FilePath != "" {
		workloadCR, err = synth.ConvertWorkloadDescriptorToWorkloadCR(params.FilePath, params)
		if err != nil {
			return nil, fmt.Errorf("failed to convert workload descriptor: %w", err)
		}
	} else {
		workloadCR, err = synth.CreateBasicWorkload(params)
		if err != nil {
			return nil, fmt.Errorf("failed to create basic workload CR: %w", err)
		}
	}

	return workloadCR, nil
}

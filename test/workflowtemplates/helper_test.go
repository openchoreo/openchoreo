// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowtemplates

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// templatesDir is the location of the workflow templates relative to this
// test package.
const templatesDir = "../../samples/getting-started/workflow-templates"

const ciWorkflowsDir = "../../samples/getting-started/ci-workflows"

// workflowTemplate is a minimal view of an Argo ClusterWorkflowTemplate — just
// enough to reach the embedded scripts and the volume/mount wiring.
type workflowTemplate struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Templates []workflowTemplateStep `yaml:"templates"`
	} `yaml:"spec"`
}

type workflowTemplateStep struct {
	Name   string `yaml:"name"`
	Inputs struct {
		Parameters []struct {
			Name    string `yaml:"name"`
			Default string `yaml:"default"`
		} `yaml:"parameters"`
	} `yaml:"inputs"`
	Container struct {
		Image        string   `yaml:"image"`
		Args         []string `yaml:"args"`
		Env          []envVar `yaml:"env"`
		VolumeMounts []struct {
			Name      string `yaml:"name"`
			MountPath string `yaml:"mountPath"`
			ReadOnly  bool   `yaml:"readOnly"`
		} `yaml:"volumeMounts"`
	} `yaml:"container"`
	Volumes []struct {
		Name   string `yaml:"name"`
		Secret *struct {
			SecretName string `yaml:"secretName"`
			Optional   *bool  `yaml:"optional"`
		} `yaml:"secret"`
	} `yaml:"volumes"`
}

type envVar struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type clusterWorkflow struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name   string            `yaml:"name"`
		Labels map[string]string `yaml:"labels"`
	} `yaml:"metadata"`
	Spec struct {
		WorkflowPlaneRef struct {
			Kind string `yaml:"kind"`
			Name string `yaml:"name"`
		} `yaml:"workflowPlaneRef"`
		Parameters struct {
			OpenAPIV3Schema schemaNode `yaml:"openAPIV3Schema"`
		} `yaml:"parameters"`
		RunTemplate struct {
			APIVersion string `yaml:"apiVersion"`
			Kind       string `yaml:"kind"`
			Metadata   struct {
				Name      string `yaml:"name"`
				Namespace string `yaml:"namespace"`
			} `yaml:"metadata"`
			Spec struct {
				ServiceAccountName string `yaml:"serviceAccountName"`
				Entrypoint         string `yaml:"entrypoint"`
				Arguments          arguments
				Templates          []argoTemplate `yaml:"templates"`
			} `yaml:"spec"`
		} `yaml:"runTemplate"`
		ExternalRefs []externalRef      `yaml:"externalRefs"`
		Resources    []workflowResource `yaml:"resources"`
	} `yaml:"spec"`
}

type externalRef struct {
	ID         string `yaml:"id"`
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Name       string `yaml:"name"`
}

type workflowResource struct {
	ID          string `yaml:"id"`
	IncludeWhen string `yaml:"includeWhen"`
	Template    struct {
		Kind     string `yaml:"kind"`
		Metadata struct {
			Name      string `yaml:"name"`
			Namespace string `yaml:"namespace"`
		} `yaml:"metadata"`
	} `yaml:"template"`
}

type schemaNode struct {
	Type       string                `yaml:"type"`
	Default    any                   `yaml:"default"`
	Required   []string              `yaml:"required"`
	Properties map[string]schemaNode `yaml:"properties"`
}

type arguments struct {
	Parameters []parameter `yaml:"parameters"`
}

type parameter struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type argoTemplate struct {
	Name  string       `yaml:"name"`
	Steps [][]argoStep `yaml:"steps"`
}

type argoStep struct {
	Name        string `yaml:"name"`
	TemplateRef struct {
		Name         string `yaml:"name"`
		ClusterScope bool   `yaml:"clusterScope"`
		Template     string `yaml:"template"`
	} `yaml:"templateRef"`
	Arguments arguments `yaml:"arguments"`
}

// loadTemplate parses a template YAML by file name (e.g. "checkout-source.yaml").
func loadTemplate(t *testing.T, filename string) workflowTemplate {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(templatesDir, filename))
	require.NoError(t, err, "reading template %s", filename)

	var wt workflowTemplate
	require.NoError(t, yaml.Unmarshal(data, &wt), "unmarshalling template %s", filename)
	return wt
}

func loadCIWorkflow(t *testing.T, filename string) clusterWorkflow {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(ciWorkflowsDir, filename))
	require.NoError(t, err, "reading CI workflow %s", filename)

	var wf clusterWorkflow
	require.NoError(t, yaml.Unmarshal(data, &wf), "unmarshalling CI workflow %s", filename)
	return wf
}

// scriptForTemplate returns container.args[0] for the named Argo template.
func scriptForTemplate(t *testing.T, filename, templateName string) string {
	t.Helper()
	tmpl := workflowTemplateByName(t, filename, templateName)
	args := tmpl.Container.Args
	require.NotEmpty(t, args, "template %s/%s has no container.args", filename, templateName)
	return args[0]
}

func envForTemplate(t *testing.T, filename, templateName string) []envVar {
	t.Helper()
	tmpl := workflowTemplateByName(t, filename, templateName)
	return tmpl.Container.Env
}

func workflowTemplateByName(t *testing.T, filename, templateName string) workflowTemplateStep {
	t.Helper()
	wt := loadTemplate(t, filename)
	for _, tmpl := range wt.Spec.Templates {
		if tmpl.Name != templateName {
			continue
		}
		return tmpl
	}
	require.Failf(t, "template not found", "template %s not found in %s", templateName, filename)
	return workflowTemplateStep{}
}

func writeExec(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o755))
}

// mountPath returns the mountPath of the named volume mount on the first
// template, or "" if not found.
func mountPath(t *testing.T, filename, volumeName string) string {
	t.Helper()
	wt := loadTemplate(t, filename)
	require.NotEmpty(t, wt.Spec.Templates, "template %s has no spec.templates", filename)
	for _, vm := range wt.Spec.Templates[0].Container.VolumeMounts {
		if vm.Name == volumeName {
			return vm.MountPath
		}
	}
	return ""
}

// inputParamDefault returns the `default` of the named input parameter on the
// first template, or "" if not found.
func inputParamDefault(t *testing.T, filename, paramName string) string {
	t.Helper()
	wt := loadTemplate(t, filename)
	require.NotEmpty(t, wt.Spec.Templates, "template %s has no spec.templates", filename)
	for _, p := range wt.Spec.Templates[0].Inputs.Parameters {
		if p.Name == paramName {
			return p.Default
		}
	}
	return ""
}

// secretVolumeOptional reports whether the named secret volume exists and is
// marked optional. Returns (found, optional).
func secretVolumeOptional(t *testing.T, filename, volumeName string) (bool, bool) {
	t.Helper()
	wt := loadTemplate(t, filename)
	require.NotEmpty(t, wt.Spec.Templates, "template %s has no spec.templates", filename)
	for _, v := range wt.Spec.Templates[0].Volumes {
		if v.Name == volumeName && v.Secret != nil {
			return true, v.Secret.Optional != nil && *v.Secret.Optional
		}
	}
	return false, false
}

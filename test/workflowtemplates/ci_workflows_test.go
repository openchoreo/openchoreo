// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowtemplates

import (
	"fmt"
	"strings"
	"testing"
)

type ciWorkflowContract struct {
	file          string
	metadataName  string
	buildTemplate string
	buildEnvArg   bool
	buildArgsArg  bool
	dockerParams  bool
}

var ciWorkflowContracts = []ciWorkflowContract{
	{
		file:          "dockerfile-builder.yaml",
		metadataName:  "dockerfile-builder",
		buildTemplate: "containerfile-build",
		buildEnvArg:   true,
		buildArgsArg:  true,
		dockerParams:  true,
	},
	{
		file:          "ballerina-buildpack-builder.yaml",
		metadataName:  "ballerina-buildpack-builder",
		buildTemplate: "ballerina-buildpack-build",
		buildEnvArg:   true,
	},
	{
		file:          "gcp-buildpacks-builder.yaml",
		metadataName:  "gcp-buildpacks-builder",
		buildTemplate: "gcp-buildpacks-build",
	},
	{
		file:          "paketo-buildpacks-builder.yaml",
		metadataName:  "paketo-buildpacks-builder",
		buildTemplate: "paketo-buildpacks-build",
	},
}

func TestCIWorkflows_ParseAndShape(t *testing.T) {
	for _, tc := range ciWorkflowContracts {
		t.Run(tc.file, func(t *testing.T) {
			wf := loadCIWorkflow(t, tc.file)

			requireEqualContract(t, wf.APIVersion, "openchoreo.dev/v1alpha1",
				"CI workflow apiVersion must stay on the OpenChoreo workflow API")
			requireEqualContract(t, wf.Kind, "ClusterWorkflow",
				"CI workflow YAML must define a ClusterWorkflow")
			requireEqualContract(t, wf.Metadata.Name, tc.metadataName,
				"CI workflow metadata.name must match the shipped builder")
			requireEqualContract(t, wf.Metadata.Labels["openchoreo.dev/workflow-type"], "component",
				"CI workflow must be advertised as a component workflow")
			requireEqualContract(t, wf.Spec.WorkflowPlaneRef.Kind, "ClusterWorkflowPlane",
				"CI workflow must target a ClusterWorkflowPlane")
			requireEqualContract(t, wf.Spec.WorkflowPlaneRef.Name, "default",
				"getting-started CI workflow must target the default workflow plane")
			requireEqualContract(t, wf.Spec.RunTemplate.APIVersion, "argoproj.io/v1alpha1",
				"CI workflow runTemplate must render an Argo Workflow")
			requireEqualContract(t, wf.Spec.RunTemplate.Kind, "Workflow",
				"CI workflow runTemplate must render an Argo Workflow")
			requireEqualContract(t, wf.Spec.RunTemplate.Spec.ServiceAccountName, "workflow-sa",
				"CI workflow runTemplate must use the workflow service account")
			requireEqualContract(t, wf.Spec.RunTemplate.Spec.Entrypoint, "build-workflow",
				"CI workflow runTemplate must enter the build-workflow DAG")
		})
	}
}

func TestCIWorkflows_RepositoryParametersFeedCheckout(t *testing.T) {
	for _, tc := range ciWorkflowContracts {
		t.Run(tc.file, func(t *testing.T) {
			wf := loadCIWorkflow(t, tc.file)

			repository := requireSchemaProperty(t, wf.Spec.Parameters.OpenAPIV3Schema, "repository",
				"CI workflow schema must expose repository configuration")
			requireRequired(t, wf.Spec.Parameters.OpenAPIV3Schema, "repository",
				"CI workflow parameters must require repository")
			requireRequired(t, repository, "url",
				"repository schema must require url so checkout always receives a clone URL")

			revision := requireSchemaProperty(t, repository, "revision",
				"repository schema must expose revision configuration")
			branch := requireSchemaProperty(t, revision, "branch",
				"repository revision schema must expose branch")
			commit := requireSchemaProperty(t, revision, "commit",
				"repository revision schema must expose commit")
			requireEqualContract(t, fmt.Sprint(branch.Default), "main",
				"repository branch must default to main")
			requireEqualContract(t, fmt.Sprint(commit.Default), "",
				"repository commit must default to empty so branch checkout is the default")

			args := wf.Spec.RunTemplate.Spec.Arguments
			requireParameterValueParts(t, args, "git-repo", []string{"parameters", "repository", "url"},
				"checkout-source must receive git-repo from repository.url")
			requireParameterValueParts(t, args, "branch", []string{"parameters", "repository", "revision", "branch"},
				"checkout-source must receive branch from repository.revision.branch")
			requireParameterValueParts(t, args, "commit", []string{"parameters", "repository", "revision", "commit"},
				"checkout-source must receive commit from repository.revision.commit")
			requireParameterValueParts(t, args, "app-path", []string{"parameters", "repository", "appPath"},
				"build and workload generation must receive app-path from repository.appPath")
			requireParameterValueParts(t, args, "git-secret", []string{"metadata", "workflowRunName", "git-secret"},
				"checkout-source must receive the generated git secret name")
		})
	}
}

func TestCIWorkflows_RunTemplateStepHandoffs(t *testing.T) {
	for _, tc := range ciWorkflowContracts {
		t.Run(tc.file, func(t *testing.T) {
			wf := loadCIWorkflow(t, tc.file)
			steps := requireBuildWorkflowSteps(t, wf)

			checkout := requireStep(t, steps, "checkout-source")
			requireTemplateRef(t, checkout, "checkout-source", "checkout",
				"CI workflow must call checkout-source ClusterWorkflowTemplate first")

			build := requireStep(t, steps, "build-image")
			requireTemplateRef(t, build, tc.buildTemplate, "build-image",
				"CI workflow build step must call the expected build ClusterWorkflowTemplate")
			requireParameterValue(t, build.Arguments, "git-revision", "{{steps.checkout-source.outputs.parameters.git-revision}}",
				"build step must receive git-revision from checkout-source output")
			if tc.buildEnvArg {
				requireParameterValue(t, build.Arguments, "build-env", "{{workflow.parameters.build-env}}",
					"build step must receive build-env workflow parameter")
			}
			if tc.buildArgsArg {
				requireParameterValue(t, build.Arguments, "build-args", "{{workflow.parameters.build-args}}",
					"containerfile build step must receive build-args workflow parameter")
			}

			publish := requireStep(t, steps, "publish-image")
			requireTemplateRef(t, publish, "publish-image", "publish-image",
				"CI workflow publish step must call publish-image ClusterWorkflowTemplate")
			requireParameterValue(t, publish.Arguments, "git-revision", "{{steps.checkout-source.outputs.parameters.git-revision}}",
				"publish step must receive git-revision from checkout-source output")

			workload := requireStep(t, steps, "generate-workload-cr")
			requireTemplateRef(t, workload, "generate-workload", "generate-workload-cr",
				"CI workflow workload step must call generate-workload ClusterWorkflowTemplate")
			requireParameterValue(t, workload.Arguments, "image", "{{steps.publish-image.outputs.parameters.image}}",
				"generate-workload step must receive the image published by publish-image")
			requireParameterValue(t, workload.Arguments, "run-name", "{{workflow.parameters.workflowrun-name}}",
				"generate-workload step must receive the WorkflowRun name for annotations")
		})
	}
}

func TestCIWorkflows_BuildParameters(t *testing.T) {
	for _, tc := range ciWorkflowContracts {
		t.Run(tc.file, func(t *testing.T) {
			wf := loadCIWorkflow(t, tc.file)
			args := wf.Spec.RunTemplate.Spec.Arguments

			requireParameterValueParts(t, args, "build-env", []string{"parameters", "buildEnv"},
				"CI workflow must expose buildEnv as workflow parameter build-env")
			requireParameterValueParts(t, args, "image-name", []string{"metadata", "namespaceName", "openchoreo.dev/project", "openchoreo.dev/component"},
				"CI workflow must derive image-name from namespace/project/component")
			requireParameterValue(t, args, "image-tag", "v1",
				"CI workflow must provide a stable image-tag parameter")
			requireParameterValueParts(t, args, "registry-push-secret", []string{"metadata", "workflowRunName", "registry-push-secret"},
				"publish-image must receive the generated registry push secret name")

			if tc.dockerParams {
				requireParameterValueParts(t, args, "docker-context", []string{"parameters", "docker", "context"},
					"dockerfile CI workflow must pass docker.context to containerfile-build")
				requireParameterValueParts(t, args, "dockerfile-path", []string{"parameters", "docker", "filePath"},
					"dockerfile CI workflow must pass docker.filePath to containerfile-build")
				requireParameterValueParts(t, args, "build-args", []string{"parameters", "buildArgs"},
					"dockerfile CI workflow must expose buildArgs as workflow parameter build-args")
			}
		})
	}
}

func TestCIWorkflows_SecretResources(t *testing.T) {
	for _, tc := range ciWorkflowContracts {
		t.Run(tc.file, func(t *testing.T) {
			wf := loadCIWorkflow(t, tc.file)

			gitRef := requireExternalRef(t, wf, "git-secret-reference")
			requireEqualContract(t, gitRef.Kind, "SecretReference",
				"CI workflow git external ref must refer to a SecretReference")
			requireEqualContract(t, gitRef.Name, "${parameters.repository.secretRef}",
				"CI workflow git external ref must use repository.secretRef")

			gitSecret := requireResource(t, wf, "git-secret")
			requireEqualContract(t, gitSecret.IncludeWhen, `${has(parameters.repository.secretRef) && parameters.repository.secretRef != ""}`,
				"git-secret ExternalSecret must only be rendered when repository.secretRef is configured")
			requireEqualContract(t, gitSecret.Template.Kind, "ExternalSecret",
				"git-secret resource must render an ExternalSecret")
			requireEqualContract(t, gitSecret.Template.Metadata.Name, "${metadata.workflowRunName}-git-secret",
				"git-secret resource name must match the git-secret workflow parameter")

			registrySecret := requireResource(t, wf, "registry-push-secret")
			requireEqualContract(t, registrySecret.Template.Kind, "ExternalSecret",
				"registry-push-secret resource must render an ExternalSecret")
			requireEqualContract(t, registrySecret.Template.Metadata.Name, "${metadata.workflowRunName}-registry-push-secret",
				"registry-push-secret resource name must match the registry-push-secret workflow parameter")
		})
	}
}

func requireSchemaProperty(t *testing.T, schema schemaNode, name string, contract string) schemaNode {
	t.Helper()
	prop, ok := schema.Properties[name]
	if ok {
		return prop
	}
	t.Fatalf(`
contract:
  %s

missing schema property:
  %s

available properties:
%s`,
		contract,
		name,
		formatStringList(keys(schema.Properties)),
	)
	return schemaNode{}
}

func requireRequired(t *testing.T, schema schemaNode, name string, contract string) {
	t.Helper()
	for _, r := range schema.Required {
		if r == name {
			return
		}
	}
	t.Fatalf(`
contract:
  %s

missing required field:
  %s

required fields:
%s`,
		contract,
		name,
		formatStringList(schema.Required),
	)
}

func requireParameterValue(t *testing.T, args arguments, name string, want string, contract string) {
	t.Helper()
	for _, p := range args.Parameters {
		if p.Name != name {
			continue
		}
		requireEqualContract(t, p.Value, want, contract)
		return
	}
	t.Fatalf(`
contract:
  %s

missing parameter:
  %s

available parameters:
%s`,
		contract,
		name,
		formatStringList(parameterNames(args.Parameters)),
	)
}

func requireParameterValueParts(t *testing.T, args arguments, name string, parts []string, contract string) {
	t.Helper()
	for _, p := range args.Parameters {
		if p.Name != name {
			continue
		}
		for _, part := range parts {
			if strings.Contains(p.Value, part) {
				continue
			}
			t.Fatalf(`
contract:
  %s

parameter:
  %s

value:
  %s

missing required fragment:
  %s`, contract, name, p.Value, part)
		}
		return
	}
	t.Fatalf(`
contract:
  %s

missing parameter:
  %s

available parameters:
%s`,
		contract,
		name,
		formatStringList(parameterNames(args.Parameters)),
	)
}

func requireBuildWorkflowSteps(t *testing.T, wf clusterWorkflow) []argoStep {
	t.Helper()
	for _, tmpl := range wf.Spec.RunTemplate.Spec.Templates {
		if tmpl.Name != "build-workflow" {
			continue
		}
		var steps []argoStep
		for _, group := range tmpl.Steps {
			steps = append(steps, group...)
		}
		return steps
	}
	t.Fatalf(`
contract:
  CI workflow runTemplate must define build-workflow template

available templates:
%s`,
		formatStringList(templateNames(wf.Spec.RunTemplate.Spec.Templates)),
	)
	return nil
}

func requireStep(t *testing.T, steps []argoStep, name string) argoStep {
	t.Helper()
	for _, step := range steps {
		if step.Name == name {
			return step
		}
	}
	t.Fatalf(`
contract:
  CI workflow build-workflow must define required step

missing step:
  %s

available steps:
%s`,
		name,
		formatStringList(stepNames(steps)),
	)
	return argoStep{}
}

func requireTemplateRef(t *testing.T, step argoStep, wantName string, wantTemplate string, contract string) {
	t.Helper()
	requireEqualContract(t, step.TemplateRef.Name, wantName, contract)
	requireEqualContract(t, step.TemplateRef.Template, wantTemplate, contract)
	requireTrueContract(t, step.TemplateRef.ClusterScope,
		"CI workflow step "+step.Name+" must reference a cluster-scoped workflow template")
}

func requireExternalRef(t *testing.T, wf clusterWorkflow, id string) externalRef {
	t.Helper()
	for _, ref := range wf.Spec.ExternalRefs {
		if ref.ID == id {
			return ref
		}
	}
	t.Fatalf(`
contract:
  CI workflow must define required externalRef

missing externalRef:
  %s`, id)
	return externalRef{}
}

func requireResource(t *testing.T, wf clusterWorkflow, id string) workflowResource {
	t.Helper()
	for _, r := range wf.Spec.Resources {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf(`
contract:
  CI workflow must define required generated resource

missing resource:
  %s`, id)
	return workflowResource{}
}

func parameterNames(params []parameter) []string {
	names := make([]string, 0, len(params))
	for _, p := range params {
		names = append(names, p.Name)
	}
	return names
}

func templateNames(templates []argoTemplate) []string {
	names := make([]string, 0, len(templates))
	for _, tmpl := range templates {
		names = append(names, tmpl.Name)
	}
	return names
}

func stepNames(steps []argoStep) []string {
	names := make([]string, 0, len(steps))
	for _, step := range steps {
		names = append(names, step.Name)
	}
	return names
}

func keys(m map[string]schemaNode) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func formatStringList(values []string) string {
	if len(values) == 0 {
		return "  (none)"
	}
	var b strings.Builder
	for _, value := range values {
		b.WriteString("  - ")
		b.WriteString(value)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

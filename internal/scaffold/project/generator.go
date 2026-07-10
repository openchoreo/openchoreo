// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package project generates OpenChoreo Project YAML from a (Cluster)ProjectType.
//
// The parameter section is rendered with the same required/optional/default
// behavior as the component scaffolder by reusing its schema-to-YAML primitives
// (imported as render).
package project

import (
	"fmt"
	"strings"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	render "github.com/openchoreo/openchoreo/internal/scaffold/component"
)

// Options configures the project scaffolding generator.
type Options struct {
	// ProjectName is the name for the generated Project.
	ProjectName string
	// Namespace is the target namespace for the Project (and its bindings).
	Namespace string
	// ProjectTypeKind is "ProjectType" or "ClusterProjectType".
	ProjectTypeKind string
	// ProjectTypeName is the referenced (Cluster)ProjectType name.
	ProjectTypeName string
	// DeploymentPipeline is the name referenced by spec.deploymentPipelineRef.
	DeploymentPipeline string
	// Environments, when non-empty, produces one ProjectReleaseBinding per entry.
	Environments []string
	// BindingsNote, when set, is rendered as an informational comment on the
	// Project document explaining why no ProjectReleaseBindings were generated.
	// It is emitted regardless of IncludeStructuralComments, since it explains
	// the shape of the output rather than documenting a field.
	BindingsNote string

	// IncludeAllFields includes optional fields (without defaults) as commented examples.
	IncludeAllFields bool
	// IncludeFieldDescriptions includes schema-derived comments.
	IncludeFieldDescriptions bool
	// IncludeStructuralComments includes section headers and guidance comments.
	IncludeStructuralComments bool
}

// Generator renders a Project manifest (and optional ProjectReleaseBindings)
// from a (Cluster)ProjectType parameter schema.
type Generator struct {
	parametersSchema *extv1.JSONSchemaProps
	opts             *Options
	renderer         *render.FieldRenderer
}

// NewGenerator creates a project scaffold generator. parametersSchema may be nil
// (a project type with no parameters), in which case spec.parameters is omitted.
func NewGenerator(parametersSchema *extv1.JSONSchemaProps, opts *Options) *Generator {
	if opts == nil {
		opts = &Options{}
	}
	return &Generator{
		parametersSchema: parametersSchema,
		opts:             opts,
		renderer:         render.NewFieldRenderer(opts.IncludeFieldDescriptions, opts.IncludeAllFields, opts.IncludeStructuralComments),
	}
}

// Generate produces the scaffolded Project YAML, followed by one
// ProjectReleaseBinding document per configured environment.
func (g *Generator) Generate() (string, error) {
	jsonSchema, defaultedObj, err := render.ApplyDefaultsToSchema(g.parametersSchema)
	if err != nil {
		return "", fmt.Errorf("processing project type parameters schema: %w", err)
	}

	docs := make([]string, 0, 1+len(g.opts.Environments))

	projectDoc, err := g.generateProject(jsonSchema, defaultedObj)
	if err != nil {
		return "", err
	}
	docs = append(docs, projectDoc)

	for _, env := range g.opts.Environments {
		bindingDoc, err := g.generateBinding(env)
		if err != nil {
			return "", err
		}
		docs = append(docs, bindingDoc)
	}

	return strings.Join(docs, "---\n"), nil
}

func (g *Generator) generateProject(jsonSchema *extv1.JSONSchemaProps, defaultedObj map[string]any) (string, error) {
	b := render.NewYAMLBuilder()
	g.projectHeader(b)

	b.AddField("apiVersion", "openchoreo.dev/v1alpha1")
	b.AddField("kind", "Project")
	b.InMapping("metadata", func(b *render.YAMLBuilder) {
		b.AddField("name", g.opts.ProjectName)
		b.AddField("namespace", g.opts.Namespace)
	})
	b.InMapping("spec", func(b *render.YAMLBuilder) {
		b.InMapping("deploymentPipelineRef", func(b *render.YAMLBuilder) {
			b.AddField("name", g.opts.DeploymentPipeline)
		})
		// spec.type is immutable after creation.
		var typeOpts []render.FieldOption
		if g.opts.IncludeStructuralComments {
			typeOpts = append(typeOpts, render.WithHeadComment("\nProject type is immutable after creation"))
		}
		b.InMapping("type", func(b *render.YAMLBuilder) {
			b.AddField("kind", g.opts.ProjectTypeKind)
			b.AddField("name", g.opts.ProjectTypeName)
		}, typeOpts...)

		if jsonSchema != nil && len(jsonSchema.Properties) > 0 {
			var paramOpts []render.FieldOption
			if g.opts.IncludeStructuralComments {
				paramOpts = append(paramOpts, render.WithHeadComment("\nParameters for the project type"))
			}
			b.InMapping("parameters", func(b *render.YAMLBuilder) {
				g.renderer.RenderFields(b, jsonSchema, defaultedObj, 0)
			}, paramOpts...)
		}
	})

	return b.Encode()
}

func (g *Generator) generateBinding(env string) (string, error) {
	b := render.NewYAMLBuilder()
	b.AddField("apiVersion", "openchoreo.dev/v1alpha1")
	b.AddField("kind", "ProjectReleaseBinding")
	b.InMapping("metadata", func(b *render.YAMLBuilder) {
		b.AddField("name", fmt.Sprintf("%s-%s", g.opts.ProjectName, env))
		b.AddField("namespace", g.opts.Namespace)
	})
	// spec.projectRelease is left unset; the Project controller seeds it with the
	// latest release. Advancing it afterwards (promotion) is manual.
	b.InMapping("spec", func(b *render.YAMLBuilder) {
		b.InMapping("owner", func(b *render.YAMLBuilder) {
			b.AddField("projectName", g.opts.ProjectName)
		})
		b.AddField("environment", env)
	})
	return b.Encode()
}

func (g *Generator) projectHeader(b *render.YAMLBuilder) {
	var lines []string
	if g.opts.IncludeStructuralComments {
		lines = append(lines,
			"# Generated by occ project scaffold",
			fmt.Sprintf("# Project: %s", g.opts.ProjectName),
			fmt.Sprintf("# Type: %s/%s", g.opts.ProjectTypeKind, g.opts.ProjectTypeName),
		)
	}
	if g.opts.BindingsNote != "" {
		if len(lines) > 0 {
			lines = append(lines, "#")
		}
		lines = append(lines, "# "+g.opts.BindingsNote)
	}
	if len(lines) == 0 {
		return
	}
	b.SetHeadComment(strings.Join(lines, "\n"))
}

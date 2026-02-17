// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package component provides scaffolding generators for OpenChoreo Component YAML files.
//
// # Generator Implementation
//
// Generator uses the ApplyDefaults approach to ensure generated YAML accurately reflects
// runtime behavior. Default values in scaffolded YAML match exactly what the OpenChoreo
// component rendering pipeline applies when processing Component definitions.
//
// # Output Format
//
// The generated YAML has a consistent, user-friendly structure:
//
//	parameters:
//	  port: <TODO_PORT>           # Required field - user must fill
//	  name: <TODO_NAME>           # Required field - user must fill
//	  database:
//	    host: <TODO_HOST>         # Required nested field
//
//	    # Empty object, or customize:
//	    options: {}               # Required object with all optional children
//	    # options:
//	      # timeout: 30           # Commented structure for reference
//
//	    # Defaults: Uncomment to customize
//	    # port: 5432              # Optional field with default
//
//	  # Defaults: Uncomment to customize
//	  # replicas: 3               # Optional field with default
//	  # cache:                    # Optional object - entirely commented
//	    # enabled: false
//
// # Field Ordering
//
// Fields are sorted for clarity:
//   - Required fields first (sorted by type, then alphabetically)
//   - Separator comment ("# Defaults: Uncomment to customize")
//   - Optional fields last (sorted by type, then alphabetically)
//
// Type order: primitives → arrays → maps → objects (keeps simple fields at top)
//
// # Field Rendering Rules
//
// Primitive fields:
//   - Required without default → Placeholder: `field: <TODO_FIELD>`
//   - Optional with default → Commented: `# field: value`
//
// Object fields:
//   - Required with some required children → Expanded normally
//   - Required with all optional children → Empty object + commented structure: `field: {}`
//   - Optional → Entirely commented out: `# field:`
//
// Array fields:
//   - Arrays of objects → Expanded with 2 example items
//   - Arrays of primitives → Inline format: `[value1, value2]`
//   - Optional arrays with defaults → Commented inline: `# tags: [a, b]`
//
// Map fields (additionalProperties):
//   - map<T> → 2 example key-value pairs with primitive values
//   - map<CustomType> → 2 example keys with nested object structures
//   - map<[]T> → 2 example keys with inline array values
//
// Collection shapes (complex nested types):
//   - []map<T> → Array of maps with primitive values
//   - []map<CustomType> → Array of maps with nested object structures
//   - map<[]T> → Map with primitive array values
//   - map<[]CustomType> → Map with arrays of custom type objects
//
// # Additional Generated Fields
//
// The generator always includes these hardcoded fields:
//   - spec.autoDeploy → Commented out by default: `# autoDeploy: true`
//   - spec.workflow.systemParameters → When workflow is included, adds repository configuration
//     (repository.url, repository.revision.branch, repository.appPath)
//
// # Structural Comments
//
// Section headers are included when IncludeStructuralComments is true:
//   - "Parameters for the ComponentType" (before spec.parameters)
//   - "Traits augment the component with additional capabilities" (before spec.traits)
//   - "Workflow configuration for building this component" (before spec.workflow)
//   - "System parameters for workflow execution" (before spec.workflow.systemParameters)
//
// All structural comments are preceded by an empty line for visual separation.
//
// # High-Level Flow
//
//  1. Schema Conversion - Convert OpenChoreo schema (shorthand syntax like "integer | default=8080")
//     to OpenAPI v3 JSON Schema, then to Kubernetes Structural Schema format.
//
//  2. Empty Structure Building - Create a minimal object structure with empty arrays/objects
//     at all levels where defaults need to be resolved.
//
//  3. ApplyDefaults - Use Kubernetes apiextensions-apiserver's defaulting logic to populate
//     defaults. This matches the OpenChoreo component rendering pipeline at runtime.
//
//  4. YAML Generation - Traverse schema and defaulted object together, applying the
//     field ordering and rendering rules described above.
//
// # Architecture
//
// The generator processes multiple schema sources:
//   - Component parameters (from ComponentType)
//   - Trait instances (from each Trait in the Component)
//   - Workflow parameters (from ComponentWorkflow)
//   - Nested objects and arrays (recursive processing with same rules)
//
// YAMLBuilder handles low-level YAML node manipulation, providing methods for sequences,
// mappings, inline arrays, and comment handling.
package component

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	corev1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Structural comment constants for consistent messaging across generated YAML.
const (
	CommentComponentParameters = "\nParameters for the ComponentType"
	CommentTraitsSection       = "\nTraits augment the component with additional capabilities"
	CommentTraitName           = "Trait resource name"
	CommentTraitInstanceName   = "Unique instance name within this Component"
	CommentTraitParameters     = "Parameters for %s trait"
	CommentWorkflowSection     = "\nWorkflow configuration for building this component"
	CommentWorkflowName        = "ComponentWorkflow to use for builds"
)

// Options configures the component scaffolding generator.
type Options struct {
	// ComponentName is the name for the generated Component
	ComponentName string `yaml:"componentName"`

	// Namespace is the target namespace
	Namespace string `yaml:"namespace"`

	// ProjectName is the owning project name (required for spec.owner.projectName)
	ProjectName string `yaml:"projectName"`

	// IncludeAllFields includes optional fields (without defaults) as commented examples.
	// When true: all fields shown (required as active, optional as commented)
	// When false: only required fields and fields with defaults are shown
	IncludeAllFields bool `yaml:"includeAllFields"`

	// IncludeFieldDescriptions includes schema-derived comments (descriptions, enum alternatives).
	// When true: field descriptions and "also: x, y" enum hints are shown
	// When false: only field values without schema documentation
	IncludeFieldDescriptions bool `yaml:"includeFieldDescriptions"`

	// IncludeStructuralComments includes section headers and guidance comments.
	// When true: comments like "Parameters for the ComponentType" are shown
	// When false: cleaner output without structural guidance
	IncludeStructuralComments bool `yaml:"includeStructuralComments"`

	// TraitInstanceNames maps trait names to instance names
	// If not provided, uses placeholders like "<trait-instance>"
	TraitInstanceNames map[string]string `yaml:"traitInstanceNames,omitempty"`

	// IncludeWorkflow includes workflow section when workflow is provided
	IncludeWorkflow bool `yaml:"includeWorkflow"`
}

// Generator generates Component YAML using ApplyDefaults approach.
// This generator builds an empty object structure, applies Kubernetes defaults,
// then generates YAML showing defaults as commented and required fields as placeholders.
type Generator struct {
	componentTypeName string
	workloadType      string
	componentSchema   *extv1.JSONSchemaProps

	traitSchemas map[string]*extv1.JSONSchemaProps

	workflowName   string
	workflowSchema *extv1.JSONSchemaProps

	opts     *Options
	renderer *FieldRenderer
}

// NewGenerator creates a new generator instance from CRD objects.
// This is a convenience constructor that extracts schemas and metadata from CRDs
// and delegates to NewGeneratorFromSchemas.
func NewGenerator(
	componentType *corev1alpha1.ComponentType,
	traits []*corev1alpha1.Trait,
	workflow *corev1alpha1.ComponentWorkflow,
	opts *Options,
) (*Generator, error) {
	componentSchema, err := extractAndConvertSchema(
		componentType.Spec.Schema.Parameters,
		componentType.Spec.Schema.Types,
	)
	if err != nil {
		return nil, fmt.Errorf("processing component schema: %w", err)
	}

	traitSchemas := make(map[string]*extv1.JSONSchemaProps)
	for _, trait := range traits {
		schema, err := extractAndConvertSchema(
			trait.Spec.Schema.Parameters,
			trait.Spec.Schema.Types,
		)
		if err != nil {
			return nil, fmt.Errorf("processing trait %s schema: %w", trait.Name, err)
		}
		traitSchemas[trait.Name] = schema
	}

	var workflowSchema *extv1.JSONSchemaProps
	var workflowName string
	if workflow != nil {
		workflowName = workflow.Name
		if workflow.Spec.Schema.Parameters != nil {
			schema, err := extractAndConvertSchema(
				workflow.Spec.Schema.Parameters,
				nil,
			)
			if err != nil {
				return nil, fmt.Errorf("processing workflow schema: %w", err)
			}
			workflowSchema = schema
		}
	}

	return NewGeneratorFromSchemas(
		componentType.Name,
		componentType.Spec.WorkloadType,
		componentSchema,
		traitSchemas,
		workflowName,
		workflowSchema,
		opts,
	)
}

// NewGeneratorFromSchemas creates a new generator instance directly from JSON schemas.
// This constructor is useful when you already have processed JSON schemas and don't
// have access to the full CRD objects.
func NewGeneratorFromSchemas(
	componentTypeName string,
	workloadType string,
	componentSchema *extv1.JSONSchemaProps,
	traitSchemas map[string]*extv1.JSONSchemaProps,
	workflowName string,
	workflowSchema *extv1.JSONSchemaProps,
	opts *Options,
) (*Generator, error) {
	if opts == nil {
		opts = &Options{}
	}

	if traitSchemas == nil {
		traitSchemas = make(map[string]*extv1.JSONSchemaProps)
	}

	return &Generator{
		componentTypeName: componentTypeName,
		workloadType:      workloadType,
		componentSchema:   componentSchema,
		traitSchemas:      traitSchemas,
		workflowName:      workflowName,
		workflowSchema:    workflowSchema,
		opts:              opts,
		renderer:          NewFieldRenderer(opts.IncludeFieldDescriptions, opts.IncludeAllFields, opts.IncludeStructuralComments),
	}, nil
}

// Generate produces the scaffolded Component YAML.
func (g *Generator) Generate() (string, error) {
	// Apply defaults to component schema
	result, err := applyDefaultsToSchema(g.componentSchema)
	if err != nil {
		return "", fmt.Errorf("processing component schema: %w", err)
	}

	// Generate YAML from defaulted object
	b := NewYAMLBuilder()
	g.generateHeader(b)
	g.generateMetadata(b)
	if err := g.generateSpec(b, result); err != nil {
		return "", err
	}

	return b.Encode()
}

// generateHeader adds the generated comment header.
func (g *Generator) generateHeader(b *YAMLBuilder) {
	header := fmt.Sprintf("# Generated by occ scaffold component\n# Component: %s\n# Type: %s/%s",
		g.opts.ComponentName,
		g.workloadType,
		g.componentTypeName)

	if len(g.traitSchemas) > 0 {
		traitNames := make([]string, 0, len(g.traitSchemas))
		for traitName := range g.traitSchemas {
			traitNames = append(traitNames, traitName)
		}
		slices.Sort(traitNames)
		header += fmt.Sprintf("\n# Traits: %s", strings.Join(traitNames, ", "))
	}

	if g.opts.IncludeWorkflow && g.workflowName != "" {
		header += fmt.Sprintf("\n# Workflow: %s", g.workflowName)
	}

	b.root.Content[0].HeadComment = header
}

// generateMetadata generates the metadata section.
func (g *Generator) generateMetadata(b *YAMLBuilder) {
	b.AddField("apiVersion", "openchoreo.dev/v1alpha1")
	b.AddField("kind", "Component")

	b.InMapping("metadata", func(b *YAMLBuilder) {
		b.AddField("name", g.opts.ComponentName)
		b.AddField("namespace", g.opts.Namespace)
	})
}

// generateSpec generates the spec section.
func (g *Generator) generateSpec(b *YAMLBuilder, result *schemaProcessingResult) error {
	var generationErr error
	b.InMapping("spec", func(b *YAMLBuilder) {
		// Owner
		b.InMapping("owner", func(b *YAMLBuilder) {
			b.AddField("projectName", g.opts.ProjectName)
		})

		// ComponentType
		b.InMapping("componentType", func(b *YAMLBuilder) {
			b.AddField("kind", "ComponentType")
			b.AddField("name", fmt.Sprintf("%s/%s", g.workloadType, g.componentTypeName))
		})

		// AutoDeploy (commented out by default)
		var autoDeployOpts []FieldOption
		if g.opts.IncludeStructuralComments {
			autoDeployOpts = append(autoDeployOpts, WithLineComment("Enable automatic deployment on changes"))
		}
		b.AddCommentedField("autoDeploy", "true", autoDeployOpts...)

		// Parameters
		if result.jsonSchema != nil && len(result.jsonSchema.Properties) > 0 {
			var opts []FieldOption
			if g.opts.IncludeStructuralComments {
				opts = append(opts, WithHeadComment(CommentComponentParameters))
			}
			b.InMapping("parameters", func(b *YAMLBuilder) {
				g.renderer.RenderFields(b, result.jsonSchema, result.defaultedObj, 0)
			}, opts...)
		}

		// Traits
		if len(g.traitSchemas) > 0 {
			if err := g.generateTraits(b); err != nil {
				generationErr = fmt.Errorf("generating traits: %w", err)
				return
			}
		}

		// Workflow
		if g.opts.IncludeWorkflow && g.workflowName != "" {
			if err := g.generateWorkflow(b); err != nil {
				generationErr = fmt.Errorf("generating workflow: %w", err)
				return
			}
		}
	})
	return generationErr
}

// generateTraits generates the traits section.
func (g *Generator) generateTraits(b *YAMLBuilder) error {
	var seqOpts []FieldOption
	if g.opts.IncludeStructuralComments {
		seqOpts = append(seqOpts, WithHeadComment(CommentTraitsSection))
	}
	sequenceNode := b.AddSequence("traits", seqOpts...)

	// Collect and sort trait names for deterministic output
	traitNames := make([]string, 0, len(g.traitSchemas))
	for traitName := range g.traitSchemas {
		traitNames = append(traitNames, traitName)
	}
	slices.Sort(traitNames)

	for _, traitName := range traitNames {
		traitSchema := g.traitSchemas[traitName]

		// Apply defaults to trait schema
		result, err := applyDefaultsToSchema(traitSchema)
		if err != nil {
			return fmt.Errorf("processing trait %s schema: %w", traitName, err)
		}

		// Create mapping node for trait item
		itemMappingNode := &yaml.Node{
			Kind:    yaml.MappingNode,
			Content: []*yaml.Node{},
		}

		// Use a sub-builder for the trait item
		traitBuilder := &YAMLBuilder{
			root:           &yaml.Node{Kind: yaml.DocumentNode},
			current:        itemMappingNode,
			commentedNodes: b.commentedNodes,
		}

		// Add name field
		var nameOpts []FieldOption
		if g.opts.IncludeStructuralComments {
			nameOpts = append(nameOpts, WithLineComment(CommentTraitName))
		}
		traitBuilder.AddField("name", traitName, nameOpts...)

		// Add instanceName field
		instanceName := g.opts.TraitInstanceNames[traitName]
		if instanceName == "" {
			instanceName = fmt.Sprintf("<%s-instance>", traitName)
		}
		var instanceOpts []FieldOption
		if g.opts.IncludeStructuralComments {
			instanceOpts = append(instanceOpts, WithLineComment(CommentTraitInstanceName))
		}
		traitBuilder.AddField("instanceName", instanceName, instanceOpts...)

		// Add parameters if trait has schema
		if result.jsonSchema != nil && len(result.jsonSchema.Properties) > 0 {
			var paramOpts []FieldOption
			if g.opts.IncludeStructuralComments {
				paramOpts = append(paramOpts, WithHeadComment(fmt.Sprintf(CommentTraitParameters, traitName)))
			}
			traitBuilder.InMapping("parameters", func(tb *YAMLBuilder) {
				g.renderer.RenderFields(tb, result.jsonSchema, result.defaultedObj, 0)
			}, paramOpts...)
		}

		sequenceNode.Content = append(sequenceNode.Content, itemMappingNode)
	}
	return nil
}

// generateWorkflow generates the workflow section.
func (g *Generator) generateWorkflow(b *YAMLBuilder) error {
	var workflowErr error
	var sectionOpts []FieldOption
	if g.opts.IncludeStructuralComments {
		sectionOpts = append(sectionOpts, WithHeadComment(CommentWorkflowSection))
	}
	b.InMapping("workflow", func(b *YAMLBuilder) {
		// Add name field
		var nameOpts []FieldOption
		if g.opts.IncludeStructuralComments {
			nameOpts = append(nameOpts, WithLineComment(CommentWorkflowName))
		}
		b.AddField("name", g.workflowName, nameOpts...)

		// Apply defaults to workflow schema if it has parameters
		if g.workflowSchema != nil {
			result, err := applyDefaultsToSchema(g.workflowSchema)
			if err != nil {
				workflowErr = fmt.Errorf("processing workflow %s schema: %w", g.workflowName, err)
				return
			}

			// Add parameters if workflow has schema
			if result.jsonSchema != nil && len(result.jsonSchema.Properties) > 0 {
				b.InMapping("parameters", func(b *YAMLBuilder) {
					g.renderer.RenderFields(b, result.jsonSchema, result.defaultedObj, 0)
				})
			}
		}

		// Add systemParameters with repository structure
		var systemParamsOpts []FieldOption
		if g.opts.IncludeStructuralComments {
			systemParamsOpts = append(systemParamsOpts, WithHeadComment("\nSystem parameters for workflow execution"))
		}
		b.InMapping("systemParameters", func(b *YAMLBuilder) {
			b.InMapping("repository", func(b *YAMLBuilder) {
				b.AddField("url", "<TODO_REPOSITORY_URL>", WithLineComment("Git repository URL"))
				b.InMapping("revision", func(b *YAMLBuilder) {
					b.AddField("branch", "<TODO_BRANCH>", WithLineComment("Git branch to build from"))
				})
				b.AddField("appPath", "<TODO_APP_PATH>", WithLineComment("Path to application code within repository"))
			})
		}, systemParamsOpts...)
	}, sectionOpts...)
	return workflowErr
}

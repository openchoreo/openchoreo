// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"context"

	"gopkg.in/yaml.v3"
)

// Position represents a source location (1-indexed line and column).
type Position struct {
	Line   int
	Column int
}

// Range represents a span in source text.
type Range struct {
	Start Position
	End   Position
}

// Field represents a YAML mapping key-value pair with its location.
type Field struct {
	Key       string
	KeyNode   *yaml.Node
	ValueNode *yaml.Node
	Range     Range
}

// Node returns the value node, equivalent to ValueNode.
// Convenience alias for cleaner downstream code.
func (f *Field) Node() *yaml.Node {
	return f.ValueNode
}

// MetadataNode represents the metadata section of an OpenChoreo resource.
type MetadataNode struct {
	Name        string
	Namespace   string
	Labels      *yaml.Node
	Annotations *yaml.Node

	Node *yaml.Node
}

// DocumentNode represents one parsed OpenChoreo YAML document.
type DocumentNode struct {
	// ValidationContext carries immutable request-scoped dependencies.
	ValidationContext context.Context `json:"-" yaml:"-"`
	APIVersion        string
	Kind              string

	Metadata MetadataNode

	Spec   *yaml.Node
	Status *yaml.Node

	// Fields holds all top-level mapping fields for iteration / lookup.
	Fields map[string]*Field

	// Root is the original yaml.Node for the entire document.
	Root *yaml.Node

	// DocumentIndex is the 0-based index of this document in the file.
	DocumentIndex int

	// Range is the byte span of this document in the source file.
	Range Range

	// Source is the raw YAML source bytes for this document.
	Source []byte
}

// FieldAt returns the Field at the given position, or nil if none.
func (d *DocumentNode) FieldAt(pos Position) *Field {
	for _, f := range d.Fields {
		if pos.Line >= f.Range.Start.Line && pos.Line <= f.Range.End.Line {
			return f
		}
	}
	return nil
}

// GetField returns a top-level field by key name, or nil.
func (d *DocumentNode) GetField(key string) *Field {
	return d.Fields[key]
}

// KnownKinds lists all OpenChoreo resource kinds the LSP knows about.
var KnownKinds = map[string]bool{
	"Project":                                true,
	"Component":                              true,
	"ComponentType":                          true,
	"ClusterComponentType":                   true,
	"Environment":                            true,
	"DeploymentPipeline":                     true,
	"ProjectReleaseBinding":                  true,
	"ReleaseBinding":                         true,
	"Workflow":                               true,
	"ClusterWorkflow":                        true,
	"WorkflowRun":                            true,
	"Workload":                               true,
	"ClusterProjectType":                     true,
	"ClusterResourceType":                    true,
	"ResourceType":                           true,
	"ClusterDataPlane":                       true,
	"ClusterObservabilityPlane":              true,
	"Trait":                                  true,
	"ClusterTrait":                           true,
	"AuthzRole":                              true,
	"AuthzRoleBinding":                       true,
	"ClusterAuthzRole":                       true,
	"ClusterAuthzRoleBinding":                true,
	"ClusterWorkflowPlane":                   true,
	"ComponentRelease":                       true,
	"DataPlane":                              true,
	"ObservabilityAlertRule":                 true,
	"ObservabilityAlertsNotificationChannel": true,
	"ObservabilityPlane":                     true,
	"ProjectRelease":                         true,
	"ProjectType":                            true,
	"RenderedRelease":                        true,
	"Resource":                               true,
	"ResourceRelease":                        true,
	"ResourceReleaseBinding":                 true,
	"SecretReference":                        true,
	"WorkflowPlane":                          true,
}

// ValidAPIVersions lists accepted apiVersion values.
var ValidAPIVersions = map[string]bool{
	"openchoreo.dev/v1alpha1": true,
}

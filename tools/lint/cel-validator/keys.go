// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package celvalidator

// Kinds carrying template ${...} expressions. They are the single source of
// truth for which documents the validator runs over; ruleengine/template
// references them when registering the CEL rule per kind.
const (
	KindProjectType          = "ProjectType"
	KindClusterProjectType   = "ClusterProjectType"
	KindComponentType        = "ComponentType"
	KindClusterComponentType = "ClusterComponentType"
	KindTrait                = "Trait"
	KindClusterTrait         = "ClusterTrait"
	KindResourceType         = "ResourceType"
	KindClusterResourceType  = "ClusterResourceType"
	KindWorkflow             = "Workflow"
	KindClusterWorkflow      = "ClusterWorkflow"
)

// Field keys beneath spec, identical across the CEL-bearing kinds and shared
// with the schema section walker (parameters / environmentConfigs flow through
// the same mappingValue lookups).
const (
	fieldParameters            = "parameters"
	fieldEnvironmentConfigs    = "environmentConfigs"
	fieldOpenAPIV3Schema       = "openAPIV3Schema"
	fieldResources             = "resources"
	fieldCreates               = "creates"
	fieldPatches               = "patches"
	fieldRemoves               = "removes"
	fieldValidations           = "validations"
	fieldPreRenderValidations  = "preRenderValidations"
	fieldPostRenderValidations = "postRenderValidations"
	fieldOutputs               = "outputs"
	fieldID                    = "id"
	fieldTemplate              = "template"
	fieldIncludeWhen           = "includeWhen"
	fieldReadyWhen             = "readyWhen"
	fieldForEach               = "forEach"
	fieldVar                   = "var"
	fieldWhen                  = "when"
	fieldTarget                = "target"
	fieldWhere                 = "where"
	fieldRule                  = "rule"
	fieldOperations            = "operations"
	fieldOp                    = "op"
	fieldValue                 = "value"
	fieldSecretKeyRef          = "secretKeyRef"
	fieldConfigMapKeyRef       = "configMapKeyRef"
	fieldName                  = "name"
	fieldKey                   = "key"
	fieldRunTemplate           = "runTemplate"
	fieldExternalRefs          = "externalRefs"
)

// varForEachDefault is the loop variable name when a forEach omits `var`.
const varForEachDefault = "item"

// Patch operation values a template body is validated for.
const (
	patchOpAdd     = "add"
	patchOpReplace = "replace"
)

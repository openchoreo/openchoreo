// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common"
	"github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/parser"
)

const (
	configurationsIdentifier = "configurations"
	dependenciesIdentifier   = "dependencies"
	derivedIdentifier        = "derived"
)

// CELExtensions returns CEL environment options for configuration, workload,
// and dependency helpers. All macros rewrite method-call syntax to field
// selects on the precomputed "derived" context variable.
func CELExtensions() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Macros(
			toConfigFileListMacro,
			toSecretFileListMacro,
			toContainerEnvFromMacro,
			toContainerVolumeMountsMacro,
			toVolumesMacro,
			toConfigEnvsByContainerMacro,
			toSecretEnvsByContainerMacro,
			toServicePortsMacro,
			toContainerEnvsMacro,
			toEndpointResourcesMacro,
		),
	}
}

func derivedField(eh parser.ExprHelper, fieldName string) ast.Expr {
	return eh.NewSelect(eh.NewIdent(derivedIdentifier), fieldName)
}

var toConfigFileListMacro = cel.ReceiverMacro("toConfigFileList", 0,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == configurationsIdentifier {
			return derivedField(eh, "configFileList"), nil
		}
		return nil, nil
	})

var toSecretFileListMacro = cel.ReceiverMacro("toSecretFileList", 0,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == configurationsIdentifier {
			return derivedField(eh, "secretFileList"), nil
		}
		return nil, nil
	})

var toContainerEnvFromMacro = cel.ReceiverMacro("toContainerEnvFrom", 0,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == configurationsIdentifier {
			return derivedField(eh, "containerEnvFrom"), nil
		}
		return nil, nil
	})

var toContainerVolumeMountsMacro = cel.ReceiverMacro("toContainerVolumeMounts", 0,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() != ast.IdentKind {
			return nil, nil
		}
		switch target.AsIdent() {
		case configurationsIdentifier:
			return derivedField(eh, "configVolumeMounts"), nil
		case dependenciesIdentifier:
			return derivedField(eh, "dependencyVolumeMounts"), nil
		}
		return nil, nil
	})

var toVolumesMacro = cel.ReceiverMacro("toVolumes", 0,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() != ast.IdentKind {
			return nil, nil
		}
		switch target.AsIdent() {
		case configurationsIdentifier:
			return derivedField(eh, "configVolumes"), nil
		case dependenciesIdentifier:
			return derivedField(eh, "dependencyVolumes"), nil
		}
		return nil, nil
	})

var toConfigEnvsByContainerMacro = cel.ReceiverMacro("toConfigEnvsByContainer", 0,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == configurationsIdentifier {
			return derivedField(eh, "configEnvs"), nil
		}
		return nil, nil
	})

var toSecretEnvsByContainerMacro = cel.ReceiverMacro("toSecretEnvsByContainer", 0,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == configurationsIdentifier {
			return derivedField(eh, "secretEnvs"), nil
		}
		return nil, nil
	})

var toServicePortsMacro = cel.ReceiverMacro("toServicePorts", 0,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == "workload" {
			return derivedField(eh, "servicePorts"), nil
		}
		return nil, nil
	})

var toContainerEnvsMacro = cel.ReceiverMacro("toContainerEnvs", 0,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == dependenciesIdentifier {
			return derivedField(eh, "dependencyEnvVars"), nil
		}
		return nil, nil
	})

// toEndpointResourcesMacro rewrites workload.toEndpointResources() to
// derived.endpointResources (a map keyed by endpoint name). The backing map is
// only computed when this macro is used, so schema parsing and context cost are
// avoided unless a template opts in.
var toEndpointResourcesMacro = cel.ReceiverMacro("toEndpointResources", 0,
	func(eh parser.ExprHelper, target ast.Expr, args []ast.Expr) (ast.Expr, *common.Error) {
		if target.Kind() == ast.IdentKind && target.AsIdent() == "workload" {
			return derivedField(eh, "endpointResources"), nil
		}
		return nil, nil
	})

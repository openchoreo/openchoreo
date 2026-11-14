// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package celvalidator

import (
	"encoding/json"
)

var celEnvDir = ""

func SetCelEnvDir(dir string) { celEnvDir = dir }
func GetCelEnvDir() string    { return celEnvDir }

type Var struct {
	Name     string          `json:"name"`
	CELType  string          `json:"celType,omitempty"`
	DeclType json.RawMessage `json:"declType,omitempty"`
}

type Ext struct {
	Base            []string `json:"base,omitempty"`
	CustomMacros    []string `json:"customMacros,omitempty"`
	CustomFunctions []string `json:"customFunctions,omitempty"`
	ContextMacros   []string `json:"contextMacros,omitempty"`
}

type EnvDesc struct {
	Kind       string `json:"kind"`
	Variables  []Var  `json:"variables"`
	HasApplied bool   `json:"hasApplied"`
	Extensions Ext    `json:"extensions"`
}

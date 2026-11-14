// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package contextdocs

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

// Generate reads source comments and JSON tags without maintaining a field list.
func Generate(pipeline string) ([]byte, error) {
	out := map[string]map[string]string{}
	for family, dir := range map[string]string{"component": "component/context", "resource": "resource", "project": "project", "workflow": "workflow"} {
		docs := map[string]string{}
		files, err := filepath.Glob(filepath.Join(pipeline, dir, "*.go"))
		if err != nil {
			return nil, err
		}
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
			if err != nil {
				return nil, err
			}
			for _, decl := range file.Decls {
				general, ok := decl.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, spec := range general.Specs {
					typ, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					object, ok := typ.Type.(*ast.StructType)
					if !ok {
						continue
					}
					if general.Doc != nil {
						docs[typ.Name.Name] = strings.TrimSpace(general.Doc.Text())
					}
					for _, field := range object.Fields.List {
						if field.Tag == nil || field.Doc == nil {
							continue
						}
						tag, err := strconv.Unquote(field.Tag.Value)
						if err != nil {
							return nil, err
						}
						name := strings.Split(reflect.StructTag(tag).Get("json"), ",")[0]
						if name == "" || name == "-" {
							continue
						}
						docs[typ.Name.Name+"."+name] = strings.TrimSpace(field.Doc.Text())
					}
				}
			}
		}
		out[family] = docs
	}
	return json.MarshalIndent(out, "", "  ")
}

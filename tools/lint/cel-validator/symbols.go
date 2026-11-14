// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package celvalidator

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"gopkg.in/yaml.v3"
	apiservercel "k8s.io/apiserver/pkg/cel"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/strdist"
)

// ExpressionInfo describes a symbol in the exact environment used to check it.
type ExpressionInfo struct {
	Name, Type, Description, Suggestion string
	Range                               parser.Range
	ChildrenParent                      string
	Children                            []ExpressionChild
	Functions                           []ExpressionChild
	OpenKeys                            bool
}

// ExpressionChild is a declared child, not a runtime value or a guessed map key.
type ExpressionChild struct{ Name, Type, Description string }
type inspectionKey struct{}
type inspection struct {
	line, column int
	result       *ExpressionInfo
	schemas      SchemaPair
}

// InspectExpression reuses the validation walk, including target and forEach scopes.
// It never renders resources or evaluates user expressions.
func InspectExpression(doc *parser.DocumentNode, pair SchemaPair, line, column int) (*ExpressionInfo, error) {
	copyDoc := *doc
	base := doc.ValidationContext
	if base == nil {
		base = context.Background()
	}
	query := &inspection{line: line, column: column, schemas: pair}
	copyDoc.ValidationContext = context.WithValue(base, inspectionKey{}, query)
	_, err := ValidateDocument(&copyDoc, pair)
	return query.result, err
}

var symbolPaths = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*`)

// Mask CEL strings/comments before locating identifier paths. This is lexical
// source selection only; all field names/types still come from CEL's provider.
func expressionSymbols(expr string) [][2]int {
	masked := []byte(expr)
	for i := 0; i < len(masked); {
		if masked[i] == '\'' || masked[i] == '"' {
			quote := masked[i]
			start := i
			i++
			triple := i+1 < len(masked) && masked[i] == quote && masked[i+1] == quote
			if triple {
				i += 2
			}
			for i < len(masked) {
				if masked[i] == '\\' {
					i += 2
					continue
				}
				if masked[i] == quote {
					if !triple {
						i++
						break
					}
					if i+2 < len(masked) && masked[i+1] == quote && masked[i+2] == quote {
						i += 3
						break
					}
				}
				i++
			}
			if i > len(masked) {
				i = len(masked)
			}
			for j := start; j < i; j++ {
				masked[j] = ' '
			}
		} else if i+1 < len(masked) && masked[i] == '/' && masked[i+1] == '/' {
			for i < len(masked) && masked[i] != '\n' {
				masked[i] = ' '
				i++
			}
		} else {
			i++
		}
	}
	var out [][2]int
	for _, span := range symbolPaths.FindAllIndex(masked, -1) {
		end := span[1]
		rest := strings.TrimSpace(expr[end:])
		if strings.HasPrefix(rest, "(") {
			if dot := strings.LastIndex(expr[span[0]:end], "."); dot >= 0 {
				end = span[0] + dot
			} else {
				continue
			}
		}
		out = append(out, [2]int{span[0], end})
	}
	return out
}

func variableType(env *cel.Env, name string) *cel.Type {
	for _, v := range env.Variables() {
		if v.Name() == name {
			return v.Type()
		}
	}
	return nil
}
func pathType(env *cel.Env, parts []string) (*cel.Type, string) {
	if len(parts) == 0 {
		return nil, ""
	}
	typ := variableType(env, parts[0])
	parent := ""
	for _, field := range parts[1:] {
		if typ == nil {
			return nil, ""
		}
		if typ.Kind() == types.MapKind {
			params := typ.Parameters()
			if len(params) != 2 {
				return nil, ""
			}
			typ = params[1]
			parent = ""
			continue
		}
		if typ.Kind() != types.StructKind {
			return nil, ""
		}
		parent = typ.TypeName()
		child, ok := env.CELTypeProvider().FindStructFieldType(parent, field)
		if !ok {
			return nil, parent
		}
		typ = child.Type
	}
	return typ, parent
}
func closestField(name string, names []string) string {
	if len(name) < 4 {
		return ""
	}
	names = append([]string(nil), names...)
	sort.Strings(names)
	distance := 1
	if len(name) >= 6 {
		distance = 2
	}
	candidate, _, confident := strdist.Closest(name, names, distance)
	if confident {
		return candidate
	}
	return ""
}
func symbolSuggestion(env *cel.Env, parts []string) string {
	if len(parts) == 1 {
		names := make([]string, 0, len(env.Variables()))
		for _, v := range env.Variables() {
			names = append(names, v.Name())
		}
		return closestField(parts[0], names)
	}
	parent, _ := pathType(env, parts[:len(parts)-1])
	if parent == nil || parent.Kind() != types.StructKind {
		return ""
	}
	return closestField(parts[len(parts)-1], structFieldNames(env, parent))
}
func structFieldNames(env *cel.Env, typ *cel.Type) []string {
	if typ == nil || typ.Kind() != types.StructKind {
		return nil
	}
	names, ok := env.CELTypeProvider().FindStructFieldNames(typ.TypeName())
	if !ok {
		// Kubernetes DeclTypeProvider exposes declaration fields but deliberately
		// does not implement FindStructFieldNames. Read its existing declaration.
		if provider, supports := env.CELTypeProvider().(interface {
			FindDeclType(string) (*apiservercel.DeclType, bool)
		}); supports {
			if declaration, exists := provider.FindDeclType(typ.TypeName()); exists {
				for name := range declaration.Fields {
					names = append(names, name)
				}
				ok = true
			}
		}
	}
	if !ok {
		return nil
	}
	names = append([]string(nil), names...)
	sort.Strings(names)
	return names
}

func fieldSuggestion(env *cel.Env, expr, message string, offset int) string {
	if env == nil {
		return ""
	}
	bad := undefinedTargetField.FindStringSubmatch(message)
	if len(bad) != 2 {
		if root := undeclaredToken.FindStringSubmatch(message); len(root) == 2 {
			for _, span := range symbolPaths.FindAllStringIndex(expr, -1) {
				executable := false
				for _, selected := range expressionSymbols(expr) {
					if selected[0] == span[0] {
						executable = true
						break
					}
				}
				if !executable {
					continue
				}
				path := expr[span[0]:span[1]]
				parts := strings.Split(path, ".")
				if len(parts) < 2 || parts[len(parts)-1] != root[1] || !strings.HasPrefix(strings.TrimSpace(expr[span[1]:]), "(") {
					continue
				}
				typ, _ := pathType(env, parts[:len(parts)-1])
				var names []string
				for _, fn := range receiverFunctions(env, typ, strings.Join(parts[:len(parts)-1], ".")) {
					names = append(names, fn.Description)
				}
				if candidate := closestField(root[1], names); candidate != "" {
					return fmt.Sprintf("%s. Did you mean %q?", message, candidate)
				}
			}
			if candidate := symbolSuggestion(env, []string{root[1]}); candidate != "" {
				return fmt.Sprintf("%s. Did you mean %q?", message, candidate)
			}
		}
		return ""
	}
	for _, span := range expressionSymbols(expr) {
		if offset < span[0] || offset >= span[1] {
			continue
		}
		parts := strings.Split(expr[span[0]:span[1]], ".")
		for i, part := range parts {
			if part == bad[1] {
				if candidate := symbolSuggestion(env, parts[:i+1]); candidate != "" {
					return fmt.Sprintf("%s. Did you mean %q?", message, candidate)
				}
			}
		}
	}
	return ""
}
func (c *checker) inspectSymbol(node *yaml.Node, env *cel.Env, expr string) {
	q := c.inspection
	if q == nil || q.result != nil {
		return
	}
	for _, span := range expressionSymbols(expr) {
		path := expr[span[0]:span[1]]
		location, ok := c.exactScalarToken(node, path)
		// Ambiguous/repeated or noncontiguous source paths retain normal YAML hover.
		if !ok || location.Start.Line != q.line || location.End.Line != q.line || q.column < location.Start.Column || q.column >= location.End.Column {
			continue
		}
		parts := strings.Split(path, ".")
		offset := q.column - location.Start.Column
		length := 0
		for i, part := range parts {
			if offset >= length && offset < length+len(part) {
				prefix := parts[:i+1]
				typ, parent := pathType(env, prefix)
				r := location
				r.Start.Column += length
				r.End = r.Start
				r.End.Column += len(part)
				if typ != nil {
					if typ.Kind() == types.DynKind {
						return
					}
					q.result = &ExpressionInfo{Name: strings.Join(prefix, "."), Type: typ.String(), Range: r, Description: c.symbolDescription(prefix, parent)}
				} else if suggestion := symbolSuggestion(env, prefix); suggestion != "" {
					q.result = &ExpressionInfo{Name: strings.Join(prefix, "."), Type: "unresolved", Range: r, Suggestion: suggestion}
				}
				if q.result != nil {
					c.addSymbolChildren(q.result, env, prefix, typ)
				}
				return
			}
			length += len(part) + 1
		}
	}
}

func (c *checker) addSymbolChildren(info *ExpressionInfo, env *cel.Env, prefix []string, typ *cel.Type) {
	path := append([]string(nil), prefix...)
	if typ != nil && typ.Kind() == types.MapKind {
		info.OpenKeys = true
	}
	// Objects show their own children. Leaves show the declared children of
	// their parent so users can discover siblings from an existing expression.
	if typ == nil || (typ.Kind() != types.StructKind && typ.Kind() != types.ListKind) {
		if len(path) < 2 {
			return
		}
		path = path[:len(path)-1]
		typ, _ = pathType(env, path)
	}
	if typ != nil && typ.Kind() == types.MapKind {
		info.OpenKeys = true
		return
	}
	if typ != nil && typ.Kind() == types.ListKind {
		params := typ.Parameters()
		if len(params) != 1 {
			return
		}
		typ = params[0]
		path[len(path)-1] += "[]"
	}
	if typ == nil || typ.Kind() != types.StructKind {
		return
	}
	info.ChildrenParent = strings.Join(path, ".")
	info.Functions = receiverFunctions(env, typ, info.ChildrenParent)
	for _, name := range structFieldNames(env, typ) {
		field, ok := env.CELTypeProvider().FindStructFieldType(typ.TypeName(), name)
		if !ok {
			continue
		}
		childPath := append(append([]string(nil), path...), name)
		info.Children = append(info.Children, ExpressionChild{Name: strings.Join(childPath, "."), Type: field.Type.String(), Description: c.symbolDescription(childPath, typ.TypeName())})
	}
}

// receiverFunctions reads member overloads from the checking environment. Dynamic
// and generic receivers are excluded because they cannot establish membership.
func receiverFunctions(env *cel.Env, typ *cel.Type, receiver string) []ExpressionChild {
	if typ == nil || typ.Kind() == types.DynKind {
		return nil
	}
	var result []ExpressionChild
	seen := map[string]bool{}
	// OpenChoreo helpers are receiver macros, not overload declarations.
	// Compile a zero-argument call to establish receiver eligibility and result
	// type using the production macro expansion, without evaluating anything.
	if receiver != "" {
		for _, macro := range env.Macros() {
			if !macro.IsReceiverStyle() || macro.ArgCount() != 0 {
				continue
			}
			signature := receiver + "." + macro.Function() + "()"
			checked, issues := env.Compile(signature)
			if issues != nil && issues.Err() != nil {
				continue
			}
			if checked == nil {
				continue
			}
			key := signature + checked.OutputType().String()
			if seen[key] {
				continue
			}
			seen[key] = true
			result = append(result, ExpressionChild{Name: signature, Type: checked.OutputType().String(), Description: macro.Function()})
		}
	}
	for name, fn := range env.Functions() {
		for _, overload := range fn.OverloadDecls() {
			args := overload.ArgTypes()
			if !overload.IsMemberFunction() || len(args) == 0 || args[0].String() != typ.String() {
				continue
			}
			var params []string
			for _, arg := range args[1:] {
				params = append(params, arg.String())
			}
			signature := name + "(" + strings.Join(params, ", ") + ")"
			if receiver != "" {
				signature = receiver + "." + signature
			}
			key := signature + overload.ResultType().String()
			if seen[key] {
				continue
			}
			seen[key] = true
			result = append(result, ExpressionChild{Name: signature, Type: overload.ResultType().String(), Description: name})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

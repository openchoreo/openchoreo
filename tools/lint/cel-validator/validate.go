// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package celvalidator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common"
	celast "github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/operators"
	"github.com/google/cel-go/common/types"
	"gopkg.in/yaml.v3"
	apiservercel "k8s.io/apiserver/pkg/cel"

	"github.com/openchoreo/openchoreo/internal/template"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/targetschema"
)

// Diagnostic codes reported for CEL expressions. Each is unique across the
// rule engine (see tools/lint/ruleengine for the full catalog).
const (
	// CodeCELParseError reports a ${...} expression that does not parse.
	CodeCELParseError = "cel-parse-error"
	// CodeCELTypeError reports a ${...} expression that is ill-typed, e.g. a
	// select on a field the context does not declare, or a value that forces
	// something that is not boolean/iterable.
	CodeCELTypeError = "cel-type-error"
	// CodeCELUnknownName reports a reference to a name not in scope.
	CodeCELUnknownName = "cel-unknown-name"
	// CodeCELUnknownFunction reports a call with no matching overload.
	CodeCELUnknownFunction = "cel-unknown-function"
)

// Finding is one CEL error resolved back to its location in the document.
type Finding struct {
	Range   parser.Range
	Code    string
	Message string
}

// ValidateDocument type-checks every ${...} expression in a CEL-bearing
// document against the schema-derived environment for its kind, mirroring the
// webhook validation in internal/validation. Kinds that carry no template CEL
// (Project, Component, ComponentRelease, ...) yield no findings.
//
// The reported range covers the offending token (identifier/dotted path, or a
// call and its arguments) at its exact document location; parse errors and
// structural issues (e.g. a boolean field that does not return a boolean) span
// the whole ${...} marker instead. Block scalar values are mapped through the
// source indentation so the underline lands inside the expression, not on the
// args: | header.
func ValidateDocument(doc *parser.DocumentNode, pair SchemaPair) ([]Finding, error) {
	switch doc.Kind {
	case KindComponentType, KindClusterComponentType, KindProjectType, KindClusterProjectType:
		return validateComponent(doc, pair)
	case KindTrait, KindClusterTrait:
		return validateTrait(doc, pair)
	case KindResourceType, KindClusterResourceType:
		return validateResourceType(doc, pair)
	case KindWorkflow, KindClusterWorkflow:
		return validateWorkflow(doc, pair)
	default:
		return nil, nil
	}
}

// checker collects findings as the walk descends. The CEL env changes per path
// (forEach loop variables, applied scope, resource binding) and is threaded
// explicitly through the walk so no call site has to restore it.
type checker struct {
	targetNode *yaml.Node
	currentEnv *cel.Env
	inspection *inspection
	kind       string
	schemas    SchemaPair
	resolver   targetschema.Resolver
	findings   []Finding

	// appliedIDs, when non-nil, enables the applied.<id> declaration check
	// (ResourceType/ClusterResourceType only).
	appliedIDs map[string]bool

	// lines holds the raw document source split into lines. Block scalar
	// values (args: |) are de-indented by yaml.v3 and positioned at the |>,
	// so recovering the absolute column of a token inside them requires the
	// real indentation, which only the source text can provide.
	lines []string
}

// newChecker builds a checker with the document's source lines loaded.
func newChecker(doc *parser.DocumentNode) *checker {
	c := &checker{lines: sourceLines(doc.Source), resolver: targetschema.FromContext(doc.ValidationContext), kind: doc.Kind}
	if doc.ValidationContext != nil {
		c.inspection, _ = doc.ValidationContext.Value(inspectionKey{}).(*inspection)
	}
	if c.inspection != nil {
		c.schemas = c.inspection.schemas
	}
	return c
}

// sourceLines splits raw source bytes into lines (kept 1-indexed by slice
// position). A nil/empty source yields nil, which the position helpers treat
// as "no indentation information".
func sourceLines(src []byte) []string {
	if len(src) == 0 {
		return nil
	}
	return strings.Split(string(src), "\n")
}

// isBlockScalar reports whether a scalar node's text does not begin at its
// yaml position: literal/folded block scalars position the node at the |/>>
// header (scannerc.go) and store de-indented content that starts on the next
// line. Plain and quoted scalars start exactly at node.Line/Column.
func isBlockScalar(node *yaml.Node) bool {
	return node.Style == yaml.LiteralStyle || node.Style == yaml.FoldedStyle
}

// lineIndent returns the number of leading spaces on the 1-indexed source
// line, or 0 when the line is unknown or empty.
func (c *checker) lineIndent(line int) int {
	if line <= 0 || line > len(c.lines) {
		return 0
	}
	n := 0
	for _, r := range c.lines[line-1] {
		if r != ' ' {
			break
		}
		n++
	}
	return n
}

// markerOffset returns the byte offset of full (a ${...} marker, or the entire
// scalar value for whole-value callers) within node.Value, or 0 when absent.
func markerOffset(node *yaml.Node, full string) int {
	if full == "" || full == node.Value {
		return 0
	}
	if off := strings.Index(node.Value, full); off >= 0 {
		return off
	}
	return 0
}

// innerOffset locates expr within node.Value and returns the byte offset at
// which it begins. It prefers the full ${expr} marker so embedded text is
// unambiguous, and falls back to a bare expr for unwrapped values (a where:
// filter may legally omit ${...}). Returns 0 when neither is found.
func innerOffset(node *yaml.Node, expr string) int {
	if expr == "" {
		return 0
	}
	if idx := strings.Index(node.Value, "${"+expr+"}"); idx >= 0 {
		return idx + len("${")
	}
	if idx := strings.Index(node.Value, expr); idx >= 0 {
		return idx
	}
	return 0
}

// valueOffsetToPos maps a byte offset into a scalar node's Value back to
// absolute document coordinates. For plain/quoted scalars the value begins at
// the node's own position; for literal/folded block scalars it begins on the
// line after the header and is re-aligned with the file's real indentation
// (recovered from the source lines).
func (c *checker) valueOffsetToPos(node *yaml.Node, off int) parser.Position {
	value := node.Value
	if off < 0 {
		off = 0
	}
	if off > len(value) {
		off = len(value)
	}

	linesBefore := 0
	for _, b := range value[:off] {
		if b == '\n' {
			linesBefore++
		}
	}
	lineStart := strings.LastIndex(value[:off], "\n") + 1
	valueCol := off - lineStart

	line := node.Line + linesBefore
	colZero := node.Column - 1
	if node.Style == yaml.SingleQuotedStyle || node.Style == yaml.DoubleQuotedStyle {
		colZero++
	}
	if isBlockScalar(node) {
		line = node.Line + 1 + linesBefore
		colZero = c.lineIndent(line)
	}
	if line < 1 {
		line = 1
	}
	return parser.Position{Line: line, Column: colZero + valueCol + 1}
}

// valueLineEndPos returns the document position of the last character of the
// value line containing off (used to widen an unclosed-marker diagnostic).
func (c *checker) valueLineEndPos(node *yaml.Node, off int) parser.Position {
	value := node.Value
	if off >= 0 && off < len(value) {
		if idx := strings.IndexByte(value[off:], '\n'); idx > 0 {
			return c.valueOffsetToPos(node, off+idx-1)
		}
	}
	return c.valueOffsetToPos(node, len(value))
}

// markerSpan returns the positions of a ${...} marker's first and last
// characters. Without a recognized marker (whole-value callers) the whole
// scalar value is spanned.
func (c *checker) markerSpan(node *yaml.Node, full string) parser.Range {
	off := markerOffset(node, full)
	start := c.valueOffsetToPos(node, off)
	end := start
	if full != "" && full != node.Value && off+len(full)-1 < len(node.Value) {
		end = c.valueOffsetToPos(node, off+len(full)-1)
	} else if node.Value != "" {
		end = c.valueOffsetToPos(node, len(node.Value)-1)
	}
	return parser.Range{Start: start, End: end}
}

// exprOffset maps a CEL issue location (1-based line, 0-based column within
// expr) to a byte offset in expr, or -1 when the location is out of range.
func exprOffset(expr string, loc common.Location) int {
	line := loc.Line()
	if line < 1 {
		line = 1
	}
	col := loc.Column()
	if col < 0 {
		col = 0
	}
	off := 0
	for i := 1; i < line; i++ {
		idx := strings.IndexByte(expr[off:], '\n')
		if idx < 0 {
			return -1
		}
		off += idx + 1
	}
	off += col
	if off > len(expr) {
		off = len(expr)
	}
	return off
}

// celdTokenEnd returns the byte offset (within node.Value) just past the CEL
// token beginning at off: a quoted string literal, an identifier/dotted path
// (numeric literals too), and — when the token is a call — its balanced
// argument list. It returns off unchanged when no token characters follow.
func celdTokenEnd(value string, off int) int {
	n := len(value)
	i := off
	var quote byte
	for i < n {
		ch := value[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			} else if ch == '\\' && i+1 < n {
				i++
			}
			i++
			continue
		}
		switch {
		case ch == '\'' || ch == '"':
			quote = ch
			i++
		case (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '_' || ch == '.':
			i++
		default:
			goto afterIdent
		}
	}

afterIdent:
	if i < n && value[i] == '(' {
		depth := 0
		for i < n {
			switch value[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					i++
					goto done
				}
			case '\'', '"':
				q := value[i]
				i++
				for i < n && value[i] != q {
					if value[i] == '\\' && i+1 < n {
						i++
					}
					i++
				}
				if i < n {
					i++
				}
				continue
			}
			i++
		}
	}
done:
	return i
}

// checkCompile parses and type-checks expr against env, emitting one finding
// on the first issue. It returns the checked AST, or nil when anything failed.
func (c *checker) checkCompile(node *yaml.Node, env *cel.Env, expr, full string) *cel.Ast {
	c.currentEnv = env
	c.inspectSymbol(node, env, expr)
	ast, issues := env.Parse(expr)
	if issues != nil && issues.Err() != nil {
		c.addIssues(node, CodeCELParseError, issues, full, expr)
		return nil
	}

	checked, issues := env.Check(ast)
	if issues != nil && issues.Err() != nil {
		c.addIssues(node, classifyCELIssues(issues), issues, full, expr)
		return nil
	}

	if c.appliedIDs != nil {
		c.checkAppliedReferences(checked, node)
	}

	return checked
}

// checkAny validates a general expression (any result type is accepted).
func (c *checker) checkAny(node *yaml.Node, env *cel.Env, expr, full string) {
	c.checkCompile(node, env, expr, full)
}

// checkBoolean validates an expression whose runtime contract is a boolean.
func (c *checker) checkBoolean(node *yaml.Node, env *cel.Env, expr, full string) {
	checked := c.checkCompile(node, env, expr, full)
	if checked == nil {
		return
	}
	outputType := checked.OutputType()
	if !outputType.IsExactType(cel.BoolType) && outputType != cel.DynType {
		c.reportAt(c.markerSpan(node, full), CodeCELTypeError,
			fmt.Sprintf("expression must return boolean, got %s", outputType))
	}
}

// checkIterable validates an expression whose runtime contract is a list or
// map (forEach sources).
func (c *checker) checkIterable(node *yaml.Node, env *cel.Env, expr, full string) {
	checked := c.checkCompile(node, env, expr, full)
	if checked == nil {
		return
	}
	outputType := checked.OutputType()
	kind := outputType.Kind()
	if kind != types.ListKind && kind != types.MapKind && outputType != cel.DynType {
		c.reportAt(c.markerSpan(node, full), CodeCELTypeError,
			fmt.Sprintf("forEach expression must return list or map, got %s", outputType))
	}
}

// checkString locates every ${...} expression in a scalar value and validates
// each one. A '}' that immediately abuts the end of an expression marker is
// reported as a stray brace: it is almost always a typo, and without this check
// the render engine would interpolate the marker and silently leak the extra
// '}' into the output as literal text. A `${` that never reaches a balanced
// close (an unclosed marker, or braces that never balance inside the
// expression) is reported too — FindCELExpressions only surfaces balanced
// markers, so an unclosed one would otherwise be silently ignored.
func (c *checker) checkString(node *yaml.Node, env *cel.Env) {
	matches, err := template.FindCELExpressions(node.Value)
	if err != nil {
		c.reportAt(c.markerSpan(node, node.Value), CodeCELParseError,
			fmt.Sprintf("failed to parse CEL expressions: %v", err))
		return
	}
	offset := 0
	for _, m := range matches {
		idx := strings.Index(node.Value[offset:], m.FullExpr)
		matchStart := offset
		if idx >= 0 {
			matchStart = offset + idx
		}
		if end := matchStart + len(m.FullExpr); end < len(node.Value) && node.Value[end] == '}' {
			c.reportAt(parser.Range{
				Start: c.valueOffsetToPos(node, matchStart),
				End:   c.valueOffsetToPos(node, end),
			}, CodeCELParseError,
				"stray '}' after CEL expression (did you add an extra brace?)")
		}
		if idx >= 0 {
			offset = matchStart + len(m.FullExpr)
		}
		c.checkAny(node, env, m.InnerExpr, m.FullExpr)
	}
	if rest := strings.Index(node.Value[offset:], "${"); rest >= 0 {
		at := offset + rest
		c.reportAt(parser.Range{
			Start: c.valueOffsetToPos(node, at),
			End:   c.valueLineEndPos(node, at),
		}, CodeCELParseError,
			"unclosed CEL expression marker (missing closing '}')")
	}
}

// walk recursively validates every scalar in a template body, including
// dynamic (${...}) mapping keys.
func (c *checker) walk(node *yaml.Node, env *cel.Env) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.ScalarNode:
		c.checkString(node, env)
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			if strings.Contains(key.Value, "${") {
				c.checkString(key, env)
			}
			c.walk(node.Content[i+1], env)
		}
	case yaml.SequenceNode:
		for _, item := range node.Content {
			c.walk(item, env)
		}
	}
}

// templateBool validates a field whose whole value must be a single
// ${...} boolean expression (includeWhen, readyWhen, rule, when).
func (c *checker) templateBool(node *yaml.Node, env *cel.Env, what string) {
	if node == nil {
		return
	}
	inner, ok := extractTemplate(node.Value)
	if !ok {
		c.reportAt(c.markerSpan(node, node.Value), CodeCELParseError,
			fmt.Sprintf("%s must be a template expression wrapped in ${...}", what))
		return
	}
	c.checkBoolean(node, env, inner, node.Value)
}

// includeWhen validates an includeWhen field (optional) against the given env.
func (c *checker) includeWhen(node *yaml.Node, env *cel.Env) {
	c.templateBool(node, env, fieldIncludeWhen)
}

// withResource returns an env extended with the `resource` variable. Resource
// bindings are structurally dynamic; callers expect only that the name exists.
func withResource(env *cel.Env) *cel.Env {
	if env == nil {
		return nil
	}
	ext, err := env.Extend(cel.Variable("resource", cel.DynType))
	if err != nil {
		return env
	}
	return ext
}

// where validates an (optional) patch-target where filter. Unlike other CEL
// fields, `resource` is in scope and the value may legally appear without
// ${...} wrapping (the webhook accepts both).
func (c *checker) where(node *yaml.Node, env *cel.Env) {
	c.whereResource(node, withResource(env))
}

func (c *checker) whereResource(node *yaml.Node, env *cel.Env) {
	if node == nil {
		return
	}
	expr := node.Value
	if inner, ok := extractTemplate(node.Value); ok {
		expr = inner
	}
	c.checkBoolean(node, env, expr, node.Value)
}

// forEachEnv validates a forEach field and, when it can, returns an env that
// binds the loop variable. A nil return means there is no forEach to handle (or
// it could not be extended); the caller keeps the base env.
func (c *checker) forEachEnv(env *cel.Env, provider *apiservercel.DeclTypeProvider, forEach, varNode *yaml.Node) *cel.Env {
	if forEach == nil {
		return nil
	}
	inner, ok := extractTemplate(forEach.Value)
	if !ok {
		c.reportAt(c.markerSpan(forEach, forEach.Value), CodeCELParseError,
			"forEach must be a template expression wrapped in ${...}")
		return nil
	}
	c.checkIterable(forEach, env, inner, forEach.Value)

	info, err := analyzeForEachExpression(inner, scalarString(varNode), env)
	if err != nil && !strings.Contains(err.Error(), "type check") {
		c.reportAt(c.markerSpan(forEach, forEach.Value), CodeCELTypeError,
			fmt.Sprintf("failed to analyze forEach: %v", err))
		return nil
	}
	if info == nil {
		return nil
	}

	ext, err := extendEnvWithForEach(env, info, provider)
	if err != nil {
		c.reportAt(c.markerSpan(forEach, forEach.Value), CodeCELTypeError,
			fmt.Sprintf("failed to extend environment with forEach: %v", err))
		return nil
	}
	return ext
}

// resourceEntry validates one resource template holder (component resources and
// trait creates): includeWhen with the base env before forEach binds the loop
// variable, then the template body under the (possibly extended) env.
func (c *checker) resourceEntry(env *cel.Env, provider *apiservercel.DeclTypeProvider, entry *yaml.Node) {
	c.includeWhen(mappingValue(entry, fieldIncludeWhen), env)
	bodyEnv := env
	if ext := c.forEachEnv(env, provider, mappingValue(entry, fieldForEach), mappingValue(entry, fieldVar)); ext != nil {
		bodyEnv = ext
	}
	c.walk(mappingValue(entry, fieldTemplate), bodyEnv)
}

// validationRules validates spec.validations / spec.preRenderValidations rule
// fields (boolean, base env).
func (c *checker) validationRules(seq *yaml.Node, env *cel.Env) {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	for _, entry := range seq.Content {
		c.templateBool(mappingValue(entry, fieldRule), env, fieldRule)
	}
}

// postRenderValidations validates the same fields the webhook applies to a
// post-render validation: when (base env, before forEach), forEach/var,
// target.where and rule (boolean, with `resource` and the loop variable).
func (c *checker) postRenderValidations(seq *yaml.Node, env *cel.Env, provider *apiservercel.DeclTypeProvider) {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	for _, entry := range seq.Content {
		c.templateBool(mappingValue(entry, fieldWhen), env, fieldWhen)
		bodyEnv := env
		if ext := c.forEachEnv(env, provider, mappingValue(entry, fieldForEach), mappingValue(entry, fieldVar)); ext != nil {
			bodyEnv = ext
		}
		target := mappingValue(entry, fieldTarget)
		c.targetNode = target
		resourceEnv := withResolvedTargetResource(bodyEnv, target, c.resolver)
		if target != nil {
			c.whereResource(mappingValue(target, fieldWhere), resourceEnv)
		}
		c.templateBool(mappingValue(entry, fieldRule), resourceEnv, fieldRule)
		c.targetNode = nil
	}
}

// patch validates one trait patch: forEach/var, target.where, and (for add /
// replace) the operation values, plus all operation paths, under the loop environment.
func (c *checker) patch(env *cel.Env, provider *apiservercel.DeclTypeProvider, entry *yaml.Node) {
	bodyEnv := env
	if ext := c.forEachEnv(env, provider, mappingValue(entry, fieldForEach), mappingValue(entry, fieldVar)); ext != nil {
		bodyEnv = ext
	}
	if target := mappingValue(entry, fieldTarget); target != nil {
		c.where(mappingValue(target, fieldWhere), bodyEnv)
	}
	c.operations(mappingValue(entry, fieldOperations), bodyEnv)
}

// operations validates interpolated paths for every operation and add/replace
// values. Paths use the same environment as the production Trait renderer.
func (c *checker) operations(seq *yaml.Node, env *cel.Env) {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	for _, op := range seq.Content {
		c.walk(mappingValue(op, "path"), env)
		switch scalarString(mappingValue(op, fieldOp)) {
		case patchOpAdd, patchOpReplace:
			c.walk(mappingValue(op, fieldValue), env)
		}
	}
}

// validateComponent validates a (Cluster)ComponentType document.
func validateComponent(doc *parser.DocumentNode, pair SchemaPair) ([]Finding, error) {
	built, err := envFor(doc.Kind, pair)
	if err != nil {
		return nil, fmt.Errorf("build CEL env: %w", err)
	}
	env, provider := built.env, built.provider

	c := newChecker(doc)
	spec := specMapping(doc)
	if spec == nil {
		return c.findings, nil
	}

	c.resourceEntries(mappingValue(spec, fieldResources), env, provider)
	c.validationRules(mappingValue(spec, fieldValidations), env)
	c.validationRules(mappingValue(spec, fieldPreRenderValidations), env)
	c.postRenderValidations(mappingValue(spec, fieldPostRenderValidations), env, provider)
	return c.findings, nil
}

// validateTrait validates a (Cluster)Trait document.
func validateTrait(doc *parser.DocumentNode, pair SchemaPair) ([]Finding, error) {
	built, err := envFor(doc.Kind, pair)
	if err != nil {
		return nil, fmt.Errorf("build CEL env: %w", err)
	}
	env, provider := built.env, built.provider

	c := newChecker(doc)
	spec := specMapping(doc)
	if spec == nil {
		return c.findings, nil
	}

	c.resourceEntries(mappingValue(spec, fieldCreates), env, provider)
	c.patchEntries(mappingValue(spec, fieldPatches), env, provider)
	c.patchEntries(mappingValue(spec, fieldRemoves), env, provider)
	c.validationRules(mappingValue(spec, fieldValidations), env)
	c.validationRules(mappingValue(spec, fieldPreRenderValidations), env)
	c.postRenderValidations(mappingValue(spec, fieldPostRenderValidations), env, provider)
	return c.findings, nil
}

// resourceEntries validates a sequence of template holders (component
// resources / trait creates).
func (c *checker) resourceEntries(seq *yaml.Node, env *cel.Env, provider *apiservercel.DeclTypeProvider) {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	for _, entry := range seq.Content {
		c.resourceEntry(env, provider, entry)
	}
}

// patchEntries validates a sequence of trait patches/removes.
func (c *checker) patchEntries(seq *yaml.Node, env *cel.Env, provider *apiservercel.DeclTypeProvider) {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	for _, entry := range seq.Content {
		c.patch(env, provider, entry)
	}
}

// validateResourceType validates a (Cluster)ResourceType document: resource
// templates and includeWhen against the base env, readyWhen and outputs
// against the env-with-applied, and applied.<id> references against the
// declared resources[].id set.
func validateResourceType(doc *parser.DocumentNode, pair SchemaPair) ([]Finding, error) {
	built, err := envFor(doc.Kind, pair)
	if err != nil {
		return nil, fmt.Errorf("build CEL env: %w", err)
	}
	base := built.env
	applied, err := WithApplied(base)
	if err != nil {
		return nil, fmt.Errorf("extend CEL env with applied: %w", err)
	}

	c := newChecker(doc)
	spec := specMapping(doc)
	if spec == nil {
		return c.findings, nil
	}

	res := mappingValue(spec, fieldResources)
	ids := map[string]bool{}
	if res != nil && res.Kind == yaml.SequenceNode {
		for _, entry := range res.Content {
			if id := scalarString(mappingValue(entry, fieldID)); id != "" {
				ids[id] = true
			}
		}
	}
	c.appliedIDs = ids

	if res != nil && res.Kind == yaml.SequenceNode {
		for _, entry := range res.Content {
			c.includeWhen(mappingValue(entry, fieldIncludeWhen), base)
			c.walk(mappingValue(entry, fieldTemplate), base)
			c.templateBool(mappingValue(entry, fieldReadyWhen), applied, fieldReadyWhen)
		}
	}

	c.outputs(mappingValue(spec, fieldOutputs), applied)
	return c.findings, nil
}

// validateWorkflow validates a Workflow or ClusterWorkflow document:
// spec.runTemplate and spec.resources[].template walk the base env,
// resources[].includeWhen is a boolean expression, and externalRefs[].name
// supports CEL expressions. Workflow resources have no forEach/var, so the
// base env applies throughout. Mirrors
// internal/pipeline/workflow.(*Pipeline).BuildCELContext's declared surface.
func validateWorkflow(doc *parser.DocumentNode, pair SchemaPair) ([]Finding, error) {
	built, err := envFor(doc.Kind, pair)
	if err != nil {
		return nil, fmt.Errorf("build CEL env: %w", err)
	}
	env := built.env

	c := newChecker(doc)
	spec := specMapping(doc)
	if spec == nil {
		return c.findings, nil
	}

	c.walk(mappingValue(spec, fieldRunTemplate), env)
	c.workflowResources(mappingValue(spec, fieldResources), env)
	c.externalRefs(mappingValue(spec, fieldExternalRefs), env)
	return c.findings, nil
}

// workflowResources validates a sequence of WorkflowResource holders.
func (c *checker) workflowResources(seq *yaml.Node, env *cel.Env) {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	for _, entry := range seq.Content {
		c.includeWhen(mappingValue(entry, fieldIncludeWhen), env)
		c.walk(mappingValue(entry, fieldTemplate), env)
	}
}

// externalRefs validates the CEL-supporting `name` field of each externalRef
// declaration; id/apiVersion/kind are plain strings.
func (c *checker) externalRefs(seq *yaml.Node, env *cel.Env) {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	for _, entry := range seq.Content {
		c.walk(mappingValue(entry, fieldName), env)
	}
}

// outputs validates value / secretKeyRef / configMapKeyRef template strings.
func (c *checker) outputs(seq *yaml.Node, env *cel.Env) {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	for _, out := range seq.Content {
		if v := mappingValue(out, fieldValue); v != nil {
			c.walk(v, env)
		}
		for _, ref := range []string{fieldSecretKeyRef, fieldConfigMapKeyRef} {
			refNode := mappingValue(out, ref)
			if refNode == nil {
				continue
			}
			c.walk(mappingValue(refNode, fieldName), env)
			c.walk(mappingValue(refNode, fieldKey), env)
		}
	}
}

// checkAppliedReferences walks a checked AST and reports any applied.<id>
// reference whose <id> is not in c.appliedIDs. Both applied.<ident>
// (SelectKind) and applied["<lit>"] (CallKind with the index operator) are
// recognized. Mirrors internal/validation/resource.resourcetype.go.
func (c *checker) checkAppliedReferences(checked *cel.Ast, node *yaml.Node) {
	expr := checked.NativeRep().Expr()
	if expr == nil {
		return
	}

	var undeclared []string
	seen := make(map[string]bool)

	visitor := celast.NewExprVisitor(func(e celast.Expr) {
		var id string

		switch e.Kind() {
		case celast.SelectKind:
			sel := e.AsSelect()
			operand := sel.Operand()
			if operand.Kind() == celast.IdentKind && operand.AsIdent() == "applied" {
				id = sel.FieldName()
			}
		case celast.CallKind:
			call := e.AsCall()
			if call.FunctionName() != operators.Index {
				return
			}
			args := call.Args()
			if len(args) != 2 {
				return
			}
			target := args[0]
			key := args[1]
			if target.Kind() != celast.IdentKind || target.AsIdent() != "applied" {
				return
			}
			if key.Kind() != celast.LiteralKind {
				// applied[someVar] — cannot validate at compile time; skip.
				return
			}
			lit, ok := key.AsLiteral().Value().(string)
			if !ok {
				return
			}
			id = lit
		default:
			return
		}

		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		if !c.appliedIDs[id] {
			undeclared = append(undeclared, id)
		}
	})
	celast.PostOrderVisit(expr, visitor)

	if len(undeclared) == 0 {
		return
	}
	sort.Strings(undeclared)
	c.reportAt(c.markerSpan(node, node.Value), CodeCELUnknownName,
		fmt.Sprintf("applied.<id> references unknown ids %v; declared ids: %v", undeclared, sortKeys(c.appliedIDs)))
}

// addIssues emits a finding for the first issue reported by the CEL parser or
// checker. The range covers the offending token (identifier/dotted path, or a
// call and its arguments) resolved to absolute document coordinates; parse
// errors and fallback cases span the whole ${...} marker instead.
func (c *checker) addIssues(node *yaml.Node, code string, issues *cel.Issues, full, expr string) {
	whole := c.markerSpan(node, full)
	errs := issues.Errors()
	if len(errs) == 0 {
		c.reportAt(whole, code,
			fmt.Sprintf("invalid CEL expression %q: %s", expr, issues.Err()))
		return
	}
	e := errs[0]
	message := e.Message
	if code == CodeCELTypeError || code == CodeCELUnknownName {
		if hint := c.targetFieldMessage(expr, e.Message, exprOffset(expr, e.Location)); hint != "" {
			message = hint
		} else if hint := fieldSuggestion(c.currentEnv, expr, e.Message, exprOffset(expr, e.Location)); hint != "" {
			message = hint
		}
	}

	if code != CodeCELParseError {
		token := ""
		if match := undefinedTargetField.FindStringSubmatch(e.Message); len(match) == 2 {
			token = match[1]
		}
		if code == CodeCELUnknownName {
			if match := undeclaredToken.FindStringSubmatch(e.Message); len(match) == 2 {
				token = match[1]
			}
		}
		if token != "" {
			if exact, ok := c.exactScalarToken(node, token); ok {
				c.reportAt(exact, code, fmt.Sprintf("invalid CEL expression %q: %s", expr, message))
				return
			}
		}
	}
	rng := whole
	if code != CodeCELParseError {
		if innerOff := exprOffset(expr, e.Location); innerOff >= 0 {
			startOff := innerOffset(node, expr) + innerOff
			if startOff < len(node.Value) && node.Value[startOff] == '.' {
				startOff++
			}
			start := c.valueOffsetToPos(node, startOff)
			if tokenEnd := celdTokenEnd(node.Value, startOff); tokenEnd > startOff {
				c.reportAt(parser.Range{Start: start, End: c.valueOffsetToPos(node, tokenEnd)}, code,
					fmt.Sprintf("invalid CEL expression %q: %s", expr, message))
				return
			}
			rng = parser.Range{Start: start, End: start}
		}
	}
	c.reportAt(rng, code,
		fmt.Sprintf("invalid CEL expression %q: %s", expr, message))
}

// classifyCELIssues maps the first checker message to a diagnostic code. The
// distinction between unknown names and unknown functions is picked out of the
// message because cel-go exposes no stable error code; everything else falls
// back to a plain type error.
func classifyCELIssues(issues *cel.Issues) string {
	msgs := make([]string, 0, len(issues.Errors()))
	for _, e := range issues.Errors() {
		msgs = append(msgs, e.Message)
	}
	joined := strings.Join(msgs, "\n")
	switch {
	case strings.Contains(joined, "undeclared reference"):
		return CodeCELUnknownName
	case strings.Contains(joined, "no matching overload"), strings.Contains(joined, "no such overload"):
		return CodeCELUnknownFunction
	default:
		return CodeCELTypeError
	}
}

func (c *checker) reportAt(r parser.Range, code, message string) {
	c.findings = append(c.findings, Finding{
		Range:   r,
		Code:    code,
		Message: message,
	})
}

// --- yaml helpers ---------------------------------------------------------

func specMapping(doc *parser.DocumentNode) *yaml.Node {
	if doc == nil || doc.Spec == nil || doc.Spec.Kind != yaml.MappingNode {
		return nil
	}
	return doc.Spec
}

// scalarString returns the value of a scalar node, or "".
func scalarString(n *yaml.Node) string {
	if n == nil || n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.Value
}

// extractTemplate strips a single ${...} wrapper, matching the webhook's
// extractCELFromTemplate semantics.
func extractTemplate(s string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "${") || !strings.HasSuffix(trimmed, "}") {
		return "", false
	}
	return strings.TrimSpace(trimmed[2 : len(trimmed)-1]), true
}

func sortKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

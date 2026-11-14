// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package ruleengine_test

import (
	"strings"
	"testing"

	celvalidator "github.com/openchoreo/openchoreo/tools/lint/cel-validator"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/template"
)

// parse parses a YAML string and returns the first DocumentNode.
// Fails the test if parsing fails or produces no documents.
func parse(t *testing.T, yaml string) *parser.DocumentNode {
	t.Helper()
	docs, err := parser.ParseYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("no documents parsed")
	}
	return docs[0]
}

// eval parses a YAML string and runs the rule engine against the first document.
// Returns all diagnostics produced by the engine.
func eval(t *testing.T, yaml string) ruleengine.Diagnostics {
	t.Helper()
	doc := parse(t, yaml)
	e := ruleengine.NewEngine()
	return e.Evaluate(doc)
}

// hasCode returns true if any diagnostic in the slice has the given code.
func hasCode(diags ruleengine.Diagnostics, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

// --- Naming Rules ---

// TestNaming_ValidName verifies that a valid RFC 1123 resource name
// (lowercase alphanumeric and hyphens, starts/ends with alphanumeric)
// produces no naming errors.
func TestNaming_ValidName(t *testing.T) {
	diags := eval(t, "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  name: my-project\n")
	if hasCode(diags, "name-invalid") {
		t.Errorf("expected no name-invalid error, got %v", diags)
	}
}

// TestNaming_InvalidName_Uppercase verifies that a name containing
// uppercase characters is rejected with a name-invalid error.
func TestNaming_InvalidName_Uppercase(t *testing.T) {
	diags := eval(t, "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  name: My-Project\n")
	if !hasCode(diags, "name-invalid") {
		t.Errorf("expected name-invalid error for uppercase name")
	}
}

// TestNaming_InvalidName_SpecialChars verifies that a name containing
// underscores (or other invalid characters) is rejected.
func TestNaming_InvalidName_SpecialChars(t *testing.T) {
	diags := eval(t, "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  name: my_project\n")
	if !hasCode(diags, "name-invalid") {
		t.Errorf("expected name-invalid error for underscore")
	}
}

// TestNaming_InvalidName_TooLong verifies that a name exceeding 63
// characters is rejected with a name-too-long error.
func TestNaming_InvalidName_TooLong(t *testing.T) {
	longName := "a234567890123456789012345678901234567890123456789012345678901234"
	diags := eval(t, "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  name: "+longName+"\n")
	if !hasCode(diags, "name-too-long") {
		t.Errorf("expected name-too-long error for name > 63 chars")
	}
}

// TestNaming_InvalidName_StartsWithHyphen verifies that a name starting
// with a hyphen is rejected (must start with an alphanumeric character).
func TestNaming_InvalidName_StartsWithHyphen(t *testing.T) {
	diags := eval(t, "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  name: -my-project\n")
	if !hasCode(diags, "name-invalid") {
		t.Errorf("expected name-invalid error for name starting with hyphen")
	}
}

// TestNaming_ValidName_Short verifies that a single-character name is
// accepted as valid (boundary case for the start/end alphanumeric rule).
func TestNaming_ValidName_Short(t *testing.T) {
	diags := eval(t, "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  name: a\n")
	if hasCode(diags, "name-invalid") {
		t.Errorf("expected no name-invalid for single char name")
	}
}

// --- Required Fields Rules ---

// TestRequired_MissingAPIVersion verifies that omitting the apiVersion
// field produces a missing-apiVersion error.
func TestRequired_MissingAPIVersion(t *testing.T) {
	diags := eval(t, "kind: Project\nmetadata:\n  name: my-project\n")
	if !hasCode(diags, "missing-apiVersion") {
		t.Errorf("expected missing-apiVersion error")
	}
}

// TestRequired_MissingKind verifies that omitting the kind field
// produces a missing-kind error.
func TestRequired_MissingKind(t *testing.T) {
	diags := eval(t, "apiVersion: openchoreo.dev/v1alpha1\nmetadata:\n  name: my-project\n")
	if !hasCode(diags, "missing-kind") {
		t.Errorf("expected missing-kind error")
	}
}

// TestRequired_MissingMetadataName verifies that omitting metadata.name
// produces a missing-metadata-name error.
func TestRequired_MissingMetadataName(t *testing.T) {
	diags := eval(t, "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  namespace: default\n")
	if !hasCode(diags, "missing-metadata-name") {
		t.Errorf("expected missing-metadata-name error")
	}
}

// TestRequired_AllPresent verifies that a document with all required
// fields (apiVersion, kind, metadata.name) produces no required-field errors.
func TestRequired_AllPresent(t *testing.T) {
	diags := eval(t, "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  name: my-project\n")
	errs := diags.Errors()
	for _, e := range errs {
		if e.Code == "missing-apiVersion" || e.Code == "missing-kind" || e.Code == "missing-metadata-name" {
			t.Errorf("unexpected required-field error: %s", e.Code)
		}
	}
}

// --- Enum Rules ---

// TestEnum_InvalidAPIVersion verifies that an unrecognized apiVersion
// (e.g., "custom.io/v1") produces an invalid-apiVersion error.
func TestEnum_InvalidAPIVersion(t *testing.T) {
	diags := eval(t, "apiVersion: custom.io/v1\nkind: Project\nmetadata:\n  name: my-project\n")
	if !hasCode(diags, "invalid-apiVersion") {
		t.Errorf("expected invalid-apiVersion error")
	}
}

// TestEnum_ValidAPIVersion verifies that the standard OpenChoreo
// apiVersion "openchoreo.dev/v1alpha1" produces no enum errors.
func TestEnum_ValidAPIVersion(t *testing.T) {
	diags := eval(t, "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  name: my-project\n")
	if hasCode(diags, "invalid-apiVersion") {
		t.Errorf("unexpected invalid-apiVersion error")
	}
}

// TestEnum_UnknownKind verifies that an unrecognized kind produces
// an error.
func TestEnum_UnknownKind(t *testing.T) {
	diags := eval(t, "apiVersion: openchoreo.dev/v1alpha1\nkind: UnknownKind\nmetadata:\n  name: my-project\n")
	if !hasCode(diags, "unknown-kind") {
		t.Errorf("expected unknown-kind error")
	}
}

// TestEnum_KnownKind verifies that "Component" (a known OpenChoreo kind)
// produces no unknown-kind warning.
func TestEnum_KnownKind(t *testing.T) {
	diags := eval(t, "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata:\n  name: my-component\n")
	if hasCode(diags, "unknown-kind") {
		t.Errorf("unexpected unknown-kind warning for Component")
	}
}

// TestEnum_AllKnownKinds verifies that all recognized OpenChoreo kinds
// pass validation without producing unknown-kind warnings.
func TestEnum_AllKnownKinds(t *testing.T) {
	for _, k := range template.AllKinds() {
		diags := eval(t, "apiVersion: openchoreo.dev/v1alpha1\nkind: "+k+"\nmetadata:\n  name: test\n")
		if hasCode(diags, "unknown-kind") {
			t.Errorf("unexpected unknown-kind for %s", k)
		}
	}
}

// --- Reference Rules ---

// TestReferences_ComponentType_MissingKind verifies that a componentType
// reference without a kind field produces a missing-ref-kind error.
func TestReferences_ComponentType_MissingKind(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-comp
spec:
  componentType:
    name: deployment/service
`
	diags := eval(t, yaml)
	if !hasCode(diags, "missing-ref-kind") {
		t.Errorf("expected missing-ref-kind error for componentType")
	}
}

// TestReferences_ComponentType_MissingName verifies that a componentType
// reference without a name field produces a missing-ref-name error.
func TestReferences_ComponentType_MissingName(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-comp
spec:
  componentType:
    kind: ClusterComponentType
`
	diags := eval(t, yaml)
	if !hasCode(diags, "missing-ref-name") {
		t.Errorf("expected missing-ref-name error for componentType")
	}
}

// TestReferences_ComponentType_Valid verifies that a complete componentType
// reference with both kind and name produces no reference errors.
func TestReferences_ComponentType_Valid(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-comp
spec:
  componentType:
    kind: ClusterComponentType
    name: deployment/service
`
	diags := eval(t, yaml)
	if hasCode(diags, "missing-ref-kind") || hasCode(diags, "missing-ref-name") {
		t.Errorf("unexpected reference errors for valid componentType")
	}
}

// TestReferences_DeploymentPipelineRef_Missing verifies that an empty
// deploymentPipelineRef (no name) produces a missing-ref-name error.
func TestReferences_DeploymentPipelineRef_Missing(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: my-project
spec:
  deploymentPipelineRef: {}
`
	diags := eval(t, yaml)
	if !hasCode(diags, "missing-ref-name") {
		t.Errorf("expected missing-ref-name for empty deploymentPipelineRef")
	}
}

// TestReferences_DeploymentPipelineRef_Valid verifies that a valid
// deploymentPipelineRef with a name produces no reference errors.
func TestReferences_DeploymentPipelineRef_Valid(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: my-project
spec:
  deploymentPipelineRef:
    name: default
`
	diags := eval(t, yaml)
	if hasCode(diags, "missing-ref-name") {
		t.Errorf("unexpected missing-ref-name for valid deploymentPipelineRef")
	}
}

// --- Engine Integration ---

// TestEngine_EvaluateAll verifies that EvaluateAll processes multiple
// documents in a single call and returns combined diagnostics.
func TestEngine_EvaluateAll(t *testing.T) {
	docs, err := parser.ParseYAML([]byte("apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  name: proj-a\nspec: {}\n---\napiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata:\n  name: comp-a\nspec: {}\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	e := ruleengine.NewEngine()
	diags := e.EvaluateAll(docs)

	if diags.HasErrors() {
		t.Errorf("expected no errors for valid docs, got %v", diags.Errors())
	}
}

// TestEngine_Evaluate_MultipleErrors verifies that a document missing
// apiVersion produces at least the missing-apiVersion error.
func TestEngine_Evaluate_MultipleErrors(t *testing.T) {
	yaml := "kind: Project\nmetadata:\n  name: my-project\n"
	diags := eval(t, yaml)

	errs := diags.Errors()
	if len(errs) < 1 {
		t.Errorf("expected at least 1 error (missing apiVersion), got %d", len(errs))
	}

	if !hasCode(diags, "missing-apiVersion") {
		t.Error("expected missing-apiVersion")
	}
}

// TestEngine_Evaluate_CompleteValidDoc verifies that a fully valid
// Component document with all required fields and references produces
// zero errors.
func TestEngine_Evaluate_CompleteValidDoc(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-service
spec:
  componentType:
    kind: ClusterComponentType
    name: deployment/service
  workflow:
    kind: ClusterWorkflow
    name: dockerfile-builder
  deploymentPipelineRef:
    name: default
`
	diags := eval(t, yaml)
	errs := diags.Errors()
	if len(errs) > 0 {
		t.Errorf("expected no errors for complete valid doc, got %v", errs)
	}
}

// --- Diagnostics Helpers ---

// TestDiagnostics_Errors verifies that the Errors() helper filters
// diagnostics to only return those with SeverityError.
func TestDiagnostics_Errors(t *testing.T) {
	diags := ruleengine.Diagnostics{
		{Severity: ruleengine.SeverityError, Code: "e1"},
		{Severity: ruleengine.SeverityWarning, Code: "w1"},
		{Severity: ruleengine.SeverityError, Code: "e2"},
	}

	errs := diags.Errors()
	if len(errs) != 2 {
		t.Errorf("expected 2 errors, got %d", len(errs))
	}
}

// TestDiagnostics_Warnings verifies that the Warnings() helper filters
// diagnostics to only return those with SeverityWarning.
func TestDiagnostics_Warnings(t *testing.T) {
	diags := ruleengine.Diagnostics{
		{Severity: ruleengine.SeverityError, Code: "e1"},
		{Severity: ruleengine.SeverityWarning, Code: "w1"},
		{Severity: ruleengine.SeverityWarning, Code: "w2"},
	}

	warns := diags.Warnings()
	if len(warns) != 2 {
		t.Errorf("expected 2 warnings, got %d", len(warns))
	}
}

// TestDiagnostics_HasErrors verifies that HasErrors() returns false
// when only warnings are present, and true after an error is added.
func TestDiagnostics_HasErrors(t *testing.T) {
	diags := ruleengine.Diagnostics{
		{Severity: ruleengine.SeverityWarning, Code: "w1"},
	}
	if diags.HasErrors() {
		t.Error("expected no errors")
	}

	diags = append(diags, ruleengine.Diagnostic{Severity: ruleengine.SeverityError, Code: "e1"})
	if !diags.HasErrors() {
		t.Error("expected errors")
	}
}

// --- Rule Template Integration ---

// evalWithKindRules parses YAML, builds an engine with both common and
// kind-specific rules registered, and returns diagnostics.
func evalWithKindRules(t *testing.T, yamlStr string) ruleengine.Diagnostics {
	t.Helper()
	doc := parse(t, yamlStr)
	e := ruleengine.NewEngine()
	for _, k := range template.AllKinds() {
		e.AddKindRules(k, template.RulesForKind(k))
	}
	return e.Evaluate(doc)
}

// validProjectYAML is a minimal valid Project document used in kind-specific tests.
const validProjectYAML = `apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: my-project
spec:
  deploymentPipelineRef:
    kind: DeploymentPipeline
    name: default
  type:
    kind: ProjectType
    name: basic
`

// validComponentYAML is a minimal valid Component document used in kind-specific tests.
const validComponentYAML = `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-component
  namespace: default
  uid: 95ff883c-b99c-45bc-96bd-7bcd573a7a41
  creationTimestamp: 2026-07-23T10:09:04Z
  labels:
    openchoreo.dev/project: default
  annotations:
    openchoreo.dev/description: test
    openchoreo.dev/display-name: Test
spec:
  componentType:
    kind: ComponentType
    name: deployment/web-app
  owner:
    projectName: default
  parameters: {}
  workflow:
    kind: Workflow
    name: default
    parameters: {}
status:
  conditions:
    - lastTransitionTime: 2026-07-23T10:09:04Z
      message: Component is created
      observedGeneration: 1
      reason: ComponentCreated
      status: "True"
      type: Created
  observedGeneration: 1
`

// TestKindRules_Project_Valid verifies that a valid Project document
// produces no project-specific errors.
func TestKindRules_Project_Valid(t *testing.T) {
	diags := evalWithKindRules(t, validProjectYAML)
	if hasCode(diags, "missing-spec") {
		t.Errorf("unexpected missing-spec for valid Project")
	}
	if hasCode(diags, "missing-deploymentPipelineRef") {
		t.Errorf("unexpected missing-deploymentPipelineRef for valid Project")
	}
	if hasCode(diags, "missing-type") {
		t.Errorf("unexpected missing-type for valid Project")
	}
}

// TestKindRules_Environment_Valid verifies that a valid Environment document
// produces no environment-specific errors.
func TestKindRules_Environment_Valid(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Environment
metadata:
  name: development
  namespace: default
  annotations:
    openchoreo.dev/description: Development environment
    openchoreo.dev/display-name: Development
spec:
  dataPlaneRef:
    kind: ClusterDataPlane
    name: default
  isProduction: false
`
	diags := evalWithKindRules(t, yaml)
	if hasCode(diags, "unknown-field") {
		t.Errorf("unexpected unknown-field for valid Environment: %v", diags)
	}
	if hasCode(diags, "missing-environment.spec") {
		t.Errorf("unexpected missing-spec for valid Environment")
	}
	if hasCode(diags, "missing-environment.spec.dataPlaneRef") {
		t.Errorf("unexpected missing-dataPlaneRef for valid Environment")
	}
	if hasCode(diags, "missing-environment.spec.isProduction") {
		t.Errorf("unexpected missing-isProduction for valid Environment")
	}
}

// TestKindRules_Component_Valid verifies that a valid Component document
// produces no component-specific errors.
func TestKindRules_Component_Valid(t *testing.T) {
	diags := evalWithKindRules(t, validComponentYAML)
	if hasCode(diags, "unknown-field") {
		t.Errorf("unexpected unknown-field for valid Component: %v", diags)
	}
	if hasCode(diags, "missing-component.spec") {
		t.Errorf("unexpected missing-spec for valid Component")
	}
	if hasCode(diags, "missing-component.spec.owner") {
		t.Errorf("unexpected missing-owner for valid Component")
	}
	if hasCode(diags, "missing-component.spec.workflow") {
		t.Errorf("unexpected missing-workflow for valid Component")
	}
}

// TestKindRules_Workload_RejectsUnknownField verifies that a Workload
// document with an unknown field produces an unknown-field error.
func TestKindRules_Workload_RejectsUnknownField(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: my-workload
  namespace: default
  annotations:
    openchoreo.dev/description: test
spec:
  bogusField: true
`
	diags := evalWithKindRules(t, yaml)
	if !hasCode(diags, "unknown-field") {
		t.Errorf("expected unknown-field diagnostic for Workload, got %v", diags)
	}
}

// TestKindRules_AllKinds_RejectUnknownFields verifies that every known kind
// runs real schema validation and rejects an unknown top-level field.
func TestKindRules_AllKinds_RejectUnknownFields(t *testing.T) {
	for _, kind := range template.AllKinds() {
		yaml := "apiVersion: openchoreo.dev/v1alpha1\nkind: " + kind + "\nmetadata:\n  name: test\n  bogusMeta: 1\n"
		diags := evalWithKindRules(t, yaml)
		if !hasCode(diags, "unknown-field") {
			t.Errorf("kind %s: expected unknown-field diagnostic, got %v", kind, diags)
		}
	}
}

// TestKindRules_UnknownKind_NoKindDiagnostics verifies that an unknown kind
// does not produce any kind-specific diagnostics.
func TestKindRules_UnknownKind_NoKindDiagnostics(t *testing.T) {
	diags := evalWithKindRules(t, "apiVersion: openchoreo.dev/v1alpha1\nkind: UnknownKind\nmetadata:\n  name: test\n")
	for _, d := range diags {
		if d.Code == "unknown-field" || strings.HasPrefix(d.Code, "missing-") {
			t.Errorf("unexpected kind-specific diagnostic for unknown kind: %v", d)
		}
	}
}

// TestKindRules_CommonPlusKindRules verifies that common rules and
// kind-specific rules both run in a single Evaluate call.
func TestKindRules_CommonPlusKindRules(t *testing.T) {
	diags := evalWithKindRules(t, validProjectYAML)

	// Common rules should NOT produce name-invalid (name is valid).
	if hasCode(diags, "name-invalid") {
		t.Errorf("unexpected name-invalid from common rules")
	}

	// Kind-specific rules should NOT produce project errors for valid YAML.
	if hasCode(diags, "missing-spec") {
		t.Errorf("unexpected missing-spec from kind-specific rules")
	}
	if hasCode(diags, "missing-deploymentPipelineRef") {
		t.Errorf("unexpected missing-deploymentPipelineRef from kind-specific rules")
	}
	if hasCode(diags, "missing-type") {
		t.Errorf("unexpected missing-type from kind-specific rules")
	}
}

// TestKindRules_CommonErrorsStillPresent verifies that common rule errors
// are still reported alongside kind-specific diagnostics.
func TestKindRules_CommonErrorsStillPresent(t *testing.T) {
	diags := evalWithKindRules(t, "kind: Project\nmetadata:\n  name: my-project\nspec: {}\n")

	if !hasCode(diags, "missing-apiVersion") {
		t.Errorf("expected missing-apiVersion from common rules")
	}
	// Project kind-specific rules should also fire and report missing required
	// fields within spec (spec itself is optional per the API definition).
	if !hasCode(diags, "missing-project.spec.deploymentPipelineRef") {
		t.Errorf("expected missing-project.spec.deploymentPipelineRef from kind-specific rules, got %v", diags)
	}
}

// TestKindRules_NoKindRulesForUnknown verifies that RulesForKind returns
// nil for unknown kinds, and AddKindRules with nil is safe.
func TestKindRules_NoKindRulesForUnknown(t *testing.T) {
	rules := template.RulesForKind("UnknownKind")
	if rules != nil {
		t.Errorf("expected nil rules for unknown kind, got %v", rules)
	}
}

// TestCELRules_TypeChecksTemplateExpressions verifies the CEL rule surfaces
// schema-aware findings for CEL-bearing kinds through the engine wiring.
func TestCELRules_TypeChecksTemplateExpressions(t *testing.T) {
	doc := parse(t, `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: svc
spec:
  resources:
    - id: deployment
      template:
        metadata:
          name: ${metadata.nmae}
`)

	rule := &ruleengine.CELRules{}
	diags := rule.Evaluate(doc)

	if !hasCode(diags, "cel-type-error") {
		t.Fatalf("expected a cel-type-error diagnostic, got %v", diags)
	}
	for _, d := range diags {
		if d.Severity != ruleengine.SeverityError {
			t.Errorf("CEL diagnostics must be errors, got %v", d)
		}
	}
}

// TestCELRules_NoopOutsideCELKinds verifies the CEL rule ignores documents of
// kinds that carry no template expressions, so it can be registered broadly.
func TestCELRules_NoopOutsideCELKinds(t *testing.T) {
	doc := parse(t, `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-app
spec:
  componentType:
    name: deployment/service
`)
	rule := &ruleengine.CELRules{}
	if diags := rule.Evaluate(doc); len(diags) != 0 {
		t.Fatalf("expected no diagnostics for a Component, got %v", diags)
	}
}

// --- Runtime Rules ---

// TestRuntime_NegativeReplicas verifies that a negative replica count under
// spec.parameters is reported.
func TestRuntime_NegativeReplicas(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: nats
spec:
  parameters:
    replicas: -1
    port: 4222
`
	diags := eval(t, yaml)
	if !hasCode(diags, "negative-value") {
		t.Errorf("expected negative-value error for replicas: -1, got %v", diags)
	}
}

// TestRuntime_NegativePort verifies that a negative port number under
// spec.parameters is reported.
func TestRuntime_NegativePort(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: nats
spec:
  parameters:
    replicas: 1
    port: -80
`
	diags := eval(t, yaml)
	if !hasCode(diags, "negative-value") {
		t.Errorf("expected negative-value error for port: -80, got %v", diags)
	}
}

// TestRuntime_PositiveValues verifies that valid non-negative replicas and
// port values produce no numeric errors.
func TestRuntime_PositiveValues(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: nats
spec:
  parameters:
    replicas: 1
    port: 4222
`
	diags := eval(t, yaml)
	if hasCode(diags, "negative-value") {
		t.Errorf("unexpected negative-value error for positive values, got %v", diags)
	}
}

// TestRuntime_ZeroAllowed verifies that the boundary value zero is accepted.
func TestRuntime_ZeroAllowed(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: nats
spec:
  parameters:
    replicas: 0
`
	diags := eval(t, yaml)
	if hasCode(diags, "negative-value") {
		t.Errorf("unexpected negative-value error for replicas: 0, got %v", diags)
	}
}

// TestRuntime_NestedParameters verifies that negative values are detected in
// nested parameter blocks (for example trait parameters), not only at the top
// level of spec.parameters.
func TestRuntime_NestedParameters(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: nats
spec:
  traits:
    - name: autoscaler
      parameters:
        replicas: -3
`
	diags := eval(t, yaml)
	if !hasCode(diags, "negative-value") {
		t.Errorf("expected negative-value error for nested replicas: -3, got %v", diags)
	}
}

// TestRuntime_TemplatedValueIgnored verifies that a templated expression is
// never treated as a negative number.
func TestRuntime_TemplatedValueIgnored(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: nats
spec:
  parameters:
    replicas: ${environmentConfigs.replicas}
`
	diags := eval(t, yaml)
	if hasCode(diags, "negative-value") {
		t.Errorf("unexpected negative-value error for templated value, got %v", diags)
	}
}

// TestRuntime_ReasonMentionsField verifies the message names the offending
// field so the diagnostic is actionable.
func TestRuntime_ReasonMentionsField(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: nats
spec:
  parameters:
    port: -1
`
	diags := eval(t, yaml)
	for _, d := range diags {
		if d.Code == "negative-value" {
			if !strings.Contains(d.Message, "port") {
				t.Errorf("expected message to mention the field name, got %q", d.Message)
			}
			return
		}
	}
	t.Errorf("expected a negative-value diagnostic, got %v", diags)
}

// TestRuntime_FloatRejected verifies that a fractional value is rejected even
// though it is numeric.
func TestRuntime_FloatRejected(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: nats
spec:
  parameters:
    replicas: 1.5
`
	diags := eval(t, yaml)
	if !hasCode(diags, "invalid-integer") {
		t.Errorf("expected invalid-integer error for replicas: 1.5, got %v", diags)
	}
}

// TestRuntime_StringRejected verifies that a quoted number is rejected: the
// field must be an integer, not a string.
func TestRuntime_StringRejected(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: nats
spec:
  parameters:
    port: "8080"
`
	diags := eval(t, yaml)
	if !hasCode(diags, "invalid-integer") {
		t.Errorf("expected invalid-integer error for port: \"8080\", got %v", diags)
	}
}

// TestRuntime_BoolRejected verifies that a boolean value is rejected.
func TestRuntime_BoolRejected(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: nats
spec:
  parameters:
    replicas: true
`
	diags := eval(t, yaml)
	if !hasCode(diags, "invalid-integer") {
		t.Errorf("expected invalid-integer error for replicas: true, got %v", diags)
	}
}

// TestRuntime_ListRejected verifies that a sequence value is rejected.
func TestRuntime_ListRejected(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: nats
spec:
  parameters:
    replicas:
      - 1
      - 2
`
	diags := eval(t, yaml)
	if !hasCode(diags, "invalid-integer") {
		t.Errorf("expected invalid-integer error for a list, got %v", diags)
	}
}

// TestRuntime_NullRejected verifies that an empty (null) value is rejected.
func TestRuntime_NullRejected(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: nats
spec:
  parameters:
    port:
`
	diags := eval(t, yaml)
	if !hasCode(diags, "invalid-integer") {
		t.Errorf("expected invalid-integer error for null port, got %v", diags)
	}
}

// TestRuntime_ComponentTypeSchemaIgnored verifies that a ComponentType's
// openAPIV3Schema definition of replicas (a mapping with type/default) is not
// mistaken for a literal value.
func TestRuntime_ComponentTypeSchemaIgnored(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: service
spec:
  environmentConfigs:
    openAPIV3Schema:
      type: object
      properties:
        replicas:
          type: integer
          default: 1
  resources:
    - id: deployment
      template:
        spec:
          replicas: ${environmentConfigs.replicas}
`
	diags := eval(t, yaml)
	if hasCode(diags, "invalid-integer") || hasCode(diags, "negative-value") {
		t.Errorf("unexpected numeric error for ComponentType schema/template, got %v", diags)
	}
}

// TestRuntime_ParameterSchemaIgnored verifies that a ComponentType's
// spec.parameters openAPIV3Schema definition is skipped.
func TestRuntime_ParameterSchemaIgnored(t *testing.T) {
	yaml := `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: service
spec:
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        replicas:
          type: integer
`
	diags := eval(t, yaml)
	if hasCode(diags, "invalid-integer") || hasCode(diags, "negative-value") {
		t.Errorf("unexpected numeric error for parameters schema, got %v", diags)
	}
}

// TestKindRules_SelectorReturnsCorrectCount verifies that RulesForKind
// returns the schema rule for every known kind, plus the CEL rule for the
// CEL-bearing kinds.
func TestKindRules_SelectorReturnsCorrectCount(t *testing.T) {
	celKinds := map[string]bool{
		celvalidator.KindComponentType:        true,
		celvalidator.KindClusterComponentType: true,
		celvalidator.KindTrait:                true,
		celvalidator.KindClusterTrait:         true,
		celvalidator.KindResourceType:         true,
		celvalidator.KindClusterResourceType:  true,
		celvalidator.KindWorkflow:             true,
		celvalidator.KindClusterWorkflow:      true,
	}
	for _, k := range template.AllKinds() {
		rules := template.RulesForKind(k)
		want := 1
		if celKinds[k] {
			want = 2
		}
		if len(rules) != want {
			t.Errorf("kind %s: expected %d rules, got %d", k, want, len(rules))
		}
	}
}

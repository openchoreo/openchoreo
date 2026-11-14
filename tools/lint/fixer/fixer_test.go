// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package fixer_test

import (
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/tools/lint/fixer"
)

// fixSource runs a fix over src and fails the test on a parse error.
func fixSource(t *testing.T, src string, opts fixer.Options) fixer.Result {
	t.Helper()
	res, err := fixer.Fix([]byte(src), opts)
	if err != nil {
		t.Fatalf("Fix: %v", err)
	}
	return res
}

func appliedCodes(res fixer.Result) []string {
	out := make([]string, 0, len(res.Applied))
	for _, a := range res.Applied {
		out = append(out, a.Code)
	}
	return out
}

func appliedMessages(res fixer.Result) []string {
	out := make([]string, 0, len(res.Applied))
	for _, a := range res.Applied {
		out = append(out, a.Message)
	}
	return out
}

const typoProject = `apiVersion: openchoreo.dev/v1alpha1
kind: Projectf
metadata:
  name: doclet
spec:
  deploymentPipelinveRef:
    kind: DeploymentPipelin
    name: standard
  type:
    kind: ProjectType
    name: doclet
`

// TestFix_FieldAndKindTypos verifies a misspelled kind and its misspelled spec
// fields are all corrected, and that the follow-up pass picks up fields that
// only the corrected kind's schema reveals.
func TestFix_FieldAndKindTypos(t *testing.T) {
	res := fixSource(t, typoProject, fixer.Options{})

	want := "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  name: doclet\nspec:\n  deploymentPipelineRef:\n    kind: DeploymentPipeline\n    name: standard\n  type:\n    kind: ProjectType\n    name: doclet\n"
	if got := string(res.Data); got != want {
		t.Fatalf("fixed content:\n%q\nwant:\n%q", got, want)
	}
	if got := appliedCodes(res); len(got) != 3 {
		t.Fatalf("applied codes = %v, want 3 fixes (kind, field, ref kind)", got)
	}
	joined := strings.Join(appliedMessages(res), "\n")
	for _, want := range []string{`fixed kind "Projectf" -> "Project"`, `fixed field spec.deploymentPipelinveRef "deploymentPipelinveRef" -> "deploymentPipelineRef"`, `fixed value for spec.deploymentPipelineRef.kind "DeploymentPipelin" -> "DeploymentPipeline"`} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing applied message %q in:\n%s", want, joined)
		}
	}
}

// TestFix_NoChangeLeavesContentIdentical verifies a clean file is returned
// byte for byte and reports no repair.
func TestFix_NoChangeLeavesContentIdentical(t *testing.T) {
	src := "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  name: doclet\nspec:\n  type:\n    kind: ProjectType\n    name: doclet\n  deploymentPipelineRef:\n    kind: DeploymentPipeline\n    name: standard\n"
	res := fixSource(t, src, fixer.Options{})
	if got := string(res.Data); got != src {
		t.Errorf("content changed: %q", got)
	}
	if len(res.Applied) != 0 {
		t.Errorf("expected no applied fixes, got %v", appliedMessages(res))
	}
}

// TestFix_KindTypoWithoutSchema verifies a misspelled kind is repaired even when
// nothing in the document identifies the intended kind, because the candidate
// set is the closed list of registered kinds.
func TestFix_KindTypoWithoutSchema(t *testing.T) {
	src := `apiVersion: openchoreo.dev/v1alpha1
kind: Projectf
metadata:
  name: doclet
`
	res := fixSource(t, src, fixer.Options{})
	if strings.Contains(string(res.Data), "Projectf") {
		t.Errorf("kind not repaired:\n%s", res.Data)
	}
	if !strings.Contains(string(res.Data), "kind: Project\n") {
		t.Errorf("unexpected content:\n%s", res.Data)
	}
}

// TestFix_KindTypoAmbiguousWithoutSchema verifies an ambiguous kind is left alone
// and reported as skipped.
func TestFix_KindTypoAmbiguousWithoutSchema(t *testing.T) {
	src := `apiVersion: openchoreo.dev/v1alpha1
kind: Zzzzzzzzz
`
	res := fixSource(t, src, fixer.Options{})
	if got := string(res.Data); got != src {
		t.Errorf("ambiguous kind was changed:\n%s", got)
	}
	if len(res.Applied) != 0 {
		t.Errorf("applied = %v, want nothing", appliedMessages(res))
	}
}

// TestFix_Idempotent verifies a second run over fixed content changes nothing.
func TestFix_Idempotent(t *testing.T) {
	first := fixSource(t, typoProject, fixer.Options{})
	second := fixSource(t, string(first.Data), fixer.Options{})
	if len(second.Applied) != 0 {
		t.Errorf("second run applied %v, want nothing", appliedMessages(second))
	}
	if got := string(second.Data); got != string(first.Data) {
		t.Errorf("second run changed content")
	}
}

// TestFix_RespectsMaxDistance verifies a token further away than the limit is
// left alone and reported as skipped.
func TestFix_RespectsMaxDistance(t *testing.T) {
	src := `apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: doclet
spec:
  deploymentPipelineReferences:
    name: standard
`
	res := fixSource(t, src, fixer.Options{})
	if string(res.Data) != src {
		t.Errorf("content changed:\n%s", res.Data)
	}
	if len(res.Applied) != 0 {
		t.Errorf("expected no fixes, got %v", appliedMessages(res))
	}
	if len(res.Skipped) == 0 || !strings.Contains(strings.Join(res.Skipped, "\n"), "no schema field within 3 edits") {
		t.Errorf("expected a skip reason, got %v", res.Skipped)
	}
}

// TestFix_AcceptsDistanceThree verifies a token exactly at the limit is fixed.
func TestFix_AcceptsDistanceThree(t *testing.T) {
	src := `apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: doclet
spec:
  parametrs: []
`
	res := fixSource(t, src, fixer.Options{})
	if !strings.Contains(string(res.Data), "parameters: []") {
		t.Errorf("expected parametrs -> parameters, got:\n%s", res.Data)
	}
}

// TestFix_CaseOnlyDifference verifies a casing typo is corrected.
func TestFix_CaseOnlyDifference(t *testing.T) {
	src := `apiVersion: openchoreo.dev/v1alpha1
kind: project
metadata:
  name: doclet
spec:
  type:
    kind: projecttype
    name: doclet
`
	res := fixSource(t, src, fixer.Options{})
	got := string(res.Data)
	if !strings.Contains(got, "kind: Project\n") || !strings.Contains(got, "kind: ProjectType\n") {
		t.Errorf("expected casing fixes, got:\n%s", got)
	}
}

// TestFix_AmbiguousCandidateIsSkipped verifies two equally close candidates
// are never guessed between.
func TestFix_AmbiguousCandidateIsSkipped(t *testing.T) {
	// "status" and "statis" are both one edit from "statt".
	src := `apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: doclet
spec:
  statt: {}
`
	res := fixSource(t, src, fixer.Options{MaxDistance: 1})
	if strings.Contains(string(res.Data), "status: {}") || strings.Contains(string(res.Data), "statis: {}") {
		t.Errorf("ambiguous candidate was guessed:\n%s", res.Data)
	}
	if len(res.Applied) != 0 {
		t.Errorf("expected no fixes, got %v", appliedMessages(res))
	}
}

// TestFix_SiblingCollisionIsSkipped verifies a rename that would duplicate a
// key is refused.
func TestFix_SiblingCollisionIsSkipped(t *testing.T) {
	src := `apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: doclet
spec:
  type:
    kind: ProjectType
    name: doclet
  parametrs: []
  parameters: []
`
	res := fixSource(t, src, fixer.Options{})
	if string(res.Data) != src {
		t.Errorf("renamed onto an existing key:\n%s", res.Data)
	}
	if !strings.Contains(strings.Join(res.Skipped, "\n"), "is already defined here") {
		t.Errorf("expected collision skip reason, got %v", res.Skipped)
	}
}

// TestFix_EnumValue verifies a misspelled enum value is corrected.
func TestFix_EnumValue(t *testing.T) {
	src := `apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: doclet
spec:
  type:
    kind: ProjectTyp
    name: doclet
`
	res := fixSource(t, src, fixer.Options{})
	if !strings.Contains(string(res.Data), "kind: ProjectType\n") {
		t.Errorf("expected enum value fix, got:\n%s", res.Data)
	}
	if len(res.Applied) != 1 {
		t.Errorf("applied = %v, want 1 fix", appliedMessages(res))
	}
}

// TestFix_ImagePullPolicy verifies the one enum the generated schemas omit is
// still repaired.
func TestFix_ImagePullPolicy(t *testing.T) {
	src := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: doclet
spec:
  imagePullPolicy: Al` + `wyas
`
	res := fixSource(t, src, fixer.Options{})
	if !strings.Contains(string(res.Data), "imagePullPolicy: Always\n") {
		t.Errorf("expected imagePullPolicy fix, got:\n%s", res.Data)
	}
}

// TestFix_SuppressedDiagnosticIsLeftAlone verifies # occ:ignore also suppresses
// the repair for that line.
func TestFix_SuppressedDiagnosticIsLeftAlone(t *testing.T) {
	src := `apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: doclet
spec:
  parametrs: [] # occ:ignore unknown-field
`
	res := fixSource(t, src, fixer.Options{Skip: func(line int, code string) bool {
		return line == 6 && code == "unknown-field"
	}})
	if string(res.Data) != src {
		t.Errorf("suppressed field was fixed:\n%s", res.Data)
	}
}

// TestFix_NonOpenChoreoUntouched verifies a plain YAML file is never touched.
func TestFix_NonOpenChoreoUntouched(t *testing.T) {
	src := "kind: Projectf\nspec:\n  parametrs: []\n"
	res := fixSource(t, src, fixer.Options{})
	if string(res.Data) != src {
		t.Errorf("non-OpenChoreo content changed:\n%s", res.Data)
	}
	if len(res.Applied) != 0 {
		t.Errorf("expected no fixes, got %v", appliedMessages(res))
	}
}

// TestFix_QuotedKeyIsSkipped verifies a key whose raw text is quoted is not
// rewritten from the parsed position, which does not point at the token.
func TestFix_QuotedKeyIsSkipped(t *testing.T) {
	src := `apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: doclet
spec:
  "parametrs": []
`
	res := fixSource(t, src, fixer.Options{})
	if strings.Contains(string(res.Data), "parameters") {
		t.Errorf("quoted key was rewritten:\n%s", res.Data)
	}
}

// TestFix_MultipleDocuments verifies every document in a multi-document file is
// repaired.
func TestFix_MultipleDocuments(t *testing.T) {
	src := `apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: one
spec:
  type:
    kind: ProjectType
    name: one
---
apiVersion: openchoreo.dev/v1alpha1
kind: Projectf
metadata:
  name: two
spec:
  type:
    kind: ProjectType
    name: two
`
	res := fixSource(t, src, fixer.Options{})
	if strings.Contains(string(res.Data), "Projectf") {
		t.Errorf("second document not fixed:\n%s", res.Data)
	}
	if len(res.Applied) != 1 {
		t.Errorf("applied = %v, want 1 fix", appliedMessages(res))
	}
}

// TestFix_PreservesCommentsAndOrder verifies a repair rewrites the key only,
// leaving comments and key order alone.
func TestFix_PreservesCommentsAndOrder(t *testing.T) {
	src := `# top comment
apiVersion: openchoreo.dev/v1alpha1
# which resource
kind: Project
metadata:
  name: doclet # inline
spec:
  # the pipeline
  deploymentPipelinveRef:
    name: standard
`
	res := fixSource(t, src, fixer.Options{})
	got := string(res.Data)
	for _, want := range []string{"# top comment", "# which resource", "  name: doclet # inline", "  # the pipeline", "  deploymentPipelineRef:"} {
		if !strings.Contains(got, want) {
			t.Errorf("lost %q in:\n%s", want, got)
		}
	}
	if strings.Index(got, "apiVersion") > strings.Index(got, "kind: Project") {
		t.Errorf("key order changed:\n%s", got)
	}
}

// TestFix_ParseErrorIsReported verifies an unparsable file is returned
// unchanged with the parse error.
func TestFix_ParseErrorIsReported(t *testing.T) {
	src := "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nspec:\n  type: x\n   bad: indent\n"
	res, err := fixer.Fix([]byte(src), fixer.Options{})
	if err == nil {
		t.Fatal("expected a parse error")
	}
	if string(res.Data) != src {
		t.Errorf("content changed on parse error: %q", res.Data)
	}
}

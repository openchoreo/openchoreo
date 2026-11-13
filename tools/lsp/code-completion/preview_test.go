package codecompletion

import (
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/template"
	"regexp"
	"strings"
	"testing"
)

func TestRequiredPreviewComponent(t *testing.T) {
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component"
	got := RequiredPreview(text, 1, len("kind: Component"), "  ")
	for _, key := range []string{"spec:", "owner:", "projectName:", "componentType:", "name:"} {
		if !strings.Contains(got, key) {
			t.Fatalf("missing %s: %s", key, got)
		}
	}
	if strings.Contains(got, "autoDeploy:") {
		t.Fatal(got)
	}
	if got := RequiredPreview(text+"\nspec: {}", 2, 8, "  "); got != "" {
		t.Fatal("unsafe duplicate spec", got)
	}
	if got := RequiredPreview(strings.Replace(text, "Component", "Unknown", 1), 1, 13, "  "); got != "" {
		t.Fatal(got)
	}
}

func TestPreviewAuthoringMetadata(t *testing.T) {
	for _, kind := range []string{"Component", "Project", "ClusterComponentType"} {
		t.Run(kind, func(t *testing.T) {
			text := "apiVersion: openchoreo.dev/v1alpha1\nkind: " + kind
			before := template.SchemaFor(kind).Properties["metadata"].Properties["name"].Required
			got := RequiredPreview(text, 1, len("kind: "+kind), "  ")
			if !strings.Contains(got, "metadata:\n  name: ") {
				t.Fatalf("missing metadata identity: %s", got)
			}
			if strings.Contains(got, "status:") {
				t.Fatalf("controller status must not be authored: %s", got)
			}
			if strings.HasPrefix(kind, "Cluster") {
				if strings.Contains(got, "namespace:") {
					t.Fatal(got)
				}
			} else if !strings.Contains(got, "namespace: default") {
				t.Fatal(got)
			}
			if template.SchemaFor(kind).Properties["metadata"].Properties["name"].Required != before {
				t.Fatal("cached schema mutated")
			}
		})
	}
}

func TestPreviewExistingTypingContexts(t *testing.T) {
	header := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\n"
	cases := []struct{ name, text, contains, absent string }{
		{"metadata after Enter", header + "metadata:\n  ", "name:", "metadata:"},
		{"namespace after name", header + "metadata:\n  name: frontend\n  ", "namespace: default", "name:"},
		{"spec after Enter", header + "metadata:\n  name: frontend\n  namespace: default\nspec:\n  ", "componentType:", "metadata:"},
		{"owner after Enter", header + "spec:\n  owner:\n    ", "projectName:", "componentType:"},
		{"missing root fields", header + "metadata:\n  name: frontend\n\n", "spec:", "metadata:"},
		{"existing spec not repeated", header + "spec:\n  owner:\n    projectName: demo\n\n", "metadata:", "spec:"},
		{"second document", "apiVersion: v1\nkind: ConfigMap\n---\n" + header + "metadata:\n  ", "name:", "spec:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := strings.Split(tc.text, "\n")
			got := RequiredPreview(tc.text, len(lines)-1, len(lines[len(lines)-1]), "  ")
			if !strings.Contains(got, tc.contains) || strings.Contains(got, tc.absent) {
				t.Fatalf("bad context preview: %q", got)
			}
		})
	}
}

func TestPreviewSafetyAndFormatting(t *testing.T) {
	header := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component"
	got := RequiredPreview(header, 1, 15, "    ")
	if !strings.Contains(got, "metadata:\n    name:") {
		t.Fatal(got)
	}
	crlf := strings.ReplaceAll(header, "\n", "\r\n")
	if !strings.Contains(RequiredPreview(crlf, 1, 15, "  "), "\r\nmetadata:") {
		t.Fatal("CRLF lost")
	}
	for _, text := range []string{header + "\nspec: [", header + "\nmetadata: {name: app}\n  ", header + "\nspec:\n  parameters:\n    ", header + "\nspec:\n  owner: {projectName: demo}\n    "} {
		lines := strings.Split(text, "\n")
		if got := RequiredPreview(text, len(lines)-1, len(lines[len(lines)-1]), "  "); got != "" {
			t.Fatalf("unsafe preview %q for %q", got, text)
		}
	}
}

func TestPreviewPartialKey(t *testing.T) {
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmeta"
	got := RequiredPreview(text, 2, 4, "  ")
	if !strings.HasPrefix(got, "data:\n") || !strings.Contains(got, "name:") {
		t.Fatalf("partial mapping preview: %q", got)
	}
	text = "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nspec:\n  own"
	got = RequiredPreview(text, 3, 5, "  ")
	if !strings.HasPrefix(got, "er:\n") || !strings.Contains(got, "projectName:") {
		t.Fatal(got)
	}
}

func TestPreviewReferenceKindsSatisfySharedRules(t *testing.T) {
	header := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component"
	preview := RequiredPreview(header, 1, len("kind: Component"), "  ")
	if !strings.Contains(preview, "kind: ComponentType") || !strings.Contains(preview, "kind: ClusterWorkflow") {
		t.Fatalf("missing template reference defaults: %s", preview)
	}
	concrete := regexp.MustCompile(`\$\{[0-9]+:([^}]*)\}`).ReplaceAllString(preview, "$1")
	docs, err := parser.ParseYAML([]byte(header + concrete))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range (&ruleengine.ReferenceRules{}).Evaluate(docs[0]) {
		if d.Code == "missing-ref-kind" || d.Code == "unknown-ref-kind" {
			t.Fatalf("generated invalid reference: %+v", d)
		}
	}
	for _, tc := range []struct{ field, kind string }{{"componentType", "ComponentType"}, {"workflow", "ClusterWorkflow"}} {
		text := header + "\nspec:\n  " + tc.field + ":\n    name: existing\n    "
		lines := strings.Split(text, "\n")
		got := RequiredPreview(text, len(lines)-1, 4, "  ")
		if !strings.Contains(got, "kind: "+tc.kind) || strings.Contains(got, "name:") {
			t.Fatalf("existing reference must retain name: %q", got)
		}
	}
}

func TestReferencePreviewUsesSelectedTemplates(t *testing.T) {
	original := template.CurrentVersion()
	t.Cleanup(func() { _ = template.SetVersion(original) })
	for _, version := range template.AvailableVersions() {
		if err := template.SetVersion(version); err != nil {
			t.Fatal(err)
		}
		for _, kind := range []string{"Component", "Resource", "Project", "WorkflowRun"} {
			root := template.SchemaFor(kind)
			if root == nil || root.Properties["spec"] == nil {
				continue
			}
			for field, node := range root.Properties["spec"].Properties {
				kindSchema := node.Properties["kind"]
				if kindSchema == nil || !referenceKindHint(node, "kind", kindSchema) || kindSchema.Default == "" {
					continue
				}
				before := kindSchema.Required
				text := "apiVersion: openchoreo.dev/v1alpha1\nkind: " + kind + "\nspec:\n  " + field + ":\n    name: existing\n    "
				lines := strings.Split(text, "\n")
				got := RequiredPreview(text, len(lines)-1, 4, "  ")
				if !strings.Contains(got, "kind: "+kindSchema.Default) {
					t.Fatalf("%s %s.%s must use template kind default %s: %q", version, kind, field, kindSchema.Default, got)
				}
				if kindSchema.Required != before {
					t.Fatal("template required flag mutated")
				}
			}
		}
	}
}

func TestAcceptedComponentPreviewPassesStructuralRules(t *testing.T) {
	header := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component"
	preview := RequiredPreview(header, 1, len("kind: Component"), "  ")
	if !strings.Contains(preview, "deployment/a") {
		t.Fatalf("expected template-pattern example: %s", preview)
	}
	accepted := header + regexp.MustCompile(`\$\{[0-9]+:([^}]*)\}`).ReplaceAllString(preview, "$1")
	docs, err := parser.ParseYAML([]byte(accepted))
	if err != nil {
		t.Fatal(err)
	}
	engine := ruleengine.NewEngine()
	engine.AddKindRules("Component", template.RulesForKind("Component"))
	for _, finding := range engine.Evaluate(docs[0]) {
		if finding.Severity == ruleengine.SeverityError {
			t.Fatalf("accepted initial skeleton has structural error: %+v\n%s", finding, accepted)
		}
	}
}

func TestPatternExamples(t *testing.T) {
	for _, pattern := range []string{`^(deployment|statefulset|cronjob|job|proxy)/[a-z0-9]([-a-z0-9]*[a-z0-9])?$`, `^[A-Z]{2}[0-9]+$`, `^team-[a-z]+$`, `^item_[0-9]{3}$`} {
		got, ok := patternExample(pattern)
		if !ok || !regexp.MustCompile(pattern).MatchString(got) {
			t.Fatalf("invalid pattern example %q for %s", got, pattern)
		}
	}
	for _, pattern := range []string{`[`, `^a{1000}$`} {
		if _, ok := patternExample(pattern); ok {
			t.Fatalf("unsafe pattern accepted: %s", pattern)
		}
	}
}

func TestAcceptedEnvironmentPreviewPassesStructuralRules(t *testing.T) {
	header := "apiVersion: openchoreo.dev/v1alpha1\nkind: Environment"
	preview := RequiredPreview(header, 1, len("kind: Environment"), "  ")
	accepted := header + regexp.MustCompile(`\$\{[0-9]+:([^}]*)\}`).ReplaceAllString(preview, "$1")
	if !strings.Contains(accepted, "kind: DataPlane") {
		t.Fatalf("missing template enum example: %s", accepted)
	}
	docs, err := parser.ParseYAML([]byte(accepted))
	if err != nil {
		t.Fatal(err)
	}
	engine := ruleengine.NewEngine()
	engine.AddKindRules("Environment", template.RulesForKind("Environment"))
	for _, finding := range engine.Evaluate(docs[0]) {
		if finding.Severity == ruleengine.SeverityError {
			t.Fatalf("accepted Environment preview has structural error: %+v\n%s", finding, accepted)
		}
	}
	text := header + "\nspec:\n  dataPlaneRef:\n    name: existing\n    "
	lines := strings.Split(text, "\n")
	got := RequiredPreview(text, len(lines)-1, 4, "  ")
	if !strings.Contains(got, ":DataPlane}") || strings.Contains(got, "name:") {
		t.Fatalf("enum must be editable and preserve existing name: %s", got)
	}
}

func TestGeneratedEnumsAcrossTemplateVersions(t *testing.T) {
	original := template.CurrentVersion()
	t.Cleanup(func() { _ = template.SetVersion(original) })
	for _, version := range template.AvailableVersions() {
		if err := template.SetVersion(version); err != nil {
			t.Fatal(err)
		}
		for _, kind := range template.AllKinds() {
			header := "apiVersion: openchoreo.dev/v1alpha1\nkind: " + kind
			preview := RequiredPreview(header, 1, len("kind: "+kind), "  ")
			if preview == "" {
				continue
			} // Unsafe/unsupported contexts deliberately abstain.
			accepted := header + regexp.MustCompile(`\$\{[0-9]+:([^}]*)\}`).ReplaceAllString(preview, "$1")
			docs, err := parser.ParseYAML([]byte(accepted))
			if err != nil {
				t.Fatalf("%s/%s: %v", version, kind, err)
			}
			for _, rule := range template.RulesForKind(kind) {
				for _, finding := range rule.Evaluate(docs[0]) {
					if strings.Contains(finding.Message, "must be one of") {
						t.Fatalf("%s/%s generated an invalid enum: %+v", version, kind, finding)
					}
				}
			}
		}
	}
}

func TestDescriptivePlaceholders(t *testing.T) {
	for _, tc := range []struct{ field, label string }{{"projectName", "project-name"}, {"secretKeyRef", "secret-key-ref"}, {"HTTPPort", "http-port"}, {"httpURL", "http-url"}, {"name", "name"}} {
		if got := placeholderLabel(tc.field); got != tc.label {
			t.Fatalf("%s: %s", tc.field, got)
		}
	}
	header := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component"
	preview := RequiredPreview(header, 1, len("kind: Component"), "  ")
	if !strings.Contains(preview, ":<project-name>}") {
		t.Fatalf("missing descriptive placeholder: %s", preview)
	}
	accepted := acceptAuditPreview(header, preview, 1)
	if !strings.Contains(accepted, "projectName: <project-name>") || !strings.Contains(accepted, "name: component-name") {
		t.Fatal(accepted)
	}
	if !strings.Contains(accepted, "name: deployment/a") || !strings.Contains(accepted, "kind: ComponentType") {
		t.Fatalf("constraints must retain valid examples: %s", accepted)
	}
	docs, err := parser.ParseYAML([]byte(accepted))
	if err != nil {
		t.Fatal(err)
	}
	engine := ruleengine.NewEngine()
	engine.AddKindRules("Component", template.RulesForKind("Component"))
	for _, finding := range engine.Evaluate(docs[0]) {
		if finding.Severity == ruleengine.SeverityError {
			t.Fatalf("descriptive placeholder invalidated skeleton: %+v", finding)
		}
	}
}

func TestPreviewScalarBeforeStructure(t *testing.T) {
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: ComponentType"
	got := RequiredPreview(text, 1, len("kind: ComponentType"), "  ")
	scalar, nested := strings.Index(got, "workloadType:"), strings.Index(got, "resources:")
	if scalar < 0 || nested < 0 || scalar > nested {
		t.Fatalf("wrong preview order: %s", got)
	}
}

func TestPreviewCompactObjectArray(t *testing.T) {
	for _, indent := range []string{" ", "  ", "    "} {
		text := "apiVersion: openchoreo.dev/v1alpha1\nkind: ComponentType"
		got := RequiredPreview(text, 1, len("kind: ComponentType"), indent)
		if !strings.Contains(got, "- id:") || strings.Contains(got, "-\n") {
			t.Fatalf("noncompact preview: %s", got)
		}
		// Existing audit helpers decode snippets and validate all selectable kinds.
		concrete := regexp.MustCompile(`\$\{[0-9]+:([^}]*)\}`).ReplaceAllString(got, "$1")
		if _, err := parser.ParseYAML([]byte(text + concrete)); err != nil {
			t.Fatalf("indent %q: %v\n%s", indent, err, concrete)
		}
	}
}

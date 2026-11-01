package schemavalidator

import (
	"context"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"testing"
)

type snapshotProbe struct {
	t     *testing.T
	calls int
}

func (p *snapshotProbe) Supports(kind string) bool { return kind == "Environment" }
func (p *snapshotProbe) Validate(_ context.Context, r Resource, c *ValidationContext) []Finding {
	p.calls++
	key := ResourceKey{"openchoreo.dev/v1alpha1", "Environment", "a", "dev"}
	if len(c.Resolve(key)) != 2 {
		p.t.Fatal("duplicate candidates were lost")
	}
	key.Namespace = "b"
	if len(c.Resolve(key)) != 0 {
		p.t.Fatal("resolution crossed namespaces")
	}
	return nil
}
func TestDocumentSnapshotDispatch(t *testing.T) {
	docs, err := parser.ParseYAML([]byte("apiVersion: openchoreo.dev/v1alpha1\nkind: Environment\nmetadata:\n  name: dev\n  namespace: a\n"))
	if err != nil {
		t.Fatal(err)
	}
	probe := &snapshotProbe{t: t}
	resources := []Resource{{Path: "unsaved.yaml", Document: docs[0]}, {Path: "duplicate.yaml", Document: docs[0]}, {}}
	if got := ValidateDocuments(context.Background(), resources, probe); len(got) != 0 {
		t.Fatalf("unexpected findings: %v", got)
	}
	if probe.calls != 2 {
		t.Fatalf("calls = %d", probe.calls)
	}
}

func TestRepeatedDocumentDoesNotBecomeAmbiguous(t *testing.T) {
	resources := parseResources(t, "apiVersion: openchoreo.dev/v1alpha1\nkind: Environment\nmetadata:\n  name: dev\n")
	resources = append(resources, resources[0])
	if got := ValidateDocuments(context.Background(), resources); len(got) != 0 {
		t.Fatalf("repeated input produced findings: %v", got)
	}
}

func TestUnrelatedAndAnonymousDefinitionsAreNotAmbiguous(t *testing.T) {
	for _, text := range []string{
		"apiVersion: other.example/v1\nkind: Trait\nmetadata:\n  name: external\n---\napiVersion: other.example/v1\nkind: Trait\nmetadata:\n  name: external\n",
		"apiVersion: openchoreo.dev/v1alpha1\nkind: Trait\nspec: {}\n---\napiVersion: openchoreo.dev/v1alpha1\nkind: Trait\nspec: {}\n",
	} {
		got := ValidateDocuments(context.Background(), parseResources(t, text))
		if len(got) != 0 {
			t.Fatalf("unrelated or unnamed resource produced false positives: %+v", got)
		}
	}
}

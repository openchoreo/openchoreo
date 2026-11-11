package celvalidator

import (
	"github.com/google/cel-go/cel"
	"gopkg.in/yaml.v3"
	"testing"
)

func TestTargetResourceFields(t *testing.T) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte("group: apps\nversion: v1\nkind: Deployment\n"), &doc); err != nil {
		t.Fatal(err)
	}
	base, err := cel.NewEnv()
	if err != nil {
		t.Fatal(err)
	}
	env := withTargetResource(base, doc.Content[0])
	for _, tc := range []struct {
		expr  string
		valid bool
	}{{"resource.spec.replicas <= 2", true}, {"resource.specc.replicas <= 2", false}, {"resource.spec.repclicas <= 2", false}, {"resocurce.spec.replicas <= 2", false}, {"resource.metadata.name == 'a'", true}, {"resource.metadata.namme == 'a'", false}, {"resource.spec.template.spec.containers.all(c, c.image != '')", true}} {
		_, issues := env.Compile(tc.expr)
		ok := issues == nil || issues.Err() == nil
		if ok != tc.valid {
			t.Errorf("%s valid=%v expected=%v issues=%v", tc.expr, ok, tc.valid, issues)
		}
	}
}

func TestServiceIntOrStringAndMaps(t *testing.T) {
	var doc yaml.Node
	_ = yaml.Unmarshal([]byte("group: ''\nversion: v1\nkind: Service\n"), &doc)
	base, _ := cel.NewEnv()
	env := withTargetResource(base, doc.Content[0])
	for _, expression := range []string{"resource.spec.ports[0].targetPort == 80", "resource.spec.ports[0].targetPort == 'http'", "resource.metadata.labels['custom-key'] == 'demo'", "has(resource.spec.clusterIP)"} {
		_, issues := env.Compile(expression)
		if issues != nil && issues.Err() != nil {
			t.Errorf("%s: %v", expression, issues)
		}
	}
}

func TestUnknownTargetRemainsDynamic(t *testing.T) {
	var doc yaml.Node
	_ = yaml.Unmarshal([]byte("group: example.invalid\nversion: v1\nkind: Unknown\n"), &doc)
	base, _ := cel.NewEnv()
	env := withTargetResource(base, doc.Content[0])
	_, issues := env.Compile("resource.anything.nested == 1")
	if issues != nil && issues.Err() != nil {
		t.Fatal(issues)
	}
}

func TestJSONResourceStringFormats(t *testing.T) {
	var target yaml.Node
	_ = yaml.Unmarshal([]byte("group: ''\nversion: v1\nkind: Secret\n"), &target)
	base, _ := cel.NewEnv()
	env := withTargetResource(base, target.Content[0])
	for _, expression := range []string{"resource.data['key'] == 'dmFsdWU='", "resource.metadata.creationTimestamp.startsWith('2026')"} {
		_, issues := env.Compile(expression)
		if issues != nil && issues.Err() != nil {
			t.Errorf("JSON resource string format rejected: %s %v", expression, issues)
		}
	}
}

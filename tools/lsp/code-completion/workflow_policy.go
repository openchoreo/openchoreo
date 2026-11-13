package codecompletion

import (
	"encoding/json"
	"github.com/openchoreo/openchoreo/tools/lint/indexer"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/schema"
	protocol "github.com/tliron/glsp/protocol_3_16"
	"gopkg.in/yaml.v3"
	"sort"
)

type workflowChoice struct{ kind, name string }

func allowedWorkflowChoices(text string, line int, uri string, idx ResourceIndexer) ([]workflowChoice, bool, *parser.DocumentNode) {
	workspace, ok := idx.(interface {
		Snapshot(string) []indexer.SnapshotDocument
	})
	if !ok {
		return nil, false, nil
	}
	docs, err := parser.ParseYAML([]byte(text))
	if err != nil {
		return nil, false, nil
	}
	var doc *parser.DocumentNode
	for _, d := range docs {
		if d.Root.Line <= line {
			doc = d
		}
	}
	if doc == nil || doc.Kind != "Component" {
		return nil, false, doc
	}
	ref := dynamicNode(doc.Spec, "componentType")
	kind := dynamicScalar(dynamicNode(ref, "kind"))
	if kind == "" {
		kind = "ComponentType"
	}
	if kind != "ComponentType" && kind != "ClusterComponentType" {
		return nil, false, doc
	}
	definition := uniqueDefinition(workspace.Snapshot(uri), doc, kind, dynamicScalar(dynamicNode(ref, "name")))
	allowed := dynamicNode(definition, "allowedWorkflows")
	if allowed == nil || allowed.Kind != yaml.SequenceNode {
		return nil, false, doc
	}
	var result []workflowChoice
	for _, item := range allowed.Content {
		k, n := dynamicScalar(dynamicNode(item, "kind")), dynamicScalar(dynamicNode(item, "name"))
		if k == "" {
			k = "ClusterWorkflow"
		}
		if (k == "Workflow" || k == "ClusterWorkflow") && n != "" {
			result = append(result, workflowChoice{k, n})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].kind != result[j].kind {
			return result[i].kind < result[j].kind
		}
		return result[i].name < result[j].name
	})
	return result, true, doc
}
func workflowPolicyItems(text string, line, col int, uri string, idx ResourceIndexer) ([]protocol.CompletionItem, bool) {
	choices, known, doc := allowedWorkflowChoices(text, line, uri, idx)
	if !known {
		return nil, false
	}
	ctx := resolveContext(text, line, col)
	if ctx.path != "spec.workflow.name" && ctx.path != "spec.workflow.kind" {
		return nil, false
	}
	current := dynamicNode(doc.Spec, "workflow")
	kind := dynamicScalar(dynamicNode(current, "kind"))
	name := dynamicScalar(dynamicNode(current, "name"))
	var values []string
	seen := map[string]bool{}
	for _, choice := range choices {
		value := choice.name
		if ctx.path == "spec.workflow.name" {
			if kind != "" && choice.kind != kind {
				continue
			}
		} else {
			if name != "" && choice.name != name {
				continue
			}
			value = choice.kind
		}
		if !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	return buildNameItems(values, ctx), true
}
func applyWorkflowPolicy(root *schema.FieldSchema, choices []workflowChoice, doc *parser.DocumentNode) {
	spec := schema.FindSchema(root, "spec")
	workflow := schema.FindSchema(root, "spec.workflow")
	if spec == nil || workflow == nil {
		return
	}
	current := dynamicNode(doc.Spec, "workflow")
	kind, name := dynamicScalar(dynamicNode(current, "kind")), dynamicScalar(dynamicNode(current, "name"))
	var selected *workflowChoice
	for i := range choices {
		c := &choices[i]
		if (kind == "" || c.kind == kind) && (name == "" || c.name == name) {
			selected = c
			break
		}
	}
	if selected == nil {
		delete(spec.Properties, "workflow")
		return
	}
	for key, value := range map[string]string{"kind": selected.kind, "name": selected.name} {
		if n := workflow.Properties[key]; n != nil {
			n.Default = value
			n.Enum = []string{value}
		}
	}
}
func cloneCompletionSchema(base *schema.FieldSchema) *schema.FieldSchema {
	if base == nil {
		return nil
	}
	data, _ := json.Marshal(base)
	var root schema.FieldSchema
	if json.Unmarshal(data, &root) != nil {
		return nil
	}
	return &root
}

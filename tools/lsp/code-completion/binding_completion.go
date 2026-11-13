package codecompletion

import (
	"encoding/json"
	celvalidator "github.com/openchoreo/openchoreo/tools/lint/cel-validator"
	"github.com/openchoreo/openchoreo/tools/lint/indexer"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/schema"
	protocol "github.com/tliron/glsp/protocol_3_16"
	"gopkg.in/yaml.v3"
	"sort"
)

func bindingDocument(text string, line int) *parser.DocumentNode {
	docs, err := parser.ParseYAML([]byte(text))
	if err != nil {
		return nil
	}
	var doc *parser.DocumentNode
	for _, d := range docs {
		if d.Root.Line <= line {
			doc = d
		}
	}
	return doc
}
func bindingReleaseItems(text string, line, col int, uri string, idx ResourceIndexer) ([]protocol.CompletionItem, bool) {
	doc := bindingDocument(text, line)
	if doc == nil {
		return nil, false
	}
	rel, ok := bindingRelationships[doc.Kind]
	if !ok {
		return nil, false
	}
	ctx := resolveContext(text, line, col)
	if ctx.path != "spec."+rel.pin {
		return nil, false
	}
	workspace, ok := idx.(interface {
		Snapshot(string) []indexer.SnapshotDocument
	})
	if !ok {
		return nil, false
	}
	counts := map[string]int{}
	for _, entry := range workspace.Snapshot(uri) {
		d := entry.Document
		if d.Kind == rel.release && d.APIVersion == doc.APIVersion && indexer.NamespaceOrDefault(d.Metadata.Namespace) == indexer.NamespaceOrDefault(doc.Metadata.Namespace) {
			counts[d.Metadata.Name]++
		}
	}
	var names []string
	for name, count := range counts {
		if name != "" && count == 1 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return buildNameItems(names, ctx), true
}
func environmentCompletionSchema(spec *yaml.Node) *schema.FieldSchema {
	pair, err := celvalidator.ExtractSchemas(spec)
	if err != nil || pair.EnvironmentConfigs == nil {
		return nil
	}
	data, err := json.Marshal(pair.EnvironmentConfigs.ToKubeOpenAPI())
	if err != nil {
		return nil
	}
	var raw map[string]any
	if json.Unmarshal(data, &raw) != nil {
		return nil
	}
	adaptDynamicSchema(raw)
	data, _ = json.Marshal(raw)
	result, err := schema.Parse(data)
	if err != nil {
		return nil
	}
	return result
}
func bindingPreviewFields(text string, line int, uri string, idx ResourceIndexer) map[string]*schema.FieldSchema {
	doc := bindingDocument(text, line)
	if doc == nil {
		return nil
	}
	rel, ok := bindingRelationships[doc.Kind]
	if !ok {
		return nil
	}
	fields := map[string]*schema.FieldSchema{rel.pin: {Type: "string", Required: true}}
	workspace, ok := idx.(interface {
		Snapshot(string) []indexer.SnapshotDocument
	})
	if !ok {
		return fields
	}
	frozen := uniqueDefinition(workspace.Snapshot(uri), doc, rel.release, dynamicScalar(dynamicNode(doc.Spec, rel.pin)))
	if frozen == nil {
		return fields
	}
	if s := environmentCompletionSchema(dynamicAt(frozen, rel.definition+".spec")); s != nil {
		fields[rel.values] = s
	}
	if doc.Kind == "ReleaseBinding" {
		if s := environmentCompletionSchema(traitConfigMapSchema(frozen)); s != nil {
			fields["traitEnvironmentConfigs"] = s
		}
	}
	return fields
}

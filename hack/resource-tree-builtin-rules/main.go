// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Command resource-tree-builtin-rules renders config.ResourceTreeDefaults()
// into the control-plane chart's built-in resource tree rules file, so the Go
// defaults are the single authored copy and the chart file is a generated
// artifact:
//
//	go run ./hack/resource-tree-builtin-rules \
//	  -output install/helm/openchoreo-control-plane/files/resource-tree-builtin-rules.yaml
//
// With no -output the rendered file goes to stdout. The output is
// deterministic: the same defaults always yield byte identical YAML, so the
// result can be checked in and diffed.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	yamlv3 "gopkg.in/yaml.v3"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
)

// banner heads the generated YAML. It is static so that the output stays
// deterministic; nothing here records a path, a version, or a timestamp. The
// ReplicaSet hide rationale and every other per-rule note live on
// ResourceTreeDefaults() in the Go source, which is now the authored copy.
const banner = `# Built-in resource tree traversal rules.
#
# GENERATED FILE — DO NOT EDIT. Generated from config.ResourceTreeDefaults()
# in internal/openchoreo-api/config by hack/resource-tree-builtin-rules.
# Regenerate with: make helm-generate.openchoreo-control-plane
#
# These are product behavior, not an operator setting. The chart renders this
# list into the API server's config.yaml and no Helm value can edit it. To run
# a different set entirely, set
# openchoreoApi.config.resourceTree.disableBuiltInRules to true in values.yaml
# and supply the complete set through openchoreoApi.config.resourceTree.rules.
`

// generatedFileMode is the permission the generated chart file carries. Temp
// files are created private, so the rendered file is chmodded before it takes
// the destination's place.
const generatedFileMode = 0o644

// yamlIndent is the nesting width of the emitted YAML. Two spaces, with block
// sequences indented under their key, is how the rules are written by hand in
// the chart's values.yaml.
const yamlIndent = 2

// keyOrder is the order mapping keys are emitted in, most identifying first:
// a rule leads with its root, a kind reference reads group/version/kind before
// its REST plural, and the child edges that nest below come last. One flat
// list covers every mapping because no two levels want the same key in
// different places — "kind" is a sibling of "group" in a kind reference and a
// sibling of "matcher" in a child edge, never both at once. The tag coverage
// test in main_test.go fails if a config field is missing here.
var keyOrder = []string{
	"root",
	"group",
	"version",
	"kind",
	"resource",
	"matcher",
	"label_selector",
	"match_labels",
	"namespaces",
	"metadata_only",
	"hide",
	"children",
}

// opaqueKeys names the values whose own keys are data rather than schema
// fields, and so are left in the order the marshaller produced them.
var opaqueKeys = []string{"match_labels"}

func main() {
	if err := run(os.Stdout, os.Args[0], os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "resource-tree-builtin-rules: %v\n", err)
		os.Exit(1)
	}
}

// run parses flags and writes the rendered rules, either to out or to the file
// named by -output. Rendering completes before anything is written, so a
// failure never leaves a half written file behind.
func run(out io.Writer, program string, args []string) error {
	fs := flag.NewFlagSet(program, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	outputFlag := fs.String("output", "",
		"path of the chart file to write; empty writes to stdout")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr,
			"Usage: %s [-output <chart-file.yaml>]\n\n"+
				"Renders the built-in resource tree traversal rules from\n"+
				"config.ResourceTreeDefaults() as the bare YAML list the control-plane\n"+
				"chart reads.\n\n", program)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		// Parse has already written the failure and the usage text to stderr.
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return fmt.Errorf("parse flags: %w", err)
	}

	rendered, err := renderRules()
	if err != nil {
		return err
	}

	if *outputFlag == "" {
		_, err = out.Write(rendered)

		return err
	}

	return writeFileAtomic(*outputFlag, rendered)
}

// renderRules marshals the built-in rules as the bare YAML list the chart's
// fromYamlArray expects, not the wrapping ResourceTreeConfig. sigs.k8s.io/yaml
// honors the structs' json tags, which the tag parity test in
// internal/openchoreo-api/config pins to the koanf tags the server loads with,
// so the emitted spellings are the ones the loader reads back.
//
// That marshaller sorts keys alphabetically, which reads badly: children
// before the root they hang off, and a kind reference spelled
// group/kind/resource/version. So the document is reordered into keyOrder
// before it is encoded, matching how rules are written by hand in values.yaml
// and in docs/resource-tree/child-discovery.md. The output stays
// deterministic: keyOrder is fixed, and any key missing from it sorts
// alphabetically after the known ones.
func renderRules() ([]byte, error) {
	marshaled, err := yaml.Marshal(config.ResourceTreeDefaults().Rules)
	if err != nil {
		return nil, fmt.Errorf("marshal built-in rules: %w", err)
	}

	var doc yamlv3.Node
	if err := yamlv3.Unmarshal(marshaled, &doc); err != nil {
		return nil, fmt.Errorf("reparse built-in rules for ordering: %w", err)
	}
	if len(doc.Content) != 1 {
		return nil, fmt.Errorf("expected one YAML document of built-in rules, got %d", len(doc.Content))
	}

	orderKeys(doc.Content[0])

	var buf bytes.Buffer
	enc := yamlv3.NewEncoder(&buf)
	enc.SetIndent(yamlIndent)

	if err := enc.Encode(doc.Content[0]); err != nil {
		enc.Close()

		return nil, fmt.Errorf("encode built-in rules: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("flush built-in rules: %w", err)
	}

	return append([]byte(banner), buf.Bytes()...), nil
}

// orderKeys rewrites every mapping in the node into keyOrder, recursively.
// Values under opaqueKeys are left alone: their keys are user data — label
// names — rather than schema fields, so a label literally called "kind" must
// not be hoisted.
func orderKeys(node *yamlv3.Node) {
	switch node.Kind {
	case yamlv3.SequenceNode:
		for _, item := range node.Content {
			orderKeys(item)
		}

	case yamlv3.MappingNode:
		// Content alternates key, value; sort the pairs, not the entries.
		pairs := make([][2]*yamlv3.Node, 0, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if !slices.Contains(opaqueKeys, key.Value) {
				orderKeys(value)
			}
			pairs = append(pairs, [2]*yamlv3.Node{key, value})
		}

		slices.SortStableFunc(pairs, func(a, b [2]*yamlv3.Node) int {
			return compareKeys(a[0].Value, b[0].Value)
		})

		node.Content = node.Content[:0]
		for _, pair := range pairs {
			node.Content = append(node.Content, pair[0], pair[1])
		}
	}
}

// compareKeys ranks two mapping keys by keyOrder, with unlisted keys sorting
// alphabetically after every listed one so a newly added field still renders.
// The tag coverage test keeps keyOrder complete, so that fallback is a safety
// net rather than the normal path.
func compareKeys(a, b string) int {
	rankA, rankB := slices.Index(keyOrder, a), slices.Index(keyOrder, b)

	switch {
	case rankA < 0 && rankB < 0:
		return strings.Compare(a, b)
	case rankA < 0:
		return 1
	case rankB < 0:
		return -1
	default:
		return rankA - rankB
	}
}

// writeFileAtomic renders into a temp file beside the destination and renames
// it over the destination, so a failed write leaves the existing chart file
// intact rather than truncated or partial the way an in place write would.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp")
	if err != nil {
		return fmt.Errorf("create temp file next to %s: %w", path, err)
	}
	tmpName := tmp.Name()

	// From here on every failure has to take the temp file with it.
	cleanup := func(cause error) error {
		tmp.Close()
		os.Remove(tmpName)

		return cause
	}

	if _, err := tmp.Write(data); err != nil {
		return cleanup(fmt.Errorf("write %s: %w", tmpName, err))
	}
	if err := tmp.Close(); err != nil {
		return cleanup(fmt.Errorf("close %s: %w", tmpName, err))
	}
	if err := os.Chmod(tmpName, generatedFileMode); err != nil {
		os.Remove(tmpName)

		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)

		return fmt.Errorf("rename %s to %s: %w", tmpName, path, err)
	}

	return nil
}

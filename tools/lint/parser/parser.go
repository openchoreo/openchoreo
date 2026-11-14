// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"fmt"
	"os"
)

// ParseFile reads a YAML file from disk and returns its DocumentNode ASTs.
func ParseFile(path string) ([]*DocumentNode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}
	return ParseYAML(data)
}

// DocumentForPosition finds which DocumentNode in the list contains the
// given line/column position. Returns nil if no document matches.
func DocumentForPosition(docs []*DocumentNode, line, _ int) *DocumentNode {
	pos := Position{Line: line}
	for _, doc := range docs {
		if pos.Line >= doc.Range.Start.Line && pos.Line <= doc.Range.End.Line {
			return doc
		}
	}
	return nil
}

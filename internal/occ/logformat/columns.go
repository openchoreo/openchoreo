// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package logformat validates log output formats and projects JSON attributes into columns.
package logformat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"
)

const customColumnsPrefix = "custom-columns="

// Columns holds attribute paths in the order requested by the user.
type Columns struct {
	paths [][]string
}

// Validate accepts text (including an empty format), JSON, or a custom column projection.
func Validate(format string) error {
	switch format {
	case "", "text", "json":
		return nil
	}
	if !strings.HasPrefix(format, customColumnsPrefix) {
		return fmt.Errorf("invalid log format %q: must be text, json, or 'custom-columns=FIELD,...'", format)
	}
	_, err := ParseColumns(format)
	return err
}

// ParseColumns parses a custom-columns format, returning nil for other formats.
// Unescaped periods separate nested attributes; \. and \\ select literal periods and backslashes in keys.
func ParseColumns(format string) (*Columns, error) {
	fields, custom := strings.CutPrefix(format, customColumnsPrefix)
	if !custom {
		return nil, nil
	}
	columns := &Columns{}
	for _, field := range strings.Split(fields, ",") {
		field = strings.TrimSpace(field)
		if strings.Contains(field, ":") {
			return nil, fmt.Errorf("invalid custom column %q: named columns (NAME:path) are not supported; use custom-columns=timestamp,log", field)
		}
		path, err := splitColumnPath(field)
		if err != nil {
			return nil, err
		}
		for _, segment := range path {
			if segment == "" || strings.ContainsFunc(segment, unicode.IsSpace) {
				return nil, fmt.Errorf("invalid custom column %q: use non-empty dot-separated attribute names", field)
			}
		}
		columns.paths = append(columns.paths, path)
	}
	return columns, nil
}

func splitColumnPath(field string) ([]string, error) {
	var path []string
	var segment strings.Builder
	escaped := false
	for _, char := range field {
		if escaped {
			if char != '.' && char != '\\' {
				return nil, fmt.Errorf("invalid custom column %q: unsupported escape for %q", field, char)
			}
			segment.WriteRune(char)
			escaped = false
			continue
		}
		switch char {
		case '\\':
			escaped = true
		case '.':
			path = append(path, segment.String())
			segment.Reset()
		default:
			segment.WriteRune(char)
		}
	}
	if escaped {
		return nil, fmt.Errorf("invalid custom column %q: trailing escape", field)
	}
	return append(path, segment.String()), nil
}

// Print writes selected JSON attributes separated by tabs, without a header row. Missing or null values print as '-'.
// Strings retain their contents; other JSON values use their compact JSON representation.
func (c *Columns) Print(out io.Writer, entry any) error {
	encoded, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to encode log entry: %w", err)
	}
	var attributes map[string]any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&attributes); err != nil {
		return fmt.Errorf("failed to read log attributes: %w", err)
	}
	values := make([]string, len(c.paths))
	for i, path := range c.paths {
		var value any = attributes
		for _, segment := range path {
			object, ok := value.(map[string]any)
			if !ok {
				value = nil
				break
			}
			value = object[segment]
		}
		switch value := value.(type) {
		case nil:
			values[i] = "-"
		case string:
			values[i] = value
		default:
			// These values came from JSON decoding, so they can always be encoded again.
			data, err := json.Marshal(value)
			if err != nil {
				return fmt.Errorf("failed to encode custom column: %w", err)
			}
			values[i] = string(data)
		}
	}
	if _, err := fmt.Fprintln(out, strings.Join(values, "\t")); err != nil {
		return fmt.Errorf("failed to write log entry: %w", err)
	}
	return nil
}

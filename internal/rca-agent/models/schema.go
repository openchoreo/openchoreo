// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package models

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed rca_report_schema.yaml
var rcaSchemaYAML []byte

//go:embed chat_response_schema.yaml
var chatSchemaYAML []byte

//go:embed remediation_result_schema.yaml
var remediationSchemaYAML []byte

func RCAReportSchema() (map[string]any, error) {
	m, err := SchemaFromYAML(rcaSchemaYAML)
	if err != nil {
		return nil, fmt.Errorf("parse rca_report_schema.yaml: %w", err)
	}
	return m, nil
}

func ChatResponseSchema() (map[string]any, error) {
	m, err := SchemaFromYAML(chatSchemaYAML)
	if err != nil {
		return nil, fmt.Errorf("parse chat_response_schema.yaml: %w", err)
	}
	return m, nil
}

func RemediationResultSchema() (map[string]any, error) {
	m, err := SchemaFromYAML(remediationSchemaYAML)
	if err != nil {
		return nil, fmt.Errorf("parse remediation_result_schema.yaml: %w", err)
	}
	return m, nil
}

// SchemaFromYAML parses a YAML-encoded JSON Schema into a map[string]any.
// This can be used for any embedded YAML schema, not just the RCA report.
func SchemaFromYAML(data []byte) (map[string]any, error) {
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

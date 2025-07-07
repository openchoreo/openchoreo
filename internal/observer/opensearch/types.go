// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package opensearch

import (
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/labels"
)

// SearchResponse represents the response from an OpenSearch search query
type SearchResponse struct {
	Hits struct {
		Total struct {
			Value    int    `json:"value"`
			Relation string `json:"relation"`
		} `json:"total"`
		Hits []Hit `json:"hits"`
	} `json:"hits"`
	Took     int  `json:"took"`
	TimedOut bool `json:"timed_out"`
}

// Hit represents a single search result hit
type Hit struct {
	Source map[string]interface{} `json:"_source"`
	Score  *float64               `json:"_score"`
}

// MappingResponse represents the response from an index mapping query
type MappingResponse struct {
	Mappings map[string]IndexMapping `json:",inline"`
}

// IndexMapping represents the mapping for a single index
type IndexMapping struct {
	Mappings struct {
		Properties map[string]FieldMapping `json:"properties"`
	} `json:"mappings"`
}

// FieldMapping represents the mapping for a single field
type FieldMapping struct {
	Type       string                  `json:"type"`
	Fields     map[string]FieldMapping `json:"fields,omitempty"`
	Properties map[string]FieldMapping `json:"properties,omitempty"`
}

// LogEntry represents a parsed log entry from OpenSearch
type LogEntry struct {
	Timestamp     time.Time         `json:"timestamp"`
	Log           string            `json:"log"`
	LogLevel      string            `json:"logLevel"`
	ComponentID   string            `json:"componentId"`
	EnvironmentID string            `json:"environmentId"`
	ProjectID     string            `json:"projectId"`
	Version       string            `json:"version"`
	VersionID     string            `json:"versionId"`
	Namespace     string            `json:"namespace"`
	PodID         string            `json:"podId"`
	ContainerName string            `json:"containerName"`
	Labels        map[string]string `json:"labels"`
}

// QueryParams holds common query parameters
type QueryParams struct {
	StartTime      string   `json:"startTime"`
	EndTime        string   `json:"endTime"`
	SearchPhrase   string   `json:"searchPhrase"`
	LogLevels      []string `json:"logLevels"`
	Limit          int      `json:"limit"`
	SortOrder      string   `json:"sortOrder"`
	ComponentID    string   `json:"componentId,omitempty"`
	EnvironmentID  string   `json:"environmentId,omitempty"`
	ProjectID      string   `json:"projectId,omitempty"`
	OrganizationID string   `json:"organizationId,omitempty"`
	Namespace      string   `json:"namespace,omitempty"`
	Versions       []string `json:"versions,omitempty"`
	VersionIDs     []string `json:"versionIds,omitempty"`
}

// GatewayQueryParams holds gateway-specific query parameters
type GatewayQueryParams struct {
	QueryParams
	OrganizationID    string            `json:"organizationId"`
	APIIDToVersionMap map[string]string `json:"apiIdToVersionMap"`
	GatewayVHosts     []string          `json:"gatewayVHosts"`
}

// buildSearchBody converts a query map to an io.Reader for the search request
func buildSearchBody(query map[string]interface{}) io.Reader {
	body, _ := json.Marshal(query)
	return strings.NewReader(string(body))
}

// parseSearchResponse parses the search response from OpenSearch
func parseSearchResponse(body io.Reader) (*SearchResponse, error) {
	var response SearchResponse
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&response); err != nil {
		return nil, err
	}
	return &response, nil
}

// parseMappingResponse parses the mapping response from OpenSearch
func parseMappingResponse(body io.Reader) (*MappingResponse, error) {
	var response MappingResponse
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&response); err != nil {
		return nil, err
	}
	return &response, nil
}

// ParseLogEntry converts a search hit to a LogEntry struct
func ParseLogEntry(hit Hit) LogEntry {
	source := hit.Source
	entry := LogEntry{
		Labels: make(map[string]string),
	}

	// Parse timestamp
	if ts, ok := source["@timestamp"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			entry.Timestamp = parsed
		}
	}

	// Parse log content
	if log, ok := source["log"].(string); ok {
		entry.Log = log
		entry.LogLevel = extractLogLevel(log)
	}

	// Parse Kubernetes metadata
	if k8s, ok := source["kubernetes"].(map[string]interface{}); ok {
		// Parse labels
		if labelMap, ok := k8s["labels"].(map[string]interface{}); ok {
			entry.ComponentID = getStringValue(labelMap, labels.ComponentID)
			entry.EnvironmentID = getStringValue(labelMap, labels.EnvironmentID)
			entry.ProjectID = getStringValue(labelMap, labels.ProjectID)
			entry.Version = getStringValue(labelMap, labels.Version)
			entry.VersionID = getStringValue(labelMap, labels.VersionID)

			// Convert all labels to string map
			for k, v := range labelMap {
				if str, ok := v.(string); ok {
					entry.Labels[k] = str
				}
			}
		}

		// Parse other Kubernetes fields
		entry.Namespace = getStringValue(k8s, "namespace_name")
		entry.PodID = getStringValue(k8s, "pod_id")
		entry.ContainerName = getStringValue(k8s, "container_name")
	}

	return entry
}

// getStringValue safely extracts a string value from a map
func getStringValue(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// extractLogLevel extracts log level from log content using common patterns
func extractLogLevel(log string) string {
	log = strings.ToUpper(log)

	logLevels := []string{"ERROR", "FATAL", "SEVERE", "WARN", "WARNING", "INFO", "DEBUG"}

	for _, level := range logLevels {
		if strings.Contains(log, level) {
			// Normalize WARN/WARNING to WARN
			if level == "WARNING" {
				return "WARN"
			}
			return level
		}
	}

	return "INFO" // Default to INFO if no level found
}

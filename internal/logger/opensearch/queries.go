package opensearch

import (
	"fmt"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/logger/labels"
)

// QueryBuilder provides methods to build OpenSearch queries
type QueryBuilder struct {
	indexPrefix string
}

// NewQueryBuilder creates a new query builder with the given index prefix
func NewQueryBuilder(indexPrefix string) *QueryBuilder {
	return &QueryBuilder{
		indexPrefix: indexPrefix,
	}
}

// BuildComponentLogsQuery builds a query for component logs with wildcard search
func (qb *QueryBuilder) BuildComponentLogsQuery(params QueryParams) map[string]interface{} {
	query := map[string]interface{}{
		"size": params.Limit,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"match": map[string]interface{}{
							labels.OSComponentID: map[string]interface{}{
								"query":             params.ComponentID,
								"zero_terms_query": "none",
							},
						},
					},
					{
						"match": map[string]interface{}{
							labels.OSEnvironmentID: map[string]interface{}{
								"query":             params.EnvironmentID,
								"zero_terms_query": "none",
							},
						},
					},
					{
						"match": map[string]interface{}{
							"kubernetes.namespace_name": map[string]interface{}{
								"query":             params.Namespace,
								"zero_terms_query": "none",
							},
						},
					},
				},
			},
		},
		"sort": []map[string]interface{}{
			{
				"@timestamp": map[string]interface{}{
					"order": params.SortOrder,
				},
			},
		},
	}

	// Add time range filter
	if params.StartTime != "" && params.EndTime != "" {
		timeFilter := map[string]interface{}{
			"range": map[string]interface{}{
				"@timestamp": map[string]interface{}{
					"gte": params.StartTime,
					"lte": params.EndTime,
				},
			},
		}
		query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = 
			append(query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]map[string]interface{}), timeFilter)
	}

	// Add search phrase wildcard filter (V2 feature)
	if params.SearchPhrase != "" {
		searchFilter := map[string]interface{}{
			"wildcard": map[string]interface{}{
				"log": fmt.Sprintf("*%s*", params.SearchPhrase),
			},
		}
		query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = 
			append(query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]map[string]interface{}), searchFilter)
	}

	// Add version filters as "should" conditions
	if len(params.Versions) > 0 || len(params.VersionIDs) > 0 {
		shouldConditions := []map[string]interface{}{}
		
		for _, version := range params.Versions {
			shouldConditions = append(shouldConditions, map[string]interface{}{
				"match": map[string]interface{}{
					labels.OSVersion: map[string]interface{}{
						"query":             version,
						"zero_terms_query": "none",
					},
				},
			})
		}
		
		for _, versionID := range params.VersionIDs {
			shouldConditions = append(shouldConditions, map[string]interface{}{
				"match": map[string]interface{}{
					labels.OSVersionID: map[string]interface{}{
						"query":             versionID,
						"zero_terms_query": "none",
					},
				},
			})
		}
		
		if len(shouldConditions) > 0 {
			query["query"].(map[string]interface{})["bool"].(map[string]interface{})["should"] = shouldConditions
			query["query"].(map[string]interface{})["bool"].(map[string]interface{})["minimum_should_match"] = 1
		}
	}

	return query
}

// BuildProjectLogsQuery builds a query for project logs with wildcard search
func (qb *QueryBuilder) BuildProjectLogsQuery(params QueryParams, componentIDs []string) map[string]interface{} {
	query := map[string]interface{}{
		"size": params.Limit,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"match": map[string]interface{}{
							labels.OSProjectID: map[string]interface{}{
								"query":             params.ProjectID,
								"zero_terms_query": "none",
							},
						},
					},
					{
						"match": map[string]interface{}{
							labels.OSEnvironmentID: map[string]interface{}{
								"query":             params.EnvironmentID,
								"zero_terms_query": "none",
							},
						},
					},
				},
			},
		},
		"sort": []map[string]interface{}{
			{
				"@timestamp": map[string]interface{}{
					"order": params.SortOrder,
				},
			},
		},
	}

	// Add time range filter
	if params.StartTime != "" && params.EndTime != "" {
		timeFilter := map[string]interface{}{
			"range": map[string]interface{}{
				"@timestamp": map[string]interface{}{
					"gte": params.StartTime,
					"lte": params.EndTime,
				},
			},
		}
		query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = 
			append(query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]map[string]interface{}), timeFilter)
	}

	// Add search phrase wildcard filter (V2 feature)
	if params.SearchPhrase != "" {
		searchFilter := map[string]interface{}{
			"wildcard": map[string]interface{}{
				"log": fmt.Sprintf("*%s*", params.SearchPhrase),
			},
		}
		query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = 
			append(query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]map[string]interface{}), searchFilter)
	}

	// Add component ID filters as "should" conditions
	if len(componentIDs) > 0 {
		shouldConditions := []map[string]interface{}{}
		
		for _, componentID := range componentIDs {
			shouldConditions = append(shouldConditions, map[string]interface{}{
				"match": map[string]interface{}{
					labels.OSComponentID: map[string]interface{}{
						"query":             componentID,
						"zero_terms_query": "none",
					},
				},
			})
		}
		
		query["query"].(map[string]interface{})["bool"].(map[string]interface{})["should"] = shouldConditions
		query["query"].(map[string]interface{})["bool"].(map[string]interface{})["minimum_should_match"] = 1
	}

	return query
}

// BuildGatewayLogsQuery builds a query for gateway logs with wildcard search
func (qb *QueryBuilder) BuildGatewayLogsQuery(params GatewayQueryParams) map[string]interface{} {
	query := map[string]interface{}{
		"size": params.Limit,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{},
			},
		},
		"sort": []map[string]interface{}{
			{
				"@timestamp": map[string]interface{}{
					"order": params.SortOrder,
				},
			},
		},
	}

	mustConditions := []map[string]interface{}{}

	// Add time range filter
	if params.StartTime != "" && params.EndTime != "" {
		timeFilter := map[string]interface{}{
			"range": map[string]interface{}{
				"@timestamp": map[string]interface{}{
					"gte": params.StartTime,
					"lte": params.EndTime,
				},
			},
		}
		mustConditions = append(mustConditions, timeFilter)
	}

	// Add organization path filter
	if params.OrganizationID != "" {
		orgFilter := map[string]interface{}{
			"wildcard": map[string]interface{}{
				"log": fmt.Sprintf("*\"apiPath\":\"/%s*", params.OrganizationID),
			},
		}
		mustConditions = append(mustConditions, orgFilter)
	}

	// Add gateway vhost filters
	if len(params.GatewayVHosts) > 0 {
		shouldConditions := []map[string]interface{}{}
		
		for _, vhost := range params.GatewayVHosts {
			shouldConditions = append(shouldConditions, map[string]interface{}{
				"wildcard": map[string]interface{}{
					"log": fmt.Sprintf("*\"gwHost\":\"%s\"*", vhost),
				},
			})
		}
		
		if len(shouldConditions) > 0 {
			query["query"].(map[string]interface{})["bool"].(map[string]interface{})["should"] = shouldConditions
			query["query"].(map[string]interface{})["bool"].(map[string]interface{})["minimum_should_match"] = 1
		}
	}

	// Add API ID filters
	if len(params.APIIDToVersionMap) > 0 {
		apiShouldConditions := []map[string]interface{}{}
		
		for apiID := range params.APIIDToVersionMap {
			apiShouldConditions = append(apiShouldConditions, map[string]interface{}{
				"wildcard": map[string]interface{}{
					"log": fmt.Sprintf("*\"apiUuid\":\"%s\"*", apiID),
				},
			})
		}
		
		if len(apiShouldConditions) > 0 {
			// Combine with existing should conditions using nested bool
			if existing := query["query"].(map[string]interface{})["bool"].(map[string]interface{})["should"]; existing != nil {
				// Create a nested bool query to combine both should conditions
				nestedBool := map[string]interface{}{
					"bool": map[string]interface{}{
						"should": []map[string]interface{}{
							{
								"bool": map[string]interface{}{
									"should":               existing,
									"minimum_should_match": 1,
								},
							},
							{
								"bool": map[string]interface{}{
									"should":               apiShouldConditions,
									"minimum_should_match": 1,
								},
							},
						},
						"minimum_should_match": 2, // Both conditions must match
					},
				}
				mustConditions = append(mustConditions, nestedBool)
				delete(query["query"].(map[string]interface{})["bool"].(map[string]interface{}), "should")
			} else {
				query["query"].(map[string]interface{})["bool"].(map[string]interface{})["should"] = apiShouldConditions
				query["query"].(map[string]interface{})["bool"].(map[string]interface{})["minimum_should_match"] = 1
			}
		}
	}

	// Add general search phrase filter (V2 feature)
	if params.SearchPhrase != "" {
		searchFilter := map[string]interface{}{
			"wildcard": map[string]interface{}{
				"log": fmt.Sprintf("*%s*", params.SearchPhrase),
			},
		}
		mustConditions = append(mustConditions, searchFilter)
	}

	query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = mustConditions

	return query
}

// GenerateIndices generates the list of indices to search based on time range
func (qb *QueryBuilder) GenerateIndices(startTime, endTime string) ([]string, error) {
	if startTime == "" || endTime == "" {
		return []string{qb.indexPrefix + "*"}, nil
	}

	start, err := time.Parse(time.RFC3339, startTime)
	if err != nil {
		return nil, fmt.Errorf("invalid start time format: %w", err)
	}

	end, err := time.Parse(time.RFC3339, endTime)
	if err != nil {
		return nil, fmt.Errorf("invalid end time format: %w", err)
	}

	indices := []string{}
	current := start

	for current.Before(end) || current.Equal(end) {
		indexName := qb.indexPrefix + current.Format("2006.01.02")
		indices = append(indices, indexName)
		current = current.AddDate(0, 0, 1) // Add 1 day
	}

	// Handle edge case where end date might need its own index
	endIndexName := qb.indexPrefix + end.Format("2006.01.02")
	if !contains(indices, endIndexName) {
		indices = append(indices, endIndexName)
	}

	return indices, nil
}

// BuildOrganizationLogsQuery builds a query for organization logs with wildcard search
func (qb *QueryBuilder) BuildOrganizationLogsQuery(params QueryParams, podLabels map[string]string) map[string]interface{} {
	query := map[string]interface{}{
		"size": params.Limit,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{},
			},
		},
		"sort": []map[string]interface{}{
			{
				"@timestamp": map[string]interface{}{
					"order": params.SortOrder,
				},
			},
		},
	}

	mustConditions := []map[string]interface{}{}

	// Add organization filter - this is the key fix!
	if params.OrganizationID != "" {
		orgFilter := map[string]interface{}{
			"match": map[string]interface{}{
				labels.OSOrganizationUUID: map[string]interface{}{
					"query":             params.OrganizationID,
					"zero_terms_query": "none",
				},
			},
		}
		mustConditions = append(mustConditions, orgFilter)
	}

	// Add environment filter if specified
	if params.EnvironmentID != "" {
		envFilter := map[string]interface{}{
			"match": map[string]interface{}{
				labels.OSEnvironmentID: map[string]interface{}{
					"query":             params.EnvironmentID,
					"zero_terms_query": "none",
				},
			},
		}
		mustConditions = append(mustConditions, envFilter)
	}

	// Add namespace filter if specified
	if params.Namespace != "" {
		namespaceFilter := map[string]interface{}{
			"match": map[string]interface{}{
				"kubernetes.namespace_name": map[string]interface{}{
					"query":             params.Namespace,
					"zero_terms_query": "none",
				},
			},
		}
		mustConditions = append(mustConditions, namespaceFilter)
	}

	// Add time range filter
	if params.StartTime != "" && params.EndTime != "" {
		timeFilter := map[string]interface{}{
			"range": map[string]interface{}{
				"@timestamp": map[string]interface{}{
					"gte": params.StartTime,
					"lte": params.EndTime,
				},
			},
		}
		mustConditions = append(mustConditions, timeFilter)
	}

	// Add search phrase wildcard filter
	if params.SearchPhrase != "" {
		searchFilter := map[string]interface{}{
			"wildcard": map[string]interface{}{
				"log": fmt.Sprintf("*%s*", params.SearchPhrase),
			},
		}
		mustConditions = append(mustConditions, searchFilter)
	}

	// Add pod labels filters
	for key, value := range podLabels {
		labelFilter := map[string]interface{}{
			"match": map[string]interface{}{
				fmt.Sprintf("kubernetes.labels.%s", key): map[string]interface{}{
					"query":             value,
					"zero_terms_query": "none",
				},
			},
		}
		mustConditions = append(mustConditions, labelFilter)
	}

	query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = mustConditions

	return query
}

// CheckQueryVersion determines if the index supports V2 wildcard queries
func (qb *QueryBuilder) CheckQueryVersion(mapping *MappingResponse, indexName string) string {
	for name, indexMapping := range mapping.Mappings {
		if strings.Contains(name, indexName) || strings.Contains(indexName, name) {
			if logField, exists := indexMapping.Mappings.Properties["log"]; exists {
				if logField.Type == "wildcard" {
					return "v2"
				}
			}
		}
	}
	return "v1"
}

// contains checks if a slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
#!/bin/bash

## NOTE
# Please ensure that any commands in this script are idempotent as the script may run multiple times

# 1. Check OpenSearch cluster status and wait for it to become ready. Any API calls to configure
#    the cluster should be made only after the cluster is ready.

openSearchHost="${OPENSEARCH_ADDRESS:-https://opensearch:9200}"
authnToken=$(echo -n "$OPENSEARCH_USERNAME:$OPENSEARCH_PASSWORD" | base64)

echo "Checking OpenSearch cluster status"
attempt=1
max_attempts=30

while [ $attempt -le $max_attempts ]; do
    clusterHealth=$(curl --header "Authorization: Basic $authnToken" \
                         --insecure \
                         --location "$openSearchHost/_cluster/health" \
                         --show-error \
                         --silent)
    echo $clusterHealth | jq
    clusterStatus=$(echo "$clusterHealth" | jq --raw-output '.status')
    if [[ "$clusterStatus" == "green" || "$clusterStatus" == "yellow" ]]; then
        echo -e "OpenSearch cluster ready. Continuing with setup...\n"
        break
    fi
    echo "Waiting for OpenSearch cluster to become ready... (attempt $attempt/$max_attempts)"

    if [ $attempt -eq $max_attempts ]; then
        echo "ERROR: OpenSearch cluster did not become ready after $max_attempts attempts. Exiting."
        exit 1
    fi

    attempt=$((attempt + 1))
    sleep 10
done


# 2. Create index templates

# Template for indices which hold container logs
containerLogsIndexTemplate='
{
  "index_patterns": [
    "container-logs-*"
  ],
  "template": {
    "settings": {
      "number_of_shards": 1,
      "number_of_replicas": 1
    },
    "mappings": {
      "properties": {
        "timestamp": {
          "type": "date"
        },
        "log": {
          "type": "wildcard"
        }
      }
    }
  }
}'

# Template for indices which hold OpenTelemetry traces
otelTracesIndexTemplate='
{
  "index_patterns": [
    "otel-traces-*"
  ],
  "template": {
    "settings": {
      "number_of_shards": 1,
      "number_of_replicas": 1
    },
    "mappings": {
      "properties": {
        "endTime": {
          "type": "date_nanos"
        },
        "parentSpanId": {
          "type": "keyword"
        },
        "resource": {
          "properties": {
            "k8s.namespace.name": {
              "type": "keyword"
            },
            "k8s.node.name": {
              "type": "keyword"
            },
            "k8s.pod.name": {
              "type": "keyword"
            },
            "k8s.pod.uid": {
              "type": "keyword"
            },
            "openchoreo.dev/component-uid": {
              "type": "keyword"
            },
            "openchoreo.dev/environment-uid": {
              "type": "keyword"
            },
            "openchoreo.dev/project-uid": {
              "type": "keyword"
            },
            "service.name": {
              "type": "keyword"
            }
          }
        },
        "spanId": {
          "type": "keyword"
        },
        "startTime": {
          "type": "date_nanos"
        },
        "traceId": {
          "type": "keyword"
        }
      }
    }
  }
}'

# Template for indices which hold RCA reports
rcaReportsIndexTemplate='
{
  "index_patterns": [
    "rca-reports-*"
  ],
  "template": {
    "settings": {
      "number_of_shards": 1,
      "number_of_replicas": 1
    },
    "mappings": {
      "properties": {
        "@timestamp": {
          "type": "date"
        },
        "reportId": {
          "type": "keyword"
        },
        "alertId": {
          "type": "keyword"
        },
        "status": {
          "type": "keyword"
        },
        "version": {
          "type": "integer"
        },
        "resource": {
          "properties": {
            "openchoreo.dev/project-uid": {
              "type": "keyword"
            },
            "openchoreo.dev/component-uids": {
              "type": "keyword"
            },
            "openchoreo.dev/environment-uid": {
              "type": "keyword"
            },
            "openchoreo.dev/organization-uid": {
              "type": "keyword"
            }
          }
        }
      }
    }
  }
}'

# The following array holds pairs of index template names and their definitions. Define more templates above
# and add them to this array.
# Format: (templateName1 templateDefinition1 templateName2 templateDefinition2 ...)
indexTemplates=("container-logs" "containerLogsIndexTemplate" "otel-traces" "otelTracesIndexTemplate" "rca-reports" "rcaReportsIndexTemplate")

# Create index templates through a loop using the above array
echo "Creating index templates..."
for ((i=0; i<${#indexTemplates[@]}; i+=2)); do
    templateName="${indexTemplates[i]}"
    templateDefinition="${indexTemplates[i+1]}"

    echo "Creating index template $templateName"
    templateContent="${!templateDefinition}"

    response=$(curl --data "$templateContent" \
                    --header "Authorization: Basic $authnToken" \
                    --header "Content-Type: application/json" \
                    --insecure \
                    --request PUT \
                    --show-error \
                    --silent \
                    --write-out "\n%{http_code}" \
                    "$openSearchHost/_index_template/$templateName")

    httpCode=$(echo "$response" | tail -n1)
    responseBody=$(echo "$response" | head -n-1)

    if [ "$httpCode" -eq 200 ]; then
        echo "Response: $responseBody"
        echo "Successfully created/updated index template $templateName. HTTP response code: $httpCode"

    else
        echo "Response: $responseBody"
        echo "Failed to create/update index template: $templateName. HTTP response code: $httpCode"
    fi
done

echo -e "Index template creation complete\n"

# 3. Add Channel for Notifications
# Reference: https://opensearch.org/docs/latest/observing-your-data/notifications/api
webhookName="openchoreo-observer-alerting-webhook"
webhookUrl="${OBSERVER_ALERTING_WEBHOOK_URL:-http://observer.openchoreo-observability-plane:8080/api/alerting/webhook/opensearch}"

# Desired webhook configuration payload (used for both create and update operations).
webhookConfig="{
  \"config_id\": \"$webhookName\",
  \"config\": {
    \"name\": \"$webhookName\",
    \"description\": \"OpenChoreo Observer Alerting Webhook destination\",
    \"config_type\": \"webhook\",
    \"is_enabled\": true,
    \"webhook\": {
      \"url\": \"$webhookUrl\"
    }
  }
}"

echo -e "Checking if webhook destination already exists..."
webhookCheckResponseCode=$(curl --location "$openSearchHost/_plugins/_notifications/configs/$webhookName" \
                                --header "Authorization: Basic $authnToken" \
                                --insecure \
                                --output /dev/null \
                                --request GET \
                                --silent \
                                --write-out "%{http_code}")

if [ "$webhookCheckResponseCode" -eq 200 ]; then
    echo "Webhook destination already exists. Checking if configuration is up to date..."

    # Fetch the existing webhook configuration to compare against the desired URL.
    existingWebhookConfig=$(curl --location "$openSearchHost/_plugins/_notifications/configs/$webhookName" \
                                 --header "Authorization: Basic $authnToken" \
                                 --insecure \
                                 --silent)

    existingWebhookUrl=$(echo "$existingWebhookConfig" | jq -r '.config.webhook.url // empty')

    if [ "$existingWebhookUrl" = "$webhookUrl" ]; then
        echo "Webhook destination configuration matches the desired state. No update required."
    else
        echo "Webhook destination configuration differs from desired state. Updating destination..."
        updateWebhookResponse=$(curl --location "$openSearchHost/_plugins/_notifications/configs/$webhookName" \
                                     --data "$webhookConfig" \
                                     --header "Authorization: Basic $authnToken" \
                                     --header 'Content-Type: application/json' \
                                     --insecure \
                                     --request PUT)
        echo "HTTP response of webhook destination update API request: $updateWebhookResponse"
    fi
elif [ "$webhookCheckResponseCode" -eq 404 ]; then
    echo "Webhook destination does not exist. Creating a new webhook destination..."
    createWebhookResponse=$(curl --location "$openSearchHost/_plugins/_notifications/configs/" \
                                  --data "$webhookConfig" \
                                  --header "Authorization: Basic $authnToken" \
                                  --header 'Content-Type: application/json' \
                                  --insecure \
                                  --request POST)
    echo "HTTP response of webhook destination creation API request: $createWebhookResponse"
else
    echo "Error checking webhook destination. HTTP response code: $webhookCheckResponseCode"
fi
# 4. Add/Update ISM Policies
# Reference: https://docs.opensearch.org/latest/im-plugin/ism/api/
echo -e "\nManaging ISM Policies..."

# Read retention periods from environment variables or use defaults
containerLogsRetention="${CONTAINER_LOGS_MIN_INDEX_AGE:-30d}"
otelTracesRetention="${OTEL_TRACES_MIN_INDEX_AGE:-30d}"
rcaReportsRetention="${RCA_REPORTS_MIN_INDEX_AGE:-90d}"

# container logs
containerLogsIsmPolicy='{
  "policy": {
    "description": "Delete container logs older than '"$containerLogsRetention"'",
    "default_state": "active",
    "states": [
      {
        "name": "active",
        "actions": [],
        "transitions": [
          {
            "state_name": "delete",
            "conditions": {
              "min_index_age": "'"$containerLogsRetention"'"
            }
          }
        ]
      },
      {
        "name": "delete",
        "actions": [
          {
            "delete": {}
          }
        ],
        "transitions": []
      }
    ],
    "ism_template": [
      {
        "index_patterns": ["container-logs-*"],
        "priority": 100
      }
    ]
  }
}'

# OpenTelemetry traces
otelTracesIsmPolicy='{
  "policy": {
    "description": "Delete OTEL traces older than '"$otelTracesRetention"'",
    "default_state": "active",
    "states": [
      {
        "name": "active",
        "actions": [],
        "transitions": [
          {
            "state_name": "delete",
            "conditions": {
              "min_index_age": "'"$otelTracesRetention"'"
            }
          }
        ]
      },
      {
        "name": "delete",
        "actions": [
          {
            "delete": {}
          }
        ],
        "transitions": []
      }
    ],
    "ism_template": [
      {
        "index_patterns": ["otel-traces-*"],
        "priority": 100
      }
    ]
  }
}'

# RCA reports
rcaReportsIsmPolicy='{
  "policy": {
    "description": "Delete RCA reports older than '"$rcaReportsRetention"'",
    "default_state": "active",
    "states": [
      {
        "name": "active",
        "actions": [],
        "transitions": [
          {
            "state_name": "delete",
            "conditions": {
              "min_index_age": "'"$rcaReportsRetention"'"
            }
          }
        ]
      },
      {
        "name": "delete",
        "actions": [
          {
            "delete": {}
          }
        ],
        "transitions": []
      }
    ],
    "ism_template": [
      {
        "index_patterns": ["rca-reports-*"],
        "priority": 100
      }
    ]
  }
}'

# Array to hold policy names and their definitions
# Format: (ismPolicyName1 ismPolicyDefinition1 ismPolicyName2 ismPolicyDefinition2 ...)
ismPolicies=("container-logs" "containerLogsIsmPolicy" "otel-traces" "otelTracesIsmPolicy" "rca-reports" "rcaReportsIsmPolicy")

# Function to normalize JSON for comparison (removes whitespace differences)
normalize_json() {
    echo "$1" | jq -c -S '.'
}

# Create or update ISM policies through a loop
for ((i=0; i<${#ismPolicies[@]}; i+=2)); do
    ismPolicyName="${ismPolicies[i]}"
    ismPolicyDefinition="${ismPolicies[i+1]}"
    ismPolicyContent="${!ismPolicyDefinition}"

    echo "Processing ISM policy: $ismPolicyName"

    # Check if policy exists
    checkResponse=$(curl --location "$openSearchHost/_plugins/_ism/policies/$ismPolicyName" \
                         --header "Authorization: Basic $authnToken" \
                         --insecure \
                         --silent \
                         --write-out "\n%{http_code}")

    httpCode=$(echo "$checkResponse" | tail -n1)
    responseBody=$(echo "$checkResponse" | head -n-1)

    if [ "$httpCode" -eq 200 ]; then
        echo "Policy $ismPolicyName exists. Checking for updates..."

        # Extract and normalize policy definitions for comparison
        # Remove OpenSearch-generated metadata fields that change on every update or are auto-added
        existingPolicy=$(echo "$responseBody" | jq -c -S '
            .policy | 
            del(.policy_id, .last_updated_time, .schema_version, .error_notification) | 
            del(.ism_template[]?.last_updated_time) |
            walk(if type == "object" then del(.retry) else . end)
        ')
        desiredPolicy=$(echo "$ismPolicyContent" | jq -c -S '.policy')

        # Compare normalized JSON
        if [ "$existingPolicy" = "$desiredPolicy" ]; then
            echo "Policy $ismPolicyName is up to date. No changes needed."
        else
            echo "Policy $ismPolicyName has changes. Updating policy..."

            # Get current sequence number and primary term for optimistic concurrency control
            seqNo=$(echo "$responseBody" | jq -r '._seq_no')
            primaryTerm=$(echo "$responseBody" | jq -r '._primary_term')

            updateResponse=$(curl --data "$ismPolicyContent" \
                                  --header "Authorization: Basic $authnToken" \
                                  --header "Content-Type: application/json" \
                                  --insecure \
                                  --request PUT \
                                  --show-error \
                                  --silent \
                                  --write-out "\n%{http_code}" \
                                  "$openSearchHost/_plugins/_ism/policies/$ismPolicyName?if_seq_no=$seqNo&if_primary_term=$primaryTerm")

            updateHttpCode=$(echo "$updateResponse" | tail -n1)
            updateResponseBody=$(echo "$updateResponse" | head -n-1)

            if [ "$updateHttpCode" -eq 200 ]; then
                echo "Successfully updated ISM policy $ismPolicyName. HTTP response code: $updateHttpCode"
                echo "Response: $updateResponseBody"
            else
                echo "Failed to update ISM policy $ismPolicyName. HTTP response code: $updateHttpCode"
                echo "Response: $updateResponseBody"
            fi
        fi

    elif [ "$httpCode" -eq 404 ]; then
        echo "Policy $ismPolicyName does not exist. Creating new policy..."

        # Create the ISM policy
        createResponse=$(curl --data "$ismPolicyContent" \
                              --header "Authorization: Basic $authnToken" \
                              --header "Content-Type: application/json" \
                              --insecure \
                              --request PUT \
                              --show-error \
                              --silent \
                              --write-out "\n%{http_code}" \
                              "$openSearchHost/_plugins/_ism/policies/$ismPolicyName")

        createHttpCode=$(echo "$createResponse" | tail -n1)
        createResponseBody=$(echo "$createResponse" | head -n-1)

        if [ "$createHttpCode" -eq 201 ] || [ "$createHttpCode" -eq 200 ]; then
            echo "Successfully created ISM policy $ismPolicyName. HTTP response code: $createHttpCode"
            echo "Response: $createResponseBody"

        else
            echo "Failed to create ISM policy $ismPolicyName. HTTP response code: $createHttpCode"
            echo "Response: $createResponseBody"
        fi

    else
        echo "Error checking ISM policy $ismPolicyName. HTTP response code: $httpCode"
        echo "Response: $responseBody"
    fi

    echo ""
done

echo "ISM policy management complete"

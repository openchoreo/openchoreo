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
            "openchoreo.dev/environment-uid": {
              "type": "keyword"
            },
            "openchoreo.dev/organization-uid": {
              "type": "keyword"
            }
          }
        },
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
# NOTE: The secret query parameter value must be kept in sync with the
#       value configured in the observer service
#       (see internal/observer/config/config.go -> alerting.webhook.secret).
webhookName="openchoreo-observer-alerting-webhook"
webhookUrl="${OBSERVER_ALERTING_WEBHOOK_URL:-http://observer.openchoreo-observability-plane:8080/api/alerting/webhook/openchoreo-observer-alert-secret}"

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

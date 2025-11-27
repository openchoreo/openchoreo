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

# The following array holds pairs of index template names and their definitions. Define more templates above
# and add them to this array.
# Format: (templateName1 templateDefinition1 templateName2 templateDefinition2 ...)
indexTemplates=("container-logs" "containerLogsIndexTemplate")

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
                    --request PUT \
                    --show-error \
                    --silent \
                    --write-out "\n%{http_code}" \
                    "$openSearchHost/_index_template/$templateName")
    
    httpCode=$(echo "$response" | tail -n1)
    responseBody=$(echo "$response" | head -n-1)
    
    if [ "$httpCode" -eq 200 ]; then
        echo "Successfully created/updated index template $templateName. HTTP response code: $httpCode"
        echo "Response: $responseBody"
    else
        echo "Failed to create/update index template: $templateName. HTTP response code: $httpCode"
        echo "Response: $responseBody"
    fi
done

echo -e "Index template creation complete\n"

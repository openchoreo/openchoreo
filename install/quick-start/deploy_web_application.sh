#!/bin/bash

# Connects the container to the "kind" network
container_id="$(cat /etc/hostname)"
if docker network inspect kind &>/dev/null; then
  if [ "$(docker inspect -f '{{json .NetworkSettings.Networks.kind}}' "${container_id}")" = "null" ]; then
    docker network connect "kind" "${container_id}"
    echo "Connected container ${container_id} to kind network."
  else
    echo "Container ${container_id} is already connected to kind network."
  fi
else
  echo "Docker network 'kind' does not exist. Skipping connection."
fi

YAML_FILE="react-starter.yaml"
NAMESPACE="default"

# Apply the YAML file
echo "Deploying the sample web application..."
kubectl apply -f "$YAML_FILE" > output.log 2>&1

if grep -q "component.openchoreo.dev/react-starter created" output.log; then
  echo "✅ Component \`react-starter\` created"
fi

if grep -q "workload.openchoreo.dev/react-starter created" output.log; then
  echo "✅ Workload \`react-starter\` created"
fi

if grep -q "webapplication.openchoreo.dev/react-starter created" output.log; then
  echo "✅ WebApplication \`react-starter\` created"
fi

# Clean up the log file
rm output.log

echo "Waiting for WebApplicationBinding to be created..."

while true; do
  BINDING_NAME=$(kubectl get webapplicationbindings.openchoreo.dev -n "$NAMESPACE" -o json 2>/dev/null | jq -r '.items[] | select(.metadata.name | contains("react-starter")) | .metadata.name' | head -n 1)

  if [[ -n "$BINDING_NAME" ]] && [[ "$BINDING_NAME" != "null" ]]; then
    echo "✅ WebApplicationBinding found: $BINDING_NAME"
    break
  fi

  sleep 5
done

echo "Waiting for pods to be ready..."

while true; do
  # Look for pods with correct labels across all namespaces
  POD_READY=$(kubectl get pods --all-namespaces -l component-name=react-starter,environment-name=development,organization-name=default -o json 2>/dev/null | jq -r '.items[] | select(.status.phase == "Running") | select(.status.conditions[]? | select(.type == "Ready" and .status == "True")) | .metadata.name' | head -n 1)

  if [[ -n "$POD_READY" ]] && [[ "$POD_READY" != "null" ]]; then
    echo "✅ Pod is ready: $POD_READY"
    break
  fi

  sleep 5
done

echo "✅ Web application is ready!"
echo "🌍 You can now access the Sample Web Application at: https://react-starter-development.choreoapps.localhost:8443/"
echo "   Open this URL in your browser to see the React starter application."

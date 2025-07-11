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
ENDPOINT_PREFIX="react-starter-image-deployment-webapp"

# Apply the YAML file
choreoctl apply -f "$YAML_FILE" > output.log 2>&1

if grep -q "component.openchoreo.dev/react-starter-image created" output.log; then
  echo "Component \`react-starter-image\` created.."
fi

if grep -q "deploymenttrack.openchoreo.dev/react-starter-image-main created" output.log; then
  echo "DeploymentTrack \`react-starter-image-main\` created.."
fi

if grep -q "deployableartifact.openchoreo.dev/react-starter-image created" output.log; then
  echo "DeployableArtifact \`react-starter-image\` created.."
fi

if grep -q "deployment.openchoreo.dev/react-starter-image-deployment created" output.log; then
  echo "Deployment \`react-starter-image-deployment\` created.."
fi

# Clean up the log file
rm output.log

echo "Waiting for Endpoint to be created..."

while true; do
  ENDPOINT_NAME=$(kubectl get endpoints.openchoreo.dev -n "$NAMESPACE" -o json | jq -r '.items[] | select(.metadata.name | startswith("'"$ENDPOINT_PREFIX"'")) | .metadata.name' | head -n 1)

  if [[ -n "$ENDPOINT_NAME" ]]; then
    echo "✅ Endpoint found: $ENDPOINT_NAME"
    break
  fi

  sleep 5
done

echo "Waiting for Endpoint \`$ENDPOINT_NAME\` to be ready..."

while true; do
  READY_CONDITION=$(kubectl get endpoints.openchoreo.dev "$ENDPOINT_NAME" -n "$NAMESPACE" -o json | jq -r '.status.conditions[] | select(.type=="Ready") | .status')

  if [[ "$READY_CONDITION" == "True" ]]; then
    ENDPOINT_URL=$(kubectl get endpoints.openchoreo.dev "$ENDPOINT_NAME" -n "$NAMESPACE" -o jsonpath="{.status.address}")
    ENDPOINT_URL="${ENDPOINT_URL%/}"
    echo "✅ Endpoint is ready!"
    echo "🌍 You can now access the Sample Web Application at: $ENDPOINT_URL:8443"
    break
  fi

  sleep 5
done

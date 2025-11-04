#!/bin/bash

# Connects the container to the "k3d" network
container_id="$(cat /etc/hostname)"
if docker network inspect k3d-openchoreo-quick-start &>/dev/null; then
  if [ "$(docker inspect -f '{{json .NetworkSettings.Networks.k3d-openchoreo-quick-start}}' "${container_id}")" = "null" ]; then
    docker network connect "k3d-openchoreo-quick-start" "${container_id}"
    echo "Connected container ${container_id} to k3d-openchoreo-quick-start network."
  else
    echo "Container ${container_id} is already connected to k3d-openchoreo-quick-start network."
  fi
else
  echo "Docker network 'k3d-openchoreo-quick-start' does not exist. Skipping connection."
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

echo "Waiting for WebApplicationBinding to be ready with public URL..."

while true; do
  # Check if the binding is ready and has public endpoint URL
  READY_CONDITION=$(kubectl get webapplicationbinding "$BINDING_NAME" -n "$NAMESPACE" -o json 2>/dev/null | jq -r '.status.conditions[]? | select(.type=="Ready") | .status')
  PUBLIC_URL=$(kubectl get webapplicationbinding "$BINDING_NAME" -n "$NAMESPACE" -o json 2>/dev/null | jq -r '.status.endpoints[]? | select(.type=="HTTP") | .public.uri')

  if [[ "$READY_CONDITION" == "True" ]] && [[ -n "$PUBLIC_URL" ]] && [[ "$PUBLIC_URL" != "null" ]]; then
    echo "✅ WebApplicationBinding is ready!"
    break
  fi

  sleep 5
done

echo "Waiting for WebApplicationRelease deployment to be available..."

while true; do
  # Check if the release has Deployment with MinimumReplicasAvailable condition
  DEPLOYMENT_AVAILABLE=$(kubectl get release -l "openchoreo.dev/component=react-starter" -n "$NAMESPACE" -o json 2>/dev/null | jq -r '.items[]?.status.resources[]? | select(.kind=="Deployment") | .status.conditions[]? | select(.type=="Available" and .reason=="MinimumReplicasAvailable") | .status')

  if [[ "$DEPLOYMENT_AVAILABLE" == "True" ]]; then
    echo "✅ WebApplicationRelease deployment is available!"
    break
  fi

  sleep 5
done

echo "✅ Web application is ready!"
echo "🌍 You can now access the Sample Web Application at: $PUBLIC_URL"
echo "   Open this URL in your browser to see the React starter application."

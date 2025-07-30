#!/usr/bin/env bash
set -eo pipefail

container_id="$(cat /etc/hostname)"

# Check if the "kind" network exists
if docker network inspect kind &>/dev/null; then
  # Check if the container is already connected
  if [ "$(docker inspect -f '{{json .NetworkSettings.Networks.kind}}' "${container_id}")" = "null" ]; then
    docker network connect "kind" "${container_id}"
    echo "Connected container ${container_id} to kind network."
  else
    echo "Container ${container_id} is already connected to kind network."
  fi
else
  echo "Docker network 'kind' does not exist. Skipping connection."
fi

terraform -chdir=terraform init -upgrade
terraform -chdir=terraform apply -auto-approve

echo "Finding external gateway nodeport..."
NODEPORT_EG=$(kubectl get svc -n openchoreo-data-plane -l gateway.envoyproxy.io/owning-gateway-name=gateway-external \
  -o jsonpath='{.items[0].spec.ports[0].nodePort}')

if [[ -z "$NODEPORT_EG" ]]; then
  echo "Error: Could not retrieve NodePort."
  exit 1
fi

echo "Setting up a port-forwarding proxy from 8443 to the gateway NodePort..."

socat TCP-LISTEN:8443,fork TCP:openchoreo-quick-start-worker:$NODEPORT_EG &

echo "Finding backstage nodeport..."
NODEPORT_BACKSTAGE=$(kubectl get svc -n openchoreo-control-plane -l app.kubernetes.io/component=backstage \
  -o jsonpath='{.items[0].spec.ports[0].nodePort}')

if [[ -z "$NODEPORT_BACKSTAGE" ]]; then
  echo "Error: Could not retrieve NodePort."
  exit 1
fi

echo "Setting up a port-forwarding proxy from 7007 to the gateway NodePort..."

socat TCP-LISTEN:7007,fork TCP:openchoreo-quick-start-worker:$NODEPORT_BACKSTAGE &

# enable choreoctl auto-completion
if [ -f /state/kube/config-internal.yaml ]; then
  echo "Enabling choreoctl auto-completion..."
  /usr/local/bin/choreoctl completion bash > /usr/local/bin/choreoctl-completion
  chmod +x /usr/local/bin/choreoctl-completion
  echo "source /usr/local/bin/choreoctl-completion" >> /etc/profile
fi

bash ./check-status.sh

# add default dataplane
bash ./add-default-dataplane.sh --single-cluster

# add default BuildPlane
bash ./add-build-plane.sh

exec /bin/bash -l

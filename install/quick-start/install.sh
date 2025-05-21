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
NODEPORT=$(kubectl get svc -n choreo-system -l gateway.envoyproxy.io/owning-gateway-name=gateway-external \
  -o jsonpath='{.items[0].spec.ports[0].nodePort}')

if [[ -z "$NODEPORT" ]]; then
  echo "Error: Could not retrieve NodePort."
  exit 1
fi

echo "Setting up a port-forwarding proxy from 8443 to the gateway NodePort..."
# Run socat with the retrieved NodePort
socat TCP-LISTEN:8443,fork TCP:choreo-quick-start-worker:$NODEPORT &

# enable choreoctl auto-completion
if [ -f /state/kube/config-internal.yaml ]; then
  echo "Enabling choreoctl auto-completion..."
  /usr/local/bin/choreoctl completion bash > /usr/local/bin/choreoctl-completion
  chmod +x /usr/local/bin/choreoctl-completion
  echo "source /usr/local/bin/choreoctl-completion" >> /etc/profile
fi

sh ./check-status.sh --single-cluster

# add default dataplane
sh ./add-default-dataplane.sh --single-cluster

exec /bin/bash -l

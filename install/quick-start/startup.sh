#!/bin/bash

container_id="$(cat /etc/hostname)"

# Check if the "kind" network exists and connect the container to kind network
if docker network inspect kind &>/dev/null; then
  # Check if the container is already connected
  if [ "$(docker inspect -f '{{json .NetworkSettings.Networks.kind}}' "${container_id}")" = "null" ]; then
    docker network connect "kind" "${container_id}"
    echo "Connected container ${container_id} to kind network."
  else
    echo "Container ${container_id} is already connected to kind network."
  fi
fi

# create choreoctl auto-completion if the kube config is available
if [ -f /state/kube/config-internal.yaml ]; then
  echo "Enabling choreoctl auto-completion..."
  /usr/local/bin/choreoctl completion bash > /usr/local/bin/choreoctl-completion
  chmod +x /usr/local/bin/choreoctl-completion
  echo "source /usr/local/bin/choreoctl-completion" >> /etc/profile
fi

exec /bin/bash -l

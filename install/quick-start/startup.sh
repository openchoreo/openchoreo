#!/bin/bash

container_id="$(cat /etc/hostname)"

# Check if the "k3d-openchoreo-quick-start" network exists and connect the container to k3d network
if docker network inspect k3d-openchoreo-quick-start &>/dev/null; then
  # Check if the container is already connected
  if [ "$(docker inspect -f '{{json .NetworkSettings.Networks.k3d-openchoreo-quick-start}}' "${container_id}")" = "null" ]; then
    docker network connect "k3d-openchoreo-quick-start" "${container_id}"
    echo "Connected container ${container_id} to k3d-openchoreo-quick-start network."
  else
    echo "Container ${container_id} is already connected to k3d-openchoreo-quick-start network."
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

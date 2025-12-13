#!/bin/bash

set -e

# Setup docker socket permissions for openchoreo user
# This allows k3d and docker commands to work without sudo
if [ -S /var/run/docker.sock ]; then
  DOCKER_SOCK_GID=$(stat -c '%g' /var/run/docker.sock 2>/dev/null || stat -f '%g' /var/run/docker.sock 2>/dev/null || echo "0")

  if [ "$DOCKER_SOCK_GID" = "0" ]; then
    # Docker Desktop on Mac: socket has GID 0 (root)
    # Add openchoreo user to root group to access the socket
    addgroup openchoreo root >/dev/null 2>&1 || true

    # Ensure socket has group read/write permissions
    # This is safe as we're inside the container and root group access is controlled
    chmod g+rw /var/run/docker.sock 2>/dev/null || true
  else
    # Linux with docker group: create group with socket's GID if needed
    if ! getent group "$DOCKER_SOCK_GID" >/dev/null 2>&1; then
      addgroup -g "$DOCKER_SOCK_GID" docker >/dev/null 2>&1 || true
    fi

    # Add openchoreo user to the docker group
    DOCKER_GROUP_NAME=$(getent group "$DOCKER_SOCK_GID" | cut -d: -f1)
    addgroup openchoreo "${DOCKER_GROUP_NAME:-docker}" >/dev/null 2>&1 || true
  fi
fi

# Preserve environment variables by writing them to a file that .bashrc will source
# This ensures DEV_MODE, OPENCHOREO_VERSION, and DEBUG are available after su -
cat > /home/openchoreo/.env_from_docker <<EOF
export DEV_MODE='${DEV_MODE}'
export OPENCHOREO_VERSION='${OPENCHOREO_VERSION}'
export DEBUG='${DEBUG}'
EOF
chown openchoreo:openchoreo /home/openchoreo/.env_from_docker

# Switch to openchoreo user and start interactive bash
# The '-' flag starts a login shell, which sources ~/.bash_profile
# which in turn sources ~/.bashrc.
# Note: kubeconfig setup happens in .bashrc automatically
exec su - openchoreo

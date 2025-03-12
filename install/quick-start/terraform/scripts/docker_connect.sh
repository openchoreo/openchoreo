#!/usr/bin/env bash
# This script connects the dev container to the kind network

container_id="$(cat /etc/hostname)"
if [ "$(docker inspect -f='{{json .NetworkSettings.Networks.kind}}' "${container_id}")" = 'null' ]; then
  docker network connect "kind" "${container_id}"
fi

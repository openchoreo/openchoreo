FROM alpine:3.23@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11

SHELL ["/bin/sh", "-o", "pipefail", "-c"]

ARG TARGETARCH

RUN apk update && \
   apk add --no-cache \
   curl=8.17.0-r1 \
   podman=5.7.0-r4 \
   fuse-overlayfs=1.16-r0 \
   iptables=1.8.11-r1 && \
   rm -rf /var/cache/apk/*

ENV DOCKER_HOST=unix:///run/podman/podman.sock

RUN if [ "${TARGETARCH}" = "amd64" ]; then \
        curl -sSL "https://github.com/buildpacks/pack/releases/download/v0.40.1/pack-v0.40.1-linux.tgz" | \
        tar -C /usr/local/bin -xzv pack; \
    elif [ "${TARGETARCH}" = "arm64" ]; then \
        curl -sSL "https://github.com/buildpacks/pack/releases/download/v0.40.1/pack-v0.40.1-linux-arm64.tgz" | \
        tar -C /usr/local/bin -xzv pack; \
    fi

WORKDIR /usr/src/app

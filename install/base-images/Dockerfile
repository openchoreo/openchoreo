FROM alpine:3.20

SHELL ["/bin/sh", "-o", "pipefail", "-c"]

ARG TARGETARCH

RUN apk update && \
   apk add --no-cache \
   curl=8.12.1-r0 \
   podman=5.2.5-r0 \
   fuse-overlayfs=1.14-r0 && \
   rm -rf /var/cache/apk/*

ENV DOCKER_HOST=unix:///run/podman/podman.sock

RUN if [ "${TARGETARCH}" = "amd64" ]; then \
        curl -sSL "https://github.com/buildpacks/pack/releases/download/v0.36.4/pack-v0.36.4-linux.tgz" | \
        tar -C /usr/local/bin -xzv pack; \
    elif [ "${TARGETARCH}" = "arm64" ]; then \
        curl -sSL "https://github.com/buildpacks/pack/releases/download/v0.36.4/pack-v0.36.4-linux-arm64.tgz" | \
        tar -C /usr/local/bin -xzv pack; \
    fi

WORKDIR /usr/src/app

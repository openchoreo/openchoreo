FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

SHELL ["/bin/sh", "-o", "pipefail", "-c"]

ARG TARGETARCH

RUN apk update && \
   apk add --no-cache \
   curl=8.19.0-r0 \
   fuse-overlayfs=1.16-r0 \
   jq=1.8.1-r0 \
   podman=5.7.0-r6 \
   iptables=1.8.11-r1 \
   yq-go=4.49.2-r6 && \
   rm -rf /var/cache/apk/*

ENV DOCKER_HOST=unix:///run/podman/podman.sock

RUN if [ "${TARGETARCH}" = "amd64" ]; then \
        curl -sSL "https://github.com/buildpacks/pack/releases/download/v0.40.6/pack-v0.40.6-linux.tgz" | \
        tar -C /usr/local/bin -xzv pack; \
    elif [ "${TARGETARCH}" = "arm64" ]; then \
        curl -sSL "https://github.com/buildpacks/pack/releases/download/v0.40.6/pack-v0.40.6-linux-arm64.tgz" | \
        tar -C /usr/local/bin -xzv pack; \
    fi

WORKDIR /usr/src/app

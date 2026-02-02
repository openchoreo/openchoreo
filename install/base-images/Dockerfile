FROM alpine:3.23@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659

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

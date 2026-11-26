FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

SHELL ["/bin/sh", "-o", "pipefail", "-c"]

ARG TARGETARCH

RUN apk update && \
   apk upgrade --no-cache && \
   apk add --no-cache \
   curl=8.23.0-r0 \
   fuse-overlayfs=1.18-r0 \
   jq=1.8.2-r0 \
   podman=5.8.11-r0 \
   iptables=1.8.13-r0 \
   yq-go=4.53.3-r2 && \
   rm -rf /var/cache/apk/*

ENV DOCKER_HOST=unix:///run/podman/podman.sock

ARG PACK_VERSION=v0.40.11

RUN case "${TARGETARCH}" in \
        amd64) pack_asset="pack-${PACK_VERSION}-linux.tgz"; \
               pack_sha256="280a53ff6526f1aa26cc903b5c5aab515afef25e3d7abac977fe193331d3b301" ;; \
        arm64) pack_asset="pack-${PACK_VERSION}-linux-arm64.tgz"; \
               pack_sha256="3f8ccaacd512a203849944b3591f19f3ec5994188203386cb0f7abfb5fa93c56" ;; \
        *) echo "unsupported architecture: ${TARGETARCH}" >&2; exit 1 ;; \
    esac && \
    curl -sSfL -o /tmp/pack.tgz "https://github.com/buildpacks/pack/releases/download/${PACK_VERSION}/${pack_asset}" && \
    echo "${pack_sha256}  /tmp/pack.tgz" | sha256sum -c - && \
    tar -C /usr/local/bin -xzf /tmp/pack.tgz pack && \
    rm /tmp/pack.tgz

WORKDIR /usr/src/app

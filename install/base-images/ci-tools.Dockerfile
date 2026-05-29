FROM alpine:3.23@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11

SHELL ["/bin/sh", "-o", "pipefail", "-c"]

RUN apk add --no-cache \
      curl=8.19.0-r0 \
      jq=1.8.1-r0 \
      yq-go=4.49.2-r6 && \
    rm -rf /var/cache/apk/* && \
    mkdir -p /ci-tools/bin /ci-tools/lib && \
    cp /usr/bin/curl /usr/bin/jq /usr/bin/yq /ci-tools/bin/ && \
    for bin in /ci-tools/bin/*; do \
      ldd "$bin" 2>/dev/null | awk '/=>/{print $3}' | while read -r lib; do \
        [ -f "$lib" ] && cp -n "$lib" /ci-tools/lib/; \
      done; \
    done && \
    chmod -R a+rX /ci-tools

LABEL org.opencontainers.image.source="https://github.com/openchoreo/openchoreo"
LABEL org.opencontainers.image.description="CI tools (curl, jq, yq) for OpenChoreo workflow init containers"
LABEL org.opencontainers.image.license="Apache-2.0"

USER 1000:1000

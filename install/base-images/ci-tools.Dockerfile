FROM alpine:3.23@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659

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

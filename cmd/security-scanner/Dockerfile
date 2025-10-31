FROM alpine:3.19

ARG TARGETOS
ARG TARGETARCH
ARG CHECKOV_VERSION=3.2.331

LABEL org.opencontainers.image.source="https://github.com/openchoreo/openchoreo"
LABEL org.opencontainers.image.description="OpenChoreo Security Scanner Service"
LABEL org.opencontainers.image.license="Apache-2.0"

WORKDIR /

RUN apk add --no-cache python3 py3-pip gcc musl-dev libffi-dev python3-dev git cargo rust && \
    pip3 install --no-cache-dir --break-system-packages checkov==${CHECKOV_VERSION} && \
    apk del gcc musl-dev libffi-dev python3-dev cargo rust && \
    addgroup -g 65532 nonroot && \
    adduser -u 65532 -G nonroot -D -H nonroot && \
    mkdir -p /tmp /data && \
    chown -R nonroot:nonroot /tmp /data

COPY bin/dist/${TARGETOS}/${TARGETARCH}/security-scanner .

USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["/security-scanner"]

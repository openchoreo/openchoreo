FROM alpine:3.19 AS downloader

ARG TARGETOS
ARG TARGETARCH
ARG CHECKOV_VERSION=3.2.331

RUN apk add --no-cache curl

RUN case "${TARGETARCH}" in \
    "amd64") CHECKOV_ARCH="x86_64" ;; \
    "arm64") CHECKOV_ARCH="arm64" ;; \
    *) echo "Unsupported architecture: ${TARGETARCH}" && exit 1 ;; \
    esac && \
    curl -L "https://github.com/bridgecrewio/checkov/releases/download/${CHECKOV_VERSION}/checkov_${TARGETOS}_${CHECKOV_ARCH}" -o /checkov && \
    chmod +x /checkov

FROM gcr.io/distroless/static:nonroot

ARG TARGETOS
ARG TARGETARCH

LABEL org.opencontainers.image.source="https://github.com/openchoreo/openchoreo"
LABEL org.opencontainers.image.description="OpenChoreo Security Scanner Service"
LABEL org.opencontainers.image.license="Apache-2.0"

WORKDIR /

COPY --from=downloader /checkov /usr/local/bin/checkov
COPY bin/dist/${TARGETOS}/${TARGETARCH}/security-scanner .

USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["/security-scanner"]

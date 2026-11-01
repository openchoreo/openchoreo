# Use distroless for minimal secure image
FROM gcr.io/distroless/static:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329

ARG TARGETOS
ARG TARGETARCH

LABEL org.opencontainers.image.source="https://github.com/openchoreo/openchoreo"
LABEL org.opencontainers.image.description="OpenChoreo remote-connect SNI router - single per-data-plane L4 entrypoint that SNI-routes occ tunnels to per-project+env remote-agents"
LABEL org.opencontainers.image.license="Apache-2.0"

WORKDIR /

COPY bin/dist/${TARGETOS}/${TARGETARCH}/remote-agent-router .

USER 65532:65532

ENTRYPOINT ["/remote-agent-router"]

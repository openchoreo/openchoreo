# Use distroless for minimal secure image
FROM gcr.io/distroless/static:nonroot@sha256:d62067879de890a1def9da0e7766b7d20810a5e5555cef8506b752437d7a3e88

ARG TARGETOS
ARG TARGETARCH

LABEL org.opencontainers.image.source="https://github.com/openchoreo/openchoreo"
LABEL org.opencontainers.image.description="OpenChoreo remote-connect SNI router - single per-data-plane L4 entrypoint that SNI-routes occ tunnels to per-project+env remote-agents"
LABEL org.opencontainers.image.license="Apache-2.0"

WORKDIR /

COPY bin/dist/${TARGETOS}/${TARGETARCH}/remote-agent-router .

USER 65532:65532

ENTRYPOINT ["/remote-agent-router"]

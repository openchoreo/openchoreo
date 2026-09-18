# Use distroless for minimal secure image
FROM gcr.io/distroless/static:nonroot@sha256:1c2c046bc09ed40fad370b599a0b1ae7987f55b01e247cf27a7c27cd97e5bbc7

ARG TARGETOS
ARG TARGETARCH

LABEL org.opencontainers.image.source="https://github.com/openchoreo/openchoreo"
LABEL org.opencontainers.image.description="OpenChoreo remote-connect SNI router - single per-data-plane L4 entrypoint that SNI-routes occ tunnels to per-project+env remote-agents"
LABEL org.opencontainers.image.license="Apache-2.0"

WORKDIR /

COPY bin/dist/${TARGETOS}/${TARGETARCH}/remote-agent-router .

USER 65532:65532

ENTRYPOINT ["/remote-agent-router"]

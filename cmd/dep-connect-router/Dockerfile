# Use distroless for minimal secure image
FROM gcr.io/distroless/static:nonroot@sha256:963fa6c544fe5ce420f1f54fb88b6fb01479f054c8056d0f74cc2c6000df5240

ARG TARGETOS
ARG TARGETARCH

LABEL org.opencontainers.image.source="https://github.com/openchoreo/openchoreo"
LABEL org.opencontainers.image.description="OpenChoreo dep-connect SNI router - single per-data-plane L4 entrypoint that SNI-routes occ tunnels to per-project+env dep-agents"
LABEL org.opencontainers.image.license="Apache-2.0"

WORKDIR /

COPY bin/dist/${TARGETOS}/${TARGETARCH}/dep-connect-router .

USER 65532:65532

ENTRYPOINT ["/dep-connect-router"]

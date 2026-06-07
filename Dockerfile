# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot@sha256:963fa6c544fe5ce420f1f54fb88b6fb01479f054c8056d0f74cc2c6000df5240

ARG TARGETOS
ARG TARGETARCH

LABEL org.opencontainers.image.source="https://github.com/openchoreo/openchoreo"
LABEL org.opencontainers.image.description="Kubernetes Controller for Choreo"
LABEL org.opencontainers.image.license="Apache-2.0"

WORKDIR /
COPY bin/dist/${TARGETOS}/${TARGETARCH}/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]

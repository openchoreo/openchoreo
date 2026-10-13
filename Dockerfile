# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3

ARG TARGETOS
ARG TARGETARCH

LABEL org.opencontainers.image.source="https://github.com/openchoreo/openchoreo"
LABEL org.opencontainers.image.description="Kubernetes Controller for Choreo"
LABEL org.opencontainers.image.license="Apache-2.0"

WORKDIR /
COPY bin/dist/${TARGETOS}/${TARGETARCH}/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]

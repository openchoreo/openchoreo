# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot@sha256:cba10d7abd3e203428e86f5b2d7fd5eb7d8987c387864ae4996cf97191b33764

ARG TARGETOS
ARG TARGETARCH

LABEL org.opencontainers.image.source="https://github.com/openchoreo/openchoreo"
LABEL org.opencontainers.image.description="Kubernetes Controller for Choreo"
LABEL org.opencontainers.image.license="Apache-2.0"

WORKDIR /
COPY bin/dist/${TARGETOS}/${TARGETARCH}/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]

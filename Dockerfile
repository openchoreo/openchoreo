# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot@sha256:f512d819b8f109f2375e8b51d8cfd8aafe81034bc3e319740128b7d7f70d5036

ARG TARGETOS
ARG TARGETARCH

LABEL org.opencontainers.image.source="https://github.com/openchoreo/openchoreo"
LABEL org.opencontainers.image.description="Kubernetes Controller for Choreo"
LABEL org.opencontainers.image.license="Apache-2.0"

WORKDIR /
COPY bin/dist/${TARGETOS}/${TARGETARCH}/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]

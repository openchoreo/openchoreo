# Use distroless for minimal secure image
FROM gcr.io/distroless/static:nonroot@sha256:d62067879de890a1def9da0e7766b7d20810a5e5555cef8506b752437d7a3e88

ARG TARGETOS
ARG TARGETARCH

LABEL org.opencontainers.image.source="https://github.com/openchoreo/openchoreo"
LABEL org.opencontainers.image.description="OpenChoreo remote-agent - tunnels workload dependencies to a developer's machine for local development"
LABEL org.opencontainers.image.license="Apache-2.0"

# Set working directory in the container
WORKDIR /

# Copy the pre-built binary produced by the Makefile into the image
COPY bin/dist/${TARGETOS}/${TARGETARCH}/remote-agent .

# Use non-root user for security (65532 is 'nonroot' user in distroless)
USER 65532:65532

# Entrypoint: run the binary
ENTRYPOINT ["/remote-agent"]

# Use distroless as minimal base image to package the event-forwarder binary.
# The event-forwarder watches OpenChoreo Kubernetes resources via informers
# and forwards lightweight change-notification webhooks to subscribers.
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot@sha256:d62067879de890a1def9da0e7766b7d20810a5e5555cef8506b752437d7a3e88

ARG TARGETOS
ARG TARGETARCH

LABEL org.opencontainers.image.source="https://github.com/openchoreo/openchoreo"
LABEL org.opencontainers.image.description="OpenChoreo event-forwarder"
LABEL org.opencontainers.image.license="Apache-2.0"

# Set working directory in the container
WORKDIR /

# Copy the pre-built binary produced by the Makefile into the image
COPY bin/dist/${TARGETOS}/${TARGETARCH}/event-forwarder .

# Use non-root user for security (65532 is 'nonroot' user in distroless)
USER 65532:65532

# Expose the HTTP port
EXPOSE 8080

# Entrypoint: run the binary
ENTRYPOINT ["/event-forwarder"]

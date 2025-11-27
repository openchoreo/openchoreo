# Use distroless for minimal secure image
FROM gcr.io/distroless/static:nonroot

ARG TARGETOS
ARG TARGETARCH

LABEL org.opencontainers.image.source="https://github.com/openchoreo/openchoreo"
LABEL org.opencontainers.image.description="OpenChoreo Cluster Gateway - Control plane component for data plane agent communication"
LABEL org.opencontainers.image.license="Apache-2.0"

# Set working directory in the container
WORKDIR /

# Copy the pre-built binary produced by the Makefile into the image
COPY bin/dist/${TARGETOS}/${TARGETARCH}/cluster-gateway .

# Use non-root user for security (65532 is 'nonroot' user in distroless)
USER 65532:65532

# Expose the HTTPS port for agent connections and API
EXPOSE 8443

# Entrypoint: run the binary
ENTRYPOINT ["/cluster-gateway"]

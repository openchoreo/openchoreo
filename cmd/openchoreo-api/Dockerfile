# Use distroless for minimal secure image
FROM gcr.io/distroless/static:nonroot@sha256:cba10d7abd3e203428e86f5b2d7fd5eb7d8987c387864ae4996cf97191b33764

ARG TARGETOS
ARG TARGETARCH

LABEL org.opencontainers.image.source="https://github.com/openchoreo/openchoreo"
LABEL org.opencontainers.image.description="OpenChoreo REST API"
LABEL org.opencontainers.image.license="Apache-2.0"

# Set working directory in the container
WORKDIR /

# Copy the pre-built binary produced by the Makefile into the image
COPY bin/dist/${TARGETOS}/${TARGETARCH}/openchoreo-api .

# Copy default configuration file
COPY cmd/openchoreo-api/config.yaml /etc/openchoreo/config.yaml

# Use non-root user for security (65532 is 'nonroot' user in distroless)
USER 65532:65532

# Expose the HTTP port (if your server listens on 8080)
EXPOSE 8080

# Run the binary with default config (CMD can be overridden at runtime)
ENTRYPOINT ["/openchoreo-api"]
CMD ["--config=/etc/openchoreo/config.yaml"]

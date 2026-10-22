# Use distroless for minimal secure image
FROM gcr.io/distroless/static:nonroot@sha256:d62067879de890a1def9da0e7766b7d20810a5e5555cef8506b752437d7a3e88

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

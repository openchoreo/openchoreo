# Use Alpine for minimal image with shell support
FROM alpine:3.23@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11

ARG TARGETOS
ARG TARGETARCH

LABEL org.opencontainers.image.source="https://github.com/openchoreo/openchoreo"
LABEL org.opencontainers.image.description="OpenChoreo CLI"
LABEL org.opencontainers.image.license="Apache-2.0"

# Copy the pre-built binary produced by the Makefile into the image
COPY bin/dist/${TARGETOS}/${TARGETARCH}/occ /usr/local/bin/occ

# Create home directory for nobody user with proper permissions
RUN mkdir -p /home/nobody && chown -R nobody:nobody /home/nobody

# Set HOME environment variable
ENV HOME=/home/nobody

# Use non-root user for security (nobody user in Alpine)
USER nobody:nobody

# Set working directory in the container
WORKDIR /home/nobody

# Entrypoint: run the binary
ENTRYPOINT ["occ"]

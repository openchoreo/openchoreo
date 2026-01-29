# Use Alpine for minimal image with shell support
FROM alpine:3.22@sha256:55ae5d250caebc548793f321534bc6a8ef1d116f334f18f4ada1b2daad3251b2

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

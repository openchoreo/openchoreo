FROM ubuntu:24.04@sha256:cd1dba651b3080c3686ecf4e3c4220f026b521fb76978881737d24f200828b2b

ARG TARGET_ARCH

RUN apt-get update && \
    apt-get install -y \
    podman=4.9.3+ds1-1ubuntu0.2 \
    fuse-overlayfs=1.13-1 \
    curl=8.5.0-2ubuntu10.6 \
    && rm -rf /var/lib/apt/lists/*

ENV DOCKER_HOST=unix:///run/podman/podman.sock

RUN sed -i '/unqualified-search-registries/d' /etc/containers/registries.conf
RUN echo 'unqualified-search-registries = ["docker.io"]' | cat - /etc/containers/registries.conf > temp && mv temp /etc/containers/registries.conf

RUN if [ "${TARGET_ARCH}" = "amd64" ]; then \
        curl -sSL "https://github.com/buildpacks/pack/releases/download/v0.36.4/pack-v0.36.4-linux.tgz" | \
        tar -C /usr/local/bin -xzv pack; \
    elif [ "${TARGET_ARCH}" = "arm64" ]; then \
        curl -sSL "https://github.com/buildpacks/pack/releases/download/v0.36.4/pack-v0.36.4-linux-arm64.tgz" | \
        tar -C /usr/local/bin -xzv pack; \
    fi

WORKDIR /usr/src/app

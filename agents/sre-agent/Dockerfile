FROM ghcr.io/astral-sh/uv:python3.14-trixie-slim@sha256:d21c2dd538d409d050027f67cd09f0b84882cf59072cf77720b15e21f3fe6af5 AS builder

ARG TARGETPLATFORM
ARG BUILDPLATFORM

ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy
ENV UV_PYTHON_INSTALL_DIR=/python
ENV UV_PYTHON_PREFERENCE=only-managed

COPY sre-agent/.python-version /tmp/.python-version
RUN uv python install $(cat /tmp/.python-version)

WORKDIR /app

RUN --mount=type=cache,target=/root/.cache/uv,id=uv-cache-${TARGETPLATFORM} \
    --mount=type=bind,source=sre-agent/uv.lock,target=uv.lock \
    --mount=type=bind,source=sre-agent/pyproject.toml,target=pyproject.toml \
    --mount=type=bind,source=common,target=/common \
    uv sync --frozen --no-install-workspace --no-dev --no-editable

COPY common /common
COPY sre-agent /app

RUN --mount=type=cache,target=/root/.cache/uv,id=uv-cache-${TARGETPLATFORM} \
    uv sync --frozen --no-dev --no-editable

# Install zlib1g to get libz.so in builder (ensure it is there)
RUN apt-get update && apt-get install -y zlib1g

# Detect lib path based on platform
ARG LIBZ_PATH=""

RUN --mount=type=cache,target=/var/cache/apt \
    set -eux; \
    case "$TARGETPLATFORM" in \
        "linux/amd64") LIBZ_PATH="/lib/x86_64-linux-gnu" ;; \
        "linux/arm64") LIBZ_PATH="/lib/aarch64-linux-gnu" ;; \
        *) LIBZ_PATH="/lib" ;; \
    esac; \
    echo "libz path: $LIBZ_PATH"; \
    cp "$LIBZ_PATH/libz.so.1" /lib/libz.so.1

FROM gcr.io/distroless/cc-debian12@sha256:e8e7ee4b8b106d4c5fde9e422a321b2b8a2d5cca546c97adcce927f3e1d36e36

COPY --from=builder --chown=12000:12000 /python /python
WORKDIR /app
COPY --from=builder --chown=12000:12000 /app/.venv /app/.venv
COPY --from=builder --chown=12000:12000 /app/src /app/src

# Copy zlib from builder
COPY --from=builder --chown=12000:12000 /lib/libz.so.1 /lib/libz.so.1

ENV PATH="/app/.venv/bin:$PATH"
ENV PYTHONPATH="/app"
USER 12000

CMD ["uvicorn", "src.main:app", "--host", "0.0.0.0", "--port", "8080", "--proxy-headers", "--forwarded-allow-ips", "*"]

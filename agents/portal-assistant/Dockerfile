FROM ghcr.io/astral-sh/uv:python3.14-trixie-slim@sha256:e37a03b464436ee45750a0d027e4b802ffe579c96b78dcc7e6ef736ca8c658dd AS builder

ARG TARGETPLATFORM
ARG BUILDPLATFORM

ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy
ENV UV_PYTHON_INSTALL_DIR=/python
ENV UV_PYTHON_PREFERENCE=only-managed

COPY portal-assistant/.python-version /tmp/.python-version
RUN uv python install $(cat /tmp/.python-version)

WORKDIR /app

RUN --mount=type=cache,target=/root/.cache/uv,id=uv-cache-${TARGETPLATFORM} \
    --mount=type=bind,source=portal-assistant/uv.lock,target=uv.lock \
    --mount=type=bind,source=portal-assistant/pyproject.toml,target=pyproject.toml \
    --mount=type=bind,source=common,target=/common \
    uv sync --frozen --no-install-workspace --no-dev --no-editable

COPY common /common
COPY portal-assistant /app

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

FROM gcr.io/distroless/cc@sha256:ed7c407fd64eb0af9dddb9456b94cee188a40a7f53cf38c9836e1e9ae14fca02

COPY --from=builder --chown=12000:12000 /python /python
WORKDIR /app
COPY --from=builder --chown=12000:12000 /app/.venv /app/.venv
COPY --from=builder --chown=12000:12000 /app/src /app/src
COPY --from=builder --chown=12000:12000 /app/auth-config.yaml /app/auth-config.yaml

# Copy zlib from builder
COPY --from=builder --chown=12000:12000 /lib/libz.so.1 /lib/libz.so.1

ENV PATH="/app/.venv/bin:$PATH"
ENV PYTHONPATH="/app"
# Safe default for the proxy-trust list; uvicorn reads FORWARDED_ALLOW_IPS
# directly. Override at deploy time with the cluster-internal proxy CIDRs
# (e.g. "10.0.0.0/8" for in-cluster ingress). Never set to "*" in production.
ENV FORWARDED_ALLOW_IPS="127.0.0.1"
USER 12000

CMD ["uvicorn", "src.main:app", "--host", "0.0.0.0", "--port", "8080", "--proxy-headers"]

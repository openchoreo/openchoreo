FROM ghcr.io/astral-sh/uv:python3.14-trixie-slim@sha256:afdc829233119f8bb7ffc68839aaac57b3a4eda72947b2317ad218138fbd955a AS builder

ARG TARGETPLATFORM
ARG BUILDPLATFORM

ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy
ENV UV_PYTHON_INSTALL_DIR=/python
ENV UV_PYTHON_PREFERENCE=only-managed

COPY .python-version /tmp/.python-version
RUN uv python install $(cat /tmp/.python-version)

WORKDIR /app

RUN --mount=type=cache,target=/root/.cache/uv,id=uv-cache-${TARGETPLATFORM} \
    --mount=type=bind,source=uv.lock,target=uv.lock \
    --mount=type=bind,source=pyproject.toml,target=pyproject.toml \
    uv sync --frozen --no-install-project --no-dev --no-editable

COPY . /app

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

FROM gcr.io/distroless/cc@sha256:72344f7f909a8bf003c67f55687e6d51a441b49661af8f660aa7b285f00e57df

COPY --from=builder --chown=12000:12000 /python /python
WORKDIR /app
COPY --from=builder --chown=12000:12000 /app/.venv /app/.venv
COPY --from=builder --chown=12000:12000 /app/src /app/src

# Copy zlib from builder
COPY --from=builder --chown=12000:12000 /lib/libz.so.1 /lib/libz.so.1

ENV PATH="/app/.venv/bin:$PATH"
ENV PYTHONPATH="/app"
USER 12000

CMD ["uvicorn", "src.main:app", "--host", "0.0.0.0", "--port", "8080"]

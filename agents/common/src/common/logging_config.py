# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
import re
import sys
from contextvars import ContextVar

# Context variable to store the current request_id for logging
request_id_context: ContextVar[str | None] = ContextVar("request_id", default=None)


_HEALTH_PATHS = ("/health", "/healthz")
# Exact-path match so URLs like /health-report aren't suppressed.
_HEALTH_REQUEST_RE = re.compile(
    rf"\b[A-Z]+ ({'|'.join(re.escape(p) for p in _HEALTH_PATHS)})(\?\S*)? HTTP/"
)


class HealthcheckFilter(logging.Filter):
    def filter(self, record: logging.LogRecord) -> bool:
        # Prefer uvicorn.access's structured args; fall back to regex.
        args = record.args
        if isinstance(args, tuple) and len(args) >= 3 and isinstance(args[2], str):
            path = args[2].split("?", 1)[0]
            return path not in _HEALTH_PATHS
        return _HEALTH_REQUEST_RE.search(record.getMessage()) is None


class RequestIdFormatter(logging.Formatter):
    def format(self, record: logging.LogRecord) -> str:
        request_id = request_id_context.get()
        record.request_id = f"[{request_id}] " if request_id else ""
        return super().format(record)


def setup_logging(*, log_level: str = "INFO", openai_debug_logs: bool = False):
    level = getattr(logging, log_level.upper(), logging.INFO)

    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(
        RequestIdFormatter(
            fmt="%(asctime)s - %(name)s - %(levelname)s - %(request_id)s%(message)s",
            datefmt="%Y-%m-%d %H:%M:%S",
        )
    )

    logging.basicConfig(
        level=level,
        handlers=[handler],
        force=True,
    )

    # Suppress noisy libraries
    logging.getLogger("httpx").setLevel(logging.WARNING)
    logging.getLogger("httpcore").setLevel(logging.WARNING)
    logging.getLogger("mcp.client.streamable_http").setLevel(logging.WARNING)
    logging.getLogger("aiosqlite").setLevel(logging.WARNING)

    # Control OpenAI client log level
    if openai_debug_logs:
        logging.getLogger("openai").setLevel(logging.DEBUG)
        logging.getLogger("openai._base_client").setLevel(logging.DEBUG)
    else:
        logging.getLogger("openai").setLevel(logging.WARNING)
        logging.getLogger("openai._base_client").setLevel(logging.WARNING)

    uvicorn_access = logging.getLogger("uvicorn.access")
    if not any(isinstance(f, HealthcheckFilter) for f in uvicorn_access.filters):
        uvicorn_access.addFilter(HealthcheckFilter())

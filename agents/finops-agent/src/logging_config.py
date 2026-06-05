# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
import sys
from contextvars import ContextVar

from src.config import settings

# Context variable to store the current request_id for logging
request_id_context: ContextVar[str | None] = ContextVar("request_id", default=None)


class HealthcheckFilter(logging.Filter):
    def filter(self, record: logging.LogRecord) -> bool:
        return "/health" not in record.getMessage()


class RequestIdFormatter(logging.Formatter):
    def format(self, record: logging.LogRecord) -> str:
        request_id = request_id_context.get()
        record.request_id = f"[{request_id}] " if request_id else ""
        return super().format(record)


def setup_logging():
    log_level = getattr(logging, settings.log_level.upper(), logging.INFO)

    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(
        RequestIdFormatter(
            fmt="%(asctime)s - %(name)s - %(levelname)s - %(request_id)s%(message)s",
            datefmt="%Y-%m-%d %H:%M:%S",
        )
    )

    logging.basicConfig(
        level=log_level,
        handlers=[handler],
        force=True,
    )

    # Suppress noisy libraries
    logging.getLogger("httpx").setLevel(logging.WARNING)
    logging.getLogger("httpcore").setLevel(logging.WARNING)
    logging.getLogger("mcp.client.streamable_http").setLevel(logging.WARNING)
    logging.getLogger("aiosqlite").setLevel(logging.WARNING)
    logging.getLogger("openai").setLevel(logging.WARNING)
    logging.getLogger("openai._base_client").setLevel(logging.WARNING)

    uvicorn_access = logging.getLogger("uvicorn.access")
    uvicorn_access.addFilter(HealthcheckFilter())

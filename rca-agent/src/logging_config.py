# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
import sys
from contextvars import ContextVar

from src.core.config import settings

# Context variable to store the current report_id for logging
report_id_context: ContextVar[str | None] = ContextVar("report_id", default=None)


class HealthcheckFilter(logging.Filter):
    def filter(self, record: logging.LogRecord) -> bool:
        return "/health" not in record.getMessage()


class ReportIdFormatter(logging.Formatter):
    """Formatter that includes report_id from context when available."""

    def format(self, record: logging.LogRecord) -> str:
        report_id = report_id_context.get()
        record.report_id = f"[{report_id}] " if report_id else ""
        return super().format(record)


def setup_logging():
    log_level = getattr(logging, settings.log_level.upper(), logging.INFO)

    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(
        ReportIdFormatter(
            fmt="%(asctime)s - %(name)s - %(levelname)s - %(report_id)s%(message)s",
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
    logging.getLogger("opensearch").setLevel(logging.WARNING)

    uvicorn_access = logging.getLogger("uvicorn.access")
    uvicorn_access.addFilter(HealthcheckFilter())

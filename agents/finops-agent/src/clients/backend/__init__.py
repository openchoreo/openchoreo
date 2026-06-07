# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from src.clients.backend.report_backend import ReportBackend
from src.config import settings


def get_report_backend() -> ReportBackend:
    backend = settings.report_backend
    if backend in ("sqlite", "postgresql"):
        from src.clients.backend.sql_backend import get_sql_backend

        return get_sql_backend()
    raise ValueError(f"Unknown report backend: {backend!r}. Use 'sqlite' or 'postgresql'.")


__all__ = [
    "ReportBackend",
    "get_report_backend",
]

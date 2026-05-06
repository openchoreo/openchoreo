# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from abc import ABC, abstractmethod
from datetime import datetime
from typing import Any


class ReportBackend(ABC):
    @abstractmethod
    async def upsert_report(
        self,
        report_id: str,
        status: str = "pending",
        report: dict[str, Any] | None = None,
        summary: str | None = None,
        timestamp: datetime | None = None,
        namespace: str | None = None,
        project: str | None = None,
        component: str | None = None,
        environment: str | None = None,
    ) -> dict[str, Any]: ...

    @abstractmethod
    async def get_report(
        self,
        report_id: str,
    ) -> dict[str, Any] | None: ...

    @abstractmethod
    async def list_reports(
        self,
        namespace: str,
        project: str,
        component: str | None = None,
        start_time: str | None = None,
        end_time: str | None = None,
        status: str | None = None,
        limit: int = 100,
        sort: str = "desc",
    ) -> dict[str, Any]: ...

    @abstractmethod
    async def initialize(self) -> None: ...

    @abstractmethod
    async def close(self) -> None: ...

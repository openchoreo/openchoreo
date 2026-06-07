# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from abc import ABC, abstractmethod
from datetime import datetime
from typing import Any


class ReportBackend(ABC):
    @abstractmethod
    async def upsert_rca_report(
        self,
        report_id: str,
        alert_id: str,
        status: str = "pending",
        report: dict[str, Any] | None = None,
        summary: str | None = None,
        timestamp: datetime | None = None,
        environment_uid: str | None = None,
        project_uid: str | None = None,
    ) -> dict[str, Any]: ...

    @abstractmethod
    async def get_rca_report(
        self,
        report_id: str,
    ) -> dict[str, Any] | None: ...

    @abstractmethod
    async def list_rca_reports(
        self,
        project_uid: str,
        environment_uid: str,
        start_time: str,
        end_time: str,
        status: str | None = None,
        limit: int = 100,
        sort: str = "desc",
    ) -> dict[str, Any]: ...

    @abstractmethod
    async def initialize(self) -> None: ...

    @abstractmethod
    async def close(self) -> None: ...

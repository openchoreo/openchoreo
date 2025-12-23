# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from datetime import UTC, datetime
from functools import lru_cache
from typing import Any

from opensearchpy import OpenSearch, RequestsHttpConnection
from opensearchpy.exceptions import OpenSearchException

from src.core.config import settings
from src.core.constants import oc_labels
from src.core.models.rca_report import RCAReport

logger = logging.getLogger(__name__)


class OpenSearchClient:
    def __init__(self):
        self.client = self._create_client()
        self.index_prefix = "rca-reports"

    def _create_client(self) -> OpenSearch:
        url = settings.opensearch_address
        use_ssl = url.startswith("https://")

        host_with_port = url.replace("https://", "").replace("http://", "")
        host, port = host_with_port.split(":", 1)

        client = OpenSearch(
            hosts=[{"host": host, "port": int(port)}],
            http_auth=(settings.opensearch_username, settings.opensearch_password),
            use_ssl=use_ssl,
            verify_certs=False,
            ssl_show_warn=False,
            connection_class=RequestsHttpConnection,
        )

        return client

    def upsert_rca_report(
        self,
        report_id: str,
        alert_id: str,
        status: str = "pending",
        report: RCAReport | None = None,
        timestamp: datetime | None = None,
        environment_uid: str | None = None,
        organization_uid: str | None = None,
        project_uid: str | None = None,
        component_uids: list[str] | None = None,
        _version: int = 1,
    ) -> dict[str, Any]:
        doc_timestamp = timestamp or datetime.now(UTC)
        index_name = f"{self.index_prefix}-{doc_timestamp.strftime('%Y.%m')}"

        document = {
            "@timestamp": doc_timestamp.isoformat(),
            "reportId": report_id,
            "alertId": alert_id,
            "status": status,
            # "version": version, # Temporarily disable versioning
            "resource": {
                oc_labels.ENVIRONMENT_UID: environment_uid,
                oc_labels.ORGANIZATION_UID: organization_uid,
                oc_labels.PROJECT_UID: project_uid,
                oc_labels.COMPONENT_UIDS: component_uids,
            },
        }

        if report is not None:
            document["summary"] = report.summary
            document["report"] = report.model_dump()

        try:
            response = self.client.index(index=index_name, body=document, id=report_id)
            logger.info(
                f"Successfully upserted RCA report {report_id} to {index_name} with status={status}"
            )
            return response
        except OpenSearchException as e:
            logger.error(f"Failed to upsert RCA report {report_id}: {e}")
            raise

    def check_connection(self) -> bool:
        try:
            info = self.client.info()
            logger.info(f"Successfully connected to OpenSearch: {info['version']['number']}")
            return True
        except OpenSearchException as e:
            logger.error(f"Failed to connect to OpenSearch: {e}")
            return False


@lru_cache
def get_opensearch_client() -> OpenSearchClient:
    return OpenSearchClient()

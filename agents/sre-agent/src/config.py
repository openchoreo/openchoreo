# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from pydantic import model_validator
from pydantic_settings import SettingsConfigDict

from common.config import CommonSettings

LABEL_ENVIRONMENT_UID = "openchoreo.dev/environment-uid"
LABEL_PROJECT_UID = "openchoreo.dev/project-uid"


class Settings(CommonSettings):
    model_config = SettingsConfigDict(
        env_file=".env",
        case_sensitive=False,
        extra="allow",
    )

    rca_model_name: str = ""
    # LangChain provider id passed straight to ``init_chat_model``
    # (google_genai, google_vertexai, openai, anthropic, ...). Empty
    # leaves langchain to infer the provider from the model name, which
    # only works for names it recognises. Gemini models reached through
    # a custom deployment need it set explicitly.
    rca_model_provider: str = ""
    rca_llm_api_key: str = ""
    rca_llm_base_url: str = ""
    # Read by google-auth / the vertexai client out of the process
    # environment; declared here so the chart-supplied env vars are
    # visible in settings rather than looking like strays.
    google_cloud_project: str = ""
    google_cloud_location: str = ""

    observer_api_url: str = "http://observer:8080"

    @property
    def observer_mcp_url(self) -> str:
        return f"{self.observer_api_url.rstrip('/')}/mcp"

    @property
    def openchoreo_mcp_url(self) -> str:
        return f"{self.openchoreo_api_url.rstrip('/')}/mcp"

    report_backend: str = "sqlite"
    sql_backend_uri: str = ""


    max_concurrent_analyses: int = 5
    analysis_timeout_seconds: int = 1500
    remed_agent: bool = False


    @model_validator(mode="after")
    def _validate_backend_config(self) -> Settings:
        if self.report_backend == "postgresql" and not self.sql_backend_uri:
            raise ValueError("report_backend='postgresql' requires: sql_backend_uri")
        if self.report_backend == "sqlite" and not self.sql_backend_uri:
            self.sql_backend_uri = "sqlite+aiosqlite:///data/rca_reports.db"
        if self.sql_backend_uri and not self.sql_backend_uri.startswith(self.report_backend):
            raise ValueError(
                f"sql_backend_uri scheme must match report_backend='{self.report_backend}'"
            )
        return self


settings = Settings()

# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=".env",
        case_sensitive=False,
        extra="allow",
    )

    # LLM — independent of rca-agent so ops can pick a cheaper model for chat.
    perch_model_name: str = ""
    perch_llm_api_key: str = ""

    # The openchoreo control-plane API hosts the MCP endpoint at /mcp.
    openchoreo_api_url: str = (
        "http://openchoreo-api.openchoreo-control-plane.svc.cluster.local:8080"
    )
    # The observer service hosts the observability MCP endpoint at /mcp.
    observer_api_url: str = "http://observer:8080"
    # The rca-agent service hosts an MCP endpoint at /mcp exposing
    # list_rca_reports / get_rca_report / analyze_runtime_state. Empty
    # disables registration of the third MCP server (chat falls back to
    # the openchoreo + observability MCPs only).
    rca_agent_api_url: str = ""

    @property
    def openchoreo_mcp_url(self) -> str:
        return f"{self.openchoreo_api_url.rstrip('/')}/mcp"

    @property
    def observer_mcp_url(self) -> str:
        return f"{self.observer_api_url.rstrip('/')}/mcp"

    @property
    def rca_agent_mcp_url(self) -> str:
        if not self.rca_agent_api_url:
            return ""
        # Trailing slash matters: rca-agent mounts the MCP sub-app at /mcp
        # via FastAPI's app.mount(), which 307-redirects /mcp to /mcp/. The
        # langchain-mcp-adapters httpx client does not follow redirects, so
        # we point straight at the canonical URL.
        return f"{self.rca_agent_api_url.rstrip('/')}/mcp/"

    # Auth — same JWT subject-type model as rca-agent.
    jwt_jwks_url: str = ""
    jwt_issuer: str = ""
    jwt_audience: str = ""
    jwt_jwks_refresh_interval: int = 3600
    # Explicit dev-only opt-in: skip the requirement that jwks_url, issuer,
    # and audience are configured. Production must leave this False.
    jwt_insecure_allow_unverified: bool = False
    authz_timeout_seconds: int = 30
    auth_config_path: str = "auth-config.yaml"

    @property
    def authz_service_url(self) -> str:
        return self.openchoreo_api_url.rstrip("/")

    # Confirmation flow.
    # action_ttl_seconds bounds how long a proposed action lives in the in-memory
    # store between propose-time and execute-time. Pod restarts evict everything;
    # the user must re-ask. See risks #3 in the proposal.
    action_ttl_seconds: int = 600

    # Concurrency — chat is spikier than RCA analysis (which caps at 5).
    max_concurrent_chats: int = 20

    # Operational.
    log_level: str = "INFO"
    openai_debug_logs: bool = False
    tls_insecure_skip_verify: bool = False
    cors_allowed_origins: str = ""


settings = Settings()

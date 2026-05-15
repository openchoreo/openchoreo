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
    # OpenAI gpt-5 / o-series reasoning effort. One of "minimal" /
    # "low" / "medium" / "high"; empty string leaves the model on its
    # default (medium for gpt-5-mini). Reasoning tokens are generated
    # invisibly before the user-facing response and are a large
    # contributor to the composition turn's wall-clock — tuning this
    # is the cheapest knob for latency vs. depth-of-analysis.
    perch_reasoning_effort: str = ""
    # langgraph recursion_limit — bounds the worst-case per-turn
    # supersteps before the framework aborts with GraphRecursionError
    # and ``recover_with_fallback`` returns a tool-less reply.
    #
    # 0 means "use the per-case map default in builder.py" (the
    # _DEFAULT_RECURSION_LIMIT / _RECURSION_LIMIT_FOR_CASE constants).
    # Setting a non-zero value via this env var overrides BOTH the
    # default and any per-case entry.
    #
    # Empirical sizing from N=5 on 2026-05-15:
    #   runtime_debug (3 tools): ≤9 supersteps observed.
    #   build_failure (3 tools): typically 8-9, sometimes 11+.
    # A limit of 15 gives 5 supersteps of headroom over the build
    # failure max — catches genuine loops without clipping normal
    # chats. The chart default is 15; 10 caused a 40 % build-failure
    # abort rate in the same benchmark.
    perch_recursion_limit: int = 0

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

    # Concurrency — chat is spikier than RCA analysis (which caps at 5).
    max_concurrent_chats: int = 20

    # Operational.
    log_level: str = "INFO"
    openai_debug_logs: bool = False
    jwks_url_tls_insecure_skip_verify: bool = False
    uid_resolver_tls_insecure_skip_verify: bool = False
    # Independent toggle for the authz client's TLS verification. Kept
    # strict by default; flip on only when authz uses a self-signed
    # cert and the operator has confirmed they don't want to extend
    # that trust to MCP / UID-resolver endpoints.
    authz_tls_insecure_skip_verify: bool = False
    cors_allowed_origins: str = ""


settings = Settings()

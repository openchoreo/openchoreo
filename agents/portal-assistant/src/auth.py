# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from common.auth.authz_models import SubjectContext  # noqa: F401
from common.auth.runtime import AuthRuntime
from src.config import settings

auth = AuthRuntime(settings, service_name="portal-assistant")

get_jwt_validator = auth.get_jwt_validator
get_authz_client = auth.get_authz_client
require_authn = auth.require_authn

# Coarse gate used by /chat and /warmup; fine-grained checks happen per-tool
# in the openchoreo MCP layer.
require_invoke_authz = auth.checker("portal-assistant:invoke", "portal-assistant")

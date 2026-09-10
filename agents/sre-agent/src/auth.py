# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from common.auth.runtime import (
    AuthRuntime,
    hierarchy_from_body,
    hierarchy_from_query,
)
from src.config import settings

auth = AuthRuntime(settings, service_name="rca-agent")

get_jwt_validator = auth.get_jwt_validator
get_authz_client = auth.get_authz_client
get_oauth2_auth = auth.get_oauth2_auth
check_oauth2_connection = auth.check_oauth2_connection
require_authn = auth.require_authn

require_chat_authz = auth.checker(
    "rcareport:view",
    "rcareport",
    hierarchy=hierarchy_from_body(project="project", namespace="namespace"),
)
_report_hierarchy = hierarchy_from_query(project="project", namespace="namespace")
require_reports_authz = auth.checker("rcareport:view", "rcareport", hierarchy=_report_hierarchy)
require_reports_update_authz = auth.checker(
    "rcareport:update", "rcareport", hierarchy=_report_hierarchy
)

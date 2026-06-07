# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from .agent_routes import router as agent_router
from .report_routes import router as report_router

__all__ = ["agent_router", "report_router"]

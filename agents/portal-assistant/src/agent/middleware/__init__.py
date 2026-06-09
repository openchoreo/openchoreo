# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from src.agent.middleware.empty_result_guard import EmptyResultGuardMiddleware
from src.agent.middleware.logging import LoggingMiddleware
from src.agent.middleware.loop_guard import LoopGuardMiddleware
from src.agent.middleware.output_transformer import OutputTransformerMiddleware
from src.agent.middleware.tool_error_handler import ToolErrorHandlerMiddleware
from src.agent.middleware.write_guard import WriteGuardMiddleware

# OutputTransformerMiddleware is a trimmed fork of rca-agent's middleware
# of the same name: same logs / traces / spans compression via Jinja2
# templates, but with the numpy-heavy metrics processor stripped (perch
# doesn't analyse CPU/memory). See output_transformer.py module docstring
# for the benchmarked compression numbers.

__all__ = [
    "EmptyResultGuardMiddleware",
    "LoggingMiddleware",
    "LoopGuardMiddleware",
    "OutputTransformerMiddleware",
    "ToolErrorHandlerMiddleware",
    "WriteGuardMiddleware",
]

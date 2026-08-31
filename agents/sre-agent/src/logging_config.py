# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from common.logging_config import request_id_context  # noqa: F401
from common.logging_config import setup_logging as _setup_logging
from src.config import settings


def setup_logging():
    _setup_logging(
        log_level=settings.log_level,
        openai_debug_logs=settings.openai_debug_logs,
    )

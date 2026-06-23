# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

# Set env before importing any `src` module: config.Settings() and the
# module-level agents in src.agent.agent (each calls get_model()) are built at
# import time. Assigned directly, not via setdefault, so an ambient
# REPORT_BACKEND / SQL_BACKEND_URI can't leak in and break config validation.
import os

os.environ["RCA_MODEL_NAME"] = "openai:gpt-4o-mini"
os.environ["RCA_LLM_API_KEY"] = "test-key"
os.environ["REPORT_BACKEND"] = "sqlite"
os.environ["SQL_BACKEND_URI"] = ""

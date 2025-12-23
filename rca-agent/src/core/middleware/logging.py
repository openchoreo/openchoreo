# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
import secrets
from datetime import datetime
from pathlib import Path

from langchain.agents.middleware import AgentMiddleware

from src.core.config import settings

logger = logging.getLogger(__name__)


class LoggingMiddleware(AgentMiddleware):
    def __init__(self):
        super().__init__()
        self.log_file = None

        if settings.debug:
            timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
            rand_suffix = secrets.token_hex(3)
            filename = f"log-{timestamp}-{rand_suffix}.txt"

            logs_dir = Path("logs")
            logs_dir.mkdir(exist_ok=True)
            self.log_file = logs_dir / filename

            self._write_log(f"=== Debug Log Started at {datetime.now().isoformat()} ===\n")

    def _write_log(self, content: str):
        if self.log_file:
            with open(self.log_file, "a") as f:
                f.write(content + "\n")

    async def awrap_model_call(self, request, handler):
        # Find last AI message and get everything after it
        last_ai_idx = -1
        for i in range(len(request.messages) - 1, -1, -1):
            if request.messages[i].type == "ai":
                last_ai_idx = i
                break

        new_messages = request.messages[last_ai_idx + 1 :]

        for message in new_messages:
            if message.type == "human":
                logger.info("Human message: %s", message.content)
                self._write_log(message.pretty_repr())
            elif message.type == "tool":
                logger.info("Tool result: %s", message.content[:200])
                self._write_log(message.pretty_repr())

        result = await handler(request)

        ai_message = result.result[0]

        if ai_message.content:
            logger.info("AI response: %s", ai_message.content[:200])

        if ai_message.tool_calls:
            for tool_call in ai_message.tool_calls:
                logger.info(
                    "Tool call: %s with args: %s", tool_call.get("name"), tool_call.get("args")
                )

        self._write_log(ai_message.pretty_repr())

        return result

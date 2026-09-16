# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
from collections.abc import Sequence

from langchain_core.tools import BaseTool, StructuredTool
from pydantic import BaseModel, Field

from src.extensions.config import Skill

logger = logging.getLogger(__name__)

LOAD_SKILL_TOOL = "load_skill"


class _LoadSkillInput(BaseModel):
    name: str = Field(description="Name of the skill to load, as listed in the system prompt")


def create_load_skill_tool(skills: Sequence[Skill]) -> BaseTool:
    bodies = {skill.name: skill.body for skill in skills}
    known = ", ".join(sorted(bodies)) or "none"

    def load(name: str) -> str:
        body = bodies.get(name)
        if body is None:
            logger.warning("Agent requested unknown skill %r; available: %s", name, known)
            return f"No skill named {name!r}. Available skills: {known}."
        logger.info("Loaded skill %s", name)
        return body

    return StructuredTool.from_function(
        func=load,
        name=LOAD_SKILL_TOOL,
        description=(
            "Load the full instructions for one of the skills listed in the system prompt. "
            "Call it when a skill's description matches the task in front of you."
        ),
        args_schema=_LoadSkillInput,
    )

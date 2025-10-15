"""Security policy data models."""

from typing import List, Dict, Any
from pydantic import BaseModel, Field, ConfigDict
from pydantic.alias_generators import to_snake


class SecurityPolicy(BaseModel):
    """Security policy configuration."""
    model_config = ConfigDict(alias_generator=to_snake, populate_by_name=True)

    enable: bool = Field(alias="Enable")
    whitelist: List[str] = Field(alias="WhiteList")

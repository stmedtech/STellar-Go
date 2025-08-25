"""Security policy data models."""

from typing import List, Dict, Any
from pydantic import BaseModel, Field


class SecurityPolicy(BaseModel):
    """Security policy configuration."""
    enable: bool = Field(alias="Enable")
    whitelist: List[str] = Field(alias="WhiteList")

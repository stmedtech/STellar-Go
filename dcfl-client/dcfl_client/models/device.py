"""Device data models."""

from enum import Enum
from typing import Dict, Any, List, Optional
from datetime import datetime
from pydantic import BaseModel, ConfigDict, Field
from pydantic.alias_generators import to_pascal


class DeviceStatus(str, Enum):
    """Device status enumeration."""
    UNKNOWN = ""
    DISCOVERED = "discovered"
    HEALTHY = "healthy"


class SystemInformation(BaseModel):
    """System information data model."""
    platform: Optional[str] = Field(alias="Platform", default=None)
    cpu: Optional[str] = Field(alias="CPU", default=None)
    gpu: Optional[List[str]] = Field(alias="GPU", default=None)
    ram: Optional[int] = Field(alias="RAM", default=None)


class Device(BaseModel):
    """Device data model."""
    id: str = Field(alias="ID")
    reference_token: Optional[str] = Field(alias="ReferenceToken", default=None)
    status: Optional[DeviceStatus] = Field(alias="Status", default=None)
    sys_info: Optional[SystemInformation] = Field(alias="SysInfo", default=None)
    timestamp: Optional[datetime] = Field(alias="Timestamp", default=None)

class NodeInfo(BaseModel):
    "Node Info data model"
    id: str = Field(alias="NodeID")
    addresses: List[str] = Field(alias="Addresses")
    bootstrapper: bool = Field(alias="Bootstrapper")
    relay_node: bool = Field(alias="RelayNode")
    reference_token: str = Field(alias="ReferenceToken")
    devices_count: int = Field(alias="DevicesCount")
    policy: dict = Field(alias="Policy")

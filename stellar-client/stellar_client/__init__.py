"""Stellar Client - Python client for Stellar platform.

This package provides a docker-py-like interface for interacting with Stellar nodes.

Quick Start:
    >>> import stellar_client
    >>> client = stellar_client.from_env()
    >>> devices = client.devices.list()
    >>> device = client.devices.get("device_id")
    >>> run = client.compute.run("device_id", "echo", ["hello"])
    >>> for line in run.logs(stream=True):
    ...     print(line)
"""

__version__ = "0.1.0"

# Main client and factory function
from .client import StellarClient, from_env

# Resource managers and objects
from .resources import (
    Devices,
    Device,
    Compute,
    ComputeRun,
    Proxy,
    ProxyConnection,
    Policy,
)

# Exception imports
from .exceptions import (
    StellarException,
    ConnectionError,
    SocketNotFoundError,
    NodeNotRunningError,
    ProtocolError,
    DeviceNotFoundError,
    AuthorizationError,
    ComputeError,
    FileTransferError,
    TimeoutError,
)

# Models
from .models.device import Device as DeviceModel, NodeInfo
from .models.policy import SecurityPolicy

__all__ = [
    # Factory function (main entry point)
    "from_env",
    
    # Main client interface
    "StellarClient",
    
    # Resource managers
    "Devices",
    "Compute",
    "Proxy",
    "Policy",
    
    # Resource objects
    "Device",
    "ComputeRun",
    "ProxyConnection",
    
    # Exceptions
    "StellarException",
    "ConnectionError", 
    "SocketNotFoundError",
    "NodeNotRunningError",
    "ProtocolError",
    "DeviceNotFoundError",
    "AuthorizationError",
    "ComputeError",
    "FileTransferError",
    "TimeoutError",
    
    # Models
    "DeviceModel",
    "NodeInfo",
    "SecurityPolicy",
]
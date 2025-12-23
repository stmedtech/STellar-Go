"""Stellar Client - Python client for Stellar platform."""

__version__ = "0.1.0"

# Main client
from .client import StellarClient

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

# Protocols
from .protocols.echo import EchoProtocol
from .protocols.file import FileProtocol
from .protocols.compute import ComputeProtocol
from .protocols.proxy import ProxyProtocol

__all__ = [
    # Main client interface
    "StellarClient",
    
    # Protocols
    "EchoProtocol",
    "FileProtocol",
    "ComputeProtocol",
    "ProxyProtocol",
    
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
]
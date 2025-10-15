"""DCFL Client - Python client for Decentralized Federated Learning platform."""

__version__ = "0.1.0"

# Main client
from .client import DCFLClient

# Exception imports
from .exceptions import (
    DCFLException,
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
    "DCFLClient",
    
    # Protocols
    "EchoProtocol",
    "FileProtocol",
    "ComputeProtocol",
    "ProxyProtocol",
    
    # Exceptions
    "DCFLException",
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
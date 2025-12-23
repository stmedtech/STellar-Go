"""Exception classes for Stellar Client."""


class StellarException(Exception):
    """Base exception for Stellar client."""
    pass


class ConnectionError(StellarException):
    """Connection-related errors."""
    pass


class SocketNotFoundError(ConnectionError):
    """Unix socket file not found."""
    pass


class NodeNotRunningError(ConnectionError):
    """Stellar node is not running."""
    pass


class ProtocolError(StellarException):
    """Protocol-level errors."""
    pass


class DeviceNotFoundError(ProtocolError):
    """Target device not found or unreachable."""
    pass


class AuthorizationError(ProtocolError):
    """Device not authorized (policy violation)."""
    pass


class ComputeError(ProtocolError):
    """Compute execution errors."""
    pass


class FileTransferError(ProtocolError):
    """File transfer errors."""
    pass


class TimeoutError(StellarException):
    """Operation timeout."""
    pass
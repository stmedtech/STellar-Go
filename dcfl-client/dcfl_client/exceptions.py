"""Exception classes for DCFL Client."""


class DCFLException(Exception):
    """Base exception for DCFL client."""
    pass


class ConnectionError(DCFLException):
    """Connection-related errors."""
    pass


class SocketNotFoundError(ConnectionError):
    """Unix socket file not found."""
    pass


class NodeNotRunningError(ConnectionError):
    """DCFL node is not running."""
    pass


class ProtocolError(DCFLException):
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


class TimeoutError(DCFLException):
    """Operation timeout."""
    pass
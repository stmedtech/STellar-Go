"""Helper utility functions."""

import os
import platform
import functools
from typing import Optional, Dict, Any, Callable, Union
from ..exceptions import DeviceNotFoundError, ProtocolError


def get_default_socket_path() -> Optional[str]:
    """Get the default Stellar Unix socket path if it exists.
    
    Returns:
        Socket path if it exists, None otherwise
    """
    if platform.system() == "Windows":
        # Windows named pipe path
        socket_path = r"C:\Users\Joseph\AppData\Roaming\Stellar\stellar.sock"
    else:
        # Unix socket path - check both possible locations
        home = os.path.expanduser("~")
        
        # First try the actual location used by the Go server (lowercase)
        stellar_dir = os.path.join(home, ".local", "share", "stellar")
        socket_path = os.path.join(stellar_dir, "stellar.sock")
    
    # Return socket path only if it exists
    if os.path.exists(socket_path):
        return socket_path
    return None


def resolve_connection_path(socket_path: Optional[str] = None) -> str:
    """Resolve the connection path/URL for Stellar client.
    
    Priority order (when socket_path is not provided):
    1. STELLAR_NODE_URL environment variable
    2. Default Unix socket path (if exists)
    3. HTTP fallback: http://127.0.0.1:1524
    
    Args:
        socket_path: Explicit socket path or URL to use (bypasses auto-resolution)
        
    Returns:
        Connection path (socket path) or URL string
    """
    # If explicitly provided, use it directly (bypasses all auto-resolution)
    if socket_path is not None:
        return socket_path
    
    # Priority 1: Check STELLAR_NODE_URL environment variable
    env_url = os.environ.get("STELLAR_NODE_URL")
    if env_url:
        return env_url.strip()
    
    # Priority 2: Try default Unix socket path
    default_socket = get_default_socket_path()
    if default_socket:
        return default_socket
    
    # Priority 3: Fallback to HTTP endpoint
    return "http://127.0.0.1:1524"


def validate_device_id(device_id: str) -> bool:
    """Validate device ID format."""
    if not device_id or not isinstance(device_id, str):
        return False
    
    # Basic validation - device IDs should be non-empty strings
    # Could be enhanced with more specific validation rules
    return len(device_id.strip()) > 0


def format_bytes(bytes_count: int) -> str:
    """Format byte count in human readable format."""
    for unit in ['B', 'KB', 'MB', 'GB', 'TB']:
        if bytes_count < 1024.0:
            return f"{bytes_count:.1f} {unit}"
        bytes_count /= 1024.0
    return f"{bytes_count:.1f} PB"


def sanitize_file_path(file_path: str) -> str:
    """Sanitize file path to prevent directory traversal."""
    # Handle root path specially
    if file_path == "/" or file_path == "":
        return "/"
    
    # Remove any path traversal attempts
    sanitized = os.path.normpath(file_path)
    
    # Remove leading dots to prevent directory traversal, but preserve leading slash for absolute paths
    while sanitized.startswith(('.', '\\')):
        sanitized = sanitized[1:]
    
    # Ensure path starts with / for absolute paths (but not for relative paths)
    if not sanitized.startswith('/') and file_path.startswith('/'):
        sanitized = '/' + sanitized
        
    return sanitized


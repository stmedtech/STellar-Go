"""Helper utility functions."""

import os
import platform
import functools
from typing import Optional, Dict, Any, Callable, Union
from ..exceptions import DeviceNotFoundError, ProtocolError


def get_default_socket_path() -> str:
    """Get the default Stellar Unix socket path."""
    if platform.system() == "Windows":
        # Windows named pipe path
        return r"C:\Users\Joseph\AppData\Roaming\Stellar\stellar.sock"
    else:
        # Unix socket path - check both possible locations
        home = os.path.expanduser("~")
        
        # First try the actual location used by the Go server
        stellar_dir = os.path.join(home, ".local", "share", "Stellar")
        socket_path = os.path.join(stellar_dir, "stellar.sock")
        if os.path.exists(socket_path):
            return socket_path
            
        # Fallback to the old cache location
        cache_dir = os.path.join(home, ".cache", "stellar")
        return os.path.join(cache_dir, "stellar.sock")


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
    # Remove any path traversal attempts
    sanitized = os.path.normpath(file_path)
    
    # Remove leading slashes and dots to prevent absolute path access
    while sanitized.startswith(('/', '.', '\\')):
        sanitized = sanitized[1:]
        
    return sanitized


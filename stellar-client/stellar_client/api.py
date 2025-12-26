"""Low-level API client for Stellar socket communication."""

from typing import Optional, Dict, Any, List
from .utils.socket_client import UnixSocketClient
from .utils.helpers import get_default_socket_path
from .exceptions import StellarException, ConnectionError


class APIClient:
    """Low-level API client for communicating with Stellar socket."""
    
    def __init__(self, socket_path: Optional[str] = None, timeout: int = 30):
        """Initialize API client.
        
        Args:
            socket_path: Path to Unix socket (uses default if None)
            timeout: Request timeout in seconds
        """
        self.socket_path = socket_path or get_default_socket_path()
        self._client = UnixSocketClient(self.socket_path, timeout)
    
    def get(self, path: str, params: Optional[Dict[str, Any]] = None, 
            raw: bool = False) -> Any:
        """Make GET request.
        
        Args:
            path: API endpoint path
            params: Query parameters
            raw: If True, return raw text/bytes instead of parsing JSON
            
        Returns:
            Response data (dict if JSON, str/bytes if raw)
            
        Raises:
            ConnectionError: If connection fails
        """
        try:
            if raw:
                return self._client.get_raw(path, params=params)
            return self._client.get(path, params=params)
        except Exception as e:
            raise ConnectionError(f"GET {path} failed: {e}") from e
    
    def post(self, path: str, json_data: Optional[Dict[str, Any]] = None, 
             data: Optional[Dict[str, Any]] = None) -> Any:
        """Make POST request.
        
        Args:
            path: API endpoint path
            json_data: JSON payload
            data: Form data payload
            
        Returns:
            Response data
            
        Raises:
            ConnectionError: If connection fails
        """
        try:
            return self._client.post(path, json_data=json_data, data=data)
        except Exception as e:
            raise ConnectionError(f"POST {path} failed: {e}") from e
    
    def delete(self, path: str, params: Optional[Dict[str, Any]] = None, 
               data: Optional[Dict[str, Any]] = None) -> Any:
        """Make DELETE request.
        
        Args:
            path: API endpoint path
            params: Query parameters
            data: Form data (for endpoints that expect form data in body)
            
        Returns:
            Response data
            
        Raises:
            ConnectionError: If connection fails
        """
        try:
            return self._client.delete(path, params=params, data=data)
        except Exception as e:
            raise ConnectionError(f"DELETE {path} failed: {e}") from e
    
    def close(self) -> None:
        """Close the API client connection."""
        self._client.close()
    
    def __enter__(self):
        """Context manager entry."""
        return self
    
    def __exit__(self, exc_type, exc_val, exc_tb):
        """Context manager exit."""
        self.close()


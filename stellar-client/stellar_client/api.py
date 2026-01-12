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
            raw: If True, return raw text instead of parsing JSON
            
        Returns:
            Response data (dict if JSON, str if raw)
            
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
             data: Optional[Dict[str, Any]] = None, raw_data: Optional[bytes] = None,
             files: Optional[Dict[str, Any]] = None) -> Any:
        """Make POST request.
        
        Args:
            path: API endpoint path
            json_data: JSON payload
            data: Form data payload
            raw_data: Raw bytes payload (for stdin, file uploads, etc.)
            files: Files for multipart/form-data uploads (dict of {field_name: file_object})
            
        Returns:
            Response data
            
        Raises:
            ConnectionError: If connection fails
        """
        try:
            if raw_data is not None:
                # Use internal method to send raw data
                response = self._client.request_with_retry("POST", path, raw_data=raw_data)
                try:
                    if response.status_code == 204 or not response.content:
                        return {}
                    return response.json()
                except ValueError:
                    if response.text:
                        return {"raw": response.text}
                    return {}
            if files is not None:
                # Use internal method for multipart uploads
                response = self._client.request_with_retry("POST", path, files=files, data=data)
                try:
                    if response.status_code == 204 or not response.content:
                        return {}
                    return response.json()
                except ValueError:
                    if response.text:
                        return {"raw": response.text}
                    return {}
            return self._client.post(path, json_data=json_data, data=data)
        except Exception as e:
            raise ConnectionError(f"POST {path} failed: {e}") from e
    
    def get_raw_bytes(self, path: str, params: Optional[Dict[str, Any]] = None) -> bytes:
        """Make GET request and return raw bytes (for file downloads).
        
        Args:
            path: API endpoint path
            params: Query parameters
            
        Returns:
            Raw bytes response
            
        Raises:
            ConnectionError: If connection fails
        """
        try:
            response = self._client.request_with_retry("GET", path, params=params)
            return response.content
        except Exception as e:
            raise ConnectionError(f"GET {path} failed: {e}") from e
    
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


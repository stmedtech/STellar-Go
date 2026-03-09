"""Unix socket client implementation."""

import os
import time
from typing import Dict, Any, Optional
import requests
import socket
from urllib3.connection import HTTPConnection
from urllib3.connectionpool import HTTPConnectionPool
from requests.adapters import HTTPAdapter

from ..exceptions import (
    SocketNotFoundError,
    NodeNotRunningError,
    ConnectionError,
    TimeoutError,
)
from .retry import RetryPolicy
from .circuit_breaker import CircuitBreaker


class StellarConnection(HTTPConnection):
    def __init__(self, socket_path: str, timeout: float = 30.0):
        # Initialize with dummy hostname/port for URL parsing
        # HTTPConnection requires host and port, but we'll override connect()
        super().__init__("stellar", port=80, timeout=timeout)
        self.socket_path = socket_path
        self.timeout = timeout

    def connect(self):
        # Override to use Unix socket instead of TCP
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        # Set socket timeout to prevent indefinite blocking
        # This timeout applies to all socket operations (connect, send, recv)
        self.sock.settimeout(self.timeout)
        self.sock.connect(self.socket_path)
        # Ensure timeout persists - urllib3 might reset it during operations
        # Store timeout so we can reapply it if needed
        if self.sock:
            self.sock.settimeout(self.timeout)
    
    def _read_status(self):
        """Override to ensure timeout is set before reading status."""
        # Ensure socket timeout is set before any read operations
        if self.sock:
            self.sock.settimeout(self.timeout)
        return super()._read_status()
    
    def _read_response(self, buffered=False):
        """Override to ensure timeout is set before reading response."""
        # Ensure socket timeout is set before any read operations
        if self.sock:
            self.sock.settimeout(self.timeout)
        return super()._read_response(buffered)


class StellarConnectionPool(HTTPConnectionPool):
    def __init__(self, socket_path: str, timeout: float = 30.0):
        # Use dummy hostname for URL parsing
        # Use "localhost" so URL matching works correctly
        super().__init__("localhost", port=None, timeout=timeout)
        self.socket_path = socket_path
        self.timeout = timeout

    def _new_conn(self):
        return StellarConnection(self.socket_path, timeout=self.timeout)


class StellarAdapter(HTTPAdapter):
    def __init__(self, socket_path: str = None, timeout: float = 30.0):
        # Initialize socket_path BEFORE calling super() because
        # super().__init__() may call init_poolmanager which checks self.socket_path
        self.socket_path = socket_path or ""
        self.timeout = timeout
        self._pool = None
        super().__init__()
    
    def bind_socket_path(self, socket_path: str, timeout: float = 30.0):
        self.socket_path = socket_path
        self.timeout = timeout
        self._pool = None  # Reset pool when socket path changes
    
    def init_poolmanager(self, *args, **kwargs):
        # Always initialize poolmanager for HTTP fallback
            super().init_poolmanager(*args, **kwargs)
    
    def get_connection(self, url, proxies=None):
        # Only use Unix socket for localhost URLs, use parent's HTTP for others
        if self.socket_path:
            # Check if URL is for localhost (Unix socket URLs)
            # URL can be a string like "http://localhost/health" or a parsed URL object
            url_str = str(url) if not isinstance(url, str) else url
            # Also check for urllib3 URL objects which have hostname attribute
            if hasattr(url, 'hostname'):
                hostname = url.hostname
            else:
                hostname = None
            
            # Use Unix socket if URL contains localhost or hostname is localhost
            if (hostname == "localhost" or "localhost" in url_str):
                if self._pool is None:
                    self._pool = StellarConnectionPool(self.socket_path, timeout=self.timeout)
                return self._pool
        # For other URLs (like 127.0.0.1:1524), use parent's HTTP connection
        return super().get_connection(url, proxies)
    
    def get_connection_with_tls_context(self, host, port=None, **kwargs):
        """Override to handle Unix socket connections."""
        # If we have a socket path, always use Unix socket (we're mounted to http://)
        # Only fall back to HTTP if explicitly using 127.0.0.1:1524
        if self.socket_path and not (host == "127.0.0.1" and port == 1524):
            if self._pool is None:
                self._pool = StellarConnectionPool(self.socket_path, timeout=self.timeout)
            return self._pool
        # Otherwise use parent's implementation
        return super().get_connection_with_tls_context(host, port, **kwargs)

class UnixSocketClient:
    """Unix socket HTTP client for Stellar node communication."""
    
    def __init__(self, socket_path: str, timeout: int = 30):
        """Initialize Unix socket client.
        
        Args:
            socket_path: Path to the Unix socket or HTTP URL (e.g., http://127.0.0.1:1524)
            timeout: Request timeout in seconds
        """
        self.timeout = timeout
        self.session = requests.Session()
        self.retry_policy = RetryPolicy()
        self.circuit_breaker = CircuitBreaker()
        
        # Check if socket_path is actually a URL
        if socket_path.startswith(("http://", "https://")):
            # It's a URL, use HTTP directly
            self.socket_path = None
            self.use_unix_socket = False
            self.base_url = socket_path.rstrip('/')
            self.http_adapter = HTTPAdapter()
            self.unix_adapter = None
            self.session.mount("http://", self.http_adapter)
            if socket_path.startswith("https://"):
                self.session.mount("https://", self.http_adapter)
        else:
            # It's a socket path
            self.socket_path = socket_path
            self.use_unix_socket = False
            self.base_url = "http://127.0.0.1:1524"
            self.http_adapter = HTTPAdapter()
            self.unix_adapter = StellarAdapter(socket_path=self.socket_path, timeout=float(self.timeout))
            
            # Start with HTTP adapter
            self.session.mount("http://", self.http_adapter)
            
    def _ensure_connection(self) -> None:
        """Ensure socket connection is healthy."""
        # If using a URL directly, skip socket checks
        if self.socket_path is None:
            # Already using HTTP URL, just verify connection
            try:
                response = self._make_request("GET", "/health")
                if response.status_code != 200:
                    raise NodeNotRunningError(f"Node health check failed with status {response.status_code}")
            except (requests.exceptions.ConnectionError, requests.exceptions.RequestException) as e:
                raise ConnectionError(f"Connection failed to {self.base_url}: {e}") from e
            return
        
        # Always check socket availability at connection time
        socket_exists = os.path.exists(self.socket_path) and os.name != 'nt' if self.socket_path else False
        
        # Prefer Unix socket if available
        if socket_exists:
            if not os.access(self.socket_path, os.R_OK):
                raise SocketNotFoundError(f"Socket not accessible: {self.socket_path}")
            
            # Switch to Unix socket if not already using it
            if not self.use_unix_socket:
                self.use_unix_socket = True
                self.base_url = "http://localhost"
                # Update adapter with current socket path and remount
                self.unix_adapter.bind_socket_path(self.socket_path)
                self.session.mount("http://", self.unix_adapter)
        else:
            # Socket doesn't exist, use HTTP fallback
            if self.use_unix_socket:
                self.use_unix_socket = False
                self.base_url = "http://127.0.0.1:1524"
                # Remount HTTP adapter
                self.session.mount("http://", self.http_adapter)
        
        # Test connection with health check
        try:
            response = self._make_request("GET", "/health")
            if response.status_code != 200:
                raise NodeNotRunningError(f"Node health check failed with status {response.status_code}")
        except (requests.exceptions.ConnectionError, requests.exceptions.RequestException) as e:
            # If using Unix socket and it fails, try HTTP fallback
            if self.use_unix_socket:
                try:
                    self.use_unix_socket = False
                    self.base_url = "http://127.0.0.1:1524"
                    self.session.mount("http://", self.http_adapter)
                    response = self._make_request("GET", "/health")
                    if response.status_code != 200:
                        raise NodeNotRunningError(f"Node health check failed with status {response.status_code}")
                except Exception as http_e:
                    # Both failed, raise original error
                    raise ConnectionError(f"Connection failed with both Unix socket and HTTP. Socket error: {e}, HTTP error: {http_e}") from e
            else:
                # If using HTTP and socket now exists, try Unix socket
                if socket_exists and os.name != 'nt':
                    try:
                        self.use_unix_socket = True
                        self.base_url = "http://localhost"
                        self.unix_adapter.bind_socket_path(self.socket_path)
                        self.session.mount("http://", self.unix_adapter)
                        response = self._make_request("GET", "/health")
                        if response.status_code != 200:
                            raise NodeNotRunningError(f"Node health check failed with status {response.status_code}")
                    except Exception as socket_e:
                        raise ConnectionError(f"Connection failed with both HTTP and Unix socket. HTTP error: {e}, Socket error: {socket_e}") from e
                else:
                    raise ConnectionError(f"Connection failed: {e}") from e
            
    def _make_request(
        self,
        method: str,
        endpoint: str,
        params: Optional[Dict[str, Any]] = None,
        json_data: Optional[Dict[str, Any]] = None,
        data: Optional[Dict[str, Any]] = None,
        raw_data: Optional[bytes] = None,
        files: Optional[Dict[str, Any]] = None,
    ) -> requests.Response:
        """Make HTTP request to the Unix socket.
        
        Args:
            method: HTTP method
            endpoint: API endpoint path
            params: Query parameters
            json_data: JSON payload
            data: Form data payload
            raw_data: Raw bytes payload
            files: Files for multipart/form-data uploads
        """
        # Ensure endpoint starts with / and base_url doesn't end with /
        base = self.base_url.rstrip('/')
        endpoint = endpoint if endpoint.startswith('/') else f'/{endpoint}'
        url = f"{base}{endpoint}"
        
        try:
            # Use a tuple for timeout: (connect_timeout, read_timeout)
            # This ensures both connection and read operations respect the timeout
            timeout_tuple = (self.timeout, self.timeout)
            
            if raw_data is not None:
                response = self.session.request(
                    method=method,
                    url=url,
                    params=params,
                    data=raw_data,
                    timeout=timeout_tuple,
                )
            elif files is not None:
                # Multipart/form-data upload
                response = self.session.request(
                    method=method,
                    url=url,
                    params=params,
                    files=files,
                    data=data,
                    timeout=timeout_tuple,
                )
            else:
                response = self.session.request(
                    method=method,
                    url=url,
                    params=params,
                    json=json_data,
                    data=data,
                    timeout=timeout_tuple,
                )
            response.raise_for_status()
            return response
        except (requests.exceptions.Timeout, socket.timeout) as e:
            raise TimeoutError(f"Request to {endpoint} timed out after {self.timeout}s: {e}")
        except requests.exceptions.RequestException as e:
            raise ConnectionError(f"Request failed: {e}")
            
    def request_with_retry(
        self,
        method: str,
        endpoint: str,
        raw_data: Optional[bytes] = None,
        files: Optional[Dict[str, Any]] = None,
        **kwargs,
    ) -> requests.Response:
        """Make HTTP request with retry logic and circuit breaker.
        
        Args:
            method: HTTP method
            endpoint: API endpoint path
            raw_data: Raw bytes payload
            files: Files for multipart/form-data uploads
            **kwargs: Additional arguments passed to _make_request
        """
        def _make_request_with_check():
            self._ensure_connection()
            return self._make_request(method, endpoint, raw_data=raw_data, files=files, **kwargs)
            
        return self.circuit_breaker.call(
            self.retry_policy.execute_with_retry,
            _make_request_with_check
        )
        
    def get(
        self,
        endpoint: str,
        params: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        """Make GET request and parse as JSON."""
        response = self.request_with_retry("GET", endpoint, params=params)
        try:
            # Handle empty responses
            if response.status_code == 204 or not response.content:
                return {}
            return response.json()
        except ValueError as e:
            # If JSON parsing fails, check content type
            content_type = response.headers.get('Content-Type', '')
            # For streaming endpoints that return text/plain or streaming JSON
            if 'text/plain' in content_type:
                # Return as text wrapped in dict for consistency
                raise ConnectionError(f"Response is text/plain, not JSON. Use get_raw() instead. Content: {response.text[:200]}")
            # For application/json that might be streaming JSON lines
            if 'application/json' in content_type:
                # Try to parse as JSON lines (NDJSON)
                try:
                    lines = response.text.strip().split('\n')
                    if len(lines) == 1:
                        # Single JSON object
                        return response.json()
                    else:
                        # Multiple JSON lines - return as list
                        import json
                        return {"lines": [json.loads(line) for line in lines if line.strip()]}
                except:
                    # If that fails, return raw text
                    raise ConnectionError(f"Failed to parse JSON response: {e}. Content-Type: {content_type}, Body: {response.text[:200]}") from e
            raise ConnectionError(f"Failed to parse JSON response: {e}. Content-Type: {content_type}, Body: {response.text[:200]}") from e
    
    def get_raw(
        self,
        endpoint: str,
        params: Optional[Dict[str, Any]] = None,
    ) -> str:
        """Make GET request and return raw text."""
        response = self.request_with_retry("GET", endpoint, params=params)
        return response.text
    
    def get_raw_bytes(
        self,
        endpoint: str,
        params: Optional[Dict[str, Any]] = None,
    ) -> bytes:
        """Make GET request and return raw bytes (for binary file downloads)."""
        response = self.request_with_retry("GET", endpoint, params=params)
        return response.content
        
    def post(
        self,
        endpoint: str,
        json_data: Optional[Dict[str, Any]] = None,
        data: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        """Make POST request."""
        response = self.request_with_retry("POST", endpoint, json_data=json_data, data=data)
        try:
            # Handle empty responses
            if response.status_code == 204 or not response.content:
                return {}
            return response.json()
        except ValueError as e:
            # If JSON parsing fails, try to return text or empty dict
            if response.text:
                # Try to parse as text if it's not JSON
                return {"raw": response.text}
            return {}
        except Exception as e:
            # Log the error but don't print (use proper logging in production)
            raise ConnectionError(f"Failed to parse response: {e}, status={response.status_code}, body={response.text[:200]}") from e
        
    def put(
        self,
        endpoint: str,
        json_data: Optional[Dict[str, Any]] = None,
        data: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        """Make PUT request."""
        response = self.request_with_retry("PUT", endpoint, json_data=json_data, data=data)
        return response.json()
        
    def delete(
        self,
        endpoint: str,
        params: Optional[Dict[str, Any]] = None,
        data: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        """Make DELETE request."""
        response = self.request_with_retry("DELETE", endpoint, params=params, data=data)
        return response.json()
        
    def close(self) -> None:
        """Close the session."""
        if self.session:
            self.session.close()
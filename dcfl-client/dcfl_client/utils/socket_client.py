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
    def __init__(self, socket_path: str):
        super().__init__("localhost")
        self.socket_path = socket_path

    def connect(self):
        # self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.connect(self.socket_path)


class StellarConnectionPool(HTTPConnectionPool):
    def __init__(self, socket_path: str):
        super().__init__("localhost")
        self.socket_path = socket_path

    def _new_conn(self):
        return StellarConnection(self.socket_path)


class StellarAdapter(HTTPAdapter):
    def bind_socket_path(self, socket_path: str):
        self.socket_path = socket_path
    
    def get_connection_with_tls_context(self, request, verify, proxies=None, cert=None):
        return StellarConnectionPool(self.socket_path)

class UnixSocketClient:
    """Unix socket HTTP client for DCFL node communication."""
    
    def __init__(self, socket_path: str, timeout: int = 30):
        """Initialize Unix socket client.
        
        Args:
            socket_path: Path to the Unix socket
            timeout: Request timeout in seconds
        """
        self.socket_path = socket_path
        self.timeout = timeout
        self.session = requests.Session()
        self.retry_policy = RetryPolicy()
        self.circuit_breaker = CircuitBreaker()
        
        # Configure session for Unix socket
        if os.name != 'nt':  # Unix systems
            from requests_unixsocket import UnixAdapter
            self.session.mount("http+unix://", UnixAdapter())
            self.base_url = f"http+unix://{self.socket_path.replace('/', '%2F')}"
        else:  # Windows - use named pipes (would need additional implementation)
            self.session.mount("http://", HTTPAdapter())
            self.base_url = f"http://127.0.0.1:1524"
            # raise NotImplementedError("Windows named pipes not yet implemented")
        
        # self.base_url = "http://stellar/"
        # adapter = StellarAdapter()
        # adapter.bind_socket_path(self.socket_path)
        # self.session.mount(self.base_url, adapter)
            
    def _ensure_connection(self) -> None:
        """Ensure socket connection is healthy."""
        if not os.path.exists(self.socket_path):
            raise SocketNotFoundError(f"Socket not found: {self.socket_path}")
            
        try:
            # Test connection with health check
            response = self._make_request("GET", "/health")
            if response.status_code != 200:
                raise NodeNotRunningError("Node health check failed")
        except requests.exceptions.RequestException as e:
            raise ConnectionError(f"Connection failed: {e}")
            
    def _make_request(
        self,
        method: str,
        endpoint: str,
        params: Optional[Dict[str, Any]] = None,
        json_data: Optional[Dict[str, Any]] = None,
        data: Optional[Dict[str, Any]] = None,
    ) -> requests.Response:
        """Make HTTP request to the Unix socket."""
        url = f"{self.base_url}{endpoint}"
        
        try:
            response = self.session.request(
                method=method,
                url=url,
                params=params,
                json=json_data,
                data=data,
                timeout=self.timeout,
            )
            response.raise_for_status()
            return response
        except requests.exceptions.Timeout:
            raise TimeoutError(f"Request to {endpoint} timed out")
        except requests.exceptions.RequestException as e:
            raise ConnectionError(f"Request failed: {e}")
            
    def request_with_retry(
        self,
        method: str,
        endpoint: str,
        **kwargs,
    ) -> requests.Response:
        """Make HTTP request with retry logic and circuit breaker."""
        def _make_request_with_check():
            self._ensure_connection()
            return self._make_request(method, endpoint, **kwargs)
            
        return self.circuit_breaker.call(
            self.retry_policy.execute_with_retry,
            _make_request_with_check
        )
        
    def get(
        self,
        endpoint: str,
        params: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        """Make GET request."""
        response = self.request_with_retry("GET", endpoint, params=params)
        return response.json()
        
    def post(
        self,
        endpoint: str,
        json_data: Optional[Dict[str, Any]] = None,
        data: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        """Make POST request."""
        response = self.request_with_retry("POST", endpoint, json_data=json_data, data=data)
        return response.json()
        
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
    ) -> Dict[str, Any]:
        """Make DELETE request."""
        response = self.request_with_retry("DELETE", endpoint, params=params)
        return response.json()
        
    def close(self) -> None:
        """Close the session."""
        if self.session:
            self.session.close()
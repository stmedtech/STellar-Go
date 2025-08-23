"""Proxy protocol implementation."""

from typing import List, Optional
import requests
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry

from ..utils.socket_client import UnixSocketClient
from ..models.responses import ProxyInfo
from ..exceptions import DeviceNotFoundError, ProtocolError


class ProxyProtocol:
    """Proxy protocol for network tunneling."""
    
    def __init__(self, socket_client: UnixSocketClient):
        """Initialize proxy protocol.
        
        Args:
            socket_client: Unix socket client instance
        """
        self.client = socket_client
    
    def create_tcp_proxy(
        self,
        device_id: str,
        local_port: int,
        remote_host: str,
        remote_port: int,
    ) -> ProxyInfo:
        """Create TCP proxy tunnel through remote device.
        
        Args:
            device_id: Target device ID to proxy through
            local_port: Local port to bind proxy to
            remote_host: Remote host to connect to
            remote_port: Remote port to connect to
            
        Returns:
            Proxy connection information
            
        Raises:
            DeviceNotFoundError: If device is not found
            ProtocolError: If proxy creation fails
        """
            
        if not (1 <= local_port <= 65535):
            raise ProtocolError(f"Invalid local port: {local_port}")
            
        if not (1 <= remote_port <= 65535):
            raise ProtocolError(f"Invalid remote port: {remote_port}")
            
        try:
            response = self.client.post(
                "/proxy",
                json_data={
                    "device_id": device_id,
                    "local_port": local_port,
                    "remote_host": remote_host,
                    "remote_port": remote_port,
                }
            )
            
            if response.get("error", False):
                raise ProtocolError(f"Proxy creation failed: {response.get('error', 'Unknown error')}")
                
            return ProxyInfo.model_validate(response)
            
        except Exception as e:
            raise ProtocolError(f"Failed to create TCP proxy through device {device_id}: {e}")
    
    def create_http_proxy(
        self,
        device_id: str,
        local_port: int,
        remote_url: str,
    ) -> ProxyInfo:
        """Create HTTP proxy tunnel through remote device.
        
        Args:
            device_id: Target device ID to proxy through
            local_port: Local port to bind proxy to
            remote_url: Remote URL to proxy to
            
        Returns:
            Proxy connection information
            
        Raises:
            DeviceNotFoundError: If device is not found
            ProtocolError: If proxy creation fails
        """
        # Parse URL to extract host and port
        from urllib.parse import urlparse
        
        parsed = urlparse(remote_url)
        if not parsed.hostname:
            raise ProtocolError(f"Invalid remote URL: {remote_url}")
            
        remote_port = parsed.port or (443 if parsed.scheme == "https" else 80)
        
        return self.create_tcp_proxy(
            device_id=device_id,
            local_port=local_port,
            remote_host=parsed.hostname,
            remote_port=remote_port,
        )
    
    def list_proxies(self) -> List[ProxyInfo]:
        """List all active proxy connections.
        
        Returns:
            List of active proxy connections
            
        Raises:
            ProtocolError: If proxy listing fails
        """
        try:
            response = self.client.get("/proxy")
            
            if isinstance(response, list):
                return [ProxyInfo.model_validate(proxy) for proxy in response]
            else:
                # Return empty list if no proxies
                return []
                
        except Exception as e:
            raise ProtocolError(f"Failed to list proxies: {e}")
    
    def get_proxy_info(self, local_port: int) -> Optional[ProxyInfo]:
        """Get information about a specific proxy connection.
        
        Args:
            local_port: Local port of the proxy connection
            
        Returns:
            Proxy information if found, None otherwise
            
        Raises:
            ProtocolError: If proxy info retrieval fails
        """
        try:
            proxies = self.list_proxies()
            
            for proxy in proxies:
                if proxy.local_port == local_port:
                    return proxy
                    
            return None
            
        except Exception as e:
            raise ProtocolError(f"Failed to get proxy info for port {local_port}: {e}")
    
    def close_proxy(self, port: int) -> bool:
        """Close a proxy connection.
        
        Args:
            port: Local port of the proxy connection to close
            
        Returns:
            True if proxy was successfully closed
            
        Raises:
            ProtocolError: If proxy closure fails
        """
        if not (1 <= port <= 65535):
            raise ProtocolError(f"Invalid port: {port}")
            
        try:
            response = self.client.delete(f"/proxy/{port}")
            return response.get("success", False)
        except Exception as e:
            raise ProtocolError(f"Failed to close proxy on port {port}: {e}")
    
    def close_all_proxies(self) -> int:
        """Close all active proxy connections.
        
        Returns:
            Number of proxies that were closed
            
        Raises:
            ProtocolError: If operation fails
        """
        try:
            proxies = self.list_proxies()
            closed_count = 0
            
            for proxy in proxies:
                try:
                    if self.close_proxy(proxy.local_port):
                        closed_count += 1
                except Exception:
                    # Continue trying to close other proxies
                    pass
                    
            return closed_count
            
        except Exception as e:
            raise ProtocolError(f"Failed to close all proxies: {e}")
    
    def test_http_proxy_connection(self, local_port: int) -> bool:
        """Test if a http proxy connection is working.
        
        Args:
            local_port: Local port of the proxy to test
            
        Returns:
            True if proxy is working
        """
        try:
            proxy_info = self.get_proxy_info(local_port)
            if not proxy_info:
                return False
                
            # Basic check - if proxy info exists, assume it's working
            assert proxy_info.status == "active"
            
            retry_strategy = Retry(
                total=5,
                status_forcelist=[429, 500, 502, 503, 504],
                method_whitelist=["HEAD", "GET", "OPTIONS"]
            )
            adapter = HTTPAdapter(max_retries=retry_strategy)
            http = requests.Session()
            http.mount("http://", adapter)
            response = http.get(f"http://127.0.0.1:{local_port}")
            print(response)
            assert response.status_code == 200
        except Exception:
            return False
    
    def get_proxy_statistics(self, local_port: int) -> dict:
        """Get statistics for a proxy connection.
        
        Args:
            local_port: Local port of the proxy
            
        Returns:
            Dictionary with proxy statistics
            
        Note:
            This is a placeholder implementation. Real statistics would
            require backend support for connection metrics.
        """
        proxy_info = self.get_proxy_info(local_port)
        if not proxy_info:
            return {}
            
        # Placeholder statistics
        return {
            "local_port": local_port,
            "status": proxy_info.status,
            "device_id": proxy_info.device_id,
            "remote_addr": proxy_info.remote_addr,
            "bytes_sent": 0,  # Would need backend support
            "bytes_received": 0,  # Would need backend support
            "connections": 0,  # Would need backend support
            "uptime": 0,  # Would need backend support
        }

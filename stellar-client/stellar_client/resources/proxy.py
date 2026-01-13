"""Proxy resource manager and ProxyConnection object."""

from typing import List, Optional, Dict, Any
from ..api import APIClient
from ..exceptions import ProtocolError, DeviceNotFoundError
from ..utils.helpers import validate_device_id


class ProxyConnection:
    """Represents a proxy connection."""
    
    def __init__(self, client: APIClient, port: int, attrs: Optional[dict] = None):
        """Initialize ProxyConnection object.
        
        Args:
            client: API client instance
            port: Local port number
            attrs: Connection attributes (optional)
        """
        self.client = client
        self.port = port
        self._attrs = attrs
    
    @property
    def attrs(self) -> dict:
        """Get connection attributes."""
        return self._attrs or {}
    
    @property
    def remote_addr(self) -> Optional[str]:
        """Get remote address."""
        return self.attrs.get("remote_addr")
    
    @property
    def device_id(self) -> Optional[str]:
        """Get device ID."""
        return self.attrs.get("device_id")
    
    def close(self) -> None:
        """Close the proxy connection.
        
        Raises:
            ProtocolError: If close fails
        """
        try:
            self.client.delete(f"/proxy/{self.port}")
        except Exception as e:
            raise ProtocolError(f"Failed to close proxy on port {self.port}: {e}") from e
    
    def __repr__(self) -> str:
        """String representation."""
        return f"<ProxyConnection: port={self.port}, remote={self.remote_addr}>"


class Proxy:
    """Resource manager for proxy connections."""
    
    def __init__(self, client: APIClient):
        """Initialize Proxy manager.
        
        Args:
            client: API client instance
        """
        self.client = client
    
    def create(self, device_id: str, local_port: int, remote_host: str, 
               remote_port: int) -> ProxyConnection:
        """Create a proxy connection.
        
        Args:
            device_id: Device identifier
            local_port: Local port to bind
            remote_host: Remote host address
            remote_port: Remote port
            
        Returns:
            ProxyConnection object
            
        Raises:
            DeviceNotFoundError: If device not found
            ProtocolError: If proxy creation fails
        """
        if not validate_device_id(device_id):
            raise DeviceNotFoundError(f"Invalid device ID: {device_id}")
        
        payload = {
            "device_id": device_id,
            "local_port": local_port,
            "remote_host": remote_host,
            "remote_port": remote_port,
        }
        
        try:
            response = self.client.post("/proxy", json_data=payload)
            port = response.get("local_port", local_port)
            return ProxyConnection(self.client, port, response)
        except Exception as e:
            raise ProtocolError(f"Failed to create proxy: {e}") from e
    
    def list(self) -> List[ProxyConnection]:
        """List all proxy connections.
        
        Returns:
            List of ProxyConnection objects
            
        Raises:
            ProtocolError: If listing fails
        """
        try:
            response = self.client.get("/proxy")
            proxies = []
            for proxy_data in response:
                port = proxy_data.get("local_port")
                if port:
                    proxies.append(ProxyConnection(self.client, port, proxy_data))
            return proxies
        except Exception as e:
            raise ProtocolError(f"Failed to list proxies: {e}") from e
    
    def get(self, port: int) -> Optional[ProxyConnection]:
        """Get a proxy connection by port.
        
        Args:
            port: Local port number
            
        Returns:
            ProxyConnection object or None if not found
        """
        proxies = self.list()
        for proxy in proxies:
            if proxy.port == port:
                return proxy
        return None


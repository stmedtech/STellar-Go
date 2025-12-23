"""Main Stellar client implementation."""

from typing import List, Optional, Dict, Any
from .utils.socket_client import UnixSocketClient
from .utils.helpers import get_default_socket_path, validate_device_id
from .models.device import NodeInfo, Device
from .models.policy import SecurityPolicy
from .exceptions import DeviceNotFoundError, ProtocolError


class DeviceContext:
    """Context manager for device operations with self-device detection."""
    
    def __init__(self, client: 'StellarClient'):
        self.client = client
        self._local_device_id = None
        
    def get_local_device_id(self) -> Optional[str]:
        """Get the local node's device ID."""
        if self._local_device_id is None:
            try:
                node_info = self.client.get_node_info()
                self._local_device_id = node_info.id
            except Exception:
                pass
        return self._local_device_id
        
    def is_self_device(self, device_id: str) -> bool:
        """Check if device ID refers to the local device."""
        local_id = self.get_local_device_id()
        return local_id is not None and device_id == local_id

class StellarClient:
    """Synchronous Stellar client for communicating with Stellar nodes."""
    
    def __init__(self, socket_path: Optional[str] = None, timeout: int = 30):
        """Initialize Stellar client.
        
        Args:
            socket_path: Path to Unix socket (uses default if None)
            timeout: Request timeout in seconds
        """
        self.socket_path = socket_path or get_default_socket_path()
        self.socket_client = UnixSocketClient(self.socket_path, timeout)
        
        # Initialize protocol managers (lazy loading)
        self._echo = None
        self._file = None
        self._compute = None
        self._proxy = None
        
    @property
    def echo(self):
        """Get echo protocol manager."""
        if self._echo is None:
            from .protocols.echo import EchoProtocol
            self._echo = EchoProtocol(self.socket_client)
        return self._echo
        
    @property
    def file(self):
        """Get file protocol manager."""
        if self._file is None:
            from .protocols.file import FileProtocol
            self._file = FileProtocol(self.socket_client)
        return self._file
        
    @property
    def compute(self):
        """Get compute protocol manager."""
        if self._compute is None:
            from .protocols.compute import ComputeProtocol
            self._compute = ComputeProtocol(self.socket_client)
        return self._compute
        
    @property
    def proxy(self):
        """Get proxy protocol manager."""
        if self._proxy is None:
            from .protocols.proxy import ProxyProtocol
            self._proxy = ProxyProtocol(self.socket_client)
        return self._proxy
        
    def connect(self) -> None:
        """Establish connection to Stellar node."""
        # Connection is established lazily on first request
        pass
        
    def disconnect(self) -> None:
        """Disconnect from Stellar node."""
        self.socket_client.close()
        
    def __enter__(self):
        """Context manager entry."""
        self.connect()
        return self
        
    def __exit__(self, exc_type, exc_val, exc_tb):
        """Context manager exit."""
        self.disconnect()
    
    def is_self_device(self, device_id: str):
        """Check if device ID refers to current client"""
        return DeviceContext(self).is_self_device(device_id)
        
    # Device Management Methods
    def list_devices(self, include_self: bool=False) -> List[Device]:
        """List all discovered devices.
        
        Returns:
            List of discovered devices
        """
        response = self.socket_client.get("/devices")
        devices = []
        
        # Handle both dict and list responses
        if isinstance(response, dict):
            # If response is a dict, devices might be in the values
            device_data = response.values() if response else []
        else:
            device_data = response
            
        for device_info in device_data:
            device = Device.model_validate(device_info)
            if not include_self:
                if self.is_self_device(device.id):
                    continue
            devices.append(device)
            
        return devices
        
    def get_device(self, device_id: str) -> Device:
        """Get information about a specific device.
        
        Args:
            device_id: Device ID to query
            
        Returns:
            Device information
            
        Raises:
            DeviceNotFoundError: If device is not found
        """
        if not validate_device_id(device_id):
            raise DeviceNotFoundError(f"Invalid device ID: {device_id}")
            
        try:
            response = self.socket_client.get(f"/devices/{device_id}")
            return Device.model_validate(response)
        except Exception as e:
            raise DeviceNotFoundError(f"Device {device_id} not found: {e}")
            
    def connect_to_peer(self, peer_info: str) -> bool:
        """Connect to a new peer.
        
        Args:
            peer_info: Peer connection information (multiaddr format)
            
        Returns:
            True if connection successful
            
        Raises:
            ProtocolError: If connection fails
        """
        try:
            response = self.socket_client.post(
                "/connect",
                json_data={"peer_info": peer_info}
            )
            return response.get("success", False)
        except Exception as e:
            raise ProtocolError(f"Failed to connect to peer: {e}")
            
    def get_node_info(self) -> NodeInfo:
        """Get information about the local node.
        
        Returns:
            Node information
        """
        response = self.socket_client.get("/node")
        return NodeInfo.model_validate(response)
        
    # Policy Management Methods
    def get_policy(self) -> SecurityPolicy:
        """Get current security policy.
        
        Returns:
            Security policy configuration
        """
        response = self.socket_client.get("/policy")
        return SecurityPolicy.model_validate(response)
        
    def update_policy(self, policy: SecurityPolicy) -> bool:
        """Update security policy.
        
        Args:
            policy: New security policy configuration
            
        Returns:
            True if update successful
        """
        try:
            self.socket_client.post(
                "/policy",
                data={"enable": str(policy.enable).lower()}
            )
            return True
        except Exception:
            return False
            
    def whitelist_device(self, device_id: str) -> bool:
        """Add device to security whitelist.
        
        Args:
            device_id: Device ID to whitelist
            
        Returns:
            True if successful
        """
        if not validate_device_id(device_id):
            return False
            
        try:
            self.socket_client.post(
                "/policy/whitelist",
                data={"deviceId": device_id}
            )
            return True
        except Exception:
            return False
            
    def remove_from_whitelist(self, device_id: str) -> bool:
        """Remove device from security whitelist.
        
        Args:
            device_id: Device ID to remove from whitelist
            
        Returns:
            True if successful
        """
        if not validate_device_id(device_id):
            return False
            
        try:
            self.socket_client.delete(
                "/policy/whitelist",
                params={"deviceId": device_id}
            )
            return True
        except Exception:
            return False
            
    def get_whitelist(self) -> List[str]:
        """Get current device whitelist.
        
        Returns:
            List of whitelisted device IDs
        """
        response = self.socket_client.get("/policy/whitelist")
        return response if isinstance(response, list) else []
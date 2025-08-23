"""Echo protocol implementation."""

import json
from typing import Dict, Any
from ..utils.socket_client import UnixSocketClient
from ..models.device import Device
from ..exceptions import DeviceNotFoundError, ProtocolError


class EchoProtocol:
    """Echo protocol for ping and device information."""
    
    def __init__(self, socket_client: UnixSocketClient):
        """Initialize echo protocol.
        
        Args:
            socket_client: Unix socket client instance
        """
        self.client = socket_client
    
    def ping(self, device_id: str) -> Device:
        """Ping a remote device to check connectivity.
        
        Args:
            device_id: Target device ID
            
        Returns:
            Ping response with success status and timing
            
        Raises:
            DeviceNotFoundError: If device is not found
            ProtocolError: If ping operation fails
        """
        try:
            response = self.client.post(f"/devices/{device_id}/ping")
            return Device.model_validate(response)
        except Exception as e:
            raise ProtocolError(f"Ping failed for device {device_id}: {e}")

    def get_device_info(self, device_id: str) -> Device:
        """Get detailed device information via echo protocol.
        
        Args:
            device_id: Target device ID
            
        Returns:
            Detailed device information
            
        Raises:
            DeviceNotFoundError: If device is not found
            ProtocolError: If device info retrieval fails
        """
        try:
            response = self.client.get(f"/devices/{device_id}/info")
            # The response is raw JSON from the device, parse it
            if isinstance(response, str):
                device_data = json.loads(response)
            else:
                device_data = response
                
            return Device.model_validate(device_data)
        except json.JSONDecodeError as e:
            raise ProtocolError(f"Invalid device info response from {device_id}: {e}")
        except Exception as e:
            raise ProtocolError(f"Failed to get device info for {device_id}: {e}")

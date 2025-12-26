"""Policy resource manager."""

from typing import List, Optional
from ..api import APIClient
from ..models.policy import SecurityPolicy
from ..exceptions import ProtocolError
from ..utils.helpers import validate_device_id


class Policy:
    """Resource manager for security policy."""
    
    def __init__(self, client: APIClient):
        """Initialize Policy manager.
        
        Args:
            client: API client instance
        """
        self.client = client
    
    def get(self) -> SecurityPolicy:
        """Get current security policy.
        
        Returns:
            SecurityPolicy object
            
        Raises:
            ProtocolError: If retrieval fails
        """
        try:
            response = self.client.get("/policy")
            return SecurityPolicy.model_validate(response)
        except Exception as e:
            raise ProtocolError(f"Failed to get policy: {e}") from e
    
    def update(self, enable: bool) -> None:
        """Update security policy.
        
        Args:
            enable: Whether to enable the policy
            
        Raises:
            ProtocolError: If update fails
        """
        try:
            self.client.post("/policy", data={"enable": str(enable).lower()})
        except Exception as e:
            raise ProtocolError(f"Failed to update policy: {e}") from e
    
    def get_whitelist(self) -> List[str]:
        """Get current device whitelist.
        
        Returns:
            List of whitelisted device IDs
            
        Raises:
            ProtocolError: If retrieval fails
        """
        try:
            response = self.client.get("/policy/whitelist")
            return response if isinstance(response, list) else []
        except Exception as e:
            raise ProtocolError(f"Failed to get whitelist: {e}") from e
    
    def add_to_whitelist(self, device_id: str) -> None:
        """Add device to whitelist.
        
        Args:
            device_id: Device ID to whitelist
            
        Raises:
            ProtocolError: If addition fails
        """
        if not validate_device_id(device_id):
            raise ProtocolError(f"Invalid device ID: {device_id}")
        
        try:
            self.client.post("/policy/whitelist", data={"deviceId": device_id})
        except Exception as e:
            raise ProtocolError(f"Failed to add device to whitelist: {e}") from e
    
    def remove_from_whitelist(self, device_id: str) -> None:
        """Remove device from whitelist.
        
        Args:
            device_id: Device ID to remove from whitelist
            
        Raises:
            ProtocolError: If removal fails
        """
        if not validate_device_id(device_id):
            raise ProtocolError(f"Invalid device ID: {device_id}")
        
        try:
            # Use query parameter for DELETE requests (more standard)
            self.client.delete("/policy/whitelist", params={"deviceId": device_id})
        except Exception as e:
            raise ProtocolError(f"Failed to remove device from whitelist: {e}") from e


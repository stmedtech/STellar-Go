"""Main Stellar client implementation following docker-py patterns."""

from typing import Optional
from .api import APIClient
from .resources import Devices, Compute, Proxy, Policy
from .models.device import NodeInfo
from .exceptions import ConnectionError


class StellarClient:
    """Stellar client for communicating with Stellar nodes.
    
    This client follows docker-py patterns with resource-based API:
    
    Examples:
        >>> import stellar_client
        >>> client = stellar_client.from_env()
        >>> 
        >>> # List devices
        >>> devices = client.devices.list()
        >>> 
        >>> # Get a device
        >>> device = client.devices.get("device_id")
        >>> 
        >>> # Ping device
        >>> device.ping()
        >>> 
        >>> # Run compute operation
        >>> run = client.compute.run("device_id", "echo", ["hello"])
        >>> 
        >>> # Stream logs
        >>> for line in run.logs(stream=True):
        ...     print(line)
    """
    
    def __init__(self, socket_path: Optional[str] = None, timeout: int = 30):
        """Initialize Stellar client.
        
        Args:
            socket_path: Path to Unix socket (uses default if None)
            timeout: Request timeout in seconds
        """
        self.api = APIClient(socket_path, timeout)
        
        # Initialize resource managers (lazy loading pattern)
        self._devices = None
        self._compute = None
        self._proxy = None
        self._policy = None
    
    @property
    def devices(self) -> Devices:
        """Get devices resource manager.
        
        Returns:
            Devices resource manager
        """
        if self._devices is None:
            self._devices = Devices(self.api)
        return self._devices
    
    @property
    def compute(self) -> Compute:
        """Get compute resource manager.
        
        Returns:
            Compute resource manager
        """
        if self._compute is None:
            self._compute = Compute(self.api)
        return self._compute
    
    @property
    def proxy(self) -> Proxy:
        """Get proxy resource manager.
        
        Returns:
            Proxy resource manager
        """
        if self._proxy is None:
            self._proxy = Proxy(self.api)
        return self._proxy
    
    @property
    def policy(self) -> Policy:
        """Get policy resource manager.
        
        Returns:
            Policy resource manager
        """
        if self._policy is None:
            self._policy = Policy(self.api)
        return self._policy
    
    def info(self) -> NodeInfo:
        """Get information about the local node.
        
        Returns:
            NodeInfo object
            
        Raises:
            ConnectionError: If connection fails
        """
        try:
            response = self.api.get("/node")
            return NodeInfo.model_validate(response)
        except Exception as e:
            raise ConnectionError(f"Failed to get node info: {e}") from e
    
    def ping(self) -> dict:
        """Ping the local node (health check).
        
        Returns:
            Health status dictionary
            
        Raises:
            ConnectionError: If ping fails
        """
        try:
            return self.api.get("/health")
        except Exception as e:
            raise ConnectionError(f"Health check failed: {e}") from e
    
    def close(self) -> None:
        """Close the client connection."""
        self.api.close()
    
    def __enter__(self):
        """Context manager entry."""
        return self
    
    def __exit__(self, exc_type, exc_val, exc_tb):
        """Context manager exit."""
        self.close()
    
    def __repr__(self) -> str:
        """String representation."""
        return f"<StellarClient: {self.api.socket_path}>"


def from_env(socket_path: Optional[str] = None, timeout: int = 30) -> StellarClient:
    """Create a Stellar client from environment configuration.
    
    This is the recommended way to create a client, similar to docker.from_env().
    
    Args:
        socket_path: Path to Unix socket (uses default if None)
        timeout: Request timeout in seconds
        
    Returns:
        StellarClient instance
        
    Examples:
        >>> import stellar_client
        >>> client = stellar_client.from_env()
        >>> devices = client.devices.list()
    """
    return StellarClient(socket_path=socket_path, timeout=timeout)

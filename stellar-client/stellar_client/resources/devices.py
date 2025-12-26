"""Device resource manager and Device object."""

import os
from typing import List, Optional, Iterator, Callable
from ..api import APIClient
from ..models.device import Device as DeviceModel, NodeInfo
from ..models.responses import FileEntry, FileTree
from ..exceptions import DeviceNotFoundError, FileTransferError
from ..utils.helpers import validate_device_id, sanitize_file_path


class Device:
    """Represents a Stellar device with methods for interaction."""
    
    def __init__(self, client: APIClient, device_id: str, attrs: Optional[dict] = None):
        """Initialize Device object.
        
        Args:
            client: API client instance
            device_id: Device identifier
            attrs: Device attributes (optional, will be fetched if not provided)
        """
        self.client = client
        self.id = device_id
        self._attrs = attrs
        self._model: Optional[DeviceModel] = None
    
    def reload(self) -> None:
        """Reload device information from the API."""
        if not validate_device_id(self.id):
            raise DeviceNotFoundError(f"Invalid device ID: {self.id}")
        
        try:
            response = self.client.get(f"/devices/{self.id}")
            self._attrs = response
            self._model = DeviceModel.model_validate(response)
        except Exception as e:
            raise DeviceNotFoundError(f"Device {self.id} not found: {e}") from e
    
    @property
    def attrs(self) -> dict:
        """Get device attributes, loading if necessary."""
        if self._attrs is None:
            self.reload()
        return self._attrs or {}
    
    @property
    def model(self) -> DeviceModel:
        """Get device as Pydantic model."""
        if self._model is None:
            self.reload()
        return self._model
    
    def ping(self) -> bool:
        """Ping the device.
        
        Returns:
            True if ping successful
            
        Raises:
            DeviceNotFoundError: If device not found
        """
        try:
            response = self.client.post(f"/devices/{self.id}/ping")
            return response is not None
        except Exception as e:
            raise DeviceNotFoundError(f"Failed to ping device {self.id}: {e}") from e
    
    def info(self) -> dict:
        """Get detailed device information.
        
        Returns:
            Device information dictionary
            
        Raises:
            DeviceNotFoundError: If device not found
        """
        try:
            return self.client.get(f"/devices/{self.id}/info")
        except Exception as e:
            raise DeviceNotFoundError(f"Failed to get device info for {self.id}: {e}") from e
    
    def tree(self) -> List[dict]:
        """Get device file tree.
        
        Returns:
            List of file entries
            
        Raises:
            DeviceNotFoundError: If device not found
        """
        try:
            response = self.client.get(f"/devices/{self.id}/tree")
            # Handle both list and dict responses
            # API may return empty dict {} or a list
            if isinstance(response, list):
                return response
            elif isinstance(response, dict):
                # If response is a dict, try to extract a list from common keys
                if "files" in response:
                    return response["files"] if isinstance(response["files"], list) else []
                elif "tree" in response:
                    return response["tree"] if isinstance(response["tree"], list) else []
                # If it's an empty dict or any other dict, return empty list
                return []
            else:
                # Fallback: wrap in list if it's a single item
                return [response] if response else []
        except Exception as e:
            raise DeviceNotFoundError(f"Failed to get file tree for {self.id}: {e}") from e
    
    def files(self) -> 'DeviceFiles':
        """Get file operations manager for this device.
        
        Returns:
            DeviceFiles object for file operations
        """
        return DeviceFiles(self.client, self.id)
    
    def __repr__(self) -> str:
        """String representation."""
        return f"<Device: {self.id[:12]}...>"


class DeviceFiles:
    """File operations for a specific device."""
    
    def __init__(self, client: APIClient, device_id: str):
        """Initialize DeviceFiles.
        
        Args:
            client: API client instance
            device_id: Device identifier
        """
        self.client = client
        self.device_id = device_id
    
    def list(self, path: str = "/") -> List[FileEntry]:
        """List files in a directory on the device.
        
        Args:
            path: Remote directory path to list
            
        Returns:
            List of file entries
            
        Raises:
            DeviceNotFoundError: If device not found
            FileTransferError: If listing fails
        """
        safe_path = sanitize_file_path(path)
        try:
            response = self.client.get(
                f"/devices/{self.device_id}/files",
                params={"path": safe_path}
            )
            if isinstance(response, list):
                return [FileEntry.model_validate(item) for item in response]
            else:
                raise FileTransferError("Invalid file listing response format")
        except Exception as e:
            raise FileTransferError(f"Failed to list files: {e}") from e
    
    def download(self, remote_path: str, local_path: str,
                 progress_callback: Optional[Callable[[int, int], None]] = None) -> bool:
        """Download file from the device.
        
        Args:
            remote_path: Path to file on remote device
            local_path: Local path to save the file
            progress_callback: Optional callback for progress updates
            
        Returns:
            True if download successful
            
        Raises:
            FileTransferError: If download fails
        """
        safe_remote_path = sanitize_file_path(remote_path)
        local_dir = os.path.dirname(local_path)
        if local_dir and not os.path.exists(local_dir):
            os.makedirs(local_dir, exist_ok=True)
        
        try:
            response = self.client.get(
                f"/devices/{self.device_id}/files/download",
                params={"remotePath": safe_remote_path, "destPath": local_path}
            )
            success = response.get("success", False)
            if not success:
                raise FileTransferError(f"Download failed: {response.get('error', 'Unknown error')}")
            
            if progress_callback and os.path.exists(local_path):
                file_size = os.path.getsize(local_path)
                progress_callback(file_size, file_size)
            
            return success
        except Exception as e:
            raise FileTransferError(f"Failed to download file: {e}") from e
    
    def upload(self, local_path: str, remote_path: str,
               progress_callback: Optional[Callable[[int, int], None]] = None) -> bool:
        """Upload file to the device.
        
        Args:
            local_path: Local file path to upload
            remote_path: Path to save file on remote device
            progress_callback: Optional callback for progress updates
            
        Returns:
            True if upload successful
            
        Raises:
            FileTransferError: If upload fails
        """
        if not os.path.exists(local_path):
            raise FileTransferError(f"Local file not found: {local_path}")
        if not os.path.isfile(local_path):
            raise FileTransferError(f"Path is not a file: {local_path}")
        
        safe_remote_path = sanitize_file_path(remote_path)
        file_size = os.path.getsize(local_path)
        
        if progress_callback:
            progress_callback(0, file_size)
        
        try:
            response = self.client.post(
                f"/devices/{self.device_id}/files/upload",
                data={"localPath": local_path, "remotePath": safe_remote_path}
            )
            success = response.get("success", False)
            if not success:
                raise FileTransferError(f"Upload failed: {response.get('error', 'Unknown error')}")
            
            if progress_callback:
                progress_callback(file_size, file_size)
            
            return success
        except Exception as e:
            raise FileTransferError(f"Failed to upload file: {e}") from e
    
    def tree(self) -> FileTree:
        """Get complete file tree from the device.
        
        Returns:
            Complete file tree structure
            
        Raises:
            FileTransferError: If tree retrieval fails
        """
        try:
            response = self.client.get(f"/devices/{self.device_id}/tree")
            return FileTree.model_validate({'files': response})
        except Exception as e:
            raise FileTransferError(f"Failed to get file tree: {e}") from e


class Devices:
    """Resource manager for devices."""
    
    def __init__(self, client: APIClient):
        """Initialize Devices manager.
        
        Args:
            client: API client instance
        """
        self.client = client
        self._local_device_id: Optional[str] = None
    
    def list(self, include_self: bool = False) -> List[Device]:
        """List all discovered devices.
        
        Args:
            include_self: Whether to include the local device
            
        Returns:
            List of Device objects
        """
        response = self.client.get("/devices")
        devices = []
        
        # Handle both dict and list responses
        if isinstance(response, dict):
            device_data = response.values() if response else []
        else:
            device_data = response
        
        local_id = self._get_local_device_id() if not include_self else None
        
        for device_info in device_data:
            device = DeviceModel.model_validate(device_info)
            if local_id and device.id == local_id:
                continue
            devices.append(Device(self.client, device.id, device_info))
        
        return devices
    
    def get(self, device_id: str) -> Device:
        """Get a specific device by ID.
        
        Args:
            device_id: Device identifier
            
        Returns:
            Device object
            
        Raises:
            DeviceNotFoundError: If device not found
        """
        if not validate_device_id(device_id):
            raise DeviceNotFoundError(f"Invalid device ID: {device_id}")
        
        try:
            response = self.client.get(f"/devices/{device_id}")
            return Device(self.client, device_id, response)
        except Exception as e:
            raise DeviceNotFoundError(f"Device {device_id} not found: {e}") from e
    
    def connect(self, peer_info: str) -> Device:
        """Connect to a new peer and return the device.
        
        Args:
            peer_info: Peer connection information (multiaddr format)
            
        Returns:
            Device object for the connected peer
            
        Raises:
            DeviceNotFoundError: If connection fails
        """
        try:
            response = self.client.post("/connect", json_data={"peer_info": peer_info})
            device = DeviceModel.model_validate(response)
            return Device(self.client, device.id, response)
        except Exception as e:
            raise DeviceNotFoundError(f"Failed to connect to peer: {e}") from e
    
    def _get_local_device_id(self) -> Optional[str]:
        """Get local device ID (cached)."""
        if self._local_device_id is None:
            try:
                node_info = self.client.get("/node")
                self._local_device_id = node_info.get("NodeID") or node_info.get("id")
            except Exception:
                pass
        return self._local_device_id


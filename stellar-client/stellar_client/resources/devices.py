"""Device resource manager and Device object."""

import mimetypes
import os
from pathlib import Path
from typing import List, Optional, Iterator, Callable, Union, BinaryIO
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
    
    def download(self, remote_path: Union[str, Path], local_path: Union[str, Path],
                 progress_callback: Optional[Callable[[int, int], None]] = None) -> bool:
        """Download file from the device to client's local filesystem.
        
        This is the primary download method that downloads from the remote device
        to the client's local filesystem using the raw download endpoint.
        
        Args:
            remote_path: Path to file on remote device (str or Path-like)
            local_path: Local path on client's filesystem to save the file (str or Path-like)
            progress_callback: Optional callback for progress updates (bytes_downloaded, total_bytes)
            
        Returns:
            True if download successful
            
        Raises:
            FileTransferError: If download fails
        """
        # Convert Path-like objects to strings
        remote_path_str = str(remote_path) if not isinstance(remote_path, str) else remote_path
        local_path_str = str(local_path) if not isinstance(local_path, str) else local_path
        safe_remote_path = sanitize_file_path(remote_path_str)
        
        # Ensure local directory exists
        local_dir = os.path.dirname(local_path_str)
        if local_dir and not os.path.exists(local_dir):
            os.makedirs(local_dir, exist_ok=True)
        
        try:
            # Download as bytes using raw endpoint
            file_bytes = self._download_raw_bytes(safe_remote_path, progress_callback)
            
            # Write to local file
            with open(local_path_str, 'wb') as f:
                f.write(file_bytes)
            
            return True
        except Exception as e:
            raise FileTransferError(f"Failed to download file: {e}") from e
    
    def download_bytes(self, remote_path: Union[str, Path],
                      progress_callback: Optional[Callable[[int, int], None]] = None) -> bytes:
        """Download file from the device and return as bytes.
        
        This method downloads the file directly into memory without writing to disk.
        Useful for processing files in memory or streaming to other destinations.
        
        Args:
            remote_path: Path to file on remote device (str or Path-like)
            progress_callback: Optional callback for progress updates (bytes_downloaded, total_bytes)
            
        Returns:
            File content as bytes
            
        Raises:
            FileTransferError: If download fails
        """
        # Convert Path-like objects to string
        remote_path_str = str(remote_path) if not isinstance(remote_path, str) else remote_path
        safe_remote_path = sanitize_file_path(remote_path_str)
        return self._download_raw_bytes(safe_remote_path, progress_callback)
    
    def _download_raw_bytes(self, remote_path: str,
                            progress_callback: Optional[Callable[[int, int], None]] = None) -> bytes:
        """Internal method to download file as bytes (DRY helper).
        
        Args:
            remote_path: Path to file on remote device (already sanitized)
            progress_callback: Optional callback for progress updates
            
        Returns:
            File content as bytes
        """
        try:
            file_bytes = self.client.get_raw_bytes(
                f"/devices/{self.device_id}/files/download/raw",
                params={"remotePath": remote_path}
            )
            
            if progress_callback:
                file_size = len(file_bytes)
                progress_callback(file_size, file_size)
            
            return file_bytes
        except Exception as e:
            raise FileTransferError(f"Failed to download file: {e}") from e
    
    def _download_remote(self, remote_path: str, stellar_local_path: str,
                        progress_callback: Optional[Callable[[int, int], None]] = None) -> bool:
        """Download file using Stellar instance's local paths (secondary method).
        
        This method uses paths that are local to the Stellar instance, not the client.
        Use this only when you need to work with files on the Stellar server itself.
        
        Args:
            remote_path: Path to file on remote device
            stellar_local_path: Path on Stellar instance's filesystem (not client's)
            progress_callback: Optional callback for progress updates
            
        Returns:
            True if download successful
            
        Raises:
            FileTransferError: If download fails
        """
        safe_remote_path = sanitize_file_path(remote_path)
        
        try:
            response = self.client.get(
                f"/devices/{self.device_id}/files/download",
                params={"remotePath": safe_remote_path, "destPath": stellar_local_path}
            )
            success = response.get("success", False)
            if not success:
                raise FileTransferError(f"Download failed: {response.get('error', 'Unknown error')}")
            
            if progress_callback:
                # Note: We can't get actual file size from this endpoint
                progress_callback(0, 0)
            
            return success
        except Exception as e:
            raise FileTransferError(f"Failed to download file: {e}") from e
    
    def upload(self, local_path: Union[str, Path, BinaryIO], 
               remote_path: Union[str, Path],
               progress_callback: Optional[Callable[[int, int], None]] = None) -> bool:
        """Upload file from client's local filesystem to the device.
        
        This is the primary upload method that uploads from the client's local
        filesystem to the remote device using the raw upload endpoint.
        
        Args:
            local_path: Local file path on client's filesystem (str, Path-like, or file-like object)
            remote_path: Path to save file on remote device (str or Path-like)
            progress_callback: Optional callback for progress updates (bytes_uploaded, total_bytes)
            
        Returns:
            True if upload successful
            
        Raises:
            FileTransferError: If upload fails
        """
        # Convert Path-like objects to strings
        remote_path_str = str(remote_path) if not isinstance(remote_path, str) else remote_path
        safe_remote_path = sanitize_file_path(remote_path_str)
        return self._upload_raw(local_path, safe_remote_path, progress_callback)
    
    def _upload_raw(self, file_obj: Union[str, Path, BinaryIO], remote_path: str,
                   progress_callback: Optional[Callable[[int, int], None]] = None) -> bool:
        """Internal method to upload file using raw endpoint (DRY helper).
        
        Args:
            file_obj: File-like object or file path (str, Path, or BinaryIO, on client's filesystem)
            remote_path: Path to save file on remote device (already sanitized string)
            progress_callback: Optional callback for progress updates
            
        Returns:
            True if upload successful
        """
        # Handle both file paths and file objects
        # Check if it's a path (string or Path object) vs file-like object
        if isinstance(file_obj, (str, Path)):
            # It's a path (string or Path-like object)
            file_path_str = str(file_obj)
            # Treat as file path on client's filesystem
            if not os.path.exists(file_path_str):
                raise FileTransferError(f"Local file not found: {file_path_str}")
            if not os.path.isfile(file_path_str):
                raise FileTransferError(f"Path is not a file: {file_path_str}")
            file_handle = open(file_path_str, 'rb')
            file_size = os.path.getsize(file_path_str)
            should_close = True
            filename = os.path.basename(file_path_str)
        else:
            # Assume it's a file-like object
            file_handle = file_obj
            # Try to get file size if possible
            try:
                if hasattr(file_obj, 'seek') and hasattr(file_obj, 'tell'):
                    current_pos = file_obj.tell()
                    file_obj.seek(0, 2)  # Seek to end
                    file_size = file_obj.tell()
                    file_obj.seek(current_pos)  # Restore position
                else:
                    file_size = 0
            except (AttributeError, OSError):
                file_size = 0
            should_close = False
            filename = os.path.basename(remote_path) or "uploaded_file"
        
        if progress_callback:
            progress_callback(0, file_size)
        
        try:
            # Use multipart/form-data for raw upload
            # requests library expects files as dict with tuple values: (filename, file_obj, content_type)
            content_type, _ = mimetypes.guess_type(filename)
            if content_type is None:
                content_type = "application/octet-stream"
            
            files_dict = {
                "file": (filename, file_handle, content_type)
            }
            
            # Ensure remote_path is a string (not PosixPath or other types)
            remote_path_str = str(remote_path) if not isinstance(remote_path, str) else remote_path
            
            response = self.client.post(
                f"/devices/{self.device_id}/files/upload/raw",
                files=files_dict,
                data={"remotePath": remote_path_str}
            )
            
            if should_close:
                file_handle.close()
            
            success = response.get("success", False)
            if not success:
                raise FileTransferError(f"Upload failed: {response.get('error', 'Unknown error')}")
            
            if progress_callback:
                progress_callback(file_size, file_size)
            
            return success
        except Exception as e:
            if should_close and not file_handle.closed:
                file_handle.close()
            raise FileTransferError(f"Failed to upload file: {e}") from e
    
    def _upload_remote(self, stellar_local_path: str, remote_path: str,
                      progress_callback: Optional[Callable[[int, int], None]] = None) -> bool:
        """Upload file using Stellar instance's local paths (secondary method).
        
        This method uses paths that are local to the Stellar instance, not the client.
        Use this only when you need to work with files on the Stellar server itself.
        
        Args:
            stellar_local_path: Path on Stellar instance's filesystem (not client's)
            remote_path: Path to save file on remote device
            progress_callback: Optional callback for progress updates
            
        Returns:
            True if upload successful
            
        Raises:
            FileTransferError: If upload fails
        """
        safe_remote_path = sanitize_file_path(remote_path)
        
        try:
            response = self.client.post(
                f"/devices/{self.device_id}/files/upload",
                data={"localPath": stellar_local_path, "remotePath": safe_remote_path}
            )
            success = response.get("success", False)
            if not success:
                raise FileTransferError(f"Upload failed: {response.get('error', 'Unknown error')}")
            
            if progress_callback:
                # Note: We can't get actual file size from this endpoint
                progress_callback(0, 0)
            
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


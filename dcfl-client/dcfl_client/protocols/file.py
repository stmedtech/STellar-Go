"""File protocol implementation."""

import os
from typing import List, Optional, Callable
from ..utils.socket_client import UnixSocketClient
from ..utils.helpers import validate_device_id, sanitize_file_path
from ..models.responses import FileEntry, FileTree
from ..exceptions import DeviceNotFoundError, FileTransferError, ProtocolError


class FileProtocol:
    """File protocol for file transfer and management."""
    
    def __init__(self, socket_client: UnixSocketClient):
        """Initialize file protocol.
        
        Args:
            socket_client: Unix socket client instance
        """
        self.client = socket_client
    
    def list_files(
        self, 
        device_id: str, 
        path: str = "/"
    ) -> List[FileEntry]:
        """List files in a directory on remote device.
        
        Args:
            device_id: Target device ID
            path: Remote directory path to list
            
        Returns:
            List of file entries in the directory
            
        Raises:
            DeviceNotFoundError: If device is not found
            FileTransferError: If file listing fails
        """
            
        # Sanitize path to prevent directory traversal
        safe_path = sanitize_file_path(path)
        
        try:
            response = self.client.get(
                f"/devices/{device_id}/files",
                params={"path": safe_path}
            )
            
            if isinstance(response, list):
                return [FileEntry.model_validate(item) for item in response]
            else:
                raise FileTransferError("Invalid file listing response format")
                
        except Exception as e:
            raise FileTransferError(f"Failed to list files on device {device_id}: {e}")
    
    def get_file_tree(self, device_id: str) -> FileTree:
        """Get complete file tree from remote device.
        
        Args:
            device_id: Target device ID
            
        Returns:
            Complete file tree structure
            
        Raises:
            DeviceNotFoundError: If device is not found
            FileTransferError: If file tree retrieval fails
        """
        if not validate_device_id(device_id):
            raise DeviceNotFoundError(f"Invalid device ID: {device_id}")
            
        try:
            response = self.client.get(f"/devices/{device_id}/tree")
            return FileTree.model_validate(response)
        except Exception as e:
            raise FileTransferError(f"Failed to get file tree from device {device_id}: {e}")
    
    def download_file(
        self,
        device_id: str,
        remote_path: str,
        local_path: str,
        progress_callback: Optional[Callable[[int, int], None]] = None,
    ) -> bool:
        """Download file from remote device.
        
        Args:
            device_id: Target device ID
            remote_path: Path to file on remote device
            local_path: Local path to save the file
            progress_callback: Optional callback for progress updates (bytes_received, total_bytes)
            
        Returns:
            True if download successful
            
        Raises:
            DeviceNotFoundError: If device is not found
            FileTransferError: If download fails
        """
        if not validate_device_id(device_id):
            raise DeviceNotFoundError(f"Invalid device ID: {device_id}")
            
        # Sanitize paths
        safe_remote_path = sanitize_file_path(remote_path)
        
        # Ensure local directory exists
        local_dir = os.path.dirname(local_path)
        if local_dir and not os.path.exists(local_dir):
            os.makedirs(local_dir, exist_ok=True)
            
        try:
            response = self.client.get(
                f"/devices/{device_id}/files/download",
                params={
                    "remote_path": safe_remote_path,
                    "dest_path": local_path,
                }
            )
            
            success = response.get("success", False)
            if not success:
                raise FileTransferError(f"Download failed: {response.get('error', 'Unknown error')}")
                
            # If progress callback provided and we have file size info
            if progress_callback and os.path.exists(local_path):
                file_size = os.path.getsize(local_path)
                progress_callback(file_size, file_size)  # Report completion
                
            return success
            
        except Exception as e:
            raise FileTransferError(f"Failed to download file from device {device_id}: {e}")
    
    def upload_file(
        self,
        device_id: str,
        local_path: str,
        remote_path: str,
        progress_callback: Optional[Callable[[int, int], None]] = None,
    ) -> bool:
        """Upload file to remote device.
        
        Args:
            device_id: Target device ID
            local_path: Local file path to upload
            remote_path: Path to save file on remote device
            progress_callback: Optional callback for progress updates (bytes_sent, total_bytes)
            
        Returns:
            True if upload successful
            
        Raises:
            DeviceNotFoundError: If device is not found
            FileTransferError: If upload fails or file not found
        """
        if not validate_device_id(device_id):
            raise DeviceNotFoundError(f"Invalid device ID: {device_id}")
            
        if not os.path.exists(local_path):
            raise FileTransferError(f"Local file not found: {local_path}")
            
        if not os.path.isfile(local_path):
            raise FileTransferError(f"Path is not a file: {local_path}")
            
        # Sanitize remote path
        safe_remote_path = sanitize_file_path(remote_path)
        
        try:
            # Get file size for progress tracking
            file_size = os.path.getsize(local_path)
            
            if progress_callback:
                progress_callback(0, file_size)  # Report start
                
            response = self.client.post(
                f"/devices/{device_id}/files/upload",
                data={
                    "local_path": local_path,
                    "remote_path": safe_remote_path,
                }
            )
            
            success = response.get("success", False)
            if not success:
                raise FileTransferError(f"Upload failed: {response.get('error', 'Unknown error')}")
                
            if progress_callback:
                progress_callback(file_size, file_size)  # Report completion
                
            return success
            
        except Exception as e:
            raise FileTransferError(f"Failed to upload file to device {device_id}: {e}")
    
    def get_file_info(self, device_id: str, remote_path: str) -> Optional[FileEntry]:
        """Get information about a specific file on remote device.
        
        Args:
            device_id: Target device ID
            remote_path: Path to the file on remote device
            
        Returns:
            File entry if found, None otherwise
            
        Raises:
            DeviceNotFoundError: If device is not found
            FileTransferError: If operation fails
        """
        try:
            # Get directory listing and find the specific file
            dir_path = os.path.dirname(remote_path)
            filename = os.path.basename(remote_path)
            
            files = self.list_files(device_id, dir_path if dir_path else "/")
            
            for file_entry in files:
                if file_entry.filename == filename:
                    return file_entry
                    
            return None
            
        except Exception as e:
            raise FileTransferError(f"Failed to get file info for {remote_path}: {e}")
    
    def create_directory(self, device_id: str, remote_path: str) -> bool:
        """Create a directory on remote device.
        
        Note: This functionality depends on the Go backend supporting directory creation.
        Currently returns False as it's not implemented in the backend.
        
        Args:
            device_id: Target device ID
            remote_path: Path of directory to create
            
        Returns:
            True if successful (currently always False)
        """
        # This would need to be implemented in the Go backend first
        return False
    
    def delete_file(self, device_id: str, remote_path: str) -> bool:
        """Delete a file on remote device.
        
        Note: This functionality depends on the Go backend supporting file deletion.
        Currently returns False as it's not implemented in the backend.
        
        Args:
            device_id: Target device ID
            remote_path: Path of file to delete
            
        Returns:
            True if successful (currently always False)
        """
        # This would need to be implemented in the Go backend first
        return False

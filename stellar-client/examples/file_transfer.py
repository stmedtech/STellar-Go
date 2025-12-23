#!/usr/bin/env python3
"""
File Transfer Example

This example demonstrates file operations with Stellar client:
- Listing files on remote devices
- Uploading files to remote devices
- Downloading files from remote devices
- Progress tracking
"""

import os
import tempfile
from stellar_client import StellarClient
from stellar_client.exceptions import FileTransferError, DeviceNotFoundError


def progress_callback(bytes_transferred: int, total_bytes: int):
    """Progress callback for file transfers."""
    if total_bytes > 0:
        percent = (bytes_transferred / total_bytes) * 100
        print(f"  Progress: {percent:.1f}% ({bytes_transferred}/{total_bytes} bytes)")


def create_sample_file(content: str) -> str:
    """Create a temporary sample file for testing."""
    with tempfile.NamedTemporaryFile(mode='w', delete=False, suffix='.txt') as f:
        f.write(content)
        return f.name


def main():
    print("Stellar Client - File Transfer Example")
    print("=" * 40)
    
    try:
        with StellarClient() as client:
            print("✓ Connected to Stellar node")
            
            # Get available devices
            devices = client.list_devices()
            if not devices:
                print("No devices found. Start additional Stellar nodes to test file transfer.")
                return 1
                
            target_device = devices[0]
            print(f"Using device: {target_device.id}")
            print()
            
            # 1. List files on remote device
            print("1. Listing files on remote device...")
            try:
                files = client.file.list_files(target_device.id, path="/")
                print(f"Found {len(files)} files/directories:")
                
                for file_entry in files[:10]:  # Show first 10 entries
                    file_type = "DIR" if file_entry.is_dir else "FILE"
                    size_str = f"({file_entry.size} bytes)" if not file_entry.is_dir else ""
                    print(f"  {file_type}: {file_entry.full_name()} {size_str}")
                    
                if len(files) > 10:
                    print(f"  ... and {len(files) - 10} more")
                    
            except FileTransferError as e:
                print(f"  Failed to list files: {e}")
            print()
            
            # 2. Get complete file tree (if not too large)
            print("2. Getting file tree structure...")
            try:
                file_tree = client.file.get_file_tree(target_device.id)
                print(f"File tree contains {len(file_tree.files)} top-level entries")
                
                def print_tree(entries, indent=0):
                    for entry in entries:
                        prefix = "  " * indent
                        file_type = "📁" if entry.is_dir else "📄"
                        print(f"{prefix}{file_type} {entry.filename}")
                        if entry.children and indent < 10:  # Limit depth
                            print_tree(entry.children, indent + 1)
                            
                print("File tree preview:")
                print_tree(file_tree.files)
                
            except FileTransferError as e:
                print(f"  Failed to get file tree: {e}")
            print()
            
            # 3. Upload a file
            print("3. Uploading a file to remote device...")
            sample_content = f"""# Sample Python Script
# Uploaded from Stellar Client File Transfer Example
print("Hello from Stellar!")
print("This file was transferred using the Stellar client.")

import datetime
print(f"Current time: {{datetime.datetime.now()}}")
"""
            
            local_file = create_sample_file(sample_content)
            remote_path = "uploaded_script.py"
            
            try:
                print(f"  Uploading {local_file} -> {remote_path}")
                success = client.file.upload_file(
                    device_id=target_device.id,
                    local_path=local_file,
                    remote_path=remote_path,
                    progress_callback=progress_callback
                )
                
                if success:
                    print("  ✓ Upload completed successfully!")
                else:
                    print("  ✗ Upload failed")
                    
            except FileTransferError as e:
                print(f"  Upload failed: {e}")
            finally:
                # Clean up temporary file
                os.unlink(local_file)
            print()
            
            # 4. Verify file exists and get info
            print("4. Verifying uploaded file...")
            try:
                file_info = client.file.get_file_info(target_device.id, remote_path)
                if file_info:
                    print(f"  ✓ File found: {file_info.filename}")
                    print(f"    Size: {file_info.size} bytes")
                    print(f"    Path: {file_info.full_name()}")
                else:
                    print("  ✗ Uploaded file not found")
            except FileTransferError as e:
                print(f"  Verification failed: {e}")
            print()
            
            # 5. Download the file back
            print("5. Downloading file back to local system...")
            with tempfile.NamedTemporaryFile(delete=False, suffix='_downloaded.py') as temp_download:
                download_path = temp_download.name
                
            try:
                print(f"  Downloading {remote_path} -> {download_path}")
                success = client.file.download_file(
                    device_id=target_device.id,
                    remote_path=remote_path,
                    local_path=download_path,
                    progress_callback=progress_callback
                )
                
                if success:
                    print("  ✓ Download completed successfully!")
                    
                    # Verify downloaded content
                    with open(download_path, 'r') as f:
                        downloaded_content = f.read()
                        
                    if sample_content.strip() == downloaded_content.strip():
                        print("  ✓ File content verification passed!")
                    else:
                        print("  ⚠ File content differs from original")
                        
                    print(f"  Downloaded file size: {os.path.getsize(download_path)} bytes")
                    
                else:
                    print("  ✗ Download failed")
                    
            except FileTransferError as e:
                print(f"  Download failed: {e}")
            finally:
                # Clean up downloaded file
                if os.path.exists(download_path):
                    os.unlink(download_path)
            print()
            
            # 6. Demonstrate error handling
            print("6. Testing error handling...")
            try:
                # Try to download a non-existent file
                client.file.download_file(
                    device_id=target_device.id,
                    remote_path="non_existent_file.txt",
                    local_path="/tmp/should_not_exist.txt"
                )
            except FileTransferError as e:
                print(f"  ✓ Expected error caught: {e}")
                
            try:
                # Try to upload a non-existent local file
                client.file.upload_file(
                    device_id=target_device.id,
                    local_path="non_existent_local_file.txt",
                    remote_path="remote.txt"
                )
            except FileTransferError as e:
                print(f"  ✓ Expected error caught: {e}")
                
            try:
                # Try to access invalid device
                client.file.list_files("invalid-device-id")
            except DeviceNotFoundError as e:
                print(f"  ✓ Expected error caught: {e}")
            print()
            
        print("✓ File transfer demo completed")
        
    except Exception as e:
        print(f"Unexpected error: {e}")
        return 1
        
    return 0


if __name__ == "__main__":
    exit(main())
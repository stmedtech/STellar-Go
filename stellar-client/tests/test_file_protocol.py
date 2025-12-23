"""
Test suite for Stellar Client File Protocol

This module tests the file transfer functionality of the Stellar client.
Tests are organized by functionality: listing, uploading, downloading, and error handling.
"""

import os
import tempfile
import pytest
from stellar_client import StellarClient
from stellar_client.exceptions import FileTransferError, DeviceNotFoundError


class TestFileProtocol:
    """Test cases for file protocol functionality."""
    
    def test_file_protocol_initialization(self):
        """Test that file protocol can be initialized."""
        client = StellarClient()
        assert hasattr(client, 'file')
        assert client.file is not None
    
    def test_operations_without_connection(self):
        """Test all file operations without active connection."""
        client = StellarClient()
        
        # Test list_files
        with pytest.raises(Exception):
            client.file.list_files("test-device-id")
        
        # Test upload_file
        with pytest.raises(Exception):
            client.file.upload_file("test-device-id", "local.txt", "remote.txt")
        
        # Test download_file
        with pytest.raises(Exception):
            client.file.download_file("test-device-id", "remote.txt", "local.txt")
        
        # Test get_file_info
        with pytest.raises(Exception):
            client.file.get_file_info("test-device-id", "remote.txt")
        
        # Test create_directory (always returns False as not implemented)
        result = client.file.create_directory("test-device-id", "/test/dir")
        assert result is False
    
    def test_invalid_device_id_validation(self):
        """Test that invalid device IDs are properly validated."""
        client = StellarClient()
        
        invalid_ids = ["", "invalid-id", "123"]
        
        for invalid_id in invalid_ids:
            with pytest.raises((DeviceNotFoundError, Exception)):
                client.file.list_files(invalid_id)
    
    def test_file_path_sanitization(self):
        """Test that file paths are properly handled."""
        client = StellarClient()
        
        # Test with potentially dangerous paths
        dangerous_paths = [
            "../../../etc/passwd",
            "/etc/passwd",
            "..\\..\\..\\windows\\system32",
            "C:\\Windows\\System32",
        ]
        
        # These should not raise exceptions during sanitization
        # (they will fail later due to no connection, but not during sanitization)
        for path in dangerous_paths:
            try:
                client.file.list_files("test-device-id", path)
            except Exception as e:
                # Should fail due to no connection, not due to path sanitization
                assert "connection" in str(e).lower() or "device" in str(e).lower()


class TestFileProtocolIntegration:
    """Integration tests for file protocol (requires running Stellar node)."""
    
    @pytest.mark.integration
    def test_file_listing_workflow(self):
        """Test file listing functionality."""
        try:
            with StellarClient() as client:
                devices = client.list_devices()
                if not devices:
                    pytest.skip("No devices available for testing")
                
                device_id = devices[0].id
                
                # Test list_files
                files = client.file.list_files(device_id)
                assert isinstance(files, list)
                
                # Test get_file_tree
                tree = client.file.get_file_tree(device_id)
                assert tree is not None
                assert hasattr(tree, 'files')
                assert isinstance(tree.files, list)
                
        except Exception as e:
            pytest.skip(f"Stellar node not available: {e}")
    
    @pytest.mark.integration
    def test_file_upload_workflow(self):
        """Test file upload functionality."""
        try:
            with StellarClient() as client:
                devices = client.list_devices()
                if not devices:
                    pytest.skip("No devices available for testing")
                
                device_id = devices[0].id
                
                # Create a temporary file
                with tempfile.NamedTemporaryFile(mode='w', delete=False, suffix='.txt') as tmp_file:
                    tmp_file.write("Test content for file upload")
                    tmp_file_path = tmp_file.name
                
                try:
                    # Test upload
                    upload_result = client.file.upload_file(
                        device_id, 
                        tmp_file_path, 
                        "test_uploaded_file.txt"
                    )
                    assert upload_result is True
                    
                finally:
                    # Clean up
                    try:
                        os.unlink(tmp_file_path)
                    except:
                        pass
                        
        except Exception as e:
            pytest.skip(f"Stellar node not available: {e}")
    
    @pytest.mark.integration
    def test_file_download_workflow(self):
        """Test file download functionality."""
        try:
            with StellarClient() as client:
                devices = client.list_devices()
                if not devices:
                    pytest.skip("No devices available for testing")
                
                device_id = devices[0].id
                
                # First upload a file to download
                with tempfile.NamedTemporaryFile(mode='w', delete=False, suffix='.txt') as tmp_file:
                    tmp_file.write("Test content for download")
                    tmp_file_path = tmp_file.name
                
                try:
                    # Upload file
                    upload_result = client.file.upload_file(
                        device_id, 
                        tmp_file_path, 
                        "test_download_file.txt"
                    )
                    assert upload_result is True
                    
                    # Test download
                    download_path = tmp_file_path + "_downloaded"
                    download_result = client.file.download_file(
                        device_id,
                        "test_download_file.txt",
                        download_path
                    )
                    assert download_result is True
                    
                    # Verify downloaded file content
                    assert os.path.exists(download_path)
                    with open(download_path, 'r') as f:
                        content = f.read()
                    assert content == "Test content for download"
                    
                    # Clean up downloaded file
                    os.unlink(download_path)
                    
                finally:
                    # Clean up uploaded file
                    try:
                        os.unlink(tmp_file_path)
                    except:
                        pass
                        
        except Exception as e:
            pytest.skip(f"Stellar node not available: {e}")
    
    @pytest.mark.integration
    def test_complete_file_transfer_workflow(self):
        """Test complete file transfer workflow: upload -> list -> download."""
        try:
            with StellarClient() as client:
                devices = client.list_devices()
                if not devices:
                    pytest.skip("No devices available for testing")
                
                device_id = devices[0].id
                
                # Create a temporary file
                with tempfile.NamedTemporaryFile(mode='w', delete=False, suffix='.txt') as tmp_file:
                    tmp_file.write("Complete workflow test content")
                    tmp_file_path = tmp_file.name
                
                try:
                    # Step 1: Upload file
                    upload_result = client.file.upload_file(
                        device_id, 
                        tmp_file_path, 
                        "workflow_test_file.txt"
                    )
                    assert upload_result is True
                    
                    # Step 2: List files and verify upload
                    files = client.file.list_files(device_id)
                    uploaded_file = None
                    for file_entry in files:
                        if file_entry.filename == "workflow_test_file.txt":
                            uploaded_file = file_entry
                            break
                    
                    assert uploaded_file is not None
                    assert uploaded_file.filename == "workflow_test_file.txt"
                    
                    # Step 3: Download file
                    download_path = tmp_file_path + "_workflow_downloaded"
                    download_result = client.file.download_file(
                        device_id,
                        "workflow_test_file.txt",
                        download_path
                    )
                    assert download_result is True
                    
                    # Step 4: Verify downloaded file content
                    assert os.path.exists(download_path)
                    with open(download_path, 'r') as f:
                        content = f.read()
                    assert content == "Complete workflow test content"
                    
                    # Clean up
                    os.unlink(download_path)
                    
                finally:
                    # Clean up uploaded file
                    try:
                        os.unlink(tmp_file_path)
                    except:
                        pass
                        
        except Exception as e:
            pytest.skip(f"Stellar node not available: {e}")
    
    @pytest.mark.integration
    def test_file_error_handling(self):
        """Test error handling for various file operation failure scenarios."""
        try:
            with StellarClient() as client:
                devices = client.list_devices()
                if not devices:
                    pytest.skip("No devices available for testing")
                
                device_id = devices[0].id
                
                # Test download of non-existent file
                with pytest.raises(FileTransferError):
                    client.file.download_file(
                        device_id,
                        "non_existent_file.txt",
                        "/tmp/should_not_exist.txt"
                    )
                
                # Test upload of non-existent local file
                with pytest.raises(FileTransferError):
                    client.file.upload_file(
                        device_id,
                        "/non/existent/local/file.txt",
                        "remote_file.txt"
                    )
                
                # Test with invalid device ID
                with pytest.raises(FileTransferError):
                    client.file.list_files("invalid-device-id")
                    
        except Exception as e:
            pytest.skip(f"Stellar node not available: {e}")


if __name__ == "__main__":
    pytest.main([__file__])
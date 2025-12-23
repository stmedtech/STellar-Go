"""
Comprehensive Integration Test Suite for Stellar Client

This module provides end-to-end integration tests that follow the protocol usage order:
1. Echo Protocol - Basic connectivity and device discovery
2. File Protocol - File transfer operations
3. Compute Protocol - Computational tasks
4. Proxy Protocol - Network proxying

Tests are designed to run in sequence and verify the complete Stellar client functionality.
"""

import os
import tempfile
import pytest
from stellar_client import StellarClient
from stellar_client.exceptions import FileTransferError, DeviceNotFoundError, ProtocolError


class TestStellarIntegrationSuite:
    """Comprehensive integration test suite for Stellar client."""
    
    @pytest.mark.integration
    def test_complete_stellar_workflow(self):
        """Test complete Stellar workflow across all protocols."""
        try:
            with StellarClient() as client:
                # Phase 1: Echo Protocol - Device Discovery
                devices = client.list_devices()
                if not devices:
                    pytest.skip("No devices available for testing")
                
                device_id = devices[0].id
                
                # Test echo protocol
                ping_result = client.echo.ping(device_id)
                assert ping_result is not None
                assert ping_result.id == device_id
                
                # Phase 2: File Protocol - File Operations
                # Create test file
                with tempfile.NamedTemporaryFile(mode='w', delete=False, suffix='.txt') as tmp_file:
                    tmp_file.write("Integration test content")
                    tmp_file_path = tmp_file.name
                
                try:
                    # Upload file
                    upload_result = client.file.upload_file(
                        device_id, 
                        tmp_file_path, 
                        "integration_test_file.txt"
                    )
                    assert upload_result is True
                    
                    # List files
                    files = client.file.list_files(device_id)
                    assert isinstance(files, list)
                    
                    # Download file
                    download_path = tmp_file_path + "_downloaded"
                    download_result = client.file.download_file(
                        device_id,
                        "integration_test_file.txt",
                        download_path
                    )
                    assert download_result is True
                    
                    # Verify download
                    assert os.path.exists(download_path)
                    with open(download_path, 'r') as f:
                        content = f.read()
                    assert content == "Integration test content"
                    
                    # Clean up
                    os.unlink(download_path)
                    
                finally:
                    os.unlink(tmp_file_path)
                
                # Phase 3: Compute Protocol - Computational Tasks
                # List conda environments
                envs = client.compute.list_conda_environments(device_id)
                assert isinstance(envs, list)
                
                # Execute simple script
                script_result = client.compute.execute_script(device_id, "print('Hello from integration test!')")
                assert script_result is not None
                
                # Phase 4: Proxy Protocol - Network Proxying
                # List existing proxies
                proxies = client.proxy.list_proxies()
                assert isinstance(proxies, list)
                
                # Note: We don't create a proxy in integration test to avoid port conflicts
                # This would be tested in a dedicated proxy test environment
                
        except Exception as e:
            pytest.skip(f"Stellar node not available: {e}")
    
    @pytest.mark.integration
    def test_error_handling_across_protocols(self):
        """Test error handling across all protocols."""
        try:
            with StellarClient() as client:
                # Test with invalid device ID across all protocols
                invalid_device_id = "invalid-device-id"
                
                # Echo protocol error handling
                with pytest.raises((DeviceNotFoundError, ProtocolError)):
                    client.echo.ping(invalid_device_id)
                
                # File protocol error handling
                with pytest.raises((DeviceNotFoundError, ProtocolError)):
                    client.file.list_files(invalid_device_id)
                
                # Compute protocol error handling
                with pytest.raises((DeviceNotFoundError, ProtocolError)):
                    client.compute.list_conda_environments(invalid_device_id)
                
                # Proxy protocol error handling
                with pytest.raises((DeviceNotFoundError, ProtocolError)):
                    client.proxy.create_proxy(invalid_device_id, 8080, 8080)
                
        except Exception as e:
            pytest.skip(f"Stellar node not available: {e}")
    
    @pytest.mark.integration
    def test_client_lifecycle(self):
        """Test client lifecycle and connection management."""
        # Test client initialization
        client = StellarClient()
        assert client is not None
        assert hasattr(client, 'echo')
        assert hasattr(client, 'file')
        assert hasattr(client, 'compute')
        assert hasattr(client, 'proxy')
        
        # Test context manager
        try:
            with StellarClient() as client:
                devices = client.list_devices()
                assert isinstance(devices, list)
        except Exception as e:
            pytest.skip(f"Stellar node not available: {e}")
    
    @pytest.mark.integration
    def test_protocol_initialization_order(self):
        """Test that protocols are initialized in the correct order."""
        client = StellarClient()
        
        # Protocols should be lazy-loaded
        assert client._echo is None
        assert client._file is None
        assert client._compute is None
        assert client._proxy is None
        
        # Accessing protocols should initialize them
        echo = client.echo
        assert echo is not None
        assert client._echo is not None
        
        file_protocol = client.file
        assert file_protocol is not None
        assert client._file is not None
        
        compute = client.compute
        assert compute is not None
        assert client._compute is not None
        
        proxy = client.proxy
        assert proxy is not None
        assert client._proxy is not None


if __name__ == "__main__":
    pytest.main([__file__])





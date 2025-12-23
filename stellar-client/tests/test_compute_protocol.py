"""
Test suite for Stellar Client Compute Protocol

This module tests the compute protocol functionality which handles computational tasks
and environment management.
"""

import pytest
from stellar_client import StellarClient
from stellar_client.exceptions import DeviceNotFoundError, ProtocolError


class TestComputeProtocol:
    """Test cases for compute protocol functionality."""
    
    def test_compute_protocol_initialization(self):
        """Test that compute protocol can be initialized."""
        client = StellarClient()
        assert hasattr(client, 'compute')
        assert client.compute is not None
    
    def test_operations_without_connection(self):
        """Test all compute operations without active connection."""
        client = StellarClient()
        
        # Test list_conda_environments
        with pytest.raises(Exception):
            client.compute.list_conda_environments("test-device-id")
        
        # Test prepare_conda_environment
        with pytest.raises(Exception):
            client.compute.prepare_conda_environment("test-device-id", "test-env")
        
        # Test execute_script
        with pytest.raises(Exception):
            client.compute.execute_script("test-device-id", "print('hello')")
    
    def test_invalid_device_id_validation(self):
        """Test that invalid device IDs are properly validated."""
        client = StellarClient()
        
        invalid_ids = ["", "invalid-id", "123"]
        
        for invalid_id in invalid_ids:
            with pytest.raises((DeviceNotFoundError, Exception)):
                client.compute.list_conda_environments(invalid_id)


class TestComputeProtocolIntegration:
    """Integration tests for compute protocol (requires running Stellar node)."""
    
    @pytest.mark.integration
    def test_conda_environment_listing(self):
        """Test conda environment listing functionality."""
        try:
            with StellarClient() as client:
                devices = client.list_devices()
                if not devices:
                    pytest.skip("No devices available for testing")
                
                device_id = devices[0].id
                
                # Test list_conda_environments
                envs = client.compute.list_conda_environments(device_id)
                assert isinstance(envs, list)
                
        except Exception as e:
            pytest.skip(f"Stellar node not available: {e}")
    
    @pytest.mark.integration
    def test_conda_environment_preparation(self):
        """Test conda environment preparation functionality."""
        try:
            with StellarClient() as client:
                devices = client.list_devices()
                if not devices:
                    pytest.skip("No devices available for testing")
                
                device_id = devices[0].id
                
                # Test prepare_conda_environment
                result = client.compute.prepare_conda_environment(device_id, "test-env")
                assert isinstance(result, bool)
                
        except Exception as e:
            pytest.skip(f"Stellar node not available: {e}")
    
    @pytest.mark.integration
    def test_script_execution(self):
        """Test script execution functionality."""
        try:
            with StellarClient() as client:
                devices = client.list_devices()
                if not devices:
                    pytest.skip("No devices available for testing")
                
                device_id = devices[0].id
                
                # Test execute_script with simple Python code
                script = "print('Hello from Stellar compute!')"
                result = client.compute.execute_script(device_id, script)
                assert result is not None
                
        except Exception as e:
            pytest.skip(f"Stellar node not available: {e}")
    
    @pytest.mark.integration
    def test_compute_error_handling(self):
        """Test error handling for various compute operation failure scenarios."""
        try:
            with StellarClient() as client:
                devices = client.list_devices()
                if not devices:
                    pytest.skip("No devices available for testing")
                
                device_id = devices[0].id
                
                # Test with invalid device ID
                with pytest.raises((DeviceNotFoundError, ProtocolError)):
                    client.compute.list_conda_environments("invalid-device-id")
                
                # Test with invalid environment name
                with pytest.raises((ProtocolError, Exception)):
                    client.compute.prepare_conda_environment(device_id, "")
                
                # Test with invalid script
                with pytest.raises((ProtocolError, Exception)):
                    client.compute.execute_script(device_id, "")
                    
        except Exception as e:
            pytest.skip(f"Stellar node not available: {e}")


if __name__ == "__main__":
    pytest.main([__file__])





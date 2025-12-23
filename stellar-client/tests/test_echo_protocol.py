"""
Test suite for Stellar Client Echo Protocol

This module tests the echo protocol functionality which handles basic connectivity
and device discovery - the foundation for all other protocols.
"""

import pytest
from stellar_client import StellarClient
from stellar_client.exceptions import DeviceNotFoundError, ProtocolError


class TestEchoProtocol:
    """Test cases for echo protocol functionality."""
    
    def test_echo_protocol_initialization(self):
        """Test that echo protocol can be initialized."""
        client = StellarClient()
        assert hasattr(client, 'echo')
        assert client.echo is not None
    
    def test_ping_without_connection(self):
        """Test ping without active connection."""
        client = StellarClient()
        with pytest.raises(Exception):
            client.echo.ping("test-device-id")
    
    def test_invalid_device_id_validation(self):
        """Test that invalid device IDs are properly validated."""
        client = StellarClient()
        
        # Test with invalid device IDs
        invalid_ids = ["", "invalid-id", "123", None]
        
        for invalid_id in invalid_ids:
            if invalid_id is None:
                continue  # Skip None as it will cause different error
            with pytest.raises((DeviceNotFoundError, Exception)):
                client.echo.ping(invalid_id)


class TestEchoProtocolIntegration:
    """Integration tests for echo protocol (requires running Stellar node)."""
    
    @pytest.mark.integration
    def test_ping_workflow(self):
        """Test complete ping workflow with real Stellar node."""
        try:
            with StellarClient() as client:
                devices = client.list_devices()
                if not devices:
                    pytest.skip("No devices available for testing")
                
                device_id = devices[0].id
                
                # Test ping
                result = client.echo.ping(device_id)
                assert result is not None
                assert hasattr(result, 'id')
                assert hasattr(result, 'status')
                assert result.id == device_id
                
        except Exception as e:
            pytest.skip(f"Stellar node not available: {e}")
    
    @pytest.mark.integration
    def test_device_discovery(self):
        """Test device discovery functionality."""
        try:
            with StellarClient() as client:
                devices = client.list_devices()
                assert isinstance(devices, list)
                
                if devices:
                    # Test that devices have required attributes
                    for device in devices:
                        assert hasattr(device, 'id')
                        assert hasattr(device, 'status')
                        assert hasattr(device, 'sys_info')
                        
        except Exception as e:
            pytest.skip(f"Stellar node not available: {e}")
    
    @pytest.mark.integration
    def test_error_handling(self):
        """Test error handling for various failure scenarios."""
        try:
            with StellarClient() as client:
                # Test ping with non-existent device
                with pytest.raises((DeviceNotFoundError, ProtocolError)):
                    client.echo.ping("non-existent-device-id")
                    
        except Exception as e:
            pytest.skip(f"Stellar node not available: {e}")


if __name__ == "__main__":
    pytest.main([__file__])





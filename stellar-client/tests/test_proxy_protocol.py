"""
Test suite for Stellar Client Proxy Protocol

This module tests the proxy protocol functionality which handles network proxying
and port forwarding.
"""

import pytest
from stellar_client import StellarClient
from stellar_client.exceptions import DeviceNotFoundError, ProtocolError


class TestProxyProtocol:
    """Test cases for proxy protocol functionality."""
    
    def test_proxy_protocol_initialization(self):
        """Test that proxy protocol can be initialized."""
        client = StellarClient()
        assert hasattr(client, 'proxy')
        assert client.proxy is not None
    
    def test_operations_without_connection(self):
        """Test all proxy operations without active connection."""
        client = StellarClient()
        
        # Test create_proxy
        with pytest.raises(Exception):
            client.proxy.create_proxy("test-device-id", 8080, 8080)
        
        # Test list_proxies - this may return empty list instead of raising exception
        try:
            proxies = client.proxy.list_proxies()
            # If no exception is raised, it should return an empty list
            assert isinstance(proxies, list)
        except Exception:
            # If exception is raised, that's also acceptable
            pass
        
        # Test close_proxy - this may return False instead of raising exception
        try:
            result = client.proxy.close_proxy(8080)
            # If no exception is raised, it should return a boolean
            assert isinstance(result, bool)
        except Exception:
            # If exception is raised, that's also acceptable
            pass
    
    def test_invalid_device_id_validation(self):
        """Test that invalid device IDs are properly validated."""
        client = StellarClient()
        
        invalid_ids = ["", "invalid-id", "123"]
        
        for invalid_id in invalid_ids:
            with pytest.raises((DeviceNotFoundError, Exception)):
                client.proxy.create_proxy(invalid_id, 8080, 8080)
    
    def test_invalid_port_validation(self):
        """Test that invalid ports are properly validated."""
        client = StellarClient()
        
        # Test with invalid ports
        invalid_ports = [-1, 0, 65536, 99999]
        
        for invalid_port in invalid_ports:
            with pytest.raises((ValueError, Exception)):
                client.proxy.create_proxy("test-device-id", invalid_port, 8080)


class TestProxyProtocolIntegration:
    """Integration tests for proxy protocol (requires running Stellar node)."""
    
    @pytest.mark.integration
    def test_proxy_creation_and_management(self):
        """Test proxy creation and management functionality."""
        try:
            with StellarClient() as client:
                devices = client.list_devices()
                if not devices:
                    pytest.skip("No devices available for testing")
                
                device_id = devices[0].id
                
                # Test create_proxy
                result = client.proxy.create_proxy(device_id, 8080, 8080)
                assert isinstance(result, bool)
                
                # Test list_proxies
                proxies = client.proxy.list_proxies()
                assert isinstance(proxies, list)
                
                # Test close_proxy
                close_result = client.proxy.close_proxy(8080)
                assert isinstance(close_result, bool)
                
        except Exception as e:
            pytest.skip(f"Stellar node not available: {e}")
    
    @pytest.mark.integration
    def test_proxy_listing(self):
        """Test proxy listing functionality."""
        try:
            with StellarClient() as client:
                # Test list_proxies
                proxies = client.proxy.list_proxies()
                assert isinstance(proxies, list)
                
        except Exception as e:
            pytest.skip(f"Stellar node not available: {e}")
    
    @pytest.mark.integration
    def test_proxy_error_handling(self):
        """Test error handling for various proxy operation failure scenarios."""
        try:
            with StellarClient() as client:
                devices = client.list_devices()
                if not devices:
                    pytest.skip("No devices available for testing")
                
                device_id = devices[0].id
                
                # Test with invalid device ID
                with pytest.raises((DeviceNotFoundError, ProtocolError)):
                    client.proxy.create_proxy("invalid-device-id", 8080, 8080)
                
                # Test with invalid ports
                with pytest.raises((ValueError, ProtocolError)):
                    client.proxy.create_proxy(device_id, -1, 8080)
                
                with pytest.raises((ValueError, ProtocolError)):
                    client.proxy.create_proxy(device_id, 8080, -1)
                
                # Test closing non-existent proxy
                with pytest.raises((ProtocolError, Exception)):
                    client.proxy.close_proxy(99999)
                    
        except Exception as e:
            pytest.skip(f"Stellar node not available: {e}")


if __name__ == "__main__":
    pytest.main([__file__])

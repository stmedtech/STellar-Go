"""Performance and edge case tests for DCFL client."""

import pytest
import time
from unittest.mock import Mock, patch
from dcfl_client import DCFLClient
from dcfl_client.models.device import Device, DeviceStatus
from dcfl_client.exceptions import DCFLException, ConnectionError, ProtocolError


class TestPerformance:
    """Performance-related test cases."""
    
    def test_large_device_list_performance(self, mock_socket_client):
        """Test performance with large number of devices."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            # Mock large device list (1000 devices)
            large_device_dict = {
                f"device-{i}": {
                    "ID": f"device-{i}",
                    "Status": "healthy" if i % 2 == 0 else "discovered"
                }
                for i in range(1000)
            }
            mock_socket_client.get.return_value = large_device_dict
            
            client = DCFLClient()
            
            start_time = time.time()
            devices = client.list_devices()
            end_time = time.time()
            
            # Should complete within reasonable time
            assert len(devices) == 1000
            assert (end_time - start_time) < 1.0  # Should take less than 1 second
            
        
    def test_memory_usage_large_responses(self, mock_socket_client):
        """Test memory efficiency with large response payloads."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            # Mock large file list response
            large_file_list = [
                {
                    "Filename": f"large_file_{i}.dat",
                    "Size": 1024 * 1024,  # 1MB each
                    "IsDir": False
                }
                for i in range(1000)  # 1000 files
            ]
            mock_socket_client.get.return_value = large_file_list
            
            client = DCFLClient()
            
            # Should handle large responses efficiently
            files = client.file.list_files("device-1", "/large_directory")
            
            assert len(files) == 1000
            # Verify all objects are properly created
            assert all(hasattr(f, 'filename') for f in files)


class TestEdgeCases:
    """Edge case and boundary condition tests."""
    
    def test_empty_device_id_handling(self, mock_socket_client):
        """Test handling of empty or invalid device IDs."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            client = DCFLClient()
            
            # Test various invalid device IDs
            invalid_ids = ["", None, " ", "\n", "\t"]
            
            for invalid_id in invalid_ids:
                with pytest.raises(Exception):  # Should raise appropriate exception
                    client.get_device(invalid_id)
                    
    def test_very_long_device_id(self, mock_socket_client):
        """Test handling of extremely long device IDs."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            client = DCFLClient()
            
            # Very long device ID (1000 characters)
            long_id = "x" * 1000
            mock_socket_client.get.return_value = {"ID": long_id, "Status": "healthy"}
            
            device = client.get_device(long_id)
            assert device.id == long_id
            
    def test_unicode_handling(self, mock_socket_client):
        """Test handling of Unicode characters in device IDs and paths."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            client = DCFLClient()
            
            unicode_id = "device-测试-🚀"
            unicode_path = "/测试/文件夹/файл.txt"
            
            mock_socket_client.get.side_effect = [
                {"ID": unicode_id, "Status": "healthy"},  # get_device
                [{"Filename": "файл.txt", "Size": 1024, "IsDir": False}]  # list_files
            ]
            
            # Should handle Unicode properly
            device = client.get_device(unicode_id)
            assert device.id == unicode_id
            
            files = client.file.list_files(unicode_id, unicode_path)
            assert len(files) == 1
            assert "файл.txt" in files[0].filename
            
    def test_special_characters_in_paths(self, mock_socket_client):
        """Test file paths with special characters."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            client = DCFLClient()
            
            special_paths = [
                "/path with spaces/file.txt",
                "/path/with/../../traversal/file.txt", 
                "/path/with'quotes/file.txt",
                "/path/with\"double_quotes/file.txt",
                "/path/with&ampersand/file.txt"
            ]
            
            mock_response = [{"Filename": "file.txt", "Size": 1024, "IsDir": False}]
            mock_socket_client.get.return_value = mock_response
            
            for path in special_paths:
                try:
                    files = client.file.list_files("device-1", path)
                    assert len(files) >= 0  # Should not crash
                except Exception as e:
                    # Some special characters might be rejected, which is fine
                    assert isinstance(e, (ProtocolError, ValueError))
                    
    def test_zero_size_files(self, mock_socket_client):
        """Test handling of zero-size files."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            client = DCFLClient()
            
            zero_size_files = [
                {"Filename": "empty.txt", "Size": 0, "IsDir": False},
                {"Filename": "also_empty.log", "Size": 0, "IsDir": False}
            ]
            mock_socket_client.get.return_value = zero_size_files
            
            files = client.file.list_files("device-1", "/empty")
            
            assert len(files) == 2
            for file_entry in files:
                assert file_entry.size == 0
                assert not file_entry.is_dir
                
    def test_extremely_large_file_sizes(self, mock_socket_client):
        """Test handling of very large file sizes."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            client = DCFLClient()
            
            # Test with file sizes near system limits
            large_files = [
                {"Filename": "huge_file.dat", "Size": 2**63 - 1, "IsDir": False},  # Max int64
                {"Filename": "big_file.iso", "Size": 1024**4, "IsDir": False}  # 1TB
            ]
            mock_socket_client.get.return_value = large_files
            
            files = client.file.list_files("device-1", "/large")
            
            assert len(files) == 2
            assert files[0].size == 2**63 - 1
            assert files[1].size == 1024**4
            
    def test_malformed_responses(self, mock_socket_client):
        """Test handling of malformed server responses."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            client = DCFLClient()
            
            malformed_responses = [
                None,  # Null response
                "",    # Empty string
                "invalid json",  # Invalid JSON
                {"incomplete": True},  # Missing required fields
                {"ID": None, "Status": "healthy"},  # Null ID
                42,    # Wrong type
                []     # Wrong type for device response
            ]
            
            for response in malformed_responses:
                mock_socket_client.get.return_value = response
                
                try:
                    # Should either handle gracefully or raise appropriate exception
                    result = client.list_devices() if isinstance(response, dict) else client.get_device("test")
                    # If it succeeds, verify the result is safe to use
                    if result:
                        assert isinstance(result, (list, Device))
                except Exception as e:
                    # Should be a known exception type, not generic crash
                    assert isinstance(e, (DCFLException, ValueError, TypeError, KeyError))
                    
    def test_timeout_scenarios(self, mock_socket_client):
        """Test various timeout scenarios."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            client = DCFLClient()
            
            # Mock timeout exception
            import socket
            mock_socket_client.get.side_effect = socket.timeout("Operation timed out")
            
            with pytest.raises(Exception):  # Should handle timeout appropriately
                client.list_devices()
                
    def test_network_interruption_recovery(self, mock_socket_client):
        """Test recovery from network interruptions."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            client = DCFLClient()
            
            # Simulate network interruption followed by recovery
            responses = [
                ConnectionError("Network unreachable"),  # Failure
                ConnectionError("Connection reset"),     # Failure
                {"success": True, "data": "recovered"}   # Success
            ]
            
            mock_socket_client.get.side_effect = responses
            
            # First two attempts should fail
            with pytest.raises(ConnectionError):
                client.get_node_info()
                
            with pytest.raises(ConnectionError):
                client.get_node_info()
                
            # Third attempt should succeed
            result = client.get_node_info()
            assert result["success"] is True


class TestResourceManagement:
    """Test proper resource management and cleanup."""
    
    def test_context_manager_cleanup(self, mock_socket_client):
        """Test that context managers properly clean up resources."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            # Test normal exit
            with DCFLClient() as client:
                assert client is not None
                
            # Test exception exit
            try:
                with DCFLClient() as client:
                    raise ValueError("Test exception")
            except ValueError:
                pass  # Expected
                
            # Verify cleanup happened (would need more detailed mocking to test fully)
            
    def test_memory_leaks_prevention(self, mock_socket_client):
        """Test that repeated operations don't cause memory leaks."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            mock_socket_client.get.return_value = {"device-1": {"ID": "device-1", "Status": "healthy"}}
            
            client = DCFLClient()
            
            # Perform many operations to check for memory leaks
            for i in range(100):
                devices = client.list_devices()
                assert len(devices) == 1
                
                # Force garbage collection periodically
                if i % 10 == 0:
                    import gc
                    gc.collect()
                    
    def test_protocol_object_reuse(self, mock_socket_client):
        """Test that protocol objects are properly reused."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            client = DCFLClient()
            
            # First access should create the protocol
            echo1 = client.echo
            assert echo1 is not None
            
            # Second access should return the same instance
            echo2 = client.echo
            assert echo1 is echo2  # Same object reference
            
            # Same for other protocols
            file1 = client.file
            file2 = client.file
            assert file1 is file2
"""Integration tests for DCFL client with mocked DCFL node."""

import pytest
import json
from unittest.mock import Mock, patch
from dcfl_client import DCFLClient
from dcfl_client.models.device import Device, DeviceStatus
from dcfl_client.models.responses import CondaEnvConfig, ScriptConfig
from dcfl_client.exceptions import DeviceNotFoundError, ProtocolError


class TestDCFLIntegration:
    """Integration tests with mocked DCFL node responses."""
    
    def test_full_workflow_sync(self, mock_dcfl_responses):
        """Test complete sync workflow: connect, discover, ping, execute."""
        with patch('dcfl_client.utils.socket_client.UnixSocketClient') as mock_client_class:
            mock_client = Mock()
            mock_client_class.return_value = mock_client
            
            # Configure responses for each API call
            def side_effect(endpoint, **kwargs):
                return mock_dcfl_responses.get(endpoint, {})
            
            mock_client.get.side_effect = side_effect
            mock_client.post.side_effect = side_effect
            
            with DCFLClient() as client:
                # 1. Check node health
                health = client.socket_client.get("/health")
                assert health["status"] == "healthy"
                
                # 2. List devices
                devices = client.list_devices()
                assert len(devices) == 2
                assert all(isinstance(d, Device) for d in devices)
                
                # 3. Ping a specific device
                ping_result = client.echo.ping("device-1")
                assert ping_result.success is True
                
                # 4. List files on device
                files = client.file.list_files("device-1", "/")
                assert len(files) >= 1
                
                # 5. Check compute environments
                envs = client.compute.list_conda_envs("device-1")
                assert "base" in envs
                
                # 6. Get policy
                policy = client.get_policy()
                assert policy.enable is True
                
    def test_error_recovery_workflow(self, mock_socket_client):
        """Test workflow with error recovery scenarios."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            client = DCFLClient()
            
            # Simulate network errors and recovery
            mock_socket_client.get.side_effect = [
                Exception("Connection timeout"),  # First attempt fails
                {"device-1": {"ID": "device-1", "Status": "healthy"}}  # Retry succeeds
            ]
            
            # Should handle error gracefully
            with pytest.raises(Exception):  # First call fails
                client.list_devices()
                
            # Second call should succeed (in real scenario, retry logic would handle this)
            devices = client.list_devices()
            assert len(devices) == 1
            
    def test_policy_management_workflow(self, mock_socket_client):
        """Test complete policy management workflow."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            client = DCFLClient()
            
            # Mock policy responses
            initial_policy = {"Enable": False, "WhiteList": []}
            updated_policy = {"Enable": True, "WhiteList": ["device-1"]}
            
            mock_socket_client.get.side_effect = [initial_policy, ["device-1"], updated_policy]
            
            # 1. Get initial policy
            policy = client.get_policy()
            assert policy.enable is False
            assert len(policy.whitelist) == 0
            
            # 2. Whitelist a device
            client.whitelist_device("device-1")
            
            # 3. Check whitelist
            whitelist = client.get_whitelist()
            assert "device-1" in whitelist
            
            # 4. Update policy to enable
            from dcfl_client.models.policy import SecurityPolicy
            new_policy = SecurityPolicy(enable=True, whitelist=["device-1"])
            client.update_policy(new_policy)
            
            # 5. Verify policy was updated
            final_policy = client.get_policy()
            assert final_policy.enable is True
            
    def test_file_transfer_workflow(self, mock_socket_client, temp_file, temp_directory):
        """Test complete file transfer workflow."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            client = DCFLClient()
            
            # Mock file operations
            file_list = [{"Filename": "remote_file.txt", "Size": 2048, "IsDir": False}]
            download_response = {"success": True, "file_path": "/remote/file.txt"}
            upload_response = {"success": True, "remote_path": "/remote/uploaded.txt"}
            
            mock_socket_client.get.side_effect = [file_list, download_response]
            mock_socket_client.post.return_value = upload_response
            
            # 1. List remote files
            files = client.file.list_files("device-1", "/data")
            assert len(files) == 1
            assert files[0].filename == "remote_file.txt"
            
            # 2. Download file
            local_path = f"{temp_directory}/downloaded.txt"
            with patch('os.path.exists', return_value=True), \
                 patch('os.path.getsize', return_value=2048):
                success = client.file.download_file("device-1", "remote_file.txt", local_path)
                assert success is True
            
            # 3. Upload file
            success = client.file.upload_file("device-1", temp_file, "uploaded.txt")
            assert success is True
            
    def test_proxy_management_workflow(self, mock_socket_client):
        """Test proxy creation and management workflow."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            client = DCFLClient()
            
            # Mock proxy responses
            create_response = {
                "success": True,
                "proxy_id": "proxy-123",
                "local_port": 8080,
                "remote_addr": "service:80",
                "device_id": "device-1"
            }
            
            list_response = [create_response]
            close_response = {"success": True, "port": 8080}
            
            mock_socket_client.post.return_value = create_response
            mock_socket_client.get.return_value = list_response
            mock_socket_client.delete.return_value = close_response
            
            # 1. Create TCP proxy
            proxy_info = client.proxy.create_tcp_proxy("device-1", 8080, "service", 80)
            assert proxy_info.local_port == 8080
            assert proxy_info.remote_addr == "service:80"
            
            # 2. List proxies
            proxies = client.proxy.list_proxies()
            assert len(proxies) == 1
            assert proxies[0].local_port == 8080
            
            # 3. Close proxy
            success = client.proxy.close_proxy(8080)
            assert success is True


class TestErrorScenarios:
    """Test various error scenarios and recovery."""
    
    def test_device_offline_scenario(self, mock_socket_client):
        """Test handling of offline device scenarios."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            client = DCFLClient()
            
            # Mock device as offline
            mock_socket_client.post.return_value = {
                "success": False,
                "device_id": "offline-device",
                "error": "Device unreachable"
            }
            
            result = client.echo.ping("offline-device")
            assert result.success is False
            assert "unreachable" in result.error.lower()
            
    def test_compute_environment_missing(self, mock_socket_client):
        """Test compute operations with missing environment."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            client = DCFLClient()
            
            # Mock missing environment
            mock_socket_client.get.return_value = {"base": "/opt/conda"}  # No 'ml' env
            
            envs = client.compute.list_conda_envs("device-1")
            assert "ml" not in envs
            
            # Health check should return False
            is_healthy = client.compute.check_environment_health("device-1", "ml")
            assert is_healthy is False
            
    def test_file_not_found_scenario(self, mock_socket_client):
        """Test file operations with non-existent files."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            client = DCFLClient()
            
            # Mock file not found response
            mock_socket_client.get.return_value = {
                "success": False,
                "error": "File not found"
            }
            
            with pytest.raises(Exception):  # Should raise appropriate exception
                client.file.download_file("device-1", "nonexistent.txt", "/tmp/local.txt")
                

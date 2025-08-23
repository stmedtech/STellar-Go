"""Tests for protocol implementations."""

import pytest
from unittest.mock import Mock, patch
from dcfl_client.protocols.echo import EchoProtocol
from dcfl_client.protocols.file import FileProtocol
from dcfl_client.protocols.compute import ComputeProtocol
from dcfl_client.protocols.proxy import ProxyProtocol
from dcfl_client.models.responses import (
    PingResponse,
    FileEntry,
    ExecutionResult,
    ProxyInfo,
    CondaEnvConfig,
    ScriptConfig
)
from dcfl_client.exceptions import (
    DeviceNotFoundError,
    ProtocolError,
    FileTransferError,
    ComputeError
)


class TestEchoProtocol:
    """Test cases for EchoProtocol."""
    
    @pytest.fixture
    def echo_protocol(self, mock_socket_client):
        """Echo protocol with mocked socket client."""
        return EchoProtocol(mock_socket_client)
    
    def test_ping_success(self, echo_protocol, mock_socket_client):
        """Test successful ping."""
        mock_response = {"success": True, "device_id": "test-device"}
        mock_socket_client.post.return_value = mock_response
        
        result = echo_protocol.ping("test-device")
        
        assert isinstance(result, PingResponse)
        assert result.success is True
        assert result.device_id == "test-device"
        mock_socket_client.post.assert_called_once_with("/devices/test-device/ping")
        
    def test_ping_failure(self, echo_protocol, mock_socket_client):
        """Test failed ping."""
        mock_response = {"success": False, "device_id": "test-device", "error": "Connection timeout"}
        mock_socket_client.post.return_value = mock_response
        
        result = echo_protocol.ping("test-device")
        
        assert isinstance(result, PingResponse)
        assert result.success is False
        assert result.error == "Connection timeout"
        
    def test_ping_invalid_device(self, echo_protocol, mock_socket_client):
        """Test ping with invalid device ID."""
        with pytest.raises(DeviceNotFoundError):
            echo_protocol.ping("")
            
        with pytest.raises(DeviceNotFoundError):
            echo_protocol.ping(None)
            
    def test_ping_exception(self, echo_protocol, mock_socket_client):
        """Test ping with socket exception."""
        mock_socket_client.post.side_effect = Exception("Network error")
        
        with pytest.raises(ProtocolError):
            echo_protocol.ping("test-device")
            
    def test_get_device_info_success(self, echo_protocol, mock_socket_client):
        """Test successful device info retrieval."""
        mock_response = '{"ID": "test-device", "Status": "healthy"}'
        mock_socket_client.get.return_value = mock_response
        
        with patch('json.loads') as mock_json:
            mock_json.return_value = {"ID": "test-device", "Status": "healthy"}
            
            result = echo_protocol.get_device_info("test-device")
            
            mock_socket_client.get.assert_called_once_with("/devices/test-device/info")
            mock_json.assert_called_once_with(mock_response)
            
    def test_ping_all(self, echo_protocol, mock_socket_client):
        """Test pinging multiple devices."""
        mock_socket_client.post.side_effect = [
            {"success": True, "device_id": "device-1"},
            {"success": False, "device_id": "device-2", "error": "Timeout"},
            Exception("Network error")
        ]
        
        device_ids = ["device-1", "device-2", "device-3"]
        results = echo_protocol.ping_all(device_ids)
        
        assert len(results) == 3
        assert results["device-1"].success is True
        assert results["device-2"].success is False
        assert results["device-3"].success is False  # Exception should create failed response


class TestFileProtocol:
    """Test cases for FileProtocol."""
    
    @pytest.fixture
    def file_protocol(self, mock_socket_client):
        """File protocol with mocked socket client."""
        return FileProtocol(mock_socket_client)
    
    def test_list_files_success(self, file_protocol, mock_socket_client, sample_file_entries):
        """Test successful file listing."""
        mock_response = [
            {"Filename": "file1.txt", "Size": 1024, "IsDir": False},
            {"Filename": "dir1", "Size": 0, "IsDir": True}
        ]
        mock_socket_client.get.return_value = mock_response
        
        result = file_protocol.list_files("test-device", "/test")
        
        assert len(result) == 2
        assert all(isinstance(entry, FileEntry) for entry in result)
        mock_socket_client.get.assert_called_once_with(
            "/devices/test-device/files",
            params={"path": "test"}  # Sanitized path
        )
        
    def test_list_files_invalid_device(self, file_protocol, mock_socket_client):
        """Test file listing with invalid device."""
        with pytest.raises(DeviceNotFoundError):
            file_protocol.list_files("", "/test")
            
    def test_list_files_exception(self, file_protocol, mock_socket_client):
        """Test file listing with exception."""
        mock_socket_client.get.side_effect = Exception("Access denied")
        
        with pytest.raises(FileTransferError):
            file_protocol.list_files("test-device", "/test")
            
    def test_download_file_success(self, file_protocol, mock_socket_client, temp_directory):
        """Test successful file download."""
        mock_response = {"success": True, "file_path": "/remote/file.txt"}
        mock_socket_client.get.return_value = mock_response
        
        local_path = f"{temp_directory}/downloaded_file.txt"
        
        with patch('os.path.exists', return_value=True), \
             patch('os.path.getsize', return_value=1024):
            
            result = file_protocol.download_file(
                "test-device", "remote_file.txt", local_path
            )
            
            assert result is True
            mock_socket_client.get.assert_called_once()
            
    def test_download_file_failure(self, file_protocol, mock_socket_client):
        """Test failed file download."""
        mock_response = {"success": False, "error": "File not found"}
        mock_socket_client.get.return_value = mock_response
        
        with pytest.raises(FileTransferError):
            file_protocol.download_file("test-device", "nonexistent.txt", "/tmp/local.txt")
            
    def test_upload_file_success(self, file_protocol, mock_socket_client, temp_file):
        """Test successful file upload."""
        mock_response = {"success": True, "remote_path": "/remote/file.txt"}
        mock_socket_client.post.return_value = mock_response
        
        result = file_protocol.upload_file(
            "test-device", temp_file, "remote_file.txt"
        )
        
        assert result is True
        mock_socket_client.post.assert_called_once()
        
    def test_upload_file_not_found(self, file_protocol, mock_socket_client):
        """Test file upload with non-existent local file."""
        with pytest.raises(FileTransferError):
            file_protocol.upload_file("test-device", "/nonexistent/file.txt", "remote.txt")
            
    def test_upload_file_not_file(self, file_protocol, mock_socket_client, temp_directory):
        """Test file upload with directory instead of file."""
        with pytest.raises(FileTransferError):
            file_protocol.upload_file("test-device", temp_directory, "remote.txt")


class TestComputeProtocol:
    """Test cases for ComputeProtocol."""
    
    @pytest.fixture
    def compute_protocol(self, mock_socket_client):
        """Compute protocol with mocked socket client."""
        return ComputeProtocol(mock_socket_client)
    
    def test_list_conda_envs_success(self, compute_protocol, mock_socket_client):
        """Test successful Conda environment listing."""
        mock_response = {"base": "/opt/conda", "ml_env": "/opt/conda/envs/ml_env"}
        mock_socket_client.get.return_value = mock_response
        
        result = compute_protocol.list_conda_envs("test-device")
        
        assert result == mock_response
        mock_socket_client.get.assert_called_once_with("/devices/test-device/compute/envs")
        
    def test_list_conda_envs_exception(self, compute_protocol, mock_socket_client):
        """Test Conda environment listing with exception."""
        mock_socket_client.get.side_effect = Exception("Conda not installed")
        
        with pytest.raises(ComputeError):
            compute_protocol.list_conda_envs("test-device")
            
    def test_prepare_environment_success(self, compute_protocol, mock_socket_client):
        """Test successful environment preparation."""
        mock_response = {"success": True, "env_path": "/opt/conda/envs/test_env"}
        mock_socket_client.post.return_value = mock_response
        
        config = CondaEnvConfig(
            env="test_env",
            version="3.9",
            env_yaml_path="environment.yml"
        )
        
        result = compute_protocol.prepare_environment("test-device", config)
        
        assert result == "/opt/conda/envs/test_env"
        mock_socket_client.post.assert_called_once_with(
            "/devices/test-device/compute/prepare",
            json_data=config.model_dump()
        )
        
    def test_prepare_environment_failure(self, compute_protocol, mock_socket_client):
        """Test failed environment preparation."""
        mock_response = {"success": False, "error": "Environment creation failed"}
        mock_socket_client.post.return_value = mock_response
        
        config = CondaEnvConfig(env="test_env", version="3.9", env_yaml_path="env.yml")
        
        with pytest.raises(ComputeError):
            compute_protocol.prepare_environment("test-device", config)
            
    def test_execute_script_success(self, compute_protocol, mock_socket_client, sample_execution_result):
        """Test successful script execution."""
        mock_response = {
            "success": True,
            "result": "Hello World!",
            "env": "test_env"
        }
        mock_socket_client.post.return_value = mock_response
        
        config = ScriptConfig(env="test_env", script_path="test_script.py")
        
        result = compute_protocol.execute_script("test-device", config)
        
        assert isinstance(result, ExecutionResult)
        assert result.success is True
        assert result.result == "Hello World!"
        mock_socket_client.post.assert_called_once_with(
            "/devices/test-device/compute/execute",
            json_data=config.model_dump()
        )
        
    def test_check_environment_health(self, compute_protocol, mock_socket_client):
        """Test environment health check."""
        mock_socket_client.get.return_value = {"base": "/opt/conda", "test_env": "/opt/conda/envs/test_env"}
        
        result = compute_protocol.check_environment_health("test-device", "test_env")
        assert result is True
        
        result = compute_protocol.check_environment_health("test-device", "nonexistent_env")
        assert result is False


class TestProxyProtocol:
    """Test cases for ProxyProtocol."""
    
    @pytest.fixture
    def proxy_protocol(self, mock_socket_client):
        """Proxy protocol with mocked socket client."""
        return ProxyProtocol(mock_socket_client)
    
    def test_create_tcp_proxy_success(self, proxy_protocol, mock_socket_client):
        """Test successful TCP proxy creation."""
        mock_response = {
            "success": True,
            "proxy_id": "proxy-123",
            "local_port": 8080,
            "remote_addr": "remote-host:80",
            "device_id": "test-device"
        }
        mock_socket_client.post.return_value = mock_response
        
        result = proxy_protocol.create_tcp_proxy(
            "test-device", 8080, "remote-host", 80
        )
        
        assert isinstance(result, ProxyInfo)
        assert result.local_port == 8080
        assert result.remote_addr == "remote-host:80"
        mock_socket_client.post.assert_called_once_with(
            "/proxy",
            json_data={
                "device_id": "test-device",
                "local_port": 8080,
                "remote_host": "remote-host",
                "remote_port": 80,
            }
        )
        
    def test_create_tcp_proxy_invalid_port(self, proxy_protocol, mock_socket_client):
        """Test TCP proxy creation with invalid port."""
        with pytest.raises(ProtocolError):
            proxy_protocol.create_tcp_proxy("test-device", 0, "host", 80)
            
        with pytest.raises(ProtocolError):
            proxy_protocol.create_tcp_proxy("test-device", 65536, "host", 80)
            
    def test_create_http_proxy_success(self, proxy_protocol, mock_socket_client):
        """Test successful HTTP proxy creation."""
        mock_response = {
            "success": True,
            "proxy_id": "proxy-123",
            "local_port": 8080,
            "remote_addr": "example.com:80",
            "device_id": "test-device"
        }
        mock_socket_client.post.return_value = mock_response
        
        result = proxy_protocol.create_http_proxy(
            "test-device", 8080, "http://example.com"
        )
        
        assert isinstance(result, ProxyInfo)
        assert result.remote_addr == "example.com:80"
        
    def test_create_http_proxy_invalid_url(self, proxy_protocol, mock_socket_client):
        """Test HTTP proxy creation with invalid URL."""
        with pytest.raises(ProtocolError):
            proxy_protocol.create_http_proxy("test-device", 8080, "invalid-url")
            
    def test_list_proxies_success(self, proxy_protocol, mock_socket_client):
        """Test successful proxy listing."""
        mock_response = [
            {"proxy_id": "proxy-1", "local_port": 8080, "remote_addr": "host1:80", "device_id": "dev1"},
            {"proxy_id": "proxy-2", "local_port": 8081, "remote_addr": "host2:443", "device_id": "dev2"}
        ]
        mock_socket_client.get.return_value = mock_response
        
        result = proxy_protocol.list_proxies()
        
        assert len(result) == 2
        assert all(isinstance(proxy, ProxyInfo) for proxy in result)
        mock_socket_client.get.assert_called_once_with("/proxy")
        
    def test_list_proxies_empty(self, proxy_protocol, mock_socket_client):
        """Test proxy listing when no proxies exist."""
        mock_socket_client.get.return_value = []
        
        result = proxy_protocol.list_proxies()
        
        assert len(result) == 0
        
    def test_close_proxy_success(self, proxy_protocol, mock_socket_client):
        """Test successful proxy closure."""
        mock_response = {"success": True, "port": 8080}
        mock_socket_client.delete.return_value = mock_response
        
        result = proxy_protocol.close_proxy(8080)
        
        assert result is True
        mock_socket_client.delete.assert_called_once_with("/proxy/8080")
        
    def test_close_proxy_invalid_port(self, proxy_protocol, mock_socket_client):
        """Test proxy closure with invalid port."""
        with pytest.raises(ProtocolError):
            proxy_protocol.close_proxy(0)
            
    def test_get_proxy_info(self, proxy_protocol, mock_socket_client):
        """Test getting specific proxy information."""
        mock_response = [
            {"proxy_id": "proxy-1", "local_port": 8080, "remote_addr": "host:80", "device_id": "dev1"}
        ]
        mock_socket_client.get.return_value = mock_response
        
        result = proxy_protocol.get_proxy_info(8080)
        
        assert result is not None
        assert result.local_port == 8080
        
        result = proxy_protocol.get_proxy_info(9999)  # Non-existent port
        assert result is None
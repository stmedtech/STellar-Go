"""Tests for the main DCFL client."""

import pytest
from unittest.mock import Mock, patch, MagicMock
from dcfl_client import DCFLClient
from dcfl_client.models.device import Device, DeviceStatus
from dcfl_client.models.policy import SecurityPolicy
from dcfl_client.exceptions import (
    DeviceNotFoundError,
    ProtocolError,
    ConnectionError
)


class TestDCFLClient:
    """Test cases for DCFLClient."""
    
    @pytest.fixture
    def client(self, mock_socket_client):
        """DCFL client with mocked socket client."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            return DCFLClient()
    
    def test_initialization(self):
        """Test client initialization."""
        client = DCFLClient()
        assert client.socket_path is not None
        assert client.socket_client is not None
        
    def test_initialization_with_custom_path(self):
        """Test client initialization with custom socket path."""
        custom_path = "/custom/path/stellar.sock"
        client = DCFLClient(socket_path=custom_path)
        assert client.socket_path == custom_path
        
    def test_context_manager(self, mock_socket_client):
        """Test client context manager functionality."""
        with patch('dcfl_client.client.UnixSocketClient', return_value=mock_socket_client):
            with DCFLClient() as client:
                assert client is not None
                # Client should be functional within context
                
    def test_list_devices_success(self, client, mock_socket_client, sample_devices):
        """Test successful device listing."""
        # Mock response with device data
        device_data = {device.id: {"ID": device.id, "Status": device.status.value} 
                      for device in sample_devices}
        mock_socket_client.get.return_value = device_data
        
        devices = client.list_devices()
        
        assert len(devices) == len(sample_devices)
        assert all(isinstance(device, Device) for device in devices)
        mock_socket_client.get.assert_called_once_with("/devices")
        
    def test_list_devices_empty(self, client, mock_socket_client):
        """Test device listing when no devices found."""
        mock_socket_client.get.return_value = {}
        
        devices = client.list_devices()
        
        assert len(devices) == 0
        assert isinstance(devices, list)
        
    def test_get_device_success(self, client, mock_socket_client, sample_device):
        """Test successful device retrieval."""
        device_data = {"ID": sample_device.id, "Status": sample_device.status.value}
        mock_socket_client.get.return_value = device_data
        
        device = client.get_device(sample_device.id)
        
        assert isinstance(device, Device)
        assert device.id == sample_device.id
        mock_socket_client.get.assert_called_once_with(f"/devices/{sample_device.id}")
        
    def test_get_device_not_found(self, client, mock_socket_client):
        """Test device retrieval when device not found."""
        mock_socket_client.get.side_effect = Exception("Device not found")
        
        with pytest.raises(DeviceNotFoundError):
            client.get_device("invalid-device")
            
    def test_get_device_invalid_id(self, client, mock_socket_client):
        """Test device retrieval with invalid device ID."""
        with pytest.raises(DeviceNotFoundError):
            client.get_device("")  # Empty string
            
        with pytest.raises(DeviceNotFoundError):
            client.get_device(None)  # None value
            
    def test_connect_to_peer_success(self, client, mock_socket_client):
        """Test successful peer connection."""
        mock_socket_client.post.return_value = {"success": True, "device": {}}
        
        result = client.connect_to_peer("test-peer-info")
        
        assert result is True
        mock_socket_client.post.assert_called_once_with(
            "/connect",
            json_data={"peer_info": "test-peer-info"}
        )
        
    def test_connect_to_peer_failure(self, client, mock_socket_client):
        """Test failed peer connection."""
        mock_socket_client.post.return_value = {"success": False}
        
        result = client.connect_to_peer("invalid-peer-info")
        
        assert result is False
        
    def test_connect_to_peer_exception(self, client, mock_socket_client):
        """Test peer connection with exception."""
        mock_socket_client.post.side_effect = Exception("Connection failed")
        
        with pytest.raises(ProtocolError):
            client.connect_to_peer("test-peer-info")
            
    def test_get_node_info_success(self, client, mock_socket_client):
        """Test successful node info retrieval."""
        node_info = {"id": "test-node", "addresses": ["127.0.0.1:4001"]}
        mock_socket_client.get.return_value = node_info
        
        result = client.get_node_info()
        
        assert result == node_info
        mock_socket_client.get.assert_called_once_with("/node")
        
    def test_get_policy_success(self, client, mock_socket_client):
        """Test successful policy retrieval."""
        policy_data = {"Enable": True, "WhiteList": ["device-1", "device-2"]}
        mock_socket_client.get.return_value = policy_data
        
        policy = client.get_policy()
        
        assert isinstance(policy, SecurityPolicy)
        assert policy.enable is True
        assert len(policy.whitelist) == 2
        mock_socket_client.get.assert_called_once_with("/policy")
        
    def test_update_policy_success(self, client, mock_socket_client):
        """Test successful policy update."""
        policy = SecurityPolicy(enable=True, whitelist=["device-1"])
        
        result = client.update_policy(policy)
        
        assert result is True
        mock_socket_client.post.assert_called_once_with(
            "/policy",
            data={"enable": "true"}
        )
        
    def test_update_policy_failure(self, client, mock_socket_client):
        """Test failed policy update."""
        mock_socket_client.post.side_effect = Exception("Update failed")
        policy = SecurityPolicy(enable=True, whitelist=[])
        
        result = client.update_policy(policy)
        
        assert result is False
        
    def test_whitelist_device_success(self, client, mock_socket_client):
        """Test successful device whitelisting."""
        result = client.whitelist_device("device-1")
        
        assert result is True
        mock_socket_client.post.assert_called_once_with(
            "/policy/whitelist",
            data={"deviceId": "device-1"}
        )
        
    def test_whitelist_device_invalid_id(self, client, mock_socket_client):
        """Test device whitelisting with invalid ID."""
        result = client.whitelist_device("")
        assert result is False
        
        result = client.whitelist_device(None)
        assert result is False
        
    def test_remove_from_whitelist_success(self, client, mock_socket_client):
        """Test successful whitelist removal."""
        result = client.remove_from_whitelist("device-1")
        
        assert result is True
        mock_socket_client.delete.assert_called_once_with(
            "/policy/whitelist",
            params={"deviceId": "device-1"}
        )
        
    def test_get_whitelist_success(self, client, mock_socket_client):
        """Test successful whitelist retrieval."""
        whitelist = ["device-1", "device-2", "device-3"]
        mock_socket_client.get.return_value = whitelist
        
        result = client.get_whitelist()
        
        assert result == whitelist
        mock_socket_client.get.assert_called_once_with("/policy/whitelist")
        
    def test_get_whitelist_empty(self, client, mock_socket_client):
        """Test whitelist retrieval when empty."""
        mock_socket_client.get.return_value = []
        
        result = client.get_whitelist()
        
        assert result == []
        
    def test_protocol_managers_lazy_loading(self, client):
        """Test that protocol managers are lazy loaded."""
        # Protocol managers should be None initially
        assert client._echo is None
        assert client._file is None
        assert client._compute is None
        assert client._proxy is None
        
        # Accessing properties should create instances
        with patch('dcfl_client.protocols.echo.EchoProtocol') as mock_echo:
            echo = client.echo
            mock_echo.assert_called_once_with(client.socket_client)
            
        with patch('dcfl_client.protocols.file.FileProtocol') as mock_file:
            file_proto = client.file
            mock_file.assert_called_once_with(client.socket_client)
            
        with patch('dcfl_client.protocols.compute.ComputeProtocol') as mock_compute:
            compute = client.compute
            mock_compute.assert_called_once_with(client.socket_client)
            
        with patch('dcfl_client.protocols.proxy.ProxyProtocol') as mock_proxy:
            proxy = client.proxy
            mock_proxy.assert_called_once_with(client.socket_client)
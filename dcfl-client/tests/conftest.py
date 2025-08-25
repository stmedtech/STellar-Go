"""Test configuration and fixtures."""

import pytest
import tempfile
import os
from unittest.mock import Mock, MagicMock
from dcfl_client.utils.socket_client import UnixSocketClient
from dcfl_client.models.device import Device, DeviceStatus
from dcfl_client.models.responses import PingResponse, FileEntry, ExecutionResult


@pytest.fixture
def mock_socket_client():
    """Mock Unix socket client."""
    client = Mock(spec=UnixSocketClient)
    
    # Default successful responses
    client.get.return_value = {"success": True}
    client.post.return_value = {"success": True}
    client.delete.return_value = {"success": True}
    
    return client


@pytest.fixture
def sample_device():
    """Sample device for testing."""
    return Device(
        id="test-device-123",
        reference_token="test-token",
        status=DeviceStatus.HEALTHY,
        sys_info=None,
        timestamp=None
    )


@pytest.fixture
def sample_devices():
    """List of sample devices for testing."""
    return [
        Device(
            id="device-1",
            reference_token="token-1",
            status=DeviceStatus.HEALTHY
        ),
        Device(
            id="device-2", 
            reference_token="token-2",
            status=DeviceStatus.DISCOVERED
        ),
        Device(
            id="device-3",
            reference_token="token-3",
            status=DeviceStatus.HEALTHY
        )
    ]


@pytest.fixture
def sample_ping_response():
    """Sample ping response for testing."""
    return PingResponse(
        success=True,
        device_id="test-device-123",
        timestamp=None,
        error=None
    )


@pytest.fixture
def sample_file_entries():
    """Sample file entries for testing."""
    return [
        FileEntry(
            filename="file1.txt",
            directory_name="/test",
            size=1024,
            is_dir=False
        ),
        FileEntry(
            filename="subdir",
            directory_name="/test",
            size=0,
            is_dir=True,
            children=[
                FileEntry(
                    filename="nested.txt",
                    directory_name="/test/subdir",
                    size=512,
                    is_dir=False
                )
            ]
        )
    ]


@pytest.fixture
def sample_execution_result():
    """Sample execution result for testing."""
    return ExecutionResult(
        success=True,
        result="Hello World!\nExecution completed.",
        env="test_env",
        error=None,
        execution_time=1.5
    )


@pytest.fixture
def temp_file():
    """Temporary file for testing file operations."""
    with tempfile.NamedTemporaryFile(mode='w', delete=False) as f:
        f.write("Test file content\nLine 2\nLine 3")
        temp_path = f.name
        
    yield temp_path
    
    # Cleanup
    if os.path.exists(temp_path):
        os.unlink(temp_path)


@pytest.fixture
def temp_directory():
    """Temporary directory for testing."""
    temp_dir = tempfile.mkdtemp()
    yield temp_dir
    
    # Cleanup
    import shutil
    shutil.rmtree(temp_dir, ignore_errors=True)


@pytest.fixture
def mock_dcfl_responses():
    """Mock responses for various DCFL API endpoints."""
    return {
        "/health": {"status": "healthy", "node_id": "test-node"},
        "/node": {"id": "test-node", "addresses": ["127.0.0.1:4001"]},
        "/devices": {
            "device-1": {"ID": "device-1", "Status": "healthy"},
            "device-2": {"ID": "device-2", "Status": "discovered"}
        },
        "/devices/device-1": {"ID": "device-1", "Status": "healthy"},
        "/devices/device-1/ping": {"success": True, "device_id": "device-1"},
        "/devices/device-1/info": {"ID": "device-1", "Status": "healthy"},
        "/devices/device-1/files": [
            {"Filename": "test.txt", "Size": 1024, "IsDir": False}
        ],
        "/devices/device-1/compute/envs": {"base": "/opt/conda", "test": "/opt/conda/envs/test"},
        "/policy": {"Enable": True, "WhiteList": ["device-1"]},
        "/proxy": []
    }
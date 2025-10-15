"""
Pytest configuration and fixtures for DCFL Client tests.
"""

import pytest
import tempfile
import os
from dcfl_client import DCFLClient


@pytest.fixture
def temp_file():
    """Create a temporary file for testing."""
    with tempfile.NamedTemporaryFile(mode='w', delete=False, suffix='.txt') as tmp_file:
        tmp_file.write("Test content for DCFL client testing")
        tmp_file_path = tmp_file.name
    
    yield tmp_file_path
    
    # Cleanup
    try:
        os.unlink(tmp_file_path)
    except:
        pass


@pytest.fixture
def temp_dir():
    """Create a temporary directory for testing."""
    temp_dir = tempfile.mkdtemp()
    yield temp_dir
    
    # Cleanup
    try:
        import shutil
        shutil.rmtree(temp_dir)
    except:
        pass


# Pytest markers
def pytest_configure(config):
    """Configure pytest markers."""
    config.addinivalue_line(
        "markers", "integration: mark test as integration test (requires running DCFL node)"
    )
    config.addinivalue_line(
        "markers", "slow: mark test as slow running"
    )
    config.addinivalue_line(
        "markers", "echo: mark test as echo protocol related"
    )
    config.addinivalue_line(
        "markers", "file: mark test as file protocol related"
    )
    config.addinivalue_line(
        "markers", "compute: mark test as compute protocol related"
    )
    config.addinivalue_line(
        "markers", "proxy: mark test as proxy protocol related"
    )

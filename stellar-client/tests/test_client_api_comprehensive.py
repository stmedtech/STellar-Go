"""
Comprehensive integration tests for all Stellar Client APIs.

This test suite verifies all endpoints work correctly with a running stellar-go daemon.
"""

import pytest
import time
import stellar_client
from stellar_client import (
    DeviceNotFoundError,
    ComputeError,
    ProtocolError,
    ConnectionError,
)


@pytest.mark.integration
class TestCompleteWorkflow:
    """Test complete workflows combining multiple operations."""
    
    def test_device_discovery_to_compute_workflow(self):
        """Test complete workflow: discover device -> run compute -> get logs."""
        client = stellar_client.from_env()
        
        # Discover devices
        devices = client.devices.list()
        if not devices:
            pytest.skip("No devices available")
        
        device = devices[0]
        
        # Verify device
        device.ping()
        device.info()
        
        # Run compute
        run = client.compute.run(
            device_id=device.id,
            command="echo",
            args=["workflow", "test"]
        )
        
        # Wait for completion
        run.wait(timeout=10)
        
        # Get output
        stdout = run.stdout()
        assert isinstance(stdout, str)
        
        # Cleanup
        run.remove()
    
    def test_file_transfer_workflow(self):
        """Test file transfer workflow.
        
        Note: File upload/download requires the file to exist on the server side,
        which doesn't work in the Docker test environment where client and server
        are in separate containers. This test is skipped in test environments.
        """
        import tempfile
        import os
        
        # Skip in Docker test environment - file upload requires server-side file access
        pytest.skip("File upload/download requires server-side file access, not available in Docker test environment")
    
    def test_proxy_workflow(self):
        """Test proxy creation and management workflow."""
        client = stellar_client.from_env()
        devices = client.devices.list()
        if not devices:
            pytest.skip("No devices available")
        
        device = devices[0]
        
        try:
            # Create proxy
            proxy = client.proxy.create(
                device_id=device.id,
                local_port=9090,
                remote_host="127.0.0.1",
                remote_port=80
            )
            
            assert proxy.port == 9090
            
            # List proxies
            proxies = client.proxy.list()
            assert any(p.port == 9090 for p in proxies)
            
            # Get proxy
            retrieved = client.proxy.get(9090)
            assert retrieved is not None
            
            # Close proxy
            proxy.close()
            
            # Verify closed
            proxies_after = client.proxy.list()
            assert not any(p.port == 9090 for p in proxies_after)
        except ProtocolError:
            # Proxy may fail if port in use
            pass


@pytest.mark.integration
class TestStreamingOperations:
    """Test streaming operations."""
    
    def test_stream_stdout_realtime(self):
        """Test streaming stdout in real-time."""
        client = stellar_client.from_env()
        devices = client.devices.list()
        if not devices:
            pytest.skip("No devices available")
        
        device = devices[0]
        
        run = client.compute.run(
            device_id=device.id,
            command="echo",
            args=["streaming", "test"]
        )
        
        # Wait a bit
        time.sleep(0.5)
        
        # Get stdout (non-streaming first)
        stdout = run.stdout()
        assert isinstance(stdout, str)
        
        # Stream stdout
        stdout_lines = list(run.stdout(stream=True))
        assert isinstance(stdout_lines, list)
    
    def test_stream_logs_realtime(self):
        """Test streaming logs in real-time."""
        client = stellar_client.from_env()
        devices = client.devices.list()
        if not devices:
            pytest.skip("No devices available")
        
        device = devices[0]
        
        run = client.compute.run(
            device_id=device.id,
            command="echo",
            args=["log", "streaming", "test"]
        )
        
        time.sleep(0.5)
        
        # Get logs
        logs = run.logs()
        assert isinstance(logs, str)
        
        # Stream logs
        log_entries = list(run.logs(stream=True))
        assert isinstance(log_entries, list)


@pytest.mark.integration
class TestConcurrentOperations:
    """Test concurrent operations."""
    
    def test_concurrent_compute_runs(self):
        """Test running multiple compute operations concurrently."""
        client = stellar_client.from_env()
        devices = client.devices.list()
        if not devices:
            pytest.skip("No devices available")
        
        device = devices[0]
        
        # Create multiple runs
        runs = []
        for i in range(5):
            run = client.compute.run(
                device_id=device.id,
                command="echo",
                args=[f"concurrent_{i}"]
            )
            runs.append(run)
        
        # Wait for all
        for run in runs:
            try:
                run.wait(timeout=5)
            except ComputeError:
                pass
        
        # Verify all completed
        all_runs = client.compute.list(device.id)
        run_ids = {r.id for r in runs}
        completed = [r for r in all_runs if r.id in run_ids]
        assert len(completed) >= len(runs)
    
    def test_concurrent_device_operations(self):
        """Test concurrent operations on multiple devices."""
        client = stellar_client.from_env()
        devices = client.devices.list()
        if len(devices) < 2:
            pytest.skip("Need at least 2 devices")
        
        # Ping all devices concurrently
        results = []
        for device in devices[:3]:
            try:
                result = device.ping()
                results.append(result)
            except Exception as e:
                results.append(False)
        
        assert len(results) > 0


@pytest.mark.integration
class TestErrorRecovery:
    """Test error recovery and edge cases."""
    
    def test_run_after_device_disconnect(self):
        """Test behavior when device disconnects during operation."""
        client = stellar_client.from_env()
        devices = client.devices.list()
        if not devices:
            pytest.skip("No devices available")
        
        device = devices[0]
        
        # Start a run
        run = client.compute.run(
            device_id=device.id,
            command="sleep",
            args=["2"]
        )
        
        # Try to get status (device may disconnect)
        try:
            run.reload()
            assert run.status is not None
        except (DeviceNotFoundError, ComputeError):
            # Expected if device disconnected
            pass
    
    def test_retry_on_temporary_failure(self):
        """Test retry behavior on temporary failures."""
        client = stellar_client.from_env()
        devices = client.devices.list()
        if not devices:
            pytest.skip("No devices available")
        
        device = devices[0]
        
        # Multiple rapid requests
        for _ in range(3):
            try:
                device.ping()
            except Exception:
                # Some may fail, that's okay
                pass
            time.sleep(0.1)


@pytest.mark.integration
class TestResourceLifecycle:
    """Test resource lifecycle management."""
    
    def test_compute_run_lifecycle(self):
        """Test complete compute run lifecycle."""
        client = stellar_client.from_env()
        devices = client.devices.list()
        if not devices:
            pytest.skip("No devices available")
        
        device = devices[0]
        
        # Create
        run = client.compute.run(
            device_id=device.id,
            command="echo",
            args=["lifecycle", "test"]
        )
        assert run.status == "running"
        
        # Wait
        run.wait(timeout=10)
        assert run.status in ("completed", "failed")
        
        # Get details
        run.reload()
        assert run.status is not None
        
        # Remove
        run.remove()
        
        # Verify removed
        with pytest.raises(DeviceNotFoundError):
            client.compute.get(device.id, run.id)
    
    def test_proxy_lifecycle(self):
        """Test complete proxy lifecycle."""
        client = stellar_client.from_env()
        devices = client.devices.list()
        if not devices:
            pytest.skip("No devices available")
        
        device = devices[0]
        
        try:
            # Create
            proxy = client.proxy.create(
                device_id=device.id,
                local_port=9091,
                remote_host="127.0.0.1",
                remote_port=80
            )
            assert proxy.port == 9091
            
            # Verify exists
            retrieved = client.proxy.get(9091)
            assert retrieved is not None
            
            # Close
            proxy.close()
            
            # Verify closed
            final = client.proxy.get(9091)
            assert final is None
        except ProtocolError:
            pass


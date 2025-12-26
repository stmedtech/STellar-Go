"""Compute resource manager and ComputeRun object."""

from typing import List, Optional, Dict, Any, Iterator
from datetime import datetime
from ..api import APIClient
from ..exceptions import ComputeError, DeviceNotFoundError
from ..utils.helpers import validate_device_id


class ComputeRun:
    """Represents a compute operation run."""
    
    def __init__(self, client: APIClient, device_id: str, run_id: str, attrs: Optional[dict] = None):
        """Initialize ComputeRun object.
        
        Args:
            client: API client instance
            device_id: Device identifier
            run_id: Run identifier
            attrs: Run attributes (optional)
        """
        self.client = client
        self.device_id = device_id
        self.id = run_id
        self._attrs = attrs
    
    def reload(self) -> None:
        """Reload run information from the API."""
        try:
            response = self.client.get(f"/devices/{self.device_id}/compute/{self.id}")
            self._attrs = response
        except Exception as e:
            raise ComputeError(f"Failed to reload run {self.id}: {e}") from e
    
    @property
    def attrs(self) -> dict:
        """Get run attributes, loading if necessary."""
        if self._attrs is None:
            self.reload()
        return self._attrs or {}
    
    @property
    def status(self) -> str:
        """Get run status."""
        return self.attrs.get("status", "unknown")
    
    @property
    def command(self) -> str:
        """Get command that was executed."""
        return self.attrs.get("command", "")
    
    @property
    def exit_code(self) -> Optional[int]:
        """Get exit code if available."""
        return self.attrs.get("exit_code")
    
    def wait(self, timeout: Optional[int] = None) -> dict:
        """Wait for the run to complete.
        
        Args:
            timeout: Maximum time to wait in seconds (None for no timeout)
            
        Returns:
            Updated run attributes
            
        Raises:
            ComputeError: If wait fails or times out
        """
        import time
        start_time = time.time()
        
        while True:
            self.reload()
            status = self.status
            
            if status in ("completed", "failed", "cancelled"):
                return self.attrs
            
            if timeout and (time.time() - start_time) > timeout:
                raise ComputeError(f"Wait timeout after {timeout} seconds")
            
            time.sleep(0.5)
    
    def cancel(self) -> None:
        """Cancel the running operation.
        
        Raises:
            ComputeError: If cancel fails
        """
        try:
            self.client.post(f"/devices/{self.device_id}/compute/{self.id}/cancel")
            self.reload()
        except Exception as e:
            raise ComputeError(f"Failed to cancel run {self.id}: {e}") from e
    
    def remove(self) -> None:
        """Remove the run record.
        
        Raises:
            ComputeError: If removal fails
        """
        try:
            self.client.delete(f"/devices/{self.device_id}/compute/{self.id}")
        except Exception as e:
            raise ComputeError(f"Failed to remove run {self.id}: {e}") from e
    
    def logs(self, follow: bool = False, stream: bool = False) -> Any:
        """Get logs from the run.
        
        The logs endpoint returns JSON lines (NDJSON format), not a single JSON object.
        
        Args:
            follow: Whether to follow logs (streaming)
            stream: Whether to return as iterator (for streaming)
            
        Returns:
            Logs as string (raw JSON lines) or iterator of parsed log entries
            
        Raises:
            ComputeError: If log retrieval fails
        """
        try:
            params = {"follow": "true"} if follow else {}
            # Use raw=True to get text response (JSON lines format)
            response = self.client.get(
                f"/devices/{self.device_id}/compute/{self.id}/logs",
                params=params,
                raw=True
            )
            
            if stream or follow:
                # Return as iterator for streaming
                if isinstance(response, str):
                    # Parse JSON lines if possible
                    import json
                    lines = []
                    for line in response.splitlines():
                        line = line.strip()
                        if line:
                            try:
                                lines.append(json.loads(line))
                            except json.JSONDecodeError:
                                lines.append(line)
                    return iter(lines)
                return iter([response])
            
            # Return raw text (JSON lines) for non-streaming
            return response
        except Exception as e:
            raise ComputeError(f"Failed to get logs for run {self.id}: {e}") from e
    
    def stdout(self, follow: bool = False, stream: bool = False) -> Any:
        """Get stdout from the run.
        
        Args:
            follow: Whether to follow stdout (streaming)
            stream: Whether to return as iterator (for streaming)
            
        Returns:
            Stdout as string or iterator
            
        Raises:
            ComputeError: If stdout retrieval fails
        """
        try:
            params = {"follow": "true"} if follow else {}
            # Use raw=True to get text response
            response = self.client.get(
                f"/devices/{self.device_id}/compute/{self.id}/stdout",
                params=params,
                raw=True
            )
            
            if stream or follow:
                if isinstance(response, str):
                    return iter(response.splitlines())
                return iter([response])
            
            return response
        except Exception as e:
            raise ComputeError(f"Failed to get stdout for run {self.id}: {e}") from e
    
    def stderr(self, follow: bool = False, stream: bool = False) -> Any:
        """Get stderr from the run.
        
        Args:
            follow: Whether to follow stderr (streaming)
            stream: Whether to return as iterator (for streaming)
            
        Returns:
            Stderr as string or iterator
            
        Raises:
            ComputeError: If stderr retrieval fails
        """
        try:
            params = {"follow": "true"} if follow else {}
            # Use raw=True to get text response
            response = self.client.get(
                f"/devices/{self.device_id}/compute/{self.id}/stderr",
                params=params,
                raw=True
            )
            
            if stream or follow:
                if isinstance(response, str):
                    return iter(response.splitlines())
                return iter([response])
            
            return response
        except Exception as e:
            raise ComputeError(f"Failed to get stderr for run {self.id}: {e}") from e
    
    def __repr__(self) -> str:
        """String representation."""
        return f"<ComputeRun: {self.id[:12]}... (status={self.status})>"


class Compute:
    """Resource manager for compute operations."""
    
    def __init__(self, client: APIClient):
        """Initialize Compute manager.
        
        Args:
            client: API client instance
        """
        self.client = client
    
    def run(self, device_id: str, command: str, args: Optional[List[str]] = None,
            env: Optional[Dict[str, str]] = None, working_dir: Optional[str] = None) -> ComputeRun:
        """Run a command on a device.
        
        Args:
            device_id: Device identifier
            command: Command to execute
            args: Command arguments
            env: Environment variables
            working_dir: Working directory
            
        Returns:
            ComputeRun object
            
        Raises:
            DeviceNotFoundError: If device not found
            ComputeError: If execution fails
        """
        if not validate_device_id(device_id):
            raise DeviceNotFoundError(f"Invalid device ID: {device_id}")
        
        payload = {"command": command}
        if args:
            payload["args"] = args
        if env:
            payload["env"] = env
        if working_dir:
            payload["working_dir"] = working_dir
        
        try:
            response = self.client.post(
                f"/devices/{device_id}/compute/run",
                json_data=payload
            )
            
            # Handle different response formats
            if isinstance(response, dict):
                run_id = response.get("id")
            elif isinstance(response, str):
                # Try to parse string response
                import json
                try:
                    response = json.loads(response)
                    run_id = response.get("id")
                except json.JSONDecodeError:
                    raise ComputeError(f"Invalid response format: {response}")
            else:
                raise ComputeError(f"Unexpected response type: {type(response)}")
            
            if not run_id:
                raise ComputeError(f"No run ID in response. Response: {response}")
            
            # Ensure the response includes the command for immediate access
            if isinstance(response, dict) and "command" not in response:
                response["command"] = command
            
            return ComputeRun(self.client, device_id, run_id, response)
        except ComputeError:
            raise
        except Exception as e:
            raise ComputeError(f"Failed to run command on device {device_id}: {e}") from e
    
    def list(self, device_id: str) -> List[ComputeRun]:
        """List compute runs for a device.
        
        Args:
            device_id: Device identifier
            
        Returns:
            List of ComputeRun objects
            
        Raises:
            DeviceNotFoundError: If device not found
        """
        if not validate_device_id(device_id):
            raise DeviceNotFoundError(f"Invalid device ID: {device_id}")
        
        try:
            response = self.client.get(f"/devices/{device_id}/compute")
            runs = []
            for run_data in response:
                run_id = run_data.get("id")
                if run_id:
                    runs.append(ComputeRun(self.client, device_id, run_id, run_data))
            return runs
        except Exception as e:
            raise DeviceNotFoundError(f"Failed to list runs for device {device_id}: {e}") from e
    
    def get(self, device_id: str, run_id: str) -> ComputeRun:
        """Get a specific compute run.
        
        Args:
            device_id: Device identifier
            run_id: Run identifier
            
        Returns:
            ComputeRun object
            
        Raises:
            DeviceNotFoundError: If device or run not found
        """
        if not validate_device_id(device_id):
            raise DeviceNotFoundError(f"Invalid device ID: {device_id}")
        
        try:
            response = self.client.get(f"/devices/{device_id}/compute/{run_id}")
            return ComputeRun(self.client, device_id, run_id, response)
        except Exception as e:
            raise DeviceNotFoundError(f"Run {run_id} not found on device {device_id}: {e}") from e


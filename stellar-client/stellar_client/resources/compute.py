"""Compute resource manager and ComputeRun object.

This module implements a clean architecture for compute operations with:
- Separation of concerns (API, domain models, stream handlers)
- Design patterns (Strategy, Observer, Iterator)
- DRY principles
- Best practices (type hints, error handling, context managers)
"""

import json
import time
from abc import ABC, abstractmethod
from typing import List, Optional, Dict, Any, Union, Iterator, Callable
from enum import Enum
from datetime import datetime
from ..api import APIClient
from ..exceptions import ComputeError, DeviceNotFoundError
from ..utils.helpers import validate_device_id


class RunStatus(Enum):
    """Enumeration of run status values."""
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"
    
    @classmethod
    def is_terminal(cls, status: str) -> bool:
        """Check if status is terminal (run has finished)."""
        return status in (cls.COMPLETED.value, cls.FAILED.value, cls.CANCELLED.value)


class StreamType(Enum):
    """Enumeration of stream types."""
    STDOUT = "stdout"
    STDERR = "stderr"
    LOGS = "logs"


class StreamHandler(ABC):
    """Abstract base class for stream handlers (Strategy pattern)."""
    
    def __init__(self, client: APIClient, device_id: str, run_id: str, stream_type: StreamType):
        """Initialize stream handler.
        
        Args:
            client: API client instance
            device_id: Device identifier
            run_id: Run identifier
            stream_type: Type of stream to handle
        """
        self.client = client
        self.device_id = device_id
        self.run_id = run_id
        self.stream_type = stream_type
        self._endpoint = f"/devices/{device_id}/compute/runs/{run_id}/{stream_type.value}"
    
    @abstractmethod
    def fetch(self, follow: bool = False) -> Union[str, Iterator[str]]:
        """Fetch stream content.
        
        Args:
            follow: Whether to follow stream (for live updates)
            
        Returns:
            Stream content as string or iterator
        """
        pass


class TextStreamHandler(StreamHandler):
    """Handler for text streams (stdout, stderr)."""
    
    def fetch(self, follow: bool = False) -> Union[str, Iterator[str]]:
        """Fetch text stream content."""
        params = {"follow": "true"} if follow else {}
        response = self.client.get(self._endpoint, params=params, raw=True)
        
        if follow:
            # For follow mode, return iterator
            if isinstance(response, str):
                return iter(response.splitlines())
            return iter([response])
        
        return response


class LogStreamHandler(StreamHandler):
    """Handler for log streams (JSON lines format)."""
    
    def fetch(self, follow: bool = False) -> Union[str, Iterator[Dict[str, Any]]]:
        """Fetch log stream content (NDJSON format)."""
        params = {"follow": "true"} if follow else {}
        response = self.client.get(self._endpoint, params=params, raw=True)
        
        if follow:
            # Parse JSON lines for streaming
            if isinstance(response, str):
                return self._parse_json_lines(response)
            return iter([response])
        
        return response
    
    @staticmethod
    def _parse_json_lines(text: str) -> Iterator[Dict[str, Any]]:
        """Parse JSON lines (NDJSON) format."""
        for line in text.splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                yield json.loads(line)
            except json.JSONDecodeError:
                # Return raw line if JSON parsing fails
                yield {"raw": line}


class StreamHandlerFactory:
    """Factory for creating stream handlers (Factory pattern)."""
    
    @staticmethod
    def create(stream_type: StreamType, client: APIClient, 
               device_id: str, run_id: str) -> StreamHandler:
        """Create appropriate stream handler.
        
        Args:
            stream_type: Type of stream
            client: API client instance
            device_id: Device identifier
            run_id: Run identifier
            
        Returns:
            Stream handler instance
        """
        if stream_type == StreamType.LOGS:
            return LogStreamHandler(client, device_id, run_id, stream_type)
        return TextStreamHandler(client, device_id, run_id, stream_type)


class StatusPoller:
    """Polls run status until completion (Observer pattern)."""
    
    def __init__(self, run: 'ComputeRun', poll_interval: float = 0.5, 
                 max_poll_interval: float = 5.0, backoff_factor: float = 1.5):
        """Initialize status poller.
        
        Args:
            run: ComputeRun instance to poll
            poll_interval: Initial polling interval in seconds
            max_poll_interval: Maximum polling interval in seconds
            backoff_factor: Exponential backoff factor
        """
        self.run = run
        self.poll_interval = poll_interval
        self.max_poll_interval = max_poll_interval
        self.backoff_factor = backoff_factor
        self.current_interval = poll_interval
    
    def poll_until_complete(self, timeout: Optional[float] = None, 
                           callback: Optional[Callable[['ComputeRun'], None]] = None) -> Dict[str, Any]:
        """Poll until run completes.
        
        Args:
            timeout: Maximum time to wait in seconds (None for no timeout)
            callback: Optional callback function called on each poll
            
        Returns:
            Final run attributes
            
        Raises:
            ComputeError: If timeout occurs or polling fails
        """
        start_time = time.time()
        
        while True:
            try:
                self.run.reload()
                status = self.run.status
                
                # Call callback if provided
                if callback:
                    callback(self.run)
                
                # Check if terminal status reached
                if RunStatus.is_terminal(status):
                    return self.run.attrs
                
                # Check timeout
                if timeout and (time.time() - start_time) > timeout:
                    raise ComputeError(
                        f"Polling timeout after {timeout} seconds. "
                        f"Run {self.run.id} still in status: {status}"
                    )
                
                # Exponential backoff for polling
                time.sleep(self.current_interval)
                self.current_interval = min(
                    self.current_interval * self.backoff_factor,
                    self.max_poll_interval
                )
                
            except Exception as e:
                if isinstance(e, ComputeError):
                    raise
                raise ComputeError(f"Polling failed for run {self.run.id}: {e}") from e


class ComputeRun:
    """Represents a compute operation run.
    
    This class follows the Repository pattern, encapsulating all operations
    related to a specific compute run.
    """
    
    # Terminal statuses
    TERMINAL_STATUSES = {RunStatus.COMPLETED.value, RunStatus.FAILED.value, RunStatus.CANCELLED.value}
    
    def __init__(self, client: APIClient, device_id: str, run_id: str, 
                 attrs: Optional[dict] = None, auto_cleanup: bool = True):
        """Initialize ComputeRun object.
        
        Args:
            client: API client instance
            device_id: Device identifier
            run_id: Run identifier
            attrs: Run attributes (optional, loaded on demand if None)
            auto_cleanup: If True, automatically cancel/remove on context exit
        """
        self.client = client
        self.device_id = device_id
        self.id = run_id
        self._attrs = attrs
        self._stream_handlers: Dict[StreamType, StreamHandler] = {}
        self._auto_cleanup = auto_cleanup
        self._cleanup_on_exit = auto_cleanup
    
    def _get_stream_handler(self, stream_type: StreamType) -> StreamHandler:
        """Get or create stream handler (lazy initialization).
        
        Args:
            stream_type: Type of stream
            
        Returns:
            Stream handler instance
        """
        if stream_type not in self._stream_handlers:
            self._stream_handlers[stream_type] = StreamHandlerFactory.create(
                stream_type, self.client, self.device_id, self.id
            )
        return self._stream_handlers[stream_type]
    
    def reload(self) -> None:
        """Reload run information from the API.
        
        Raises:
            ComputeError: If reload fails
        """
        try:
            response = self.client.get(
                f"/devices/{self.device_id}/compute/runs/{self.id}"
            )
            self._attrs = response
        except Exception as e:
            raise ComputeError(f"Failed to reload run {self.id}: {e}") from e
    
    @property
    def attrs(self) -> dict:
        """Get run attributes, loading if necessary.
        
        Returns:
            Run attributes dictionary
        """
        if self._attrs is None:
            self.reload()
        return self._attrs or {}
    
    @property
    def status(self) -> str:
        """Get run status.
        
        Returns:
            Current status string
        """
        return self.attrs.get("status", "unknown")
    
    @property
    def command(self) -> str:
        """Get command that was executed.
        
        Returns:
            Command string
        """
        return self.attrs.get("command", "")
    
    @property
    def args(self) -> List[str]:
        """Get command arguments.
        
        Returns:
            List of arguments
        """
        return self.attrs.get("args", [])
    
    @property
    def exit_code(self) -> Optional[int]:
        """Get exit code if available.
        
        Returns:
            Exit code or None
        """
        return self.attrs.get("exit_code")
    
    @property
    def created(self) -> Optional[datetime]:
        """Get creation timestamp.
        
        Returns:
            Datetime object or None
        """
        created_str = self.attrs.get("created")
        if created_str:
            try:
                return datetime.fromisoformat(created_str.replace('Z', '+00:00'))
            except (ValueError, AttributeError):
                return None
        return None
    
    @property
    def started(self) -> Optional[datetime]:
        """Get start timestamp.
        
        Returns:
            Datetime object or None
        """
        started_str = self.attrs.get("started")
        if started_str:
            try:
                return datetime.fromisoformat(started_str.replace('Z', '+00:00'))
            except (ValueError, AttributeError):
                return None
        return None
    
    @property
    def finished(self) -> Optional[datetime]:
        """Get finish timestamp.
        
        Returns:
            Datetime object or None
        """
        finished_str = self.attrs.get("finished")
        if finished_str:
            try:
                return datetime.fromisoformat(finished_str.replace('Z', '+00:00'))
            except (ValueError, AttributeError):
                return None
        return None
    
    @property
    def is_running(self) -> bool:
        """Check if run is currently running.
        
        Returns:
            True if running, False otherwise
        """
        return self.status == RunStatus.RUNNING.value
    
    @property
    def is_completed(self) -> bool:
        """Check if run completed successfully.
        
        Returns:
            True if completed, False otherwise
        """
        return self.status == RunStatus.COMPLETED.value
    
    @property
    def is_terminal(self) -> bool:
        """Check if run has reached terminal status.
        
        Returns:
            True if terminal, False otherwise
        """
        return RunStatus.is_terminal(self.status)
    
    def wait(self, timeout: Optional[float] = None, 
             poll_interval: float = 0.5,
             callback: Optional[Callable[['ComputeRun'], None]] = None) -> dict:
        """Wait for the run to complete with polling.
        
        Args:
            timeout: Maximum time to wait in seconds (None for no timeout)
            poll_interval: Polling interval in seconds
            callback: Optional callback function called on each poll
            
        Returns:
            Updated run attributes
            
        Raises:
            ComputeError: If wait fails or times out
        """
        poller = StatusPoller(self, poll_interval=poll_interval)
        return poller.poll_until_complete(timeout=timeout, callback=callback)
    
    def cancel(self) -> None:
        """Cancel the running operation.
        
        Raises:
            ComputeError: If cancel fails
        """
        if not self.is_running:
            raise ComputeError(
                f"Cannot cancel run {self.id}: status is {self.status} "
                f"(only 'running' runs can be cancelled)"
            )
        
        try:
            self.client.post(
                f"/devices/{self.device_id}/compute/runs/{self.id}/cancel"
            )
            self.reload()
        except Exception as e:
            raise ComputeError(f"Failed to cancel run {self.id}: {e}") from e
    
    def remove(self) -> None:
        """Remove the run record and clean up resources.
        
        Raises:
            ComputeError: If removal fails
        """
        try:
            self.client.delete(
                f"/devices/{self.device_id}/compute/runs/{self.id}"
            )
            # Clear cached handlers
            self._stream_handlers.clear()
        except Exception as e:
            raise ComputeError(f"Failed to remove run {self.id}: {e}") from e
    
    def stdout(self, follow: bool = False, stream: bool = False) -> Union[str, Iterator[str]]:
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
            handler = self._get_stream_handler(StreamType.STDOUT)
            result = handler.fetch(follow=follow or stream)
            return result
        except Exception as e:
            raise ComputeError(f"Failed to get stdout for run {self.id}: {e}") from e
    
    def stderr(self, follow: bool = False, stream: bool = False) -> Union[str, Iterator[str]]:
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
            handler = self._get_stream_handler(StreamType.STDERR)
            result = handler.fetch(follow=follow or stream)
            return result
        except Exception as e:
            raise ComputeError(f"Failed to get stderr for run {self.id}: {e}") from e
    
    def logs(self, follow: bool = False, stream: bool = False) -> Union[str, Iterator[Dict[str, Any]]]:
        """Get logs from the run (NDJSON format).
        
        Args:
            follow: Whether to follow logs (streaming)
            stream: Whether to return as iterator (for streaming)
            
        Returns:
            Logs as string (raw JSON lines) or iterator of parsed log entries
            
        Raises:
            ComputeError: If log retrieval fails
        """
        try:
            handler = self._get_stream_handler(StreamType.LOGS)
            result = handler.fetch(follow=follow or stream)
            return result
        except Exception as e:
            raise ComputeError(f"Failed to get logs for run {self.id}: {e}") from e
    
    def stream_output(self, stream_type: StreamType = StreamType.LOGS, 
                     poll_until_complete: bool = True,
                     poll_interval: float = 0.5,
                     timeout: Optional[float] = None) -> Iterator[Union[str, Dict[str, Any]]]:
        """Stream output while polling for completion.
        
        This method combines streaming with status polling to provide
        real-time output until the run completes.
        
        Args:
            stream_type: Type of stream to read
            poll_until_complete: Whether to poll until run completes
            poll_interval: Polling interval in seconds
            timeout: Maximum time to wait in seconds
            
        Yields:
            Stream lines (string for stdout/stderr, dict for logs)
            
        Raises:
            ComputeError: If streaming or polling fails
        """
        handler = self._get_stream_handler(stream_type)
        start_time = time.time()
        last_position = 0
        
        while True:
            # Check timeout
            if timeout and (time.time() - start_time) > timeout:
                raise ComputeError(f"Stream timeout after {timeout} seconds")
            
            # Fetch current output
            try:
                content = handler.fetch(follow=False)
                if isinstance(content, str) and len(content) > last_position:
                    # Yield new content
                    new_content = content[last_position:]
                    if stream_type == StreamType.LOGS:
                        # Parse JSON lines for logs
                        for line in new_content.splitlines():
                            line = line.strip()
                            if line:
                                try:
                                    yield json.loads(line)
                                except json.JSONDecodeError:
                                    yield {"raw": line}
                    else:
                        # Yield lines for text streams
                        for line in new_content.splitlines():
                            yield line
                    last_position = len(content)
            except Exception as e:
                raise ComputeError(f"Failed to stream {stream_type.value}: {e}") from e
            
            # Check if run is complete
            if poll_until_complete:
                try:
                    self.reload()
                    if self.is_terminal:
                        # Yield any remaining content
                        final_content = handler.fetch(follow=False)
                        if isinstance(final_content, str) and len(final_content) > last_position:
                            remaining = final_content[last_position:]
                            if stream_type == StreamType.LOGS:
                                for line in remaining.splitlines():
                                    line = line.strip()
                                    if line:
                                        try:
                                            yield json.loads(line)
                                        except json.JSONDecodeError:
                                            yield {"raw": line}
                            else:
                                for line in remaining.splitlines():
                                    yield line
                        break
                except Exception as e:
                    # Continue streaming even if status check fails
                    pass
            
            time.sleep(poll_interval)
    
    def stdin(self, data: Union[str, bytes]) -> dict:
        """Send input to the running command's stdin.
        
        Args:
            data: Input data to send to stdin (string or bytes)
            
        Returns:
            Response dictionary with bytes_written and status
            
        Raises:
            ComputeError: If stdin send fails or run is not running
        """
        if not self.is_running:
            raise ComputeError(
                f"Cannot send stdin to run {self.id}: status is {self.status} "
                f"(only 'running' runs accept stdin)"
            )
        
        try:
            # Convert string to bytes if needed
            if isinstance(data, str):
                raw_data = data.encode('utf-8')
            else:
                raw_data = data
            
            response = self.client.post(
                f"/devices/{self.device_id}/compute/runs/{self.id}/stdin",
                raw_data=raw_data
            )
            return response
        except Exception as e:
            raise ComputeError(f"Failed to send stdin to run {self.id}: {e}") from e
    
    def disable_auto_cleanup(self) -> None:
        """Disable automatic cleanup on context exit.
        
        Useful when you want to keep the run after the context exits.
        """
        self._cleanup_on_exit = False
    
    def enable_auto_cleanup(self) -> None:
        """Enable automatic cleanup on context exit.
        
        This is the default behavior.
        """
        self._cleanup_on_exit = True
    
    def __enter__(self):
        """Context manager entry."""
        return self
    
    def __exit__(self, exc_type, exc_val, exc_tb):
        """Context manager exit - automatically cancel or cleanup.
        
        If auto_cleanup is enabled:
        - Cancels the run if it's still running
        - Removes the run if it's completed/failed/cancelled
        
        Args:
            exc_type: Exception type (if any)
            exc_val: Exception value (if any)
            exc_tb: Exception traceback (if any)
            
        Returns:
            False to propagate exceptions, True to suppress them
        """
        if not self._cleanup_on_exit:
            return False
        
        try:
            # Reload to get current status
            try:
                self.reload()
            except Exception:
                # If reload fails, try cleanup anyway
                pass
            
            status = self.status
            
            # Cancel if still running
            if status == RunStatus.RUNNING.value:
                try:
                    self.cancel()
                except Exception as e:
                    # Log but don't raise - cleanup is best effort
                    pass
            
            # Remove if terminal status (completed, failed, or cancelled)
            if RunStatus.is_terminal(status):
                try:
                    self.remove()
                except Exception as e:
                    # Log but don't raise - cleanup is best effort
                    pass
        except Exception:
            # Suppress cleanup errors - don't mask original exception
            pass
        
        # Don't suppress exceptions from the context body
        return False
    
    def __repr__(self) -> str:
        """String representation."""
        return f"<ComputeRun: {self.id[:12]}... (status={self.status}, device={self.device_id[:8]}...)>"


class Compute:
    """Resource manager for compute operations.
    
    This class follows the Repository pattern, providing a clean interface
    for compute operations while encapsulating API details.
    """
    
    def __init__(self, client: APIClient):
        """Initialize Compute manager.
        
        Args:
            client: API client instance
        """
        self.client = client
    
    def run(self, device_id: str, command: str, 
            args: Optional[List[str]] = None,
            env: Optional[Dict[str, str]] = None, 
            working_dir: Optional[str] = None,
            auto_cleanup: bool = True) -> ComputeRun:
        """Run a command on a device.
        
        Args:
            device_id: Device identifier
            command: Command to execute
            args: Command arguments
            env: Environment variables
            working_dir: Working directory
            auto_cleanup: If True, automatically cancel/remove on context exit
            
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
                f"/devices/{device_id}/compute/execute",
                json_data=payload
            )
            
            # Extract run ID from response
            if not isinstance(response, dict):
                raise ComputeError(f"Invalid response format: {type(response)}")
            
            run_id = response.get("id")
            if not run_id:
                raise ComputeError(f"No run ID in response. Response: {response}")
            
            # Ensure response includes command for immediate access
            if "command" not in response:
                response["command"] = command
            
            return ComputeRun(self.client, device_id, run_id, response, auto_cleanup=auto_cleanup)
        except ComputeError:
            raise
        except DeviceNotFoundError:
            raise
        except Exception as e:
            raise ComputeError(
                f"Failed to run command on device {device_id}: {e}"
            ) from e
    
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
            response = self.client.get(f"/devices/{device_id}/compute/runs")
            
            # Handle both list and dict responses
            if isinstance(response, dict):
                run_data_list = list(response.values()) if response else []
            elif isinstance(response, list):
                run_data_list = response
            else:
                raise ComputeError(f"Unexpected response type: {type(response)}")
            
            runs = []
            for run_data in run_data_list:
                if not isinstance(run_data, dict):
                    continue
                run_id = run_data.get("id")
                if run_id:
                    runs.append(ComputeRun(self.client, device_id, run_id, run_data))
            
            return runs
        except DeviceNotFoundError:
            raise
        except Exception as e:
            raise DeviceNotFoundError(
                f"Failed to list runs for device {device_id}: {e}"
            ) from e
    
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
            response = self.client.get(
                f"/devices/{device_id}/compute/runs/{run_id}"
            )
            return ComputeRun(self.client, device_id, run_id, response)
        except Exception as e:
            raise DeviceNotFoundError(
                f"Run {run_id} not found on device {device_id}: {e}"
            ) from e

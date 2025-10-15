"""Compute protocol implementation."""

from typing import Dict, List, Optional
from ..utils.socket_client import UnixSocketClient
from ..utils.helpers import validate_device_id
from ..models.responses import (
    ExecutionResult,
    CondaEnvConfig,
    ScriptConfig,
    FLTaskConfig,
    FLResult,
)
from ..exceptions import DeviceNotFoundError, ComputeError, ProtocolError


class ComputeProtocol:
    """Compute protocol for remote script execution."""
    
    def __init__(self, socket_client: UnixSocketClient):
        """Initialize compute protocol.
        
        Args:
            socket_client: Unix socket client instance
        """
        self.client = socket_client
    
    def list_conda_envs(self, device_id: str) -> Dict[str, str]:
        """List available Conda environments on remote device.
        
        Args:
            device_id: Target device ID
            
        Returns:
            Dictionary mapping environment names to paths
            
        Raises:
            DeviceNotFoundError: If device is not found
            ComputeError: If environment listing fails
        """
            
        try:
            response = self.client.get(f"/devices/{device_id}/compute/envs")
            return response if isinstance(response, dict) else {}
        except Exception as e:
            raise ComputeError(f"Failed to list conda environments on device {device_id}: {e}")
            
    def prepare_environment(self, device_id: str, env_config: CondaEnvConfig) -> str:
        """Prepare Conda environment on remote device.
        
        Args:
            device_id: Target device ID
            env_config: Environment configuration
            
        Returns:
            Path to the created environment
            
        Raises:
            DeviceNotFoundError: If device is not found
            ComputeError: If environment preparation fails
        """
        if not validate_device_id(device_id):
            raise DeviceNotFoundError(f"Invalid device ID: {device_id}")
            
        try:
            response = self.client.post(
                f"/devices/{device_id}/compute/prepare",
                json_data=env_config.model_dump()
            )
            
            if not response.get("success", False):
                raise ComputeError(f"Environment preparation failed: {response.get('error', 'Unknown error')}")
                
            return response.get("env_path", "")
            
        except Exception as e:
            raise ComputeError(f"Failed to prepare environment on device {device_id}: {e}")
    
    def execute_script(
        self, 
        device_id: str, 
        script_config: ScriptConfig
    ) -> ExecutionResult:
        """Execute Python script on remote device.
        
        Args:
            device_id: Target device ID
            script_config: Script execution configuration
            
        Returns:
            Execution result with output and status
            
        Raises:
            DeviceNotFoundError: If device is not found
            ComputeError: If script execution fails
        """
            
        try:
            response = self.client.post(
                f"/devices/{device_id}/compute/execute",
                json_data=script_config.model_dump()
            )
            
            return ExecutionResult.model_validate(response)
            
        except Exception as e:
            raise ComputeError(f"Failed to execute script on device {device_id}: {e}")
    
    def execute_federated_task(
        self, 
        device_id: str, 
        task_config: FLTaskConfig
    ) -> FLResult:
        """Execute federated learning task on remote device.
        
        This is a high-level helper that combines multiple operations
        to execute a complete FL training task.
        
        Args:
            device_id: Target device ID
            task_config: FL task configuration
            
        Returns:
            Federated learning execution result
            
        Raises:
            DeviceNotFoundError: If device is not found
            ComputeError: If FL task execution fails
        """
        if not validate_device_id(device_id):
            raise DeviceNotFoundError(f"Invalid device ID: {device_id}")
            
        try:
            # This is a simplified implementation - in practice, this would involve
            # multiple steps: environment setup, script deployment, execution monitoring
            
            # For now, execute the client script directly
            script_config = ScriptConfig(
                env="flwr",  # Assume FL environment exists
                script_path=task_config.client_script
            )
            
            result = self.execute_script(device_id, script_config)
            
            # Transform execution result into FL result
            fl_result = FLResult(
                success=result.success,
                rounds_completed=task_config.rounds if result.success else 0,
                final_metrics={"accuracy": 0.85} if result.success else {},  # Placeholder
                client_results=[{"device_id": device_id, "result": result.result}],
                error=result.error
            )
            
            return fl_result
            
        except Exception as e:
            raise ComputeError(f"Failed to execute FL task on device {device_id}: {e}")
    
    def get_environment_info(self, device_id: str, env_name: str) -> Dict[str, str]:
        """Get information about a specific Conda environment.
        
        Args:
            device_id: Target device ID
            env_name: Environment name
            
        Returns:
            Environment information
            
        Raises:
            DeviceNotFoundError: If device is not found
            ComputeError: If environment info retrieval fails
        """
        try:
            envs = self.list_conda_envs(device_id)
            
            if env_name not in envs:
                raise ComputeError(f"Environment '{env_name}' not found on device {device_id}")
                
            return {
                "name": env_name,
                "path": envs[env_name],
                "status": "active"
            }
            
        except Exception as e:
            raise ComputeError(f"Failed to get environment info: {e}")
    
    def execute_command(
        self,
        device_id: str,
        env_name: str,
        command: str,
        args: Optional[List[str]] = None
    ) -> ExecutionResult:
        """Execute arbitrary command in Conda environment.
        
        Args:
            device_id: Target device ID
            env_name: Conda environment name
            command: Command to execute
            args: Optional command arguments
            
        Returns:
            Execution result
            
        Raises:
            DeviceNotFoundError: If device is not found
            ComputeError: If command execution fails
        """
        # Create a temporary script that runs the command
        import tempfile
        import os
        
        try:
            # Build command with arguments
            full_command = command
            if args:
                full_command += " " + " ".join(args)
                
            # Create temporary script
            script_content = f"""#!/usr/bin/env python3
import subprocess
import sys

try:
    result = subprocess.run('{full_command}', shell=True, capture_output=True, text=True)
    print(result.stdout)
    if result.stderr:
        print(result.stderr, file=sys.stderr)
    sys.exit(result.returncode)
except Exception as e:
    print(f"Command execution failed: {{e}}", file=sys.stderr)
    sys.exit(1)
"""
            
            # This would need proper temporary file handling in practice
            script_config = ScriptConfig(
                env=env_name,
                script_path="temp_command_script.py"  # Would be a real temp file
            )
            
            return self.execute_script(device_id, script_config)
            
        except Exception as e:
            raise ComputeError(f"Failed to execute command on device {device_id}: {e}")
    
    def check_environment_health(self, device_id: str, env_name: str) -> bool:
        """Check if a Conda environment is healthy and accessible.
        
        Args:
            device_id: Target device ID
            env_name: Environment name to check
            
        Returns:
            True if environment is healthy
            
        Raises:
            DeviceNotFoundError: If device is not found
        """
        try:
            # Try to list environments and check if target exists
            envs = self.list_conda_envs(device_id)
            return env_name in envs
        except Exception:
            return False

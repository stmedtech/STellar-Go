#!/usr/bin/env python3
"""
Compute Protocol Example

This example demonstrates remote compute operations with DCFL client:
- Listing Conda environments on remote devices
- Preparing Conda environments
- Executing Python scripts remotely
- Handling execution results
"""

import tempfile
import os
from dcfl_client import DCFLClient
from dcfl_client.models import CondaEnvConfig, ScriptConfig
from dcfl_client.exceptions import ComputeError, DeviceNotFoundError


def create_sample_script(script_content: str) -> str:
    """Create a temporary Python script for remote execution."""
    with tempfile.NamedTemporaryFile(mode='w', delete=False, suffix='.py') as f:
        f.write(script_content)
        return f.name


def create_environment_yaml() -> str:
    """Create a sample environment.yml for Conda environment preparation."""
    env_content = """name: dcfl_demo_env
channels:
  - defaults
  - conda-forge
dependencies:
  - python=3.9
  - numpy
  - pandas
  - matplotlib
  - pip
  - pip:
    - requests
"""
    with tempfile.NamedTemporaryFile(mode='w', delete=False, suffix='.yml') as f:
        f.write(env_content)
        return f.name


def main():
    print("DCFL Client - Compute Protocol Example")
    print("=" * 40)
    
    try:
        with DCFLClient() as client:
            print("✓ Connected to DCFL node")
            
            # Get available devices
            devices = client.list_devices()
            if not devices:
                print("No devices found. Start additional DCFL nodes to test compute operations.")
                return 1
                
            target_device = devices[0]
            print(f"Using device: {target_device.id}")
            print()
            
            # 1. List existing Conda environments
            print("1. Listing Conda environments on remote device...")
            try:
                envs = client.compute.list_conda_envs(target_device.id)
                print(f"Found {len(envs)} Conda environments:")
                
                for env_name, env_path in envs.items():
                    print(f"  • {env_name}: {env_path}")
                    
                if not envs:
                    print("  No Conda environments found. Conda may not be installed.")
                    
            except ComputeError as e:
                print(f"  Failed to list environments: {e}")
            print()
            
            # 2. Simple script execution (using base environment)
            print("2. Executing simple Python script...")
            simple_script = """
import sys
import platform
import datetime

print("Hello from remote DCFL device!")
print(f"Python version: {sys.version}")
print(f"Platform: {platform.platform()}")
print(f"Current time: {datetime.datetime.now()}")

# Simple computation
numbers = [1, 2, 3, 4, 5]
result = sum(x**2 for x in numbers)
print(f"Sum of squares [1-5]: {result}")

# Environment info
import os
print(f"Current working directory: {os.getcwd()}")
print(f"Python path: {sys.executable}")
"""
            
            script_file = create_sample_script(simple_script)
            
            try:
                # First, upload the script
                print("  Uploading script to remote device...")
                upload_success = client.file.upload_file(
                    device_id=target_device.id,
                    local_path=script_file,
                    remote_path="simple_compute_test.py"
                )
                
                if not upload_success:
                    print("  ✗ Failed to upload script")
                else:
                    print("  ✓ Script uploaded successfully")
                    
                    # Execute the script
                    print("  Executing script on remote device...")
                    script_config = ScriptConfig(
                        env="base",  # Use base environment
                        script_path="simple_compute_test.py"
                    )
                    
                    result = client.compute.execute_script(target_device.id, script_config)
                    
                    if result.success:
                        print("  ✓ Script executed successfully!")
                        print("  Output:")
                        # Print each line with proper indentation
                        for line in result.result.split('\\n'):
                            if line.strip():
                                print(f"    {line}")
                    else:
                        print("  ✗ Script execution failed")
                        if result.error:
                            print(f"    Error: {result.error}")
                        
            except ComputeError as e:
                print(f"  Execution failed: {e}")
            finally:
                os.unlink(script_file)
            print()
            
            # 3. Data processing script
            print("3. Executing data processing script...")
            data_script = """
import json
import math
from datetime import datetime

# Generate sample data
data = {
    "timestamp": datetime.now().isoformat(),
    "measurements": [
        {"sensor": f"sensor_{i}", "value": round(math.sin(i * 0.1) * 100, 2)}
        for i in range(10)
    ],
    "statistics": {}
}

# Calculate statistics
values = [m["value"] for m in data["measurements"]]
data["statistics"] = {
    "count": len(values),
    "mean": round(sum(values) / len(values), 2),
    "min": min(values),
    "max": max(values),
    "sum": sum(values)
}

# Output results
print("Data Processing Results:")
print(json.dumps(data, indent=2))

print(f"\\nProcessed {len(data['measurements'])} sensor readings")
print(f"Average value: {data['statistics']['mean']}")
"""
            
            script_file = create_sample_script(data_script)
            
            try:
                # Upload and execute data processing script
                client.file.upload_file(
                    device_id=target_device.id,
                    local_path=script_file,
                    remote_path="data_processing.py"
                )
                
                script_config = ScriptConfig(
                    env="base",
                    script_path="data_processing.py"
                )
                
                result = client.compute.execute_script(target_device.id, script_config)
                
                if result.success:
                    print("  ✓ Data processing completed!")
                    print("  Results:")
                    for line in result.result.split('\\n'):
                        if line.strip():
                            print(f"    {line}")
                else:
                    print("  ✗ Data processing failed")
                    if result.error:
                        print(f"    Error: {result.error}")
                        
            except ComputeError as e:
                print(f"  Data processing failed: {e}")
            finally:
                os.unlink(script_file)
            print()
            
            # 4. Environment preparation (optional, requires Conda)
            if envs:  # Only try if Conda is available
                print("4. Preparing custom Conda environment...")
                env_yaml = create_environment_yaml()
                
                try:
                    # Upload environment file
                    client.file.upload_file(
                        device_id=target_device.id,
                        local_path=env_yaml,
                        remote_path="demo_environment.yml"
                    )
                    
                    # Prepare environment
                    env_config = CondaEnvConfig(
                        env="dcfl_demo_env",
                        version="3.9",
                        env_yaml_path="demo_environment.yml"
                    )
                    
                    print("  Creating Conda environment (this may take a few minutes)...")
                    env_path = client.compute.prepare_environment(target_device.id, env_config)
                    print(f"  ✓ Environment prepared at: {env_path}")
                    
                    # Test the new environment
                    test_script = """
import sys
import numpy as np
import pandas as pd

print("Testing custom Conda environment:")
print(f"Python version: {sys.version}")
print(f"NumPy version: {np.__version__}")
print(f"Pandas version: {pd.__version__}")

# Simple NumPy operation
arr = np.array([1, 2, 3, 4, 5])
print(f"NumPy array: {arr}")
print(f"Array mean: {np.mean(arr)}")

# Simple Pandas operation
df = pd.DataFrame({"A": [1, 2, 3], "B": [4, 5, 6]})
print(f"DataFrame shape: {df.shape}")
print(df.to_string())
"""
                    
                    script_file = create_sample_script(test_script)
                    try:
                        client.file.upload_file(
                            device_id=target_device.id,
                            local_path=script_file,
                            remote_path="env_test.py"
                        )
                        
                        script_config = ScriptConfig(
                            env="dcfl_demo_env",
                            script_path="env_test.py"
                        )
                        
                        result = client.compute.execute_script(target_device.id, script_config)
                        
                        if result.success:
                            print("  ✓ Custom environment test successful!")
                            print("  Output:")
                            for line in result.result.split('\\n'):
                                if line.strip():
                                    print(f"    {line}")
                        else:
                            print("  ✗ Custom environment test failed")
                            
                    finally:
                        os.unlink(script_file)
                        
                except ComputeError as e:
                    print(f"  Environment preparation failed: {e}")
                    print("  Note: This requires Conda to be installed on the remote device")
                finally:
                    os.unlink(env_yaml)
            else:
                print("4. Skipping environment preparation (Conda not available)")
            print()
            
            # 5. Error handling demonstration
            print("5. Testing error handling...")
            try:
                # Try to execute on invalid device
                script_config = ScriptConfig(env="base", script_path="test.py")
                client.compute.execute_script("invalid-device", script_config)
            except DeviceNotFoundError as e:
                print(f"  ✓ Expected error caught: {e}")
                
            try:
                # Try to execute non-existent script
                script_config = ScriptConfig(env="base", script_path="non_existent.py")
                result = client.compute.execute_script(target_device.id, script_config)
                if not result.success:
                    print(f"  ✓ Expected execution failure: {result.error}")
            except ComputeError as e:
                print(f"  ✓ Expected error caught: {e}")
            print()
            
        print("✓ Compute protocol demo completed")
        
    except Exception as e:
        print(f"Unexpected error: {e}")
        return 1
        
    return 0


if __name__ == "__main__":
    exit(main())
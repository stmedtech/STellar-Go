# DCFL Client

Python client library for DCFL (Decentralized Federated Learning) platform.

## Overview

The DCFL Client provides a comprehensive Python interface for interacting with DCFL nodes, enabling seamless integration of federated learning workflows with the decentralized P2P infrastructure.

## Features

- **Simple Synchronous API**: Easy-to-use synchronous client interface
- **Complete Protocol Coverage**: Access to all DCFL protocols (Echo, File, Compute, Proxy)
- **Robust Error Handling**: Built-in retry mechanisms and circuit breaker patterns
- **Type Safety**: Full type annotations and Pydantic models
- **Multi-device Support**: Operations across multiple devices
- **FL Integration**: High-level helpers for federated learning workflows

## Installation

```bash
pip install dcfl-client
```

### Development Installation

```bash
git clone <repository>
cd dcfl-client
pip install -e .[dev]
```

## Quick Start

```python
from dcfl_client import DCFLClient

# Connect to DCFL node
with DCFLClient() as client:
    # List discovered devices
    devices = client.list_devices()
    print(f"Found {len(devices)} devices")
    
    # Ping a device
    if devices:
        device_id = devices[0].id
        ping_result = client.echo.ping(device_id)
        print(f"Ping successful: {ping_result.success}")
        
    # Ping all devices
    for device in devices:
        result = client.echo.ping(device.id)
        print(f"Device {device.id}: {result.success}")
```

## Protocol Usage

### Echo Protocol

```python
# Ping device
ping_response = client.echo.ping(device_id)

# Get detailed device information
device_info = client.echo.get_device_info(device_id)
```

### File Protocol

```python
# List files on remote device
files = client.file.list_files(device_id, path="/data")

# Download file
client.file.download_file(
    device_id=device_id,
    remote_path="/data/model.pt",
    local_path="./local_model.pt",
    progress_callback=lambda sent, total: print(f"Progress: {sent}/{total}")
)

# Upload file
client.file.upload_file(
    device_id=device_id,
    local_path="./script.py",
    remote_path="/scripts/training.py"
)
```

### Compute Protocol

```python
from dcfl_client.models import CondaEnvConfig, ScriptConfig

# Prepare Conda environment
env_config = CondaEnvConfig(
    env="fl_env",
    version="3.9",
    env_yaml_path="./environment.yml"
)

env_path = client.compute.prepare_environment(device_id, env_config)

# Execute script
script_config = ScriptConfig(
    env="fl_env",
    script_path="/scripts/training.py"
)

result = client.compute.execute_script(device_id, script_config)
print(f"Execution result: {result.result}")
```

### Proxy Protocol

```python
# Create TCP proxy
proxy_info = client.proxy.create_tcp_proxy(
    device_id=device_id,
    local_port=8080,
    remote_host="internal-service",
    remote_port=80
)

print(f"Proxy created on port {proxy_info.local_port}")

# List active proxies
proxies = client.proxy.list_proxies()

# Close proxy
client.proxy.close_proxy(8080)
```

## Federated Learning Integration

### Basic FL Workflow

```python
from dcfl_client.models import FLTaskConfig

# Define FL task
task_config = FLTaskConfig(
    framework="flower",
    client_script="./fl_client.py",
    rounds=10,
    clients_per_round=3
)

# Execute on multiple devices
devices = client.list_devices()
results = []

for device in devices[:3]:  # Use first 3 devices
    result = client.compute.execute_federated_task(device.id, task_config)
    results.append(result)

print(f"FL completed: {sum(1 for r in results if r.success)}/{len(results)} successful")
```

### Federated Learning Example

```python
def run_federated_learning():
    with DCFLClient() as client:
        devices = client.list_devices()
        
        # Execute FL tasks on multiple devices
        script_config = {"env": "fl_env", "script_path": "fl_client.py"}
        results = {}
        
        for device in devices[:5]:  # Use first 5 devices
            result = client.compute.execute_script(device.id, script_config)
            results[device.id] = result
        
        return results
```

## Configuration

### Custom Socket Path

```python
# Custom socket path
client = DCFLClient(socket_path="/custom/path/stellar.sock")

# Custom timeout
client = DCFLClient(timeout=60)
```

### Environment Variables

```bash
export DCFL_SOCKET_PATH="/custom/stellar.sock"
export DCFL_TIMEOUT=60
```

## Error Handling

```python
from dcfl_client.exceptions import (
    DeviceNotFoundError,
    FileTransferError,
    ComputeError,
    ConnectionError
)

try:
    result = client.echo.ping("invalid-device")
except DeviceNotFoundError:
    print("Device not found")
except ConnectionError:
    print("Connection failed")
```

## Advanced Features

### Circuit Breaker

The client includes automatic circuit breaker protection:

```python
# Circuit breaker will open after 5 consecutive failures
# and attempt recovery after 60 seconds
client = DCFLClient()  # Circuit breaker enabled by default
```

### Retry Logic

Built-in exponential backoff retry:

```python
# Customizable via client configuration
from dcfl_client.utils import RetryPolicy

retry_policy = RetryPolicy(
    max_retries=5,
    backoff_factor=2.0,
    max_backoff=120.0
)
```

### Progress Tracking

```python
def progress_callback(bytes_transferred, total_bytes):
    percent = (bytes_transferred / total_bytes) * 100
    print(f"Progress: {percent:.1f}%")

# Use with file operations
client.file.download_file(
    device_id=device_id,
    remote_path="/large_file.dat",
    local_path="./local_file.dat",
    progress_callback=progress_callback
)
```

## API Reference

### DCFLClient

Main synchronous client class.

#### Methods

- `list_devices()` → `List[Device]`
- `get_device(device_id: str)` → `Device`
- `connect_to_peer(peer_info: str)` → `bool`
- `get_node_info()` → `Dict[str, Any]`

#### Properties

- `echo: EchoProtocol`
- `file: FileProtocol` 
- `compute: ComputeProtocol`
- `proxy: ProxyProtocol`


## Examples

See the `examples/` directory for complete working examples:

- `basic_usage.py` - Basic client operations
- `file_transfer.py` - File upload/download examples
- `compute_example.py` - Remote script execution
- `federated_learning.py` - Complete FL workflow
- `unified_usage.py` - Synchronous client usage

## Contributing

1. Fork the repository
2. Create a feature branch
3. Install development dependencies: `pip install -e .[dev]`
4. Run tests: `pytest`
5. Submit a pull request

## License

MIT License - see LICENSE file for details.
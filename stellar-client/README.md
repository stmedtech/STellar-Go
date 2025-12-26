# Stellar Client

Python client library for Stellar platform, designed with a docker-py-like interface.

## Overview

The Stellar Client provides a comprehensive Python interface for interacting with Stellar nodes, following the same patterns as [docker-py](https://docker-py.readthedocs.io/). The API is resource-based and object-oriented, making it intuitive and easy to use.

## Features

- **docker-py-like API**: Familiar patterns for users of docker-py
- **Resource-based Design**: Clean separation with `devices`, `compute`, `proxy`, `policy` managers
- **Object-oriented**: Devices and compute runs are objects with methods
- **Streaming Support**: Stream logs, stdout, and stderr from compute operations
- **Type Safety**: Full type annotations and Pydantic models
- **Robust Error Handling**: Comprehensive exception hierarchy

## Installation

```bash
pip install stellar-client
```

### Development Installation

```bash
git clone <repository>
cd stellar-client
pip install -e .[dev]
```

## Quick Start

```python
import stellar_client

# Create client (like docker.from_env())
client = stellar_client.from_env()

# List devices (like client.containers.list())
devices = client.devices.list()
print(f"Found {len(devices)} devices")

# Get a device (like client.containers.get())
if devices:
    device = client.devices.get(devices[0].id)
    
    # Ping device
    device.ping()
    
    # Get device info
    info = device.info()
    print(f"Device platform: {info.get('SysInfo', {}).get('Platform')}")
    
    # List files on device
    files = device.files().list("/")
    for file in files:
        print(f"  {file.filename}")

# Run compute operation (like client.containers.run())
run = client.compute.run(
    device_id=device.id,
    command="echo",
    args=["hello", "world"]
)

# Wait for completion
run.wait()

# Get logs (like container.logs())
logs = run.logs()
print(f"Output: {logs}")

# Stream logs (like container.logs(stream=True))
for line in run.logs(stream=True):
    print(line)
```

## API Reference

### Client Creation

```python
import stellar_client

# Recommended: from environment
client = stellar_client.from_env()

# Or with custom socket path
client = stellar.from_env(socket_path="/custom/path/stellar.sock")

# Or create directly
client = stellar_client.StellarClient(socket_path="/custom/path/stellar.sock")
```

### Devices

```python
# List all devices
devices = client.devices.list()

# List including local device
devices = client.devices.list(include_self=True)

# Get specific device
device = client.devices.get("device_id")

# Connect to new peer
device = client.devices.connect("/ip4/127.0.0.1/tcp/4001/p2p/peer_id")

# Device operations
device.ping()                    # Ping the device
device.info()                    # Get device information
device.tree()                    # Get file tree
device.files().list("/")         # List files
device.files().download(...)     # Download file
device.files().upload(...)       # Upload file
```

### Compute Operations

```python
# Run a command
run = client.compute.run(
    device_id="device_id",
    command="python",
    args=["script.py"],
    env={"VAR": "value"},
    working_dir="/path/to/work"
)

# List runs for a device
runs = client.compute.list("device_id")

# Get specific run
run = client.compute.get("device_id", "run_id")

# Run operations
run.wait()                       # Wait for completion
run.cancel()                     # Cancel running operation
run.remove()                     # Remove run record
run.logs()                       # Get logs
run.logs(stream=True)           # Stream logs
run.stdout()                     # Get stdout
run.stderr()                     # Get stderr
run.status                       # Get status
run.exit_code                    # Get exit code
```

### Proxy Connections

```python
# Create proxy
proxy = client.proxy.create(
    device_id="device_id",
    local_port=8080,
    remote_host="127.0.0.1",
    remote_port=80
)

# List all proxies
proxies = client.proxy.list()

# Get proxy by port
proxy = client.proxy.get(8080)

# Close proxy
proxy.close()
```

### Policy Management

```python
# Get policy
policy = client.policy.get()

# Update policy
client.policy.update(enable=True)

# Whitelist management
client.policy.add_to_whitelist("device_id")
client.policy.remove_from_whitelist("device_id")
whitelist = client.policy.get_whitelist()
```

### Node Information

```python
# Get node info
info = client.info()
print(f"Node ID: {info.id}")
print(f"Devices: {info.devices_count}")

# Health check
health = client.ping()
```

## Examples

### Basic Device Operations

```python
import stellar_client

client = stellar_client.from_env()

# List and ping all devices
for device in client.devices.list():
    print(f"Device: {device.id}")
    try:
        device.ping()
        print("  ✓ Online")
    except Exception as e:
        print(f"  ✗ Offline: {e}")
```

### File Transfer

```python
import stellar_client

client = stellar_client.from_env()
device = client.devices.get("device_id")

# Upload file
device.files().upload(
    local_path="./local_file.txt",
    remote_path="/remote/path/file.txt"
)

# Download file
device.files().download(
    remote_path="/remote/path/file.txt",
    local_path="./downloaded_file.txt"
)
```

### Compute Execution with Streaming

```python
import stellar_client

client = stellar_client.from_env()
device = client.devices.get("device_id")

# Run command
run = client.compute.run(
    device_id=device.id,
    command="python",
    args=["-c", "import time; [print(i) for i in range(10)]"]
)

# Stream stdout in real-time
for line in run.stdout(stream=True):
    print(f"Output: {line}")

# Wait for completion
run.wait()
print(f"Exit code: {run.exit_code}")
```

### Proxy Setup

```python
import stellar_client

client = stellar_client.from_env()
device = client.devices.get("device_id")

# Create proxy to remote service
proxy = client.proxy.create(
    device_id=device.id,
    local_port=8080,
    remote_host="127.0.0.1",
    remote_port=80
)

print(f"Proxy created on port {proxy.port}")
print(f"Access remote service at http://localhost:{proxy.port}")

# Later, close the proxy
proxy.close()
```

## Error Handling

The client uses a comprehensive exception hierarchy:

```python
from stellar_client import (
    StellarException,        # Base exception
    ConnectionError,         # Connection issues
    DeviceNotFoundError,     # Device not found
    ComputeError,            # Compute operation errors
    FileTransferError,       # File transfer errors
    ProtocolError,           # Protocol-level errors
)

try:
    device = client.devices.get("invalid_id")
except DeviceNotFoundError as e:
    print(f"Device not found: {e}")
```

## Context Manager

The client supports context manager protocol:

```python
with stellar_client.from_env() as client:
    devices = client.devices.list()
    # Client automatically closed on exit
```

## Comparison with docker-py

| docker-py | stellar-client |
|-----------|----------------|
| `docker.from_env()` | `stellar_client.from_env()` |
| `client.containers.list()` | `client.devices.list()` |
| `client.containers.get(id)` | `client.devices.get(id)` |
| `container.logs()` | `run.logs()` |
| `container.logs(stream=True)` | `run.logs(stream=True)` |
| `client.images.pull(...)` | `client.compute.run(...)` |
| `client.networks.list()` | `client.proxy.list()` |

## Architecture

The client follows clean architecture principles:

- **API Layer** (`api.py`): Low-level HTTP client
- **Resources** (`resources/`): Resource managers and objects
- **Models** (`models/`): Pydantic data models
- **Utils** (`utils/`): Helper functions and utilities
- **Exceptions** (`exceptions.py`): Exception hierarchy

## Contributing

Contributions are welcome! Please follow the existing code style and add tests for new features.

## License

MIT License

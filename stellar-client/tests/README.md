# Stellar Client Test Suite

This directory contains comprehensive tests for the Stellar Client using the resource-based API.

## Test Files

### Main Test Suites

- **`test_client_api.py`** - Main integration test suite covering all resource-based APIs
  - Tests devices, compute, proxy, policy, and file operations
  - Uses the docker-py-like resource API: `client.devices.list()`, `client.compute.run()`, etc.
  - 49 tests covering all functionality

- **`test_client_api_comprehensive.py`** - Additional comprehensive tests
  - Extended test coverage for edge cases and advanced scenarios

### Configuration

- **`conftest.py`** - Pytest configuration and fixtures
  - Provides `temp_file` and `temp_dir` fixtures
  - Defines pytest markers for test organization

## Running Tests

### Using pytest

```bash
# Run all tests
pytest tests/ -v

# Run main test suite
pytest tests/test_client_api.py -v

# Run comprehensive tests
pytest tests/test_client_api_comprehensive.py -v

# Run with integration marker
pytest tests/ -m integration -v

# Skip integration tests
pytest tests/ -m "not integration" -v
```

### Using Docker Compose (Recommended)

```bash
# From stellar-go directory
docker-compose -f stellar-client/docker/docker-compose.test.yml run --rm test-runner pytest tests/test_client_api.py -v
```

## Test Structure

Tests follow the resource-based API pattern:

```python
import stellar_client

# Create client
client = stellar_client.from_env()

# Use resource managers
devices = client.devices.list()
device = client.devices.get("device_id")
device.ping()

# Compute operations
run = client.compute.run("device_id", "echo", ["hello"])
logs = run.logs()

# Proxy operations
proxy = client.proxy.create("device_id", 8080, "127.0.0.1:8080")

# Policy operations
policy = client.policy.get()
client.policy.add_to_whitelist("device_id")
```

## Test Markers

- `@pytest.mark.integration` - Integration tests requiring running Stellar node

## Prerequisites

- Python 3.8+
- pytest
- Running Stellar node (for integration tests)
- Docker Compose (for Docker-based testing)

## Test Coverage

The test suite covers:
- ✅ Device discovery and management
- ✅ Compute operations (run, logs, status, cancel)
- ✅ File operations (list, tree)
- ✅ Proxy management
- ✅ Policy management
- ✅ Error handling
- ✅ Edge cases


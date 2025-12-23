# Stellar Client Test Suite

This directory contains a comprehensive test suite for the Stellar Client, organized by protocol usage order and designed to prevent test duplication while ensuring complete coverage.

## Test Structure

The tests are organized following the **protocol usage order** as defined in the Stellar client:

### 1. Echo Protocol (`test_echo_protocol.py`)
- **Purpose**: Basic connectivity and device discovery
- **Tests**: Protocol initialization, ping functionality, device discovery, error handling
- **Dependencies**: None (foundation protocol)

### 2. File Protocol (`test_file_protocol.py`)
- **Purpose**: File transfer operations
- **Tests**: File listing, upload, download, complete workflow, error handling
- **Dependencies**: Echo protocol (for device discovery)

### 3. Compute Protocol (`test_compute_protocol.py`)
- **Purpose**: Computational tasks and environment management
- **Tests**: Conda environment management, script execution, error handling
- **Dependencies**: Echo protocol (for device discovery)

### 4. Proxy Protocol (`test_proxy_protocol.py`)
- **Purpose**: Network proxying and port forwarding
- **Tests**: Proxy creation, management, listing, error handling
- **Dependencies**: Echo protocol (for device discovery)

### 5. Integration Suite (`test_integration_suite.py`)
- **Purpose**: End-to-end testing across all protocols
- **Tests**: Complete workflow, error handling across protocols, client lifecycle
- **Dependencies**: All protocols

## Test Categories

### Unit Tests
- Test individual protocol functionality without requiring a running Stellar node
- Focus on initialization, validation, and error handling
- Run quickly and don't require external dependencies

### Integration Tests
- Test complete workflows with a running Stellar node
- Marked with `@pytest.mark.integration`
- Require proper Stellar node setup and network connectivity

## Running Tests

### Using the Custom Test Runner

```bash
# Run all tests in protocol order
python tests/run_tests.py --all --verbose

# Run unit tests only
python tests/run_tests.py --unit --verbose

# Run integration tests only
python tests/run_tests.py --integration --verbose

# Run tests for specific protocol
python tests/run_tests.py --protocol echo --verbose
python tests/run_tests.py --protocol file --verbose
python tests/run_tests.py --protocol compute --verbose
python tests/run_tests.py --protocol proxy --verbose
```

### Using pytest directly

```bash
# Run all tests
python -m pytest tests/ -v

# Run unit tests only
python -m pytest tests/ -m "not integration" -v

# Run integration tests only
python -m pytest tests/ -m "integration" -v

# Run specific protocol tests
python -m pytest tests/test_echo_protocol.py -v
python -m pytest tests/test_file_protocol.py -v
python -m pytest tests/test_compute_protocol.py -v
python -m pytest tests/test_proxy_protocol.py -v
```

## Test Markers

The following pytest markers are available:

- `@pytest.mark.integration`: Integration tests requiring running Stellar node
- `@pytest.mark.slow`: Slow-running tests
- `@pytest.mark.echo`: Echo protocol related tests
- `@pytest.mark.file`: File protocol related tests
- `@pytest.mark.compute`: Compute protocol related tests
- `@pytest.mark.proxy`: Proxy protocol related tests

## Fixtures

### Available Fixtures

- `temp_file`: Creates a temporary file for testing
- `temp_dir`: Creates a temporary directory for testing

### Removed Fixtures

The following mock fixtures have been removed to eliminate test duplication:
- `mock_stellar_client`: Mock client (removed)
- `sample_device_data`: Sample device data (removed)
- `sample_file_entry`: Sample file entry data (removed)

## Test Design Principles

### 1. No Mock Data
- All tests use real Stellar client instances
- Integration tests require actual Stellar nodes
- Unit tests focus on validation and error handling without mocks

### 2. Protocol Order
- Tests follow the natural protocol usage order
- Each protocol builds upon the previous ones
- Integration tests verify end-to-end workflows

### 3. No Duplication
- Each test has a specific purpose
- Similar functionality is consolidated
- Error handling is tested once per protocol

### 4. Comprehensive Coverage
- Unit tests for basic functionality
- Integration tests for complete workflows
- Error handling tests for failure scenarios

## Prerequisites

### For Unit Tests
- Python 3.8+
- pytest
- Stellar client dependencies

### For Integration Tests
- All unit test prerequisites
- Running Stellar node(s)
- Network connectivity between nodes
- Proper Unix socket or HTTP API access

## Test Results

### Expected Results
- **Unit Tests**: All should pass (no external dependencies)
- **Integration Tests**: May be skipped if no Stellar nodes are available
- **Total Coverage**: All Stellar client functionality should be covered

### Troubleshooting

1. **Integration tests skipped**: Ensure Stellar nodes are running
2. **Connection errors**: Check Unix socket or HTTP API connectivity
3. **Timeout errors**: Verify network connectivity and node responsiveness

## Contributing

When adding new tests:

1. Follow the protocol usage order
2. Avoid duplicating existing test functionality
3. Use appropriate test markers
4. Include both unit and integration tests where applicable
5. Update this README if adding new test categories or markers





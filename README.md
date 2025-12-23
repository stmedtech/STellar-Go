# Stellar

**Stellar** is a decentralized platform built with Go, leveraging libp2p for peer-to-peer communication and providing seamless integration with Python-based frameworks.

## Overview

Stellar enables distributed computing across multiple devices without requiring a central server. The platform uses a hybrid architecture combining Go for the decentralized networking layer and Python for computations.

### Key Features

- **Decentralized P2P Architecture**: Built on libp2p for robust peer-to-peer communication
- **Cross-Platform Support**: Runs on Linux, macOS, and Windows
- **Python FL Integration**: Seamless integration with Flower and NVFlare frameworks
- **Unix Socket API**: Lightweight communication interface for Python clients
- **GUI Interface**: User-friendly graphical interface for node management
- **Security Policies**: Configurable peer authentication and authorization
- **File Sharing**: Secure peer-to-peer file transfer capabilities
- **Proxy Services**: TCP/HTTP proxy support for network traversal
- **Environment Management**: Automated Conda environment setup and management

## Architecture

### Core Components

#### Device Layer (`core/device/`)
- **Device Management**: Handles device initialization, key management, and protocol binding
- **Socket Integration**: Unix socket server for Python client communication
- **Protocol Binding**: Registers and manages various P2P protocols (echo, file, compute)

#### Networking Layer (`p2p/`)
- **Identity Management** (`p2p/identity/`): Ed25519 key generation and management
- **Node Discovery** (`p2p/node/`): DHT-based peer discovery and health monitoring
- **Security Policies** (`p2p/policy/`): Whitelist-based peer authorization
- **Protocol Suite**:
  - **Echo Protocol** (`p2p/protocols/echo/`): Ping/pong and device info exchange
  - **File Protocol** (`p2p/protocols/file/`): Secure file transfer with checksums
  - **Proxy Protocol** (`p2p/protocols/proxy/`): TCP/HTTP proxy services

#### Compute Layer (`core/protocols/compute/`)
- **Conda Management** (`core/conda/`): Automated Python environment setup
- **Script Execution**: Remote Python script execution in managed environments
- **Environment Isolation**: Containerized execution environments

#### System Layer (`core/`)
- **Socket Server** (`core/socket/`): RESTful API over Unix sockets
- **Utilities** (`core/util/`): System information gathering and file operations
- **Constants** (`core/constant/`): System-wide configuration parameters

### CLI Commands

The Stellar platform provides several CLI commands:

#### Node Operations
```bash
# Start a regular node
./stellar node --host 0.0.0.0 --port 4001

# Start with custom settings
./stellar node --host 0.0.0.0 --port 4001 --reference_token "my-token" --metrics

# Import existing key
./stellar node --b64privkey "base64_private_key"

# Disable security policy (development only)
./stellar node --disable-policy
```

#### Bootstrapper Operations
```bash
# Start a bootstrap node
./stellar bootstrapper --host 0.0.0.0 --port 4001

# Start with relay functionality
./stellar bootstrapper --relay --debug
```

#### Key Management
```bash
# Generate new key pair
./stellar key

# Generate with custom seed
./stellar key --seed 12345
```

#### GUI Mode
```bash
# Launch GUI interface
./stellar gui

# Start node directly with default settings
./stellar gui --bypass
```

## Implemented Features

### ✅ Completed Features

#### Networking & P2P
- **libp2p Integration**: Full libp2p stack with DHT, NAT traversal, and relay support
- **Peer Discovery**: Automatic peer discovery using Kademlia DHT
- **Security Policies**: Whitelist-based peer authorization system
- **Health Monitoring**: Continuous peer connectivity and health checks

#### Protocol Suite
- **Echo Protocol**: Ping/pong messaging and device information exchange
- **File Sharing Protocol**: Secure file transfer with SHA256 checksums
- **TCP Proxy Protocol**: TCP tunneling through P2P connections
- **Compute Protocol**: Remote Python script execution with Conda environments

#### Core Infrastructure
- **Unix Socket API**: RESTful API server for Python client integration
- **Conda Management**: Automated Python environment setup and package management
- **Cross-Platform Support**: Linux, macOS, and Windows compatibility
- **CLI Interface**: Comprehensive command-line tools
- **GUI Application**: Fyne-based graphical user interface

#### Federated Learning Integration
- **Flower Framework Support**: Complete integration with Flower FL framework
- **Docker Support**: Containerized deployment options
- **Environment Isolation**: Secure execution environments for FL workloads

### 🚧 Work in Progress

#### Current Development
- **FL Proof of Concept Testing**: End-to-end federated learning workflow validation
- **Audit System**: Comprehensive logging and monitoring for system-level and task-level operations

### 📋 Planned Features

#### High Priority
- **Robust Logging**: Enhanced real-time log reporting mechanism for conda operations
- **Script Execution Optimization**: More elegant script execution and output handling
- **Compute Protocol Logs**: Comprehensive logging for computation workflows
- **TCP Proxy Enhancement**: Improved proxy protocol with proper cancel event handling

#### Security & Reliability
- **Protocol Error Handling**: Comprehensive error handling and reporting system
- **Unknown Protocol Management**: Manual protocol acceptance with user notifications
- **Peer Discovery Optimization**: Faster peer discovery mechanisms

#### Advanced Features
- **Protobuf Integration**: Protocol buffer support for efficient messaging
- **Comprehensive Testing**: Complete test suite for all components
- **NVFlare Integration**: Native support for NVIDIA Flare framework

## Installation & Setup

### Prerequisites
- Go 1.24.3 or later
- Python 3.13+ (managed automatically via Conda)
- Docker (optional, for containerized deployment)

### Building from Source
```bash
# Clone the repository
git clone <repository_url>
cd stellar-go

# Build the binary
go build -o stellar ./cmd/stellar

# Make executable (Linux/macOS)
chmod +x stellar
```

### Quick Start

1. **Start a Bootstrap Node**:
```bash
./stellar bootstrapper --host 0.0.0.0 --port 4001 --relay
```

2. **Start Regular Nodes**:
```bash
# Node 1
./stellar node --host 0.0.0.0 --port 4002

# Node 2
./stellar node --host 0.0.0.0 --port 4003
```

3. **Launch GUI** (optional):
```bash
./stellar gui
```

## API Reference

### Unix Socket Endpoints

#### Device Management
- `GET /devices` - List all discovered devices
- `GET /devices/{deviceId}` - Get specific device information
- `GET /devices/{deviceId}/tree` - Get device file tree

#### Security Policy
- `GET /policy` - Get current policy settings
- `POST /policy` - Update policy settings
- `GET /policy/whitelist` - Get whitelisted devices
- `POST /policy/whitelist` - Add device to whitelist
- `DELETE /policy/whitelist` - Remove device from whitelist

## Development

### Project Structure
```
stellar-go/
├── cmd/stellar/           # CLI application entry point
├── core/                  # Core system components
│   ├── conda/            # Conda environment management
│   ├── constant/         # System constants
│   ├── device/           # Device management
│   ├── protocols/        # Core protocols
│   ├── socket/           # Unix socket API server
│   └── util/             # Utility functions
├── p2p/                   # P2P networking layer
│   ├── bootstrap/        # Bootstrap node discovery
│   ├── identity/         # Cryptographic identity
│   ├── node/             # P2P node implementation
│   ├── policy/           # Security policies
│   ├── protocols/        # P2P protocols
│   └── util/             # P2P utilities
└── docker/               # Monitoring and metrics
```

### Contributing

1. Fork the repository
2. Create a feature branch
3. Implement changes with appropriate tests
4. Submit a pull request

### Testing
```bash
# Run tests
go test ./...

# Run specific test
go test ./p2p/protocols/file
```

## Monitoring & Metrics

The platform includes comprehensive monitoring:

- **Prometheus Metrics**: Performance and network metrics
- **Grafana Dashboards**: Visual monitoring of P2P network health
- **Debug Mode**: Detailed logging for development and troubleshooting

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- Built on [libp2p](https://libp2p.io/) for robust P2P networking
- Integrates with [Flower](https://flower.dev/) federated learning framework
- Uses [Fyne](https://fyne.io/) for cross-platform GUI development
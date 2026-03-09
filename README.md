# STellar-Go (Stellar)

**Decentralized P2P distributed computing infrastructure.** Stellar is a Go application built on libp2p that provides peer-to-peer connectivity, bootstrap and relay services, and protocols for file transfer, TCP proxy, and remote compute. It is a distributed computing infrastructure for device discovery, workspace distribution, execution on remote nodes, and firewall-compatible proxy connectivity.

## Overview

Stellar enables distributed computing and federated learning across multiple devices without requiring a central server or public IPs on participant nodes. The platform uses libp2p for P2P networking (DHT-based discovery, NAT traversal, relay) and exposes a **Unix socket** and optional **HTTP API** for the Python **stellar-client** library, which external orchestrator service uses to discover devices, upload workspace zips, run FL clients via compute, and create proxy connections for FL traffic.

### Key Features

- **libp2p-based P2P** — DHT discovery, relay, NAT port mapping, hole-punching.
- **Bootstrap and relay** — Single node can act as both; relay enables connectivity for NATted/firewalled peers.
- **Protocols** — File transfer, TCP proxy, compute (Conda-managed Python execution).
- **Cross-platform** — Linux, macOS, Windows.
- **Security** — Optional whitelist-based policy for peer authorization; configurable via API/socket.
- **Conda management** — `stellar conda` subcommands for environment create/run and stellar-client install on remote nodes.
- **Config** — `config.json` under Stellar data path; first run with `stellar node` generates defaults (including a new key) and saves them.

## Architecture

### Core Components

- **Device / node** (`core/device/`, `p2p/node/`) — Node lifecycle, key management, DHT bootstrap, relay, protocol registration. Bootstrap and relay flags passed into `p2p/node.NewNodeWithOptions` / `NewBootstrapper`.
- **P2P layer** (`p2p/`) — Identity (`p2p/identity/`), bootstrap peer list (`p2p/bootstrap/`: `bootstrappers.txt`), node implementation and discovery (`p2p/node/`), policy/whitelist (`p2p/policy/`).
- **Protocols** (`p2p/protocols/`) — **Echo**: ping/device info. **File** (`file/`): transfer with checksums. **Proxy** (`proxy/`): TCP proxy. **Compute** (`compute/`): remote run with Conda (create env, run command, stream stdout/stderr).
- **Socket / API** (`core/socket/`) — Unix socket server (default) and optional HTTP API server for stellar-client. Binds device, file, compute, proxy, policy, and config handlers.
- **Config** (`core/config/`) — Load/save `config.json`; CLI flags override defaults on first run and are persisted.

### Python client (stellar-client)

The `stellar-client` directory contains a Python client package:

- **Devices** — `client.devices.list()`, `client.devices.get(id)`, `device.ping()`, `device.files().upload/download()`.
- **Compute** — `client.compute.run(device_id, command, args)` with optional streaming via `run.stream_output()`.
- **Proxy** — `client.proxy.create(device_id, local_port, remote_host, remote_port)` for FL traffic tunneling.
- **Policy** — `client.policy.get()`, `add_to_whitelist`, `remove_from_whitelist`.

See `stellar-client/README.md` for API details and examples.

## CLI

The binary is `stellar` (or `./stellar` when built from source). Subcommands:

- **`stellar key`** — Generate Ed25519 key pair (or with `--seed`).
- **`stellar node`** — Run the P2P node (regular or bootstrapper). Config is loaded from `config.json`; CLI flags override and are saved if the config file did not exist.
- **`stellar conda`** — Conda environment and run operations (used by orchestration to prepare remote FL clients).
- **`stellar config`** — Print or manage config (e.g. show paths, config file location).

### Node command

```bash
# Regular node (listens on host:port, uses bootstrappers from bootstrappers.txt)
./stellar node --host 0.0.0.0 --port 4001

# Bootstrapper (DHT server, optional relay)
./stellar node --bootstrapper --host 0.0.0.0 --port 4001
./stellar node --bootstrapper --relay --host 0.0.0.0 --port 4001 --debug

# With options
./stellar node --host 0.0.0.0 --port 4002 --reference_token "my-token" --metrics
./stellar node --b64privkey "<base64_private_key>"
./stellar node --disable-policy   # development only
./stellar node --no-socket        # disable Unix socket server
./stellar node --api --api-port 1524   # enable HTTP API (default in config)
```

- **`--bootstrapper`** — Run as DHT bootstrapper.
- **`--relay`** — Enable relay (only with `--bootstrapper`).
- **`--disable-node`** — Run only as bootstrapper (no discovery, no socket/API); only with `--bootstrapper`.
- **`--data-dir`** — Data directory (default: `./data`); config and logs live under Stellar path derived from this.

### Key management

```bash
./stellar key
./stellar key --seed 12345
```

### Quick start (minimal)

1. **Start a bootstrap + relay node** (publicly reachable):

```bash
./stellar node --bootstrapper --relay --host 0.0.0.0 --port 4001
```

2. **Put that node’s multiaddr in `bootstrappers.txt`** in the working directory of other nodes (see `p2p/bootstrap/bootstrap.go`).

3. **Start regular nodes** (e.g. on other machines or ports):

```bash
./stellar node --host 0.0.0.0 --port 4002
./stellar node --host 0.0.0.0 --port 4003
```

4. **Use stellar-client from Python** (with a node running and socket/API enabled):

```python
import stellar_client
client = stellar_client.from_env()
devices = client.devices.list()
```

## Installation and build

### Prerequisites

- **Go 1.24+** (see `go.mod`).
- **Python 3.9+** for stellar-client (Conda on remote nodes is managed by Stellar via the compute protocol).

### Build from source

```bash
cd stellar-go
go build -o stellar ./cmd/stellar
chmod +x stellar
```

### stellar-client (Python)

From repo:

```bash
cd stellar-client
pip install -e .
```


## Configuration

On first run, `stellar node` writes a default `config.json` under the Stellar data path (see `core/constant`). Fields include `listen_host`, `listen_port`, `bootstrapper`, `relay`, `reference_token`, `metrics`, `api`, `api_port`, `data_dir`, `disable_policy`, `no_socket`, etc. Edit this file or pass flags; flags override only when the config file is missing and are then saved.

Bootstrap peers are read from `bootstrappers.txt` in the working directory (one multiaddr per line). Use the bootstrapper node’s multiaddr (e.g. from its log output) so regular nodes can discover each other and use relay if needed.

## API / socket reference

- **Devices** — List, get, connect; ping; file list/upload/download.
- **Compute** — Run command, list/get runs, stream stdout/stderr, wait, cancel, remove.
- **Proxy** — Create, list, get by port, close.
- **Policy** — Get/update policy; whitelist add/remove/get.
- **Config / info** — Node info, health.

Exact HTTP routes and socket messages are implemented in `core/socket/socket.go`. The stellar-client library wraps these.

## Project structure

```
stellar-go/
├── cmd/stellar/           # CLI: key, node, conda, config
├── core/
│   ├── config/           # config.json load/save
│   ├── constant/         # Paths, defaults
│   ├── device/            # Device init, socket/API startup
│   ├── socket/             # Unix socket + HTTP API server
│   └── util/
├── p2p/
│   ├── bootstrap/        # bootstrappers.txt, peer addr parsing
│   ├── identity/          # Ed25519 key gen/encode
│   ├── node/              # libp2p host, DHT, relay, discovery
│   ├── policy/            # Whitelist policy
│   ├── protocols/
│   │   ├── common/       # Multiplexer, protocol helpers
│   │   ├── compute/      # Compute protocol + Conda service
│   │   ├── echo/         # Echo protocol
│   │   ├── file/         # File transfer
│   │   └── proxy/        # TCP proxy
│   └── util/
├── stellar-client/        # Python client (devices, compute, proxy, policy)
├── frontend/              # Optional dashboard/frontend
└── go.mod
```

## Testing

```bash
go test ./...
go test ./p2p/protocols/file/...
go test ./p2p/protocols/proxy/...
```

## License

See the LICENSE file in the repository.

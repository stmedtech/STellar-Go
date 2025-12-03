# Stellar Go – Proxy & File Integration Log

This document records the current state, constraints, and execution details for porting the `stellar-proxy-core` control/data plane into `stellar-go`. Keep it up to date while working through the roadmap so that every contributor can see **what was observed, which specs/tests were written, and which layers are safe to change**.

## Integration Guardrails

- **Reference-only imports**: use the code under `../stellar-proxy-core/stellar-proxy/{multiplexer,protocol,proxy}` as the specification. Re-implement the needed packages inside `stellar-go`; do _not_ import `github.com/experimental/go-readwritecloser-communication-protocol`, and do not edit the reference repo.
- **Single product surface**: no tunnel-specific CLI will be added. Operational control flows through `core/socket/socket.go` (REST/Unix socket) and the existing `cmd/stellar` entrypoint, both of which must call the new proxy/file services instead of bespoke structs.
- **Design discipline**: favour clean architecture boundaries (transport adapter → protocol layer → application orchestration), apply DRY when porting helpers, and keep context-aware cancellation everywhere a goroutine is spawned.
- **Spec/TDD workflow**: for every phase, document the behaviour in this file, add/port the failing tests, then implement the minimal code to make them pass. Reference commits or file sections when tests are introduced.
- **Workflow validation**: after the proxy refactor lands, add a Docker Compose workflow that boots two nodes plus the API server and exercises `/proxy` endpoints automatically. Treat this as a required regression test.

## Current Snapshot (Updated)

### Proxy stack
- ✅ **COMPLETED**: Ported multiplexer and protocol packages from `stellar-proxy-core` to `p2p/protocols/common/{multiplexer,protocol}`
- ✅ **COMPLETED**: Ported proxy service layer (`service/{server,client,manager,control_plane}`) with full handshake, multiplexing, and stream management
- ✅ **COMPLETED**: Refactored `tcpProxy.go` to use new service layer while maintaining backward-compatible API
- ✅ **COMPLETED**: `ProxyManager` now works with refactored `TcpProxyService` which uses the new client/server internally
- ✅ **COMPLETED**: Created integration tests (`integration_test.go`) and Docker Compose workflow (`docker-compose.proxy-test.yml`)
- 🔄 **IN PROGRESS**: Legacy test cleanup and full end-to-end validation

### File stack
- `p2p/protocols/file/file.go` implements a newline-delimited command protocol (`StellarFileList`, `StellarFileGet`, `StellarFileSend`). Control messages and file bytes reuse the same libp2p stream sequentially, preventing concurrent transfers.
- Tests under `p2p/protocols/file/file_test.go` only cover the legacy flow; they must be rewritten once the packetized transport is adopted.

### Reference repository highlights (`stellar-proxy-core`)
- `stellar-proxy/proxy/server.go` and `stellar-proxy/proxy/client.go` provide the authoritative handshake, proxy lifecycle, and control-plane event dispatch. Treat them strictly as spec material and port the code/tests into `stellar-go`.
- `stellar-proxy/multiplexer/multiplexer.go` already enforces stream 0 as the control plane and offers deterministic stream IDs for data forwarding.
- The JSON packet schema (hello/ack, proxy_opened, proxy_closed, proxy_list, errors) lives in `stellar-proxy/protocol`, together with helpers such as `PacketReadWriteCloser`.

### Testing baseline
- `tests/integration/network_test.go` boots the old proxy service; these tests will start failing as soon as the new proxy goes in and therefore must be rewritten to speak the new CLI/manager API.
- `p2p/protocols/proxy/enhanced_proxy_test.go` is guarded behind `// +build ignore`, so it never executes. Decide whether to delete it or resurrect the useful cases with the new implementation.

## Execution Plan Snapshot

### Architectural refactors (ongoing)
1. **Module cleanup**: keep `p2p/protocols/proxy` focused on libp2p handlers plus a thin service shim. Introduce `p2p/protocols/proxy/internal/{server,client,multiplexer}` (names TBD) that house the code ported from `stellar-proxy-core`.
2. **Service interfaces**: define proxy/file service interfaces that the API (`core/socket`) and CLI (`cmd/stellar`) can consume. The handlers should translate HTTP/flag inputs into service calls and return structured responses.
3. **Adapter review**: document how `core/socket/socket.go` and `cmd/stellar/node.go` use the new services so the code remains compliant with clean architecture (outer layers depend on interfaces, inner layers stay unaware of HTTP/CLI).

### Phase 0 – Environment, transport, documentation ✅ COMPLETED
1. ✅ **Shared workspace**: Created `go.work` at project root to reference both `stellar-go` and `stellar-proxy-core`
2. ✅ **Transport layer**: Confirmed `network.Stream` already implements `io.ReadWriteCloser`; no adapter needed. Multiplexer and protocol packages work directly with libp2p streams.
3. ✅ **Documentation**: This `DEV.md` updated with progress and decisions

### Phase 1 – Proxy protocol 🔄 IN PROGRESS
1. ✅ **Server control loop**:
   - ✅ Ported `server.go`, `manager.go`, `control_plane.go` to `p2p/protocols/proxy/service/`
   - ✅ Replaced `tcpStreamHandler` with new server: creates `service.Server`, calls `Accept()` for handshake, runs `Serve()` in goroutine
   - 🔄 Tests: Integration tests created; unit tests with libp2p mocks pending
2. ✅ **Client lifecycle**:
   - ✅ Refactored `TcpProxyService` to use `service.Client` per remote device with connection reuse
   - ✅ Implemented `OpenWithLocalConn` for forwarding local TCP connections through proxy streams
   - ✅ Tests: Created `integration_test.go` with handshake, single proxy, and multiple concurrent proxy tests
3. ✅ **CLI & shell**:
   - ✅ No tunnel CLI needed; `core/socket/socket.go` already provides HTTP API endpoints
   - ✅ Existing `cmd/stellar` entrypoint works with refactored proxy service
4. ✅ **API & orchestration**:
   - ✅ `core/socket/socket.go` proxy endpoints (`CreateProxy`, `ListProxies`, `CloseProxy`) work with refactored `ProxyManager`
   - 🔄 Handler/unit tests with service mocks pending
5. 🔄 **Observability**:
   - 🔄 Debug logging with peer + stream IDs pending
   - 🔄 Metrics counters pending (interface stubbed in service layer)
6. ✅ **Docker Compose workflow**:
   - ✅ Created `docker-compose.proxy-test.yml` with two nodes, test server, and test runner
   - 🔄 Make target and CI integration pending

### Phase 2 – File protocol refactor
1. **Handshake adoption**: apply the same packet framing + multiplexer on top of `p2p/protocols/file`, defining `file_hello`/`file_hello_ack`.
2. **Command/control plane**: refactor list/get/send into JSON packets that always flow through stream 0, referencing the fields currently defined in `FileEntry`/`FileInfo`.
3. **Data streaming**: allocate per-transfer data streams (IDs > 0) and guard `io.Copy` with contexts/timeouts.
4. **Backward incompatible cleanup**: remove the newline protocol constants once all call sites move to the new stack. Update `file_test.go` to cover handshake failures, concurrent transfers, and checksum mismatches.

### Phase 3 – Compute protocol (blocked)
- Do not start until Phases 1–2 pass the updated test suite.
- Reuse the packet schema and data channels established above for environment sync and runtime negotiation.

## Testing Strategy

- **Unit**: each ported package (`multiplexer`, `protocol`, `proxy/client`, `proxy/server`, file helpers) must carry the same tests that exist in `stellar-proxy-core`, adapted to the libp2p transports.
- **Integration**: reuse `tests/integration/network_test.go`, but migrate it to create two libp2p nodes that run the new proxy + file stacks end-to-end. Include API-focused smoke tests hitting `core/socket` handlers via httptest.
- **Docker Compose**: after the proxy refactor, run the new compose workflow to validate `/proxy` endpoints against real nodes. This must pass before merging.
- **Bench / stress**: once the new proxy is stable, resurrect the throughput/stress scenarios from `stellar-proxy/proxy/integration_test.go` to guard against regressions.

## Decision Log

| Date | Decision |
| ---- | -------- |
| 2025-12-03 | Confirmed `network.Stream` already satisfies `io.ReadWriteCloser`; no extra adapter dependency is necessary. All libp2p transport helpers will wrap the stream directly. |
| 2025-12-03 | `stellar-go` must not import `github.com/experimental/go-readwritecloser-communication-protocol`; instead, we port code/tests from `stellar-proxy-core` into `p2p/protocols`. |
| 2025-12-03 | Adopt a strict spec-first, test-second, code-third workflow for every roadmap milestone. |
| 2025-12-03 | The API server (`core/socket`) replaces any tunnel-specific CLI. All proxy/file orchestration will be exposed via HTTP/Unix socket plus the existing `cmd/stellar` entrypoint. |
| 2025-12-03 | A Docker Compose workflow that boots two nodes and exercises `/proxy` endpoints is required before declaring Phase 1 complete. |
| 2025-12-03 | ✅ Ported multiplexer, protocol, and proxy service packages from `stellar-proxy-core` to `stellar-go/p2p/protocols/common` and `p2p/protocols/proxy/service` |
| 2025-12-03 | ✅ Refactored `tcpProxy.go` to use new service layer while maintaining backward-compatible public API (`TcpProxyService`, `ProxyManager`) |
| 2025-12-03 | ✅ Created integration tests and Docker Compose workflow test file. Service layer compiles and integrates with existing codebase. |
| 2025-12-03 | ✅ Confirmed `network.Stream` works directly with multiplexer without adapter layer. Service layer uses streams directly. |

Append new rows whenever you make an architectural or testing decision.


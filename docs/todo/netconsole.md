# NETConsole Transport Implementation Plan

## Current Status

Implementation, automated tests, fuzzing, race checks, UI verification, and
cross-platform builds are complete. A real-device smoke test has also been
reported as complete. The file remains until each detailed manual firmware
acceptance scenario below is explicitly verified; delete it when no unchecked
items remain.

## Goal

Add `netconsole` as a third, mutually exclusive node transport beside `udp` and
`serial`.

The daemon will connect to a MeshCom node through its raw TCP NETConsole
listener, authenticate, enable ExtUDP console records, decode the same records
used by the serial transport, and feed them into the existing
`packetingest.Processor`. Outgoing broadcast, channel, and direct messages will
use the same `::` console command format as serial.

Confirmed product decisions:

- transport mode name: `netconsole`;
- bidirectional transport: receive records and send messages;
- endpoint configuration: one `host:port` address, including port `2323`;
- password stored in the local TOML file, masked by the configuration API and
  UI, and never logged;
- send `--setinfo on\r\n` automatically after every successful
  authentication;
- changing NETConsole configuration requires an application restart;
- implementation must use the Go standard library only and work on Linux,
  macOS, and Windows.

## Source Material

- Firmware protocol:
  `/home/freddy/1/MeshCom-Firmware/docs/netconsole.md`
- Go interoperability prototype:
  `/home/freddy/1/MeshCom-Firmware/tools/netconsole_client.go`
- Existing serial transport:
  `internal/serialbridge`
- Shared packet ingestion:
  `internal/packetingest`
- Transport status contract:
  `internal/transport/status.go`

The prototype is reference material only. It lacks bounded authentication
reads, strict open-access validation, complete deadlines, serialized writes,
structured shutdown, typed errors, and tests.

## Target Data Flow

```text
MeshCom node :2323
        |
        | raw TCP
        v
NETConsole dial + HMAC authentication
        |
        | "--setinfo on\r\n"
        v
shared console record decoder
        |
        | extracted ExtUDP JSON
        +----------------------> configured UDP forward targets
        |
        v
packetingest.Processor
        |
        +--> persistence
        +--> SSE
        +--> chat and ACK handling
        +--> positions and telemetry
        +--> statistics
```

TX follows the inverse application path:

```text
HTTP message request
        |
        v
shared console command encoder
        |
        | "::message\r\n" or "::{destination}message\r\n"
        v
serialized NETConsole socket writer
```

## Proposed Configuration

```toml
transport_mode = "netconsole"

[netconsole]
address = "192.168.1.53:2323"
password = ""
connect_timeout = "5s"
auth_timeout = "5s"
write_timeout = "5s"
reconnect_initial = "1s"
reconnect_max = "30s"
stable_reset_after = "30s"
max_auth_line_bytes = 128
max_record_bytes = 65536
```

Environment equivalents:

```text
GOMESHCOM_NETCONSOLE_ADDRESS
GOMESHCOM_NETCONSOLE_PASSWORD
GOMESHCOM_NETCONSOLE_CONNECT_TIMEOUT
GOMESHCOM_NETCONSOLE_AUTH_TIMEOUT
GOMESHCOM_NETCONSOLE_WRITE_TIMEOUT
GOMESHCOM_NETCONSOLE_RECONNECT_INITIAL
GOMESHCOM_NETCONSOLE_RECONNECT_MAX
GOMESHCOM_NETCONSOLE_STABLE_RESET_AFTER
GOMESHCOM_NETCONSOLE_MAX_AUTH_LINE_BYTES
GOMESHCOM_NETCONSOLE_MAX_RECORD_BYTES
```

Configuration rules:

- `address` is required in `netconsole` mode;
- require `host:port` syntax through `net.SplitHostPort`;
- allow DNS names, IPv4, and bracketed IPv6;
- require a non-empty host and port in range `1..65535`;
- do not resolve DNS during configuration validation; each connection attempt
  resolves through `net.Dialer`, allowing startup while DNS or the node is
  temporarily unavailable;
- an empty password selects firmware open-access mode;
- non-empty passwords are limited to 14 bytes, matching firmware storage;
- reject password bytes containing NUL, CR, or LF;
- all timeouts and reconnect durations must be positive;
- require `reconnect_initial <= reconnect_max`;
- keep `max_auth_line_bytes` at least 72 and cap it to a small defensive upper
  bound such as 4096;
- keep `max_record_bytes` in `1..1048576`, matching serial safety limits.

The UI and `GET /api/config` return `password = "****"` when a password exists
and `password = ""` when none exists. Update semantics must distinguish:

- missing password field: preserve existing value;
- `"****"`: preserve existing value;
- a new non-empty value: replace password;
- explicit empty value: clear password and use open access.

When `GOMESHCOM_NETCONSOLE_PASSWORD` controls the value, attempts to replace or
clear it return the existing environment-lock conflict response. Sending the
mask sentinel does not count as a change.

## Implementation Steps

### 1. Establish Baseline

- [x] Run existing backend tests before changes:

  ```sh
  go test ./internal/config ./internal/serialbridge ./internal/packetingest ./internal/httpapi ./cmd/gomeshcomd
  ```

- [x] Run existing frontend tests before changes:

  ```sh
  cd web
  npm test
  ```

- [x] Record any pre-existing failures before implementation.

### 2. Extract Shared Console Codec

Serial and NETConsole carry the same console output and accept the same message
commands. Reusing `serialbridge` directly would create the wrong dependency:
NETConsole must not depend on a serial transport package.

- [x] Add a transport-neutral internal package, for example
  `internal/consolecodec`.
- [x] Move the ExtUDP line decoder from
  `internal/serialbridge/decoder.go` into the shared package.
- [x] Move the `::` text command encoder from
  `internal/serialbridge/encoder.go` into the shared package.
- [x] Rename transport-specific errors and messages to console-neutral names.
- [x] Keep serial behavior unchanged through characterization tests before the
  move.
- [x] Update `serialbridge` to consume the shared decoder and encoder.
- [x] Move or duplicate existing decoder, encoder, property, and fuzz coverage
  under the shared package.
- [x] Keep serial capture fixture coverage for LF, CRLF, split reads, malformed
  JSON, overlong records, and firmware 4.35 output.
- [x] Extract only pure reconnect/backoff helpers into `internal/transport` if
  NETConsole would otherwise duplicate them exactly. Keep connection lifecycle
  code transport-specific because serial opening and TCP authentication have
  different failure modes.

This refactor must pass all serial tests before NETConsole code is added.

### 3. Add Configuration Model and Persistence

- [x] Add `config.TransportNetConsole = "netconsole"`.
- [x] Extend `transport_mode` help and validation to
  `udp|serial|netconsole`.
- [x] Add `config.NetConsole` with fields listed above.
- [x] Add defaults in `builtInDefaultConfig`.
- [x] Normalize address whitespace without changing hostname case or password.
- [x] Add mode-specific validation without requiring NETConsole fields for UDP
  or serial deployments.
- [x] Add `[netconsole]` TOML wire type, merge logic, and default template.
- [x] Add all `GOMESHCOM_NETCONSOLE_*` keys to environment override detection.
- [x] Ensure precedence remains:
  `built-in defaults < TOML < environment`.
- [x] Add table-driven validation tests for empty address, missing port, invalid
  port, IPv4, DNS, bracketed IPv6, password byte length, forbidden password
  bytes, invalid durations, reconnect ordering, and buffer bounds.
- [x] Add TOML round-trip and environment precedence tests.
- [x] Extend configuration fuzz/property tests so normalize/serialize/load
  invariants include NETConsole fields.
- [x] Confirm legacy TOML files without `[netconsole]` still load as UDP with
  defaults.

Likely files:

- `internal/config/config.go`
- `internal/config/toml.go`
- `internal/config/config_test.go`
- `internal/config/serial_test.go` or a new `netconsole_test.go`
- existing configuration fuzz/property tests

### 4. Implement NETConsole Authentication

Create `internal/netconsole` with a small protocol layer independent from the
bridge lifecycle.

- [x] Define injectable dialing interface around `net.Dialer` for deterministic
  tests.
- [x] Connect with `net.Dialer{Timeout: connect_timeout}` using network `tcp`.
- [x] Enable standard-library TCP keepalive through `net.Dialer.KeepAlive`;
  treat it as dead-peer assistance, not a guaranteed application heartbeat.
- [x] Wrap the connection once in `bufio.Reader`.
- [x] Apply one authentication deadline covering the complete exchange.
- [x] Read authentication lines with a strict byte limit.
- [x] Strip trailing LF and one optional CR only.
- [x] Accept exactly two first-line forms:
  - `OK`: open-access session;
  - `NONCE: ` followed by exactly 32 hexadecimal characters.
- [x] Reject unknown first lines; do not copy prototype behavior that accepts
  every non-`NONCE` line.
- [x] Decode the nonce to exactly 16 bytes.
- [x] Calculate lowercase HMAC-SHA256 using:
  - `crypto/hmac`;
  - `crypto/sha256`;
  - `encoding/hex`.
- [x] Write the 64-character digest plus CRLF with complete-write semantics.
- [x] Require an exact `OK` result after authenticated login.
- [x] Return distinct errors for timeout, malformed challenge, unexpected line,
  authentication rejection, EOF, and write failure.
- [x] Clear the connection deadline after authentication.
- [x] Reuse the same `bufio.Reader` for the session so banner or console bytes
  already buffered after `OK` are not lost.
- [x] Never log password, nonce, digest, challenge/response payload, or raw
  authentication lines.

No new Go module dependency is required.

### 5. Implement NETConsole Bridge Lifecycle

- [x] Implement `Run(context.Context) error`, `SendText`, and
  `TransportStatus`, satisfying the existing `nodeTransport` interface.
- [x] Keep one active TCP session and one reader.
- [x] Use the shared console decoder for all post-authentication bytes.
- [x] After authentication, send `--setinfo on\r\n`.
- [x] Mark transport `connected` only after authentication and the complete
  `--setinfo on` write succeed.
- [x] Ignore banner, command echo, and unknown diagnostics unless they contain a
  supported ExtUDP record.
- [x] Feed each extracted payload to `packetingest.Processor` with:

  ```go
  packetingest.Source{
      Transport: "netconsole",
      Endpoint:  configuredAddress,
  }
  ```

- [x] Mirror extracted JSON to configured UDP forward targets, exactly like
  serial.
- [x] Serialize all socket writes with a mutex.
- [x] Apply `write_timeout` per write and clear the write deadline afterwards.
- [x] Handle partial TCP writes until the complete command is sent.
- [x] On TX write failure, fail and close the active session so the read loop
  cannot leave status falsely connected.
- [x] Encode outgoing broadcast, channel, and DM messages with the shared
  console encoder.
- [x] Preserve demo-mode dry-run behavior.
- [x] Return `transport.ErrUnavailable` wrapping from `SendText` when no
  authenticated session exists, producing HTTP `503`.
- [x] Close the connection exactly once on EOF, read error, write error, context
  cancellation, or shutdown.
- [x] Make context cancellation interrupt a blocking socket read by closing the
  connection.
- [x] Reconnect with capped exponential backoff and jitter, matching serial.
- [x] Reset backoff only after `stable_reset_after`.
- [x] Keep HTTP running while NETConsole reconnects.
- [x] Expose state, endpoint, last error, timestamps, and retry count through
  the existing `transport.Status`.
- [x] Log connection attempts, successful sessions, disconnect reason, and
  retry delay without logging credentials or raw console data.

Suggested files:

```text
internal/netconsole/auth.go
internal/netconsole/auth_test.go
internal/netconsole/bridge.go
internal/netconsole/bridge_test.go
internal/netconsole/dialer.go
```

### 6. Add Protocol and Bridge Tests First

Use `net.Pipe` for protocol units and a loopback fake TCP server where actual
dial/reconnect behavior matters. Tests must not require a MeshCom node,
external network, fixed port, or platform-specific socket feature.

- [x] Valid lowercase nonce and correct password.
- [x] Uppercase nonce accepted.
- [x] Open-access `OK` accepted without client auth bytes.
- [x] `FAIL` returns typed authentication rejection.
- [x] Unknown first line rejected.
- [x] Wrong nonce length rejected.
- [x] Non-hex nonce rejected.
- [x] Challenge split across multiple TCP writes reassembled.
- [x] CRLF and LF authentication lines accepted.
- [x] Overlong authentication line rejected without unbounded allocation.
- [x] Missing newline reaches authentication timeout.
- [x] EOF during authentication reported correctly.
- [x] `OK` and banner in one server write preserve banner for session decoder.
- [x] Deadline cleared after authentication.
- [x] Expected HMAC digest and CRLF sent exactly.
- [x] Empty password still rejects a nonce when response cannot authenticate;
  open-access is determined by server `OK`, not client assumption.
- [x] `--setinfo on\r\n` sent once per authenticated session.
- [x] ExtUDP record fragmented across TCP reads decoded once.
- [x] Multiple records in one TCP read decoded in order.
- [x] Async diagnostics and command echo ignored safely.
- [x] Extracted packet reaches processor and forwarder with NETConsole metadata.
- [x] Concurrent TX writes never interleave.
- [x] Partial writes complete correctly.
- [x] TX before authentication or after disconnect returns unavailable.
- [x] Read, write, remote close, and cancellation close session once.
- [x] Reconnect occurs after connect failure, authentication failure, and
  established-session loss.
- [x] Backoff caps and resets after a stable session.
- [x] Status transitions:
  `disconnected -> connecting -> connected -> degraded -> connecting`.
- [x] Demo mode validates/encodes TX but writes no socket bytes.
- [x] Run race tests for concurrent status, read, write, close, and reconnect.

Fuzz/property coverage:

- [x] Fuzz bounded authentication-line parsing with arbitrary bytes.
- [x] Seed nonce, `OK`, `FAIL`, overlong line, missing newline, NUL, and invalid
  UTF-8 cases.
- [x] Keep shared console decoder fuzzing for arbitrary post-auth bytes.
- [x] Add property test comparing HMAC output to `crypto/hmac.Equal`-verified
  expected values across generated nonce/password inputs.
- [x] Run each new fuzz target for a short bounded session and commit any crash
  input as a permanent seed.

### 7. Wire Daemon Startup

- [x] Add `case config.TransportNetConsole` in `cmd/gomeshcomd/main.go`.
- [x] Build bridge with configured endpoint, password, timeouts, reconnect
  policy, shared processor, forwarder, station identity, and demo-mode flag.
- [x] Assign `Run` as transport goroutine.
- [x] Keep startup non-blocking: initial TCP failure must enter reconnect state,
  not prevent HTTP server startup.
- [x] Include NETConsole endpoint in startup banner and structured startup log.
- [x] Preserve current shutdown timeout and unexpected-transport-stop handling.
- [x] Add startup/wiring tests or extract a small transport factory if direct
  `run` testing would otherwise require real storage and sockets.

### 8. Extend Configuration API

- [x] Add NETConsole response metadata to `GET /api/config`.
- [x] Add NETConsole patch DTO to `PUT /api/config`.
- [x] Reuse the existing password mask constant.
- [x] Implement explicit clear semantics described above.
- [x] Mark every NETConsole field `requires_restart: true`.
- [x] Add NETConsole fields to environment-lock checks.
- [x] Persist accepted updates atomically to TOML.
- [x] Ensure validation happens before persistence.
- [x] Never include real NETConsole password in JSON, validation errors, or
  logs.
- [x] Add API tests for masked GET, unchanged mask, replacement, explicit clear,
  environment lock, validation failure, restart flag, and TOML persistence.
- [x] Update `docs/openapi.yaml` if configuration schemas are represented there.

Likely files:

- `internal/httpapi/configapi.go`
- `internal/httpapi/configapi_test.go`
- `docs/openapi.yaml`

### 9. Extend Frontend Types and Settings

- [x] Extend transport union to `'udp' | 'serial' | 'netconsole'`.
- [x] Add `ConfigNetConsole` and patch types in
  `web/src/lib/api/config.ts`.
- [x] Add NETConsole option to transport selector.
- [x] Show NETConsole fields only when `transport_mode === 'netconsole'`.
- [x] Add address input with example `192.168.1.53:2323`.
- [x] Add password input using `type="password"`; show only the mask sentinel,
  never the secret.
- [x] Permit replacing or clearing the password.
- [x] Show connect, authentication, write, reconnect, stable-reset, auth-line,
  and record-size settings with environment/restart badges.
- [x] Include all fields in populate, dirty-state, and save-patch logic.
- [x] Keep UDP-only and serial-only settings hidden in NETConsole mode.
- [x] Explain that port `2323` is firmware default and connection is plaintext.
- [x] Add component tests for mode selection, conditional fields, populated
  values, password masking, clear/replace behavior, environment locks, and
  generated patch.

### 10. Extend Connection Status UI

- [x] Generalize `TransportWarning.svelte` from serial-only logic to any
  connection-oriented transport.
- [x] Show `NETConsole unavailable` when mode is `netconsole` and state is not
  `connected`.
- [x] Keep API connection state independent: browser may be connected while
  node transport reconnects.
- [x] Include latest error in title/text while preserving mobile truncation.
- [x] Keep UDP behavior unchanged.
- [x] Extend status type union.
- [x] Add tests for NETConsole connected, connecting, degraded, disconnected,
  and stopped states; retain serial and UDP regression cases.

### 11. Documentation and Tracking

- [x] Add `docs/netconsole.md` with setup, TOML, environment variables,
  authentication, status, TX/RX behavior, trusted-LAN warning, troubleshooting,
  Docker networking, and platform notes.
- [x] Add NETConsole to `docs/README.md`.
- [x] Update `README.md` prerequisites, quick start, features, forwarding text,
  and configuration table.
- [x] Update `docs/openapi.yaml` where applicable.
- [x] Update `CHANGELOG.md` under `Added`, `Changed`, and `Fixed` as appropriate.
- [x] Keep all documentation and code comments in English.

## Verification Matrix

### Backend

```sh
gofmt -w <modified-go-files>
go test ./internal/consolecodec ./internal/netconsole
go test ./internal/config ./internal/serialbridge ./internal/packetingest ./internal/httpapi ./cmd/gomeshcomd
go test -race ./internal/consolecodec ./internal/netconsole ./internal/serialbridge
go test ./...
go vet ./...
```

Run short fuzz sessions for authentication parsing and shared console decoding.

### Frontend

```sh
cd web
npm test
npm run check
```

Use exact scripts available in `web/package.json`; do not add a test dependency.

### Cross-platform Build

Build the daemon from the same source with no build-tagged NETConsole code:

```sh
GOOS=linux GOARCH=amd64 go build ./cmd/gomeshcomd
GOOS=darwin GOARCH=amd64 go build ./cmd/gomeshcomd
GOOS=windows GOARCH=amd64 go build ./cmd/gomeshcomd
```

Also retain existing `linux/arm64` container build coverage. Write build outputs
to a temporary directory so repository artifacts remain clean.

### Manual Firmware Acceptance

- [x] Enable NETConsole with `--netconsole on`.
- [ ] Test empty-password open access.
- [x] Set a password with `--passwd`, restart/reconnect, and verify HMAC login.
- [x] Verify wrong password produces degraded health and reconnect attempts.
- [x] Verify correct password changes health to connected.
- [x] Confirm `--setinfo on` activates `[EXT] Out` and `[EXT] Tele-Out`.
- [ ] Receive broadcast, channel, DM, position, telemetry, ACK, and reject
  records through existing UI/storage flows.
- [ ] Send broadcast, channel, and DM through UI.
- [ ] Disconnect Wi-Fi or power down node; verify degraded status, HTTP
  availability, TX `503`, and automatic reconnection.
- [ ] Change endpoint/password in Settings; verify restart requirement and
  persisted TOML.
- [ ] Verify password never appears in UI responses, browser logs, daemon logs,
  or test output.
- [ ] Repeat smoke test on at least Linux and one of macOS/Windows. Cross-build
  all three platforms even when physical-node testing is unavailable.

## Risks and Mitigations

### Plaintext and Unauthenticated Server

NETConsole is raw plaintext TCP. HMAC avoids sending the password directly but
does not encrypt traffic, authenticate the server, or prevent post-login
injection.

Mitigation:

- document trusted-LAN/VPN-only use;
- never expose port `2323` to the Internet;
- do not add a misleading TLS option unless firmware supports TLS;
- warn users in Settings and setup documentation.

### Firmware Credential Exposure

Current firmware documentation reports paths that may print the configured
password or authentication response on the hardware console, including
`--info`. A captured nonce/digest also permits offline guessing of short
passwords.

Mitigation:

- never log raw authentication traffic;
- avoid raw NETConsole debug logging because asynchronous firmware output may
  itself reveal credentials;
- recommend random 14-byte passwords;
- track firmware fixes separately; client code cannot remove server-side leaks.

### Secret Stored in TOML

The confirmed design persists the NETConsole password in plaintext locally.

Mitigation:

- mask all API/UI reads;
- never place the password in CLI arguments;
- recommend restrictive data-directory and TOML permissions;
- avoid copying secrets into errors, telemetry, diagnostics, or startup output;
- ensure default generated TOML contains an empty value only.

### Single Firmware Session

Firmware supports one authenticated NETConsole client and one authentication
task. Another terminal or daemon can prevent connection or replace the active
session.

Mitigation:

- surface authentication/connect errors in health and UI;
- use bounded backoff to avoid a tight retry loop;
- document that interactive NETConsole tools must be closed before daemon use.

### TCP Is a Byte Stream

TCP reads do not preserve line or command boundaries. Authentication, banner,
diagnostics, and ExtUDP records can be split or coalesced arbitrarily.

Mitigation:

- one persistent buffered reader;
- bounded line parsing for authentication;
- shared incremental console decoder after authentication;
- fragmentation/coalescing tests;
- never match a command to the next line as if NETConsole were request/response.

### Buffered Banner Loss

`OK`, banner, and initial records may arrive in one TCP packet. Replacing the
authentication reader would discard bytes already buffered.

Mitigation:

- retain the same `bufio.Reader` for the complete connection;
- add a regression test with `OK` and banner in one server write.

### Silent Output Drops

Firmware uses best-effort non-blocking output and can drop unsent bytes when its
TCP send buffer is full. The client cannot request replay.

Mitigation:

- keep reader continuously active;
- perform parsing and ingestion without blocking socket reads longer than
  necessary;
- document that NETConsole is not a guaranteed-delivery transport;
- rely on malformed/incomplete record handling without crashing the session.

### Idle Half-open Connections

A lost network path may not produce immediate EOF, so state can remain connected
until the operating system detects failure.

Mitigation:

- enable Go TCP keepalive;
- do not invent application probes that firmware does not define;
- document detection latency as OS-dependent;
- treat any read or write failure as immediate session failure.

### Wrong Password Retry Noise

A persistent wrong password will fail every reconnect and may flood logs or
consume firmware authentication slots.

Mitigation:

- capped exponential backoff with jitter;
- one concise warning per attempt;
- clear health error naming authentication rejection without exposing secrets.

### Address and DNS Variance

Node DHCP addresses may change; DNS may be unavailable during startup; IPv6
requires bracketed address syntax.

Mitigation:

- accept DNS, IPv4, and bracketed IPv6;
- validate structure without startup DNS resolution;
- resolve on each reconnect;
- document DHCP reservation or local DNS as preferred deployment setup.

### Concurrent Read, Write, and Close

TX failure, remote EOF, cancellation, and shutdown can race.

Mitigation:

- exactly one reader;
- write mutex;
- close-once session primitive;
- session identity check before clearing active connection;
- `go test -race` coverage.

### Password Byte Semantics

Firmware limits stored passwords to 14 bytes, trims stored padding spaces, and
operates on bytes rather than Unicode characters.

Mitigation:

- enforce byte length, not rune count;
- recommend printable ASCII;
- reject control separators;
- document case sensitivity and firmware trimming behavior.

### Transport Regression

Extracting shared serial codec code could alter a working serial transport.

Mitigation:

- baseline tests before edits;
- characterization tests before extraction;
- keep firmware capture fixture;
- make extraction a separate atomic commit from NETConsole behavior;
- run full serial, ingest, API, and frontend regression suites.

## Dependency Budget

Add no third-party Go dependency.

Required standard-library packages:

```text
bufio
context
crypto/hmac
crypto/sha256
encoding/hex
errors
fmt
io
net
strings
sync
sync/atomic
time
```

Reuse existing project packages for callsign validation, message validation,
console framing, packet ingestion, forwarding, status, and reconnect helpers.

## Suggested Atomic Commits

1. `refactor: share console framing across stream transports`
2. `test: define NETConsole protocol behavior`
3. `feat: add NETConsole transport and configuration`
4. `feat: expose NETConsole settings and status`
5. `docs: document NETConsole setup and security`

Each commit must keep tests green and must not include unrelated working-tree
changes.

## Definition of Done

- `netconsole` selectable through TOML, environment, API, and Settings;
- authenticated and open-access sessions supported strictly;
- password masked everywhere outside protected local configuration;
- `--setinfo on` sent after every authentication;
- ExtUDP JSON enters the same ingest and forwarding flow as serial;
- broadcast, channel, and DM TX work through the socket;
- disconnected transport reports degraded health and UI warning while HTTP
  remains available;
- reconnect and shutdown are race-free;
- no new Go dependency;
- Linux, macOS, and Windows builds succeed;
- unit, property, fuzz, race, frontend, and full regression suites pass;
- README, `docs/`, OpenAPI, and changelog updated;
- no debug logs, credentials, generated binaries, or unrelated files remain.

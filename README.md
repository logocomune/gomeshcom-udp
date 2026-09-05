# gomeshcom-client

Unofficial Go-based web client for receiving UDP, serial, or NETConsole traffic from a local [MeshCom](https://icssw.org/en/meshcom/) node.

It receives packets from a MeshCom node and provides a browser UI for real-time monitoring, chat, and node tracking.

The main executable is `gomeshcomd`.

> **Note:** `gomeshcom-client` is an independent, unofficial project.  
> It is not affiliated with, endorsed by, or maintained by the MeshCom project or its developers.

> **Note on AI-assisted development:**
>
> `gomeshcom-client` is developed with substantial assistance from AI coding
> tools, while human maintainers lead product decisions, architecture, testing,
> debugging, and final review. We disclose this because AI assistance has
> materially shaped the project's development. Human maintainers remain
> responsible for every accepted change and release. Constructive technical
> feedback and contributions are welcome.

| Dashboard | Chat | Map |
|:---------:|:----:|:---:|
| ![Dashboard](images/dashboard.jpg) | ![Chat](images/messages.jpg) | ![Map](images/map.jpg) |

## Quick Start

**Prerequisites:** a MeshCom node reachable over UDP (default port `1799`), an explicit serial device using firmware 4.35+, or an ESP32 NETConsole endpoint.

If this is your first network setup, follow the step-by-step guide first:

- [First Setup](docs/first-setup.md)

## Documentation

Browse current setup, feature, operational, and MeshCom reference material in [docs/README.md](docs/README.md).

### Binary

```bash
./gomeshcomd --my-call="QQ0YY-1"
```

Open:

```text
http://localhost:8080
```

The node address is **auto-detected** from the first incoming UDP packet — no further configuration is needed for typical setups.

For a fresh node, make sure ExtUDP on the device points to the host running `gomeshcomd`, and allow inbound UDP `1799` on that host before starting the service.

If you want to open the web UI from other machines, start `gomeshcomd` on `0.0.0.0:8080` or on the specific IP you want to expose, then allow inbound TCP `8080` in the firewall. Keep the default loopback bind if access should stay local.

### Docker

```bash
docker run -d \
  -p 8080:8080 \
  -p 1799:1799/udp \
  -v gomeshcom-data:/data \
  -e GOMESHCOM_STORAGE_SQLITE_PATH=/data/gomeshcom.db \
  -e GOMESHCOM_MY_CALL=QQ0YY-1 \
  -e GOMESHCOM_HTTP_ADDR=0.0.0.0:8080 \
  ghcr.io/logocomune/gomeshcom:latest
```

Open:

```text
http://<host-ip>:8080
```

### UDP Address and Forwarding

If the node does not broadcast automatically, or you want to pin the address, set it explicitly — this disables auto-detection:

```bash
-e GOMESHCOM_NODE_ADDR=192.168.1.100:1799
```

To mirror every received UDP packet to other consumers, set one or more forwarding targets:

```bash
-e GOMESHCOM_FORWARD_TARGETS=192.168.1.60:1799,192.168.1.61:1799
```

Use this when one MeshCom node should feed multiple tools or additional `gomeshcomd` instances. The forwarder copies incoming UDP datagrams byte-for-byte before parsing them, so downstream services receive the same payloads that this instance received.

### Serial

UDP remains the default. To connect through USB serial instead:

```bash
GOMESHCOM_TRANSPORT_MODE=serial \
GOMESHCOM_SERIAL_DEVICE=/dev/ttyUSB0 \
./gomeshcomd --my-call="QQ0YY-1"
```

Default serial framing is `115200 8N1`, no flow control. Select modem lines for
your hardware:

```sh
# ESP32 / CP2102, typically /dev/ttyUSB0: avoid reset or bootloader entry
export GOMESHCOM_SERIAL_DTR=false
export GOMESHCOM_SERIAL_RTS=false

# nRF52 / RAK USB CDC, typically /dev/ttyACM0: expose active USB serial connection
export GOMESHCOM_SERIAL_DTR=true
export GOMESHCOM_SERIAL_RTS=false
```

See [Serial Transport](docs/serial.md) for TOML, platform, reconnect, terminal,
and Docker instructions.

If the serial connection drops, the HTTP UI remains available while the daemon
reconnects automatically. The UI shows `Serial unavailable`, and transmission
requests return `503 Service Unavailable` until the connection recovers.

When running in Docker, pass the host serial device into the container. For example:

```yaml
services:
  gomeshcomd:
    devices:
      - /dev/ttyUSB0:/dev/ttyUSB0
```

Set `GOMESHCOM_SERIAL_DEVICE=/dev/ttyUSB0` inside the container to match the
container-side device path.

### NETConsole

Before starting `gomeshcomd`, enable NETConsole and configure its password from
the node console:

```text
--netconsole on
--passwd random-secret
```

Then use the same password in the client configuration:

```bash
GOMESHCOM_TRANSPORT_MODE=netconsole \
GOMESHCOM_NETCONSOLE_ADDRESS=192.168.1.53:2323 \
GOMESHCOM_NETCONSOLE_PASSWORD=random-secret \
./gomeshcomd --my-call="QQ0YY-1"
```

Port `2323` is the firmware default. `GOMESHCOM_NETCONSOLE_PASSWORD` must match
the password configured with `--passwd`. Use an empty client password only when
the firmware is configured for open access. Password-protected sessions use the
firmware HMAC-SHA256 challenge-response exchange; subsequent NETConsole traffic
remains plaintext.

After every successful authentication, `gomeshcomd` automatically sends
`--setinfo on`; no manual client-side command is required. A working session
then receives packet records prefixed with `[EXT] Out` and `[EXT] Tele-Out`.

NETConsole supports broadcast, channel, and direct-message transmission. The
daemon reconnects automatically with bounded backoff after connection failures.
During an outage, the HTTP UI remains available, shows `NETConsole unavailable`,
and returns `503 Service Unavailable` for transmission requests.

NETConsole settings are available in the web Settings page. Password values are
masked, and fields controlled by environment variables are read-only. Transport
changes require a daemon restart.

See [NETConsole Transport](docs/netconsole.md) for TOML, authentication,
reconnect, security, and container instructions.

NETConsole traffic is plaintext. Use it only on a trusted LAN or through a VPN;
never expose port `2323` to the Internet.

### Progressive Web App

The web UI is installable as a Progressive Web App and can reopen its cached
application shell while offline. Installation and service workers require an
HTTPS domain, or `http://localhost` / `http://127.0.0.1`. Access through
`http://<lan-ip>:8080` remains available as a normal website but browsers do not
enable PWA installation or offline caching for that insecure origin.

API responses, live events, messages, positions, statistics, and remote map
tiles are never cached. See [Progressive Web App](docs/pwa.md) for installation,
offline behavior, update handling, and HTTPS deployment guidance.

### Optional Web UI Authentication

For shared LAN deployments, protect the UI and API with a username and password:

```bash
docker run -d \
  -p 8080:8080 \
  -p 1799:1799/udp \
  -v gomeshcom-data:/data \
  -e GOMESHCOM_STORAGE_SQLITE_PATH=/data/gomeshcom.db \
  -e GOMESHCOM_MY_CALL=QQ0YY-1 \
  -e GOMESHCOM_HTTP_ADDR=0.0.0.0:8080 \
  -e GOMESHCOM_AUTH_USERNAME=meshcom \
  -e GOMESHCOM_AUTH_PASSWORD=change-me \
  ghcr.io/logocomune/gomeshcom:latest
```

When authentication is enabled:

- unauthenticated protected API and SSE requests return `401 Unauthorized`; health and session status remain public
- the browser UI opens a sign-in modal
- successful login creates an HTTP-only session cookie
- active sessions survive server restarts until their configured TTL expires

Keep the browser on the same origin as the Go server, for example:

```text
http://192.168.1.50:8080
```

This allows REST and SSE requests to reuse the same session cookie.

## Features

- Installable PWA with an offline application shell on secure origins
- Responsive UI — works on desktop, tablet, and smartphone browsers
- Real-time packet stream via Server-Sent Events
- Chat per conversation: broadcast, channels, and direct messages
- Serial and NETConsole connection-state warnings with automatic reconnect
- Node map with OpenLayers: color-coded by freshness, with clustering for dense areas
- [Graph view](docs/graph.md) and map path overlays
- [Traffic statistics](docs/statistics.md), including per-destination DM acknowledgement counts
- SQLite persistence with one-time legacy JSON/JSONL migration
- [Broadcast time-beacon filter](docs/chat.md) and callsign/link actions
- Outgoing messages with duplicate suppression
- UDP datagram or extracted serial/NETConsole JSON forwarding to downstream listeners
- Local UDP simulator for MeshCom-compatible test packets
- Single compact binary, with no external runtime required
- Runs on Linux, Windows, macOS, and Raspberry Pi-class devices
- Multi-arch Docker image (`linux/amd64`, `linux/arm64`)

## Configuration

Configure the daemon through `data/configs/gomeshcomd.toml`, `GOMESHCOM_*` environment variables, or CLI flags. The web Settings page exposes configuration and restart requirements. See [Backend and Configuration](docs/backend.md) for precedence and persistence details.

| Variable | Default | Description |
|---|---|---|
| `GOMESHCOM_TRANSPORT_MODE` | `udp` | Node transport: `udp`, `serial`, or `netconsole`. Missing values preserve UDP behavior. |
| `GOMESHCOM_MY_CALL` | `QQ0XX-1` | Startup station callsign default, for example `IU5PMP-1` or `QQ1ABC-1`. Input is trimmed and uppercased. The active callsign can also be changed at runtime from the web UI and is persisted in SQLite. |
| `GOMESHCOM_NODE_ADDR` | *(empty)* | Node UDP address. When empty, it is learned from the first incoming UDP packet. When set, it is used as-is and auto-detect is disabled. |
| `GOMESHCOM_HTTP_ADDR` | `127.0.0.1:8080` | HTTP listen address |
| `GOMESHCOM_UDP_LISTEN_ADDR` | `0.0.0.0:1799` | UDP listen address |
| `GOMESHCOM_SERIAL_DEVICE` | *(empty)* | Explicit serial path or COM port; required in serial mode |
| `GOMESHCOM_SERIAL_BAUD` | `115200` | Serial baud rate |
| `GOMESHCOM_SERIAL_DATA_BITS` | `8` | Serial data bits |
| `GOMESHCOM_SERIAL_PARITY` | `none` | `none` \| `odd` \| `even` \| `mark` \| `space` |
| `GOMESHCOM_SERIAL_STOP_BITS` | `1` | `1` \| `2` |
| `GOMESHCOM_SERIAL_FLOW_CONTROL` | `none` | Serial flow control; currently `none` |
| `GOMESHCOM_SERIAL_DTR` | `false` | DTR state; enable for nRF52/RAK USB CDC |
| `GOMESHCOM_SERIAL_RTS` | `false` | RTS state |
| `GOMESHCOM_NETCONSOLE_ADDRESS` | *(empty)* | NETConsole TCP `host:port`; required in NETConsole mode |
| `GOMESHCOM_NETCONSOLE_PASSWORD` | *(empty)* | Optional NETConsole password; empty selects open access |
| `GOMESHCOM_NETCONSOLE_CONNECT_TIMEOUT` | `5s` | TCP connection timeout |
| `GOMESHCOM_NETCONSOLE_AUTH_TIMEOUT` | `5s` | Authentication exchange timeout |
| `GOMESHCOM_NETCONSOLE_WRITE_TIMEOUT` | `5s` | Per-write timeout |
| `GOMESHCOM_NETCONSOLE_RECONNECT_INITIAL` | `1s` | Initial reconnect delay |
| `GOMESHCOM_NETCONSOLE_RECONNECT_MAX` | `30s` | Maximum reconnect delay |
| `GOMESHCOM_NETCONSOLE_STABLE_RESET_AFTER` | `30s` | Healthy session duration before reconnect backoff resets |
| `GOMESHCOM_NETCONSOLE_MAX_AUTH_LINE_BYTES` | `128` | Maximum authentication line size; valid range `72`–`4096` bytes |
| `GOMESHCOM_NETCONSOLE_MAX_RECORD_BYTES` | `65536` | Maximum buffered NETConsole record size; upper limit `1048576` bytes |
| `GOMESHCOM_FORWARD_TARGETS` | *(empty)* | Comma-separated `host:port` list receiving each UDP datagram or extracted serial/NETConsole JSON payload |
| `GOMESHCOM_DATA_DIR` | `./data` | Persistent data directory |
| `GOMESHCOM_MAX_MESSAGE_LENGTH` | `149` | Maximum outgoing message length, in UTF-8 characters |
| `GOMESHCOM_LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error` |
| `GOMESHCOM_RECEIVE_LOG_ENABLED` | `true` | Store received packets in SQLite for replay |
| `GOMESHCOM_RECEIVE_LOG_RETENTION_DAYS` | `365` | Legacy setting; SQLite retention uses `GOMESHCOM_STORAGE_RECEIVE_LOG_RETENTION` |
| `GOMESHCOM_RECEIVE_LOG_REPLAY_WINDOW` | `1h` | Packets replayed on SSE reconnect |
| `GOMESHCOM_CHAT_LOG_HISTORY_WINDOW` | `24h` | Default chat history window |
| `GOMESHCOM_CHAT_LOG_MAX_HISTORY_WINDOW` | `720h` | Maximum chat history via API |
| `GOMESHCOM_STORAGE_SQLITE_PATH` | `./data/gomeshcom.db` | SQLite database path; set explicitly when relocating data |
| `GOMESHCOM_DEMO_MODE` | `false` | Disable TX and lock configuration writes |
| `GOMESHCOM_STATS_ENABLED` | `true` | Collect hourly traffic statistics |
| `GOMESHCOM_STATS_RETENTION_DAYS` | `30` | Hourly statistics retention; `0` keeps all buckets |
| `GOMESHCOM_REQUEST_LOG_ENABLED` | `false` | Enable structured HTTP request logs |
| `GOMESHCOM_COMPRESSION_ENABLED` | `true` | Enable gzip for eligible HTTP responses, excluding SSE |
| `GOMESHCOM_COMPRESSION_MINIMUM_SIZE` | `1024` | Minimum response size in bytes for gzip |
| `GOMESHCOM_STORAGE_PURGE_INTERVAL` | `4h` | Interval between SQLite purge runs |
| `GOMESHCOM_STORAGE_RECEIVE_LOG_RETENTION` | `720h` | SQLite receive-log row retention (`30d` in TOML) |
| `GOMESHCOM_STORAGE_PUBLIC_CHAT_RETENTION` | `720h` | SQLite public-chat row retention (`30d` in TOML) |
| `GOMESHCOM_STORAGE_NODES_RETENTION` | `168h` | SQLite node retention based on `lastseen` (`7d` in TOML) |
| `GOMESHCOM_STORAGE_TELEMETRY_RETENTION` | `720h` | SQLite telemetry row retention (`30d` in TOML) |
| `GOMESHCOM_SEND_DEDUP_TTL` | `2s` | Duplicate suppression window; `0` disables it |
| `GOMESHCOM_AUTH_USERNAME` | *(empty)* | Optional HTTP auth username. Must be set together with `GOMESHCOM_AUTH_PASSWORD`. |
| `GOMESHCOM_AUTH_PASSWORD` | *(empty)* | Optional HTTP auth password. Must be set together with `GOMESHCOM_AUTH_USERNAME`. |
| `GOMESHCOM_AUTH_SESSION_TTL` | `24h` | Session lifetime after successful login |
| `GOMESHCOM_AUTH_COOKIE_NAME` | `meshcom_session` | Session cookie name |

## Build from Source

```bash
# Requires Go 1.26+ and Node.js 24 (used by CI and Docker)
./build.sh
# Output: bin/
```

## Nightly Builds

Every push to `develop` publishes multi-architecture container image
`ghcr.io/logocomune/gomeshcom:nightly` for `linux/amd64` and `linux/arm64`.
Immutable `nightly-<commit-sha>` tags support rollback and debugging. See
[Nightly Builds](docs/nightly-builds.md) for retention, verification, and GHCR
permissions.

## Local UDP Simulation

Use the UDP simulator to feed changing position packets into a local `gomeshcomd` instance:

```bash
go run ./cmd/iot-simulator -my-call QQ5SIM-9
```

The simulator can send MeshCom-compatible `pos` packets from `QQ1TST-1` and `QQ1TST-2`, plus scheduled DM, broadcast, and channel-2 messages to `127.0.0.1:1799`, based on enabled flags.

Use any combination of these flags to enable timed sends:

```text
-enable-pos1
-enable-pos2
-enable-dm
-enable-broadcast
-enable-chan2
```

Without them, the simulator stays in receive-only responder mode.

It also responds to DMs for `QQ1TST-1` and `QQ1TST-2` with readable TX/RX logs.

See [UDP Simulator](docs/iot-simulator.md) for flags and examples.

## Disclaimer

> **Provided "as is", without warranty of any kind.**
>
> Use of this software for radio communications is subject to the regulations of your national telecommunications authority. The user is solely responsible for compliance with applicable laws and licensing requirements.
>
> `gomeshcom-client` is an independent, unofficial project.
>
> Not affiliated with, endorsed by, or maintained by the MeshCom project or its developers.

## License

See [LICENSE](LICENSE) for details.

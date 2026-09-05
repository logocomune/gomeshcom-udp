# NETConsole Transport

`gomeshcomd` can connect to a MeshCom ESP32 node through firmware NETConsole.
NETConsole mirrors the USB/debug console over a raw TCP byte stream and carries
the same ExtUDP JSON records used by the serial transport.

NETConsole is mutually exclusive with UDP and serial transport. UDP remains the
default for backward compatibility.

## Firmware Setup

NETConsole is available on supported ESP32 builds when networking is ready.
Enable and persist it from the firmware console:

```text
--netconsole on
```

Optionally set a password:

```text
--passwd random-secret
```

Firmware stores at most 14 password bytes. Printable ASCII is recommended.
Clear the password for open access:

```text
--passwd none
```

The default TCP port is `2323`. Current firmware requires Wi-Fi connectivity;
other IP transports must not be assumed to work.

## Daemon Configuration

```toml
transport_mode = "netconsole"

[netconsole]
address = "192.168.1.53:2323"
password = "random-secret"
connect_timeout = "5s"
auth_timeout = "5s"
write_timeout = "5s"
reconnect_initial = "1s"
reconnect_max = "30s"
stable_reset_after = "30s"
max_auth_line_bytes = 128
max_record_bytes = 65536
```

`address` accepts DNS names, IPv4, and bracketed IPv6 addresses. Include the
port:

```text
meshcom.local:2323
192.168.1.53:2323
[2001:db8::53]:2323
```

Equivalent minimum environment configuration:

```sh
export GOMESHCOM_TRANSPORT_MODE=netconsole
export GOMESHCOM_NETCONSOLE_ADDRESS=192.168.1.53:2323
export GOMESHCOM_NETCONSOLE_PASSWORD=random-secret
```

All NETConsole fields can be overridden with:

- `GOMESHCOM_NETCONSOLE_CONNECT_TIMEOUT`
- `GOMESHCOM_NETCONSOLE_AUTH_TIMEOUT`
- `GOMESHCOM_NETCONSOLE_WRITE_TIMEOUT`
- `GOMESHCOM_NETCONSOLE_RECONNECT_INITIAL`
- `GOMESHCOM_NETCONSOLE_RECONNECT_MAX`
- `GOMESHCOM_NETCONSOLE_STABLE_RESET_AFTER`
- `GOMESHCOM_NETCONSOLE_MAX_AUTH_LINE_BYTES`
- `GOMESHCOM_NETCONSOLE_MAX_RECORD_BYTES`

An empty password selects firmware open-access mode. Settings and
`GET /api/config` never return the real password: `****` means a password is
configured. Replace the mask to change it or clear the field to select open
access. The password is stored in plaintext in the local TOML file; restrict
access to the data directory and configuration file.

All NETConsole configuration changes require a daemon restart.

## Authentication

Password-protected firmware sends a 16-byte random nonce as 32 hexadecimal
characters. The daemon answers with lowercase HMAC-SHA256 using the configured
password. Open-access firmware sends `OK` directly.

Authentication reads are bounded and covered by a deadline. The client accepts
only these initial states:

```text
OK
NONCE: <32 hexadecimal characters>
```

Malformed challenges, unknown greetings, timeouts, EOF, and `FAIL` reject the
session and trigger reconnect backoff.

## Receive and Send

After authentication, the daemon sends:

```text
--setinfo on
```

This enables firmware ExtUDP console output:

```text
[EXT] Out: {"type":"msg",...}
[EXT] Tele-Out: {"type":"tele",...}
```

TCP packet boundaries have no framing meaning. The incremental console decoder
accepts LF and CRLF records, preserves records split across reads, ignores
banner/diagnostic/echo lines, and extracts bounded ExtUDP JSON objects. Payloads
enter the same parsing, persistence, SSE, chat, ACK, position, telemetry,
statistics, and optional UDP-forwarding flow used by serial and UDP.

Outgoing messages use firmware console commands:

```text
::Broadcast message
::{2321}Channel message
::{QQ1ABC-7}Direct message
```

Writes are serialized, CRLF-terminated, and bounded by `write_timeout`.

## Health and Reconnect

`GET /api/health` reports NETConsole endpoint and state:

```json
{
  "status": "degraded",
  "transport": {
    "mode": "netconsole",
    "state": "connecting",
    "endpoint": "192.168.1.53:2323",
    "last_error": "netconsole authentication rejected",
    "retry_count": 2
  }
}
```

Connect, authentication, read, write, and remote-close failures trigger capped
exponential reconnect with jitter. HTTP remains available. Sends return HTTP
`503` while no authenticated session exists. The browser displays
`NETConsole unavailable` with the latest error.

TCP keepalive assists dead-peer detection, but detection time for an idle
half-open connection remains operating-system dependent.

## Security

NETConsole is plaintext. HMAC prevents direct password transmission but does
not encrypt console output or commands, authenticate the server, or prevent a
network attacker from injecting post-login traffic.

Use NETConsole only on a trusted LAN or through an authenticated VPN/tunnel.
Never expose port `2323` to the Internet.

Current firmware versions may print sensitive NETConsole data through diagnostic
paths. The daemon does not log raw authentication or NETConsole stream bytes.
Use a random 14-byte password and update firmware when upstream security fixes
become available.

Firmware permits one authenticated NETConsole session. Close interactive
NETConsole clients before starting `gomeshcomd`.

## Platform and Container Support

NETConsole uses only Go standard-library TCP APIs and has no OS-specific code.
Linux, macOS, Windows, amd64, and arm64 use identical configuration.

Containers need normal IP reachability to the node; no USB device mapping is
required. On Linux, host networking or explicit LAN routing may be needed
depending on container network policy. Allow outbound TCP traffic to node port
`2323`.

## Troubleshooting

- `connection refused`: enable NETConsole, verify node Wi-Fi, address, port, and
  firewall.
- `authentication rejected`: password differs from firmware configuration.
- `authentication timeout`: listener accepted TCP but did not complete protocol;
  close another client and retry.
- repeated disconnects: verify Wi-Fi stability and that no second client claims
  the single firmware session.
- connected but no packets: verify firmware supports ExtUDP console records;
  daemon sends `--setinfo on` automatically after login.
- address changes after DHCP renewal: use a DHCP reservation or stable local DNS
  name.

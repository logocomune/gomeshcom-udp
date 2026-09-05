# Serial Transport

`gomeshcomd` can connect directly to a MeshCom node through its USB/debug
serial console. Firmware **4.35 or newer** is required. UDP remains the default,
so existing TOML files, environment variables, CLI flags, and deployments keep
their current behavior.

## Configuration

Use an explicit device path. Automatic serial-port discovery is intentionally
not supported.

```toml
transport_mode = "serial"

[serial]
device = "/dev/ttyUSB0"
baud = 115200
data_bits = 8
parity = "none"
stop_bits = 1
flow_control = "none"
dtr = false
rts = false
read_timeout = "1s"
reconnect_initial = "1s"
reconnect_max = "30s"
stable_reset_after = "30s"
max_record_bytes = 65536
```

`max_record_bytes` accepts values from `1` through `1048576` (1 MiB). The
default 65536-byte limit is suitable for normal ExtUDP records.

Equivalent minimum environment configuration:

```sh
export GOMESHCOM_TRANSPORT_MODE=serial
export GOMESHCOM_SERIAL_DEVICE=/dev/ttyUSB0
```

All serial fields can be overridden with:

- `GOMESHCOM_SERIAL_BAUD`
- `GOMESHCOM_SERIAL_DATA_BITS`
- `GOMESHCOM_SERIAL_PARITY`
- `GOMESHCOM_SERIAL_STOP_BITS`
- `GOMESHCOM_SERIAL_FLOW_CONTROL`
- `GOMESHCOM_SERIAL_DTR`
- `GOMESHCOM_SERIAL_RTS`
- `GOMESHCOM_SERIAL_READ_TIMEOUT`
- `GOMESHCOM_SERIAL_RECONNECT_INITIAL`
- `GOMESHCOM_SERIAL_RECONNECT_MAX`
- `GOMESHCOM_SERIAL_STABLE_RESET_AFTER`
- `GOMESHCOM_SERIAL_MAX_RECORD_BYTES`

The default frame is **115200 8N1**, with no flow control. The daemon writes
CRLF-terminated commands and sends `--setinfo on` after every successful port
open, enabling ExtUDP-compatible JSON output on the console.

## DTR and RTS

Modem-line behavior depends on the board:

| Hardware | DTR | RTS | Reason |
|---|---:|---:|---|
| ESP32 with CP2102, commonly `/dev/ttyUSB*` | `false` | `false` | Prevent EN/GPIO0 transitions, reset, or accidental bootloader entry |
| nRF52/RAK4630 USB CDC-ACM, commonly `/dev/ttyACM*` | `true` | `false` | Native USB serial normally requires DTR to expose an active connection |

These values cannot be inferred safely from a device path. Select them
explicitly in TOML or Settings. Verify new hardware locally before unattended
deployment because USB bridges and board wiring can differ.

## Sending Messages

MeshCom serial-console TX requires the `::` prefix:

```text
::Broadcast message
::{2321}Channel message
::{QQ1ABC-7}Direct message
```

- channels range from `1` through `99999`;
- direct-message callsigns are normalized to uppercase;
- direct messages to the active local callsign are rejected;
- CR, LF, and NUL inside message text are rejected;
- the firmware payload limit is enforced before writing;
- commands are emitted with CRLF termination.

The application does not add the firmware sequence suffix such as `{663}`.
Firmware adds it to the local JSON echo. That echo confirms the pending outbox
entry; later `ackNNN` or reject traffic follows the normal MeshCom ACK flow.

For manual terminal testing:

```sh
picocom -b 115200 /dev/ttyUSB0
```

Terminals should enable **Implicit CR in every LF**, or otherwise send CRLF.
Terminal applications may also change DTR/RTS at open or close; do not use one
that cannot preserve the board-specific states above.

## Receive, Reconnect, and Health

The serial reader accepts LF and CRLF records, ignores diagnostics, command
echoes, GPS, heap, and BLE lines, and extracts JSON only from:

```text
[EXT] Out: {...}
[EXT] Tele-Out: {...}
```

Optional `[HH:MM:SS]` capture prefixes are accepted. Extracted JSON enters the
same parser, persistence, event, chat, position, telemetry, statistics, and ACK
flow used by UDP. Configured UDP forwarding targets also receive each extracted
JSON payload.

Port loss, EOF, read failure, and write failure close the active session exactly
once and trigger capped exponential reconnect attempts. HTTP remains available.
`GET /api/health` reports `status: "degraded"` plus serial state and last error;
message sends return `503` while disconnected.
When browser remains connected to daemon during this condition, status bar shows
`Serial unavailable` beside `Connected` on desktop and beside logo on small
screens. Latest serial error is included when daemon provides one; small-screen
label truncates it to protect header layout. Status refreshes every five seconds.

At `info` level, logs report each connection attempt and successful connection.
Disconnects report the device, error, and next retry delay at `warn` level.
Set `log_level = "debug"` to log every raw serial read as `serial RX`, including
the received bytes and text.

## Linux Permissions

Native installations usually need membership in the group owning the device,
commonly `dialout`:

```sh
ls -l /dev/ttyUSB0
sudo usermod -aG dialout "$USER"
```

Log out and back in after changing group membership. Stable `/dev/serial/by-id/`
paths are preferred over enumeration-dependent `/dev/ttyUSB0` names.

## Docker

Map the exact device into the container:

```sh
docker run -d \
  --device=/dev/ttyUSB0:/dev/ttyUSB0 \
  -p 8080:8080 \
  -v gomeshcom-data:/data \
  -e GOMESHCOM_STORAGE_SQLITE_PATH=/data/gomeshcom.db \
  -e GOMESHCOM_MY_CALL=QQ0YY-1 \
  -e GOMESHCOM_TRANSPORT_MODE=serial \
  -e GOMESHCOM_SERIAL_DEVICE=/dev/ttyUSB0 \
  -e GOMESHCOM_SERIAL_DTR=false \
  -e GOMESHCOM_SERIAL_RTS=false \
  ghcr.io/logocomune/gomeshcom:latest
```

Compose equivalent:

```yaml
services:
  gomeshcom:
    devices:
      - /dev/ttyUSB0:/dev/ttyUSB0
    environment:
      GOMESHCOM_TRANSPORT_MODE: serial
      GOMESHCOM_SERIAL_DEVICE: /dev/ttyUSB0
      GOMESHCOM_SERIAL_DTR: "false"
      GOMESHCOM_SERIAL_RTS: "false"
```

Docker Desktop does not provide uniform USB serial pass-through on macOS and
Windows. Prefer a native daemon there, or configure the platform-specific USB
device forwarding layer before starting the container.

## Platform Support

Serial access uses `go.bug.st/serial`, supporting Linux, macOS, and Windows.
Typical explicit device names are:

- Linux: `/dev/ttyUSB0`, `/dev/ttyACM0`, or `/dev/serial/by-id/...`;
- macOS: `/dev/cu.usbserial-*` or `/dev/cu.usbmodem*`;
- Windows: `COM3`.

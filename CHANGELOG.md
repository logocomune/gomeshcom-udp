# Changelog

All notable changes to this project are documented in this file.

## [0.15.0] 2026-09-05

### Added

- **Progressive Web App**: secure origins can install goMeshCom with platform
  icons and reopen its cached application shell offline. API, SSE, message,
  position, statistics, and cross-origin requests remain network-only.
- **NETConsole node transport**: ESP32 nodes can connect through raw TCP
  NETConsole with strict open-access or HMAC-SHA256 authentication, automatic
  `--setinfo on`, bounded ExtUDP decoding, broadcast/channel/DM TX, reconnect
  backoff, transport health, and no new Go dependency.
- **NETConsole configuration and UI**: TOML, `GOMESHCOM_NETCONSOLE_*`,
  `GET`/`PUT /api/config`, Settings, and status warning expose endpoint,
  masked password, timeouts, reconnect controls, and connection state on Linux,
  macOS, and Windows.

### Changed

- **Project documentation**: add nightly-build publishing and retention guidance,
  expand the serial transport guide, and disclose the project's use of
  AI-assisted development in the README.

- **Documentation alignment**: correct SQLite persistence and statistics retention, remove obsolete send-delay configuration, document NETConsole in the backend guide, clarify configuration precedence, and fix LAN setup, Docker database persistence, and environment examples.

- **Shared console codec**: serial and NETConsole reuse one bounded ExtUDP
  decoder and one validated `::` command encoder.

### Fixed

- **NETConsole lifecycle**: interrupt blocked authentication on shutdown or
  restart, and reset reconnect backoff only after a stable connected session,
  excluding connection and authentication delays.

- **Frontend cache revalidation**: service worker, manifest, index, and unknown
  SPA routes now revalidate so browser-managed PWA updates remain discoverable.

### Removed

- **Standalone NETConsole prototype**: remove the obsolete root-level
  `netconsole_client.go`; daemon-integrated NETConsole transport replaces it.

## [0.14.0] 2026-07-30

### Added

- **Serial status warning**: UI now shows `Serial unavailable` beside
  `Connected` on desktop and beside logo on small screens whenever daemon reports
  a disconnected, reconnecting, degraded, or stopped serial transport; latest
  transport error is included when available.
- **Develop nightly container**: every `develop` push publishes multi-architecture `nightly` and immutable `nightly-<commit-sha>` GHCR images. Newer pushes cancel outdated builds and automatic cleanup retains the latest 14 immutable nightly versions.
- **Serial node transport**: firmware 4.35+ nodes can connect through an explicit Linux, macOS, or Windows serial device using `go.bug.st/serial`. Includes 115200 8N1 defaults, configurable DTR/RTS, `--setinfo on` session startup, bounded ExtUDP JSON decoding, broadcast/channel/DM TX, reconnect backoff, and transport health reporting.
- **Serial settings UI and API**: TOML, `GOMESHCOM_SERIAL_*` environment variables, `GET`/`PUT /api/config`, and Settings expose serial device, framing, modem lines, timeouts, and reconnect controls. UDP remains the backward-compatible default.

### Changed

- **Serial record safety limit**: serial ExtUDP records now have a maximum configurable size of 1 MiB.
- **Transport-neutral packet ingestion**: UDP and serial packets now share parsing, persistence, SSE, chat, position, telemetry, statistics, ACK, and optional UDP-forwarding flows.
- **Degraded serial operation**: HTTP remains available after serial disconnects; health reports degraded transport state and TX returns `503` until reconnection.
- **Serial receive diagnostics**: debug logging now records every raw serial read with device, byte count, and received text.
- **Transport-neutral stream UI**: stream status and controls now refer to packets rather than UDP, matching both serial and UDP transports.
- **Serial Docker setup**: README now documents passing the serial device into the container through Compose `devices`.

### Fixed

- **Nightly CI verification**: synchronize web lockfile, build embedded web
  assets before Go tests, install Chromium for browser unit tests, and track
  serial firmware capture fixture required by decoder tests.
- **Serial forwarding without targets**: serial packet ingestion now safely skips an absent UDP forwarder instead of panicking when a received serial packet is processed.
- **Immediate TX echo matching**: outgoing messages enter the outbox before transport write, preventing fast serial echoes from racing pending-message registration. Failed writes cancel the pending timeout.

## [0.12.0] 2026-07-28

### Added

- **Chat callsign mentions**: radio-amateur callsigns in public and direct-message text are underlined. Callsigns with numeric SSID, such as `@IU5PMP-12`, additionally open a direct-chat/Map menu; Map appears when coordinates are known.
- **Chat web links**: HTTP and HTTPS links in public and direct-message text are underlined and offer a `Follow link` action that opens a separate page.
- **Broadcast time-beacon filter**: Broadcast hides `{CET}` network-time beacons by default; `Show time beacons` reveals them without affecting other channels or direct messages.
- **Chat emoji picker**: chat composer now opens a wider, vertically scrollable modal with Smile, Animal, Places & Travel, Flags, and Objects tabs, and inserts selected emoji at cursor position.

### Fixed

- **Live node identity**: Nodes and node selectors keep displaying the origin callsign after live position packets instead of replacing it with the packet message ID. ([#3](https://github.com/logocomune/gomeshcom-client/issues/3))
- **Chat composer character counter**: counts Unicode runes, matching backend message validation.

## [0.11.0] 2026-07-22

### Added

- **Graph views**: added `/graph` and Map graph overlay for direct and relayed paths, distance labels, path summaries, freshness controls, zoom, pan, and hover highlighting.
- **SQLite runtime storage**: migrated nodes, received packets, chat history/read state, channel visibility, station identity, sessions, statistics, DM statistics, and telemetry from JSON/JSONL to SQLite. Existing data imports once at startup; WAL, foreign keys, retention purge, and telemetry history are included.
- **DM delivery metadata**: stores MeshCom sequence IDs, ACK/reject details, and relay paths; matching is limited to pending outbound DMs from the previous five minutes.

### Fixed

- **Altitude units**: converted ExtUDP altitude from feet to metres in traffic, map, and simulator documentation.
- **Documentation and OpenAPI**: removed obsolete and non-English documentation, added a documentation index, and aligned API contract with authentication, SQLite persistence, and chat metadata.

## [0.10.0] - 2026-06-17

- **Tests for `ChatStore` and `ToastStore`**: added unit tests covering `appendLiveChatRecord` (broadcast, channel, DM routing, deduplication, pending removal, ACK suppression), `appendChatRecord` (DM/mention toast and sound triggers, outbound/pending suppression, failed delivery cleanup), and `ToastStore` (`addDm`, `addMention`, `dismiss`, auto-dismiss timer).

- **Demo mode** (`demo_mode = false` top-level TOML, env `GOMESHCOM_DEMO_MODE`): when enabled, TX is disabled and the config API (`GET /api/config`, `PUT /api/config`) returns HTTP 403. The settings page shows a "Demo mode active" banner instead of the form. Replaces the former `[send] disable_tx` option.

- **DM stats in chat header**: when opening a direct message conversation, the header subtitle shows cumulative sent count and ACK percentage for that contact. It also shows average round-trip time when matching messages and ACK events are available in the loaded chat history (e.g. `direct · sent: 12 · ack: 83% · avg: 420ms`). Counters are fetched from `GET /api/stats/dm` when the conversation is selected and hidden when no messages have been sent yet.

- **DM statistics per callsign** (`GET /api/stats/dm`): tracks cumulative sent and ACK counters for each DM destination callsign. For destinations with a numeric SSID (e.g. `CALL-1`), both the full entry (`CALL-1`) and the base callsign (`CALL`) are updated. The first ACK for each message is counted; subsequent echoes of the same message are ignored by outbox deduplication. Counters are persisted to `data/stats/dm_stats.json` and survive restarts.

- **Process control endpoints**: `POST /api/restart` gracefully shuts down the HTTP server and UDP bridge, then either exits the process (Docker — container restart policy relaunches) or re-executes the same binary with identical args/env (standalone). `POST /api/shutdown` stops the daemon without restarting. Both require auth when auth is enabled. SIGHUP triggers the restart sequence without hitting the HTTP endpoint. Container detection uses `RUNNING_IN_DOCKER=1` env var (set in Dockerfile and docker-compose) with `/.dockerenv` / `/proc/1/cgroup` as a Linux fallback.

- **Interface settings page** (`/settings/ui`): stores browser-local UI preferences and provides direct-message sound and toast controls. Incoming DM notifications can play a two-tone alert and show a dismissible sender toast.

- **Nodes view — Chat button**: each node row now has a "Chat" button alongside "Map". Clicking it selects the node as DM target and navigates to `/chat`, opening the thread directly.

- **HTTP gzip compression middleware**: eligible API and static responses are transparently compressed when the client advertises `Accept-Encoding: gzip`. SSE (`/api/events`), range requests, already-encoded responses, non-compressible content types, and responses below 1 KiB bypass compression. `Content-Encoding: gzip` and `Vary: Accept-Encoding` headers are set when active. `Content-Length` is removed to allow chunked transfer. Controlled via `[compression]` TOML section (`enabled`, `minimum_size`) and `GOMESHCOM_COMPRESSION_*` env vars.

- **TOML configuration file** (`data/configs/gomeshcomd.toml`): all config fields now persist in a human-readable, commented TOML file. Load order is `built-in defaults < TOML file < env vars`. A missing file is written with defaults on first startup; an invalid file causes startup to fail with a clear error. Dependency `github.com/pelletier/go-toml/v2` added.

- **`GET /api/config`**: returns the effective configuration as a typed JSON response with `env_override` and `requires_restart` metadata per field. `auth.password` is always masked. Response now includes a `server` object with `version`, `started_at` (RFC 3339), and `uptime_seconds` fields.

- **`PUT /api/config`**: accepts a partial JSON patch, validates it, persists atomically to the TOML file, and returns the updated effective config. `my_call` changes apply live via `station.Identity.Update` and broadcast an SSE `station.identity` event. Fields managed by env vars return 409 Conflict. Duration fields are strings (e.g. `"40s"`).

- **Settings page** (`/settings` route): new UI page showing all configuration fields with `env` and `restart` badges. Editable fields save via `PUT /api/config`. Live-apply fields (my_call, log_level, send options) take effect immediately.

- **Settings — About card**: System tab now shows a read-only "About" card with server version, start timestamp, and uptime (formatted as `Xd Xh Xm Xs`). Values are sourced from the `server` object returned by `GET /api/config`.

- **HTTP response compression analysis**: added `docs/todos/10_http_response_compression.md`, defining gzip coverage for eligible API/static responses, mandatory SSE exclusion, negotiation, tests, benchmarks, and alternative algorithms.

- **Runtime `my_call` change** (`GET`/`PUT /api/adm/configs/my-call`): the active station callsign can now be updated at runtime without restarting the daemon. The new value is validated, persisted synchronously to `data/configs/station.json` before responding, and broadcast via the existing `station.identity` SSE event to all connected clients.

- **`internal/station` package**: concurrency-safe runtime identity component. `Identity.Current()` provides the live callsign; `Identity.Update()` normalizes, validates, and persists the change. Persisted value wins over the `GOMESHCOM_MY_CALL` config default on startup.

- **`internal/callsign` package**: shared callsign normalization (`Normalize`) and validation (`IsValid`, `Pattern`) extracted from the config loader. Fuzz-tested.

- **About-page callsign editor**: the "My call" field in the About view now shows an inline Edit button. Saving the form calls `PUT /api/adm/configs/my-call`; all DM classification, the header callsign display, and outbox source update live via SSE.

- **Frontend DM scope toggle**: chat list shows a "My / All" toggle in the Direct Messages section. "My" (default, mycall scope) shows only conversations addressed to the active SSID; "All" (basecall scope) aggregates all devices sharing the same base callsign. Switching scope refetches conversations and current history from the server.

- **Frontend scope-aware `chatStatus`**: SSE `chatstatus.snapshot` keys (always full-SSID) are aggregated in basecall scope — `unreadCount` = max, `lastMsgReceived`/`lastMsg` = most recent across all SSID entries for each peer.

- **`?scope=` param on frontend API calls**: `fetchConversations`, `fetchHistory`, and `markConversationRead` pass the active scope; `markConversationRead` in basecall scope zeroes all per-SSID unread counts.

- **`baseCallFrom` / `dmPeerFromId` helpers** exported from `$lib/api/chat` for scope-aware id construction and peer extraction.

- **DM scope-aware API**: `?scope=mycall` (default) and `?scope=basecall` query param on `GET /api/chat/list` and `GET /api/chat/{conversation}`. In `mycall` scope the list returns SSID-specific conversation ids (`DM_<mycall>_<peer>`) and filters records to those addressed to/from the active callsign. In `basecall` scope it returns the shared basecall file id (`DM_<basecall>_<peer>`) and all records. `POST /api/chat/{conversation}/read?scope=basecall` marks all per-SSID status keys for the peer as read.

- **`chatlog.BaseCall`**: exported helper that strips numeric SSID suffix (`IU5PMP-1` → `IU5PMP`).

- **`chatlog.StatusKey`**: returns the full-SSID-namespaced `msg_idx.json` key for a message, enabling per-device read tracking while history is shared.

- **`chatlog.FileIDForAPIID`, `chatlog.DMPeer`, `chatlog.RecordMatchesSSID`, `chatlog.Sanitize`**: exported helpers for scope-aware API plumbing.

- **`internal/legacymigrate` package** (temporary, removal planned): one-shot startup migration that renames legacy `DM_<peer>.jsonl` files to `DM_<basecall(mycall)>_<peer>.jsonl`, migrates `msg_idx.json` keys to the full-SSID form, and moves `data/channel_show.json` to `data/configs/channel_show.json`. Idempotent.

### Changed

- **Nodes view direct status**: the Nodes table now uses the same `lastDirectSeen` freshness rule as the map for the `direct` label, while the hop count and path columns continue to show the latest position packet path.

- **SQLite node saves are incremental**: position flushes now write only nodes changed since the previous successful save instead of upserting every known node, reducing write-lock duration during routine node persistence.

- **SQLite runtime persistence cleanup**: removed file-backed runtime repositories for HTTP sessions, receive log, chat history, chat read status, nodes, hourly stats, DM stats, channel visibility, and station identity. Startup still reads legacy JSON/JSONL files through one-time import and migration paths when the SQLite database is first created.

- **Interface settings hidden from nav**: `/settings/ui` route still accessible via direct URL but removed from the Settings submenu to reduce nav clutter.

- **Settings organization**: server settings are grouped into Server, Web Interface, Storage, and System sections. System controls expose graceful restart and shutdown actions.

- **Configuration cleanup**: removed the unused `send_delay` field from environment, TOML, API, and Settings UI surfaces.

- **Map event pulses color-coded by packet type**: `pos` → green `#34d399`, `tele` → orange `#f97316`, broadcast (`dst=*`) → amber `#f59e0b`, ACK → purple `#a855f7`, direct message → sky blue `#38bdf8`. Pulse duration extended from 2.2 s to 5 s (5 × 1 s animation). `CET/SET` system messages and numeric destinations remain suppressed.

- **DM trace TTL extended** from 45 s to 75 s (1 m 15 s).

- **Map color legend**: shown below the live stream ticker when DM tracking is active; lists pulse dot colors and dashed-line path colors. Removed the bottom-left "positions · OSM · Maidenhead" label.

- **Relay node pulse**: intermediate hops in `packet.src` flash yellow (`#facc15`) for 2 s when any packet transits through them.

- **`my_call` editor moved from About to Settings**: the About page now shows the active callsign read-only with a link to the Settings page.

- **`config.Load` signature**: now returns `(Config, EnvOverrides, string, error)`. The `EnvOverrides` map records which `GOMESHCOM_*` env vars are present and is used by the API to mark fields read-only.

- **Settings nav entry added**: Settings icon appears in `secondaryNavRoutes` (top bar and mobile drawer).

- **Documentation cleanup for runtime callsign**: README and backend/statistics docs now describe `GOMESHCOM_MY_CALL` as the startup default, runtime callsign updates via `PUT /api/adm/configs/my-call`, persisted `data/configs/station.json`, and current DM file naming.

- **Long-lived services use live callsign**: `udpbridge.Bridge`, `chatlog.Logger`, and `stats.Collector` now read the callsign from a `myCallSource` interface (`*station.Identity` in production) instead of capturing a string at construction time. DM file routing, echo suppression, and outbox source all reflect the newest accepted callsign without restart.

- **DM file naming**: new DM files are written as `DM_<basecall(mycall)>_<peer>.jsonl` (operator base callsign, e.g. `DM_IU5PMP_IK5FCK-10.jsonl`). Device switches (`IU5PMP-1` → `IU5PMP-2`) share the same history file; read-state is tracked separately per SSID in `msg_idx.json`.

- **`msg_idx.json` DM keys**: now `DM_<mycall-with-ssid>_<peer>` (e.g. `DM_IU5PMP-1_IK5FCK-10`) for per-device unread tracking.

- **`channelshow.DefaultPath`**: returns `data/configs/channel_show.json` instead of `data/channel_show.json`. `ensureDataDirs` now creates the `configs/` sub-directory.

- **Echo suppression in bridge**: compares by basecall so echoes from any SSID of the same operator are suppressed.

> **Planned removal**: `internal/legacymigrate` is a temporary migration aid. Delete it (and its call in `cmd/gomeshcomd/main.go`) in a future release after the migration window has closed.

### Fixed

- **Graph view readability on wide relay trees**: the graph now defaults to a two-hop view, adds 2 / 3 / all hop controls, and keeps large hop-3 fan-outs from shrinking labels into unreadable text.

- **SQLite busy handling**: runtime SQLite opens now use a single pooled connection and a five-second busy timeout, reducing intermittent `SQLITE_BUSY` failures when periodic stats, node saves, purge, or other stores write at the same time.

- **SQLite chat ACK storage**: ACK/reject packets now update only the most recent matching outbound DM row and preserve relay metadata in `ack_via`.

- **SQLite startup import ordering**: legacy file-layout migration now runs before SQLite imports so moved `channel_show.json`, renamed DM JSONL files, and migrated `msg_idx.json` keys are imported into the database on first startup.

- **SQLite node import compatibility**: missing node `via` values now persist as an empty JSON array instead of `null`, satisfying the SQLite schema constraint.

- **Chat message dedupe window**: backend history, frontend deduplication, and message-list render keys now treat matching msg_id values as duplicates only when they come from the same sender within five minutes, avoiding stale packet ID collisions hiding later messages.

- **Mention toast now has its own independent setting**: `dmToastEnabled` previously also gated `@mention` toasts in channels; split into `dmToastEnabled` (DM banners) and `mentionToastEnabled` (channel @mention banners), each with its own toggle in Settings → Notifications.

- **ACK/reject regex no longer matches embedded tokens**: strings like `ack123abc` or `CALL:rej42test` are no longer classified as control messages — added `\b` word-boundary after `\d+` in both patterns.

- **DM list preview no longer shows ACK text**: incoming ACK/reject packets are now excluded from the conversation-list "last message" preview and do not increment the unread counter — both on the server (`chatstatus.RecordIncoming` skipped via `meshcom.IsAckOrReject`) and on the client (`appendLiveChatRecord`, `appendChatRecord`, and `latestPreview`). ACK/reject records are still stored in history for delivery tracking and remain hidden in the thread view as before.

- **Chat/DM list preview not updating after sending a message**: the last-message preview in the conversation list now correctly shows the most recently sent message. Previously, once a message had been received on a conversation, the preview was locked to the last _received_ message and ignored any subsequent sent messages.

- **Mobile chat — Add DM not opening thread**: after confirming a new DM via the "Add DM" modal on mobile, the thread view was not shown. `ChatView` now reacts to external conversation target changes and auto-navigates to thread on mobile.

- **Web sessions survive server restarts**: authenticated sessions are persisted atomically in `data/http-sessions.json`. Only SHA-256 token hashes and expiration times are stored with `0600` permissions; expired sessions are rejected during startup and logout removals persist immediately.

- **ACK/REJ detection — no-space callsign format**: `messageKind`, `ackSeqId`, and `rejSeqId` failed to recognise ACK/REJ packets where the target callsign is joined directly to the ack marker without a space (e.g. `IU5PMP-10:ack353`). Some firmware versions omit the space; those packets displayed as regular chat messages instead of being silently filtered. Regex updated from `(?:^|\s):?ack\d+` to `(?:^|[:\s])ack\d+` to accept both forms.

- **ACK indicator missing in basecall scope**: in "All" (basecall) scope, ACK indicators were not shown on outgoing messages sent from a different SSID of the same operator (e.g., active callsign `IU5PMP-1`, message sent from `IU5PMP-10`). Two bugs combined: `isSent` in `ChatThread` used exact callsign comparison instead of base-call comparison, causing `sequenceId` to be null and short-circuiting all ACK lookup; and `entryMatchesRecord` in `acks.ts` compared the ACK target against the active callsign with exact match instead of base-call match. Both fixed: `isSent` now uses `baseCallFrom` in basecall scope; `ackEntriesForRecord`/`rejectEntriesForRecord` accept a `scope` parameter and compare base callsigns when `scope === 'basecall'`.

- **`/api/chat/list?scope=mycall` false-positive DM entries**: the endpoint now verifies the shared basecall `.jsonl` file contains at least one record where `Src` or `Dst` matches the active full SSID before including the conversation. Previously, a file written only by another device (e.g. `IU5PMP-2`) would appear in `IU5PMP-1`'s list.

- **Frontend live DM filter in basecall scope**: SSE packets addressed to another SSID sharing the same base callsign (e.g. `IU5PMP-2`) were silently dropped in `basecall` scope. The `appendLiveChatRecord` handler and `conversationIdForRecord` now compare against `baseCallFrom(myCall)` when scope is `basecall`.

## [0.9.0]

### Changed

- **Statistics — channel table**: all DM contacts aggregated into a single "DM" row instead of one row per callsign. Public channel labels now show `CH. N - Name` (name resolved from KNOWN_GROUPS when available).

- **Accessibility — text contrast**: replaced `text-ink-dim` with `text-ink-muted` on all readable text across 14 files (timestamps, labels, descriptions, empty states, metadata lines, section headers, form labels). `ink-dim` retained only for icon button idle states (non-text contrast, 3:1 threshold), `placeholder:` (WCAG exempt), decorative `·` separators, and small telemetry icon wrappers. Contrast for all readable text now ≥4.5:1 (AA) on all backgrounds.
- **UI consistency — Map**: `+page.svelte`, `MapEventTicker`, and `MeshMapPanel` UI chrome migrated to design tokens. Map wrapper: `rounded-2xl`, `bg-surface`, `border-ink-dim/20`. Toolbar buttons: `bg-surface hover:bg-surface-hi text-ink`, inactive icons `text-ink-dim`. Status bars and tooltip: `bg-surface/90`/`bg-base`, `border-ink-dim/20`, `text-ink/ink-muted/ink-dim`. Ticker: `bg-base/80 border-ink-dim/20`, sender `text-ink`, message `text-ink-muted`. OL cluster bubble updated to azure token (`rgba(96,165,250,0.9)`); message pulse ring updated to coral token (`rgba(248,113,113,…)`).
- **UI consistency — About & Credits**: both views migrated to design tokens. `bg-[#111827]`/`bg-[#1a2030]`/`bg-[#1c2230]` replaced with `bg-base`/`bg-surface`. All `gray-*`/`blue-*` replaced with `text-ink/ink-muted/ink-dim`, `text-azure`, `border-ink-dim/20`, `border-azure/30`. Card corners migrated to `rounded-2xl`. Link buttons use `bg-surface-hi hover:border-azure/50 hover:text-azure`.
- **UI consistency — Chat**: all chat components migrated to design tokens. `ChatView`, `ChatList`, `ChatThread`, `ChannelShowModal`, `NodeCombobox`, `ChannelCombobox`, and `AppShell` modals (deleteConfirm, newDm, newChannel, rawEvent JSON viewers) now use `bg-surface/surface-soft/base`, `text-ink/ink-muted/ink-dim`, `border-ink-dim`, `text-azure/mint/coral/warm`. ACK indicators use `text-mint`; errors use `text-coral`; TX warning uses `text-warm`. All modal corners migrated to `rounded-2xl`. Header callsign badge uses `azure`. Pending spinner uses `border-azure`. Dead `ChatPanel.svelte` deleted.
- **UI consistency — Traffic view**: `UdpStreamPanel` migrated to design tokens (`bg-surface`, `bg-surface-soft`, `text-ink/ink-muted/ink-dim`, `border-ink-dim`). Outer card now `rounded-2xl`. `packetTone` in `stream.ts` updated to use token colors (mint/azure/lavender/coral) so Traffic and Dashboard show identical colors for the same packet types. JSON button hover uses `azure` instead of raw blue.
- **UI consistency**: Statistics view migrated to the shared design token system (`bg-surface`, `text-ink`, `text-azure/mint/lavender/warm/coral`). Card corners now `rounded-2xl`, section headers use `tracking-[0.2em]`, range buttons use `bg-warm` active state. `BarChart` axis labels use `var(--color-ink-dim)` instead of hardcoded hex. KPI colors now match chart series colors (DM=azure, DM ack=mint, Public=lavender, Telemetry=warm, Position=coral). Removed Errors KPI card; added type icons to remaining cards (reusing `mdiEmailOutline`, `mdiCheckCircleOutline`, `mdiEmailMultipleOutline`, `mdiBroadcast`, `mdiMapMarkerRadiusOutline`).
- **Statistics auto-refresh**: data reloads every 60 s automatically; timer is cleared on component unmount.
- **Dashboard Statistics card**: added a fourth metric card linking to `/statistics`, completing the cross-navigation set (Nodes / Unread / Events / Statistics).

### Added

- **Statistics page** (`/statistics`): new frontend page showing packet traffic heard by the node — KPI cards and a stacked bar chart (messages per hour by type: DM received, DM ack, public, telemetry, position) plus a distance histogram (position packets bucketed by km from own station). Range selector: 6 h / 24 h / 7 d / 30 d.
- **`GET /api/stats?hours=`**: returns hourly aggregated counters. Default 24 h, max 720 h.
- **`internal/stats` package**: in-memory hourly bucket store, flushed every minute via atomic write to `data/stats/stats.YYYYMMDDHH.json`. Prunes files older than `GOMESHCOM_STATS_RETENTION_DAYS` (default 30). Classifies packets from the event bus using existing `chatlog.IsDM` logic. Computes haversine distance for position packets when own station position is known.
- **`message.delivered` bus event**: published on the SSE bus when an outgoing DM echo is confirmed, enabling DM-ack counting in the stats collector.
- **Config option `GOMESHCOM_STATS_ENABLED`** (default `true`) and **`GOMESHCOM_STATS_RETENTION_DAYS`** (default `30`).
- **`positions.Store.Get(callsign)`**: new thread-safe lookup by callsign.
- **`chatlog.IsDM(dst)`**: exported for use in the stats collector without duplicating classification logic.

## [0.8.0]

### Added

- **Map live event ticker**: on desktop (`md` breakpoint and above), a compact semi-transparent overlay appears in the bottom-left corner of the map showing the 5 most recent UDP stream events. Each row displays the receive time, an event-type icon (coloured by packet kind), and the sender callsign. Clicking a row calls `focusOnNode` to centre and pulse that node on the map.

## [0.7.0] - 2026-05-27

### Added

- **Channel visibility UI**: frontend now consumes the `channelshow.snapshot` SSE event on every connection and applies the preference to the channel sidebar. In `allowlist` mode, only the selected channels are shown; `all` mode (default) shows everything. Top-nav and mobile-drawer unread dot, and the Dashboard unread count, now exclude conversations hidden by the allowlist. A new gear icon next to "Add Channel" opens a modal with Show-all / Allowlist toggle, a searchable scrollable list showing flag + full channel name + numeric ID, removable selection chips, and a free-text input for unlisted IDs; Save persists via `PUT /api/channel-show`. When a channel is added via "Add Channel" and the current mode is `allowlist`, the new channel is automatically appended to the allowlist and persisted in the same PUT call.
- **Channel visibility backend**: added persistent `channel_show.json` preferences, `GET`/`PUT` `/api/channel-show`, and `channelshow.snapshot` on every `/api/events` connection. This setting is for frontend visibility only; backend receive logging, chat status, replay, and live SSE events remain unfiltered.
- **NodeCombobox component**: searchable autocomplete for the "New Direct Message" modal. Filters live `mapPositions` by callsign (↑↓ keyboard navigation, Enter to confirm, Escape to close dropdown then modal). Shows callsign and `lastSeen` time per suggestion. Zero new dependencies — pure Tailwind + Svelte 5 runes.
- **ChannelCombobox component**: searchable autocomplete for the "Join Channel" modal. Sources from `KNOWN_GROUPS` plus a `*` broadcast entry. Filters by channel number, country prefix, group name, or flag emoji. Renders flag + channel number + description per row.
- **"Join Channel" modal**: new modal (`chatState.newChannelOpen`) reachable from the Channels section header. Accepts `*` (broadcast) or any numeric channel ID. Validates input before navigating; reuses `chatState.selectChannel`.
- **Channels section header in chat list**: "Channels" label with an `+ Add Channel` button (blue pill) now appears above the channel list in `ChatList`. Replaces the implicit unlabelled channel block.
- **"Add DM" button relocated**: `+ Add DM` button (blue pill) moved from the full-width footer strip into the "Direct Messages" section header, adjacent to the label. Removes the bottom strip; button is now contextually placed.
- **Nodes view — Distance column**: when the browser grants geolocation access, a sortable `Distance` column appears in the Nodes table showing the Haversine distance between the user's current GPS position and each node that has reported a fix. Nodes without a position show `—`; the column is always last-sorted for null entries regardless of sort direction.
- **Frontend native routes**: Dashboard, Chat, Map, Nodes, Traffic, About, and Credits are now SvelteKit pages with real URLs and browser back/forward support.
- **Frontend SSE store/context**: root layout now owns one guarded SSE connection for the app lifetime and exposes shared app state/actions through Svelte context.
- **Chat status tracking**: backend now tracks per-conversation read/unread state. Each thread (broadcast channel, numeric channel, DM) records `UnreadCount`, `LastMsgReceived`, `LastRead`, and `LastMsg` (raw text of most recent inbound message). State is persisted to `<chat_path>/msg_idx.json` via atomic write (temp+rename) every minute when dirty, and reloaded on startup.
- **API: mark conversation read** (`POST /api/chat/{conversation}/read`): zeroes `UnreadCount` and sets `LastRead` for the given conversation. Returns 204. Requires authentication when auth is enabled.
- **SSE event `chatstatus.snapshot`**: injected into every new `/api/events` SSE connection after `positions.snapshot` and before the replay window. Payload is the full `map[conversationID]Entry` snapshot.

### Changed

- **Dashboard redesigned as summary view**: replaced the 3-panel layout (Chat + Map + UDP stream with drag resizers) with a lightweight read-only summary page. Shows connection status card, three metric cards (Nodes / Unread / Events — each a navigation shortcut to the dedicated view), recent messages preview (last 4 messages across all conversations), and recent traffic preview (last 5 stream events). Reduces cognitive load on app load; each panel is now accessible via its own dedicated route.
- **Frontend navigation**: header and mobile drawer now use SvelteKit links instead of internal view state, so route changes update the URL and keep the root SSE connection alive.
- **`DELETE /api/chat/{conversation}`**: now also removes the corresponding entry from the in-memory chat status store and immediately persists `msg_idx.json` to disk, so deleted conversations do not reappear as unread after restart.
- **Chat list resizable sidebar**: on large screens the conversation list sidebar in Chat view is now resizable by dragging the handle between it and the message thread. Width (px) persisted to `localStorage` key `meshcom:chatListWidthPx`; clamped to 160–520 px. Default 256 px.
- **Frontend chat status integration**: replaced localStorage-based unread tracking (`meshcom:chat:read:*`) with the backend-provided `chatstatus.snapshot`. Last-visited chat (`meshcom:chat:last`) is still persisted locally and restored on reload; falls back to Broadcast if the conversation no longer exists. On SSE connect the snapshot populates the reactive `chatStatus` store; opening a conversation issues `POST /api/chat/{id}/read` to keep backend and UI in sync; live `packet.received` messages increment the local counter for non-focused threads without waiting for a reconnect.
- **Frontend restyle plan**: design document at `docs/restyle.md` covering multi-view navigation (Chat / Map / Nodes / Traffic / Dashboard), desktop IRC-like collapsible sidebar, mobile WhatsApp-pattern chat list, new Nodes sortable table view, and global state store extraction from monolithic `+page.svelte`.
- **Global state stores**: extracted all reactive state from `+page.svelte` into `lib/stores/connection.svelte.ts`, `lib/stores/events.svelte.ts`, `lib/stores/chat.svelte.ts`, and `lib/stores/view.svelte.ts`. UI is identical; state is now accessible from any future view component.
- **App navigation shell**: desktop persistent sidebar (IRC-style, collapsible icons ↔ labels) and mobile slide-in drawer with Chat / Map / Nodes / Traffic / Dashboard views. Existing 3-panel layout preserved as the "Dashboard" view (default). View selection persisted to `localStorage`.
- **Chat view (WhatsApp pattern)**: new `ChatView` component with `ChatList` (conversation list with unread badge, last-message preview ~40 chars, relative timestamp, sorted by recency) and `ChatThread` (messages + composer). Mobile: shows list OR thread full-screen (tap → thread, back arrow → list). Desktop: sidebar list + thread side-by-side. Utility functions in `lib/ui/chat-list.ts` covered by 13 unit tests.
- **ChatList group enrichment**: public channels (numeric `dst`) now show the resolved group name and number (`Italy · 222`) in the conversation title and a flag emoji (🇮🇹) in the avatar circle instead of `#`. Channels and Direct Messages are rendered in two separate sections separated by a labelled divider. Unread count badge moved to avatar overlay (top-right) for both channels and DMs.
- **PixelAvatar component**: deterministic 8×8 pixel identicon for DM contacts, seeded from the callsign via FNV-1a hash with symmetric mirroring and HSL color derivation. Replaces the generic person icon in DM conversation rows.
- **Nodes view**: sortable table of mesh nodes derived from live map positions (callsign, last heard relative time, hop count, RSSI/SNR, source path). "Map" button switches to Map view. No backend changes required.
- **Top header navigation**: primary navigation moved from a collapsible left sidebar into the top header on desktop as icon-only tab buttons (Chat / Map / Nodes / Traffic) with a secondary cluster (About / Credits). Left sidebar removed; content area is now full-width. Mobile slide-in drawer unchanged. Dashboard removed from navigation (view component retained; accessible programmatically).
- **Dashboard view extraction**: 3-panel layout (Chat + Map + UDP stream with drag resizers) moved into `lib/components/views/DashboardView.svelte`; drag handler functions colocated with the component; `+page.svelte` reduced to routing shell.

### Fixed

- **`callerIP` header precedence**: replaced non-existent `X-Rela-IP` header with `X-Forwarded-For` (standard nginx/proxy header) and reordered to `CF-Connecting-IP` → `X-Forwarded-For` → `X-Real-IP`. Proxy IP detection was silently falling through to `RemoteAddr` when behind nginx.
- **`chatstatus` atomic write durability**: added `Sync()` before `Close()` in `writeFileAtomically` to match the same guarantee already present in `channelshow`. Prevents silent data loss on power failure between write and rename.
- **`PUT /api/channel-show` SSE propagation**: publishing a `channelshow.snapshot` event to the bus after a successful update so active SSE connections receive the new config immediately, without requiring a reconnect.
- **`writeJSON` error path**: replaced unreachable `http.Error` call (status already committed) with `slog.Error` so encode failures are logged rather than silently swallowed.
- **Dead code in `cloneConfig`**: removed unreachable `channels == nil` check after `make([]string, n)`, which is always non-nil.
- **Traffic UDP stream replay display**: the web UI now keeps every event delivered by `/api/events` instead of capping the in-memory UDP stream at 300 items, so fresh sessions show the full backend replay window.
- **UDP stream replay cursor**: fresh sessions and login resets no longer create `/api/events?from=<now>`; the replay cursor is saved only when the UDP stream clear action is used.

### Security

- **Request body size limits**: added `http.MaxBytesReader` guards on `POST /api/session` (1 KB), `POST /api/messages` (8 KB), and `PUT /api/channel-show` (64 KB) to prevent unbounded body reads.
- **Session store eviction**: added a background goroutine (5-minute ticker) to purge expired sessions from the in-memory store, preventing unbounded map growth on long-running instances.

### Tests

- `TestSessionStoreEvictExpiredRemovesOnlyStaleTokens` — eviction removes only expired tokens, valid tokens unaffected.
- `TestSessionStoreEvictExpiredClearsAllExpired` — all expired tokens purged in one pass.
- `TestSessionStoreStartStopsOnContextCancel` — eviction goroutine exits cleanly on context cancel.
- `TestSessionStoreEvictExpiredDoesNotRemoveJustCreated` — freshly created token survives eviction.
- `TestUpdateChannelShowPublishesSSEEvent` — PUT to `/api/channel-show` broadcasts `channelshow.snapshot` to SSE subscribers.
- `TestUpdateChannelShowRejectsTooLargeBody` — oversized PUT body returns 400.
- `TestCreateMessageRejectsTooLargeBody` — oversized POST `/api/messages` body returns 400.
- `TestCreateSessionRejectsTooLargeBody` — oversized POST `/api/session` body returns 400.
- `TestRequestLogUsesXForwardedForWhenCFHeaderMissing` — `X-Forwarded-For` used as fallback IP when `CF-Connecting-IP` absent.

---

## [0.6.4] - 2026-05-23

### Added

- **IoT simulator granular auto-send flags**: `cmd/iot-simulator` now exposes `-enable-pos1`, `-enable-pos2`, `-enable-dm`, `-enable-broadcast`, and `-enable-chan2` so each timed send stream can be enabled independently while DM responders remain active. All responder transmissions now use configured `-target` UDP endpoint.
- **UDP stream replay cursor**: `/api/events` accepts `from=<RFC3339 timestamp>` and the web UDP stream clear action stores that cursor in `localStorage`, clears visible packets, and reconnects SSE from that point.
- **Map ruler overlay**: default map now has a disabled-by-default ruler button that draws green `MyCall -> direct station` lines and prints distance labels along each line in kilometers.
- **Realtime DM route tracking**: map toolbar now includes a toggle button that draws dashed hop-by-hop DM routes (`src -> via -> dst`) for live direct messages and automatically removes each route after 45 seconds.

### Changed

- **Human-friendly log output**: replaced `slog.NewTextHandler` with a zero-dependency custom handler (`internal/logfmt`) that writes columnar `YYYY-MM-DD HH:MM:SS  LEVEL  message  key=value` lines. Both `gomeshcomd` and `iot-simulator` now emit this format; level is controlled via `-log-level` flag.
- **IoT simulator logging**: migrated `cmd/iot-simulator` from `fmt.Fprintf(os.Stderr, ...)` to structured `slog` calls with the new handler, consistent with `gomeshcomd`.
- **Chat message cards**: removed the raw JSON button from public and direct chat message cards.
- **DM ACK details**: direct-message chat cards now show every ACK source with its own RTT and relay path details instead of only the preferred ACK summary.
- **Event stream replay cursor capping**: `/api/events` now caps the `from` parameter to the configured `ReplayWindow` if `from` is further back in time.
- **IoT simulator command README**: documented local usage, responder behavior, common run modes, flags, and log output for `cmd/iot-simulator`.
- **Web UI helper refactoring**: extracted `ChatPanel`, `UdpStreamPanel`, and pure chat record/UDP stream presentation helpers from the monolithic `+page.svelte`, added unit coverage for those helpers, and documented the next component extraction slices.

### Fixed

- **Goroutine/subscription leak in HTTP server**: watch goroutines in the server now correctly unsubscribe and terminate on Close/Shutdown, resolving resource leaks in runtime and tests.
- **Realtime DM trace for ACK packets**: map live tracking now keeps `msg` ACK/reject packets in route tracing, so packets like `src=IU5RTR-02,IZ5CND-10` and `dst=IU5PMP-1` render both hop segments for 45 seconds.
- **Sanitized amateur radio callsigns**: audited and updated all mock/example/placeholder amateur radio callsigns to use compliant "QQ" prefix format across simulator commands, frontend Svelte pages, test files, and API docs.
- **DM ACK scoping**: ACK and reject indicators now match the sent message destination and local callsign, preventing ACKs for different messages with the same sequence number from appearing on the wrong chat card.
- **Replay packet filtering for chat/ACK UI**: frontend ACK indexing now ignores `packet.received` SSE events with `replay:true`, so replay bursts are not counted as extra ACKs on latest chat messages.
- **ACK timing**: packet SSE events and chat JSONL records now share the same backend `received_at` timestamp, and the web client uses backend time for chat and ACK RTT instead of browser arrival time.
- **Position signal freshness**: direct `msg`, `tele`, and `pos` packets without `rssi`/`snr` now preserve existing node signal values instead of overwriting them with `0`.
- **HTTP response caching**: all `/api/*` responses now send no-store cache headers, `/_app/immutable/*` assets use one-year immutable caching, and `index.html` requires revalidation.
- **Broadcast clear backend deletion**: the web UI now always sends the delete request when clearing the Broadcast chat so backend chat log files are removed even if local history state is empty.
- **DM send echo matching**: pending outbound DM records are now removed when the node echo appends a truncated sequence suffix such as `{42`, preventing duplicate spinner records.

---

## [0.5.0] - 2026-05-18

### Added

- **Chat message filter**: the web chat header now includes a filter field beside the delete/clear action so operators can search visible messages by text, source, destination, or message type.
- **About page reference repository**: the web About page now links the upstream reference repository and shows the `github.com` domain alongside existing GitHub issue reporting.
- **Persistent failed send status**: outbound chat messages appear immediately in the web chat with a pending spinner. After the accepted message is written to UDP, the backend waits up to 5 seconds for the node echo. If no echo arrives, it persists the message with `delivery_status:"failed"` and emits a `message.failed` event so the web chat shows a red `X` that survives reloads.
- **TX dry-run mode** (`GOMESHCOM_SEND_DISABLE_TX=true`): suppresses all outbound UDP writes. Each message that would have been sent is logged at `WARN` level with its JSON payload. The web UI shows a persistent amber banner and disables the send composer so operators immediately see that TX is disabled. Useful for monitoring-only deployments.
- **Responsive mobile layout**: the web UI adapts to narrow viewports (< 768px). On phones, Chat, Map, and UDP stream panels stack vertically. Drag handles are hidden on mobile, status indicators collapse to compact variants, chat typography shrinks slightly, and UDP stream rows hide secondary fields.
- **Chat sidebar collapse**: the chat channels column now has a header button that shrinks it into a narrow left rail so the message pane gets more horizontal space. The collapsed state persists in `localStorage`.
- **Collapsed `New DM` button**: when the chat sidebar is collapsed, the `New DM` action shortens to `DM +` to save space in the narrow rail.
- **Mobile collapsed chat rail**: when the chat sidebar is collapsed on small screens, the rail stays on the left of the message pane instead of stacking above it.
- **Configurable HTTP request logging**: `GOMESHCOM_REQUEST_LOG_ENABLED=true` logs structured request records with endpoint, status, caller IP, timestamp, and duration. Caller IP prefers `CF-Connecting-IP`, then `X-Real-IP`.
- **Remember last chat**: the web UI stores the last selected chat in `localStorage` and restores it on restart. If that conversation no longer exists, it opens Broadcast.
- **UDP RX forwarder** (`GOMESHCOM_FORWARD_TARGETS=host:port,...`): mirrors every received UDP datagram byte-for-byte to one or more downstream `gomeshcomd` instances. Enables multi-instance topologies where a single node feeds several processing nodes. Forwarding is best-effort (per-target buffered channel, drop-oldest on overflow) and happens before parsing so even malformed packets are mirrored.
- **`udpsend` tool** (`cmd/udpsend`): CLI utility to send a single UDP datagram to a `host:port`. Accepts payload as UTF-8 string (`-payload`) or hex string (`-hex`). Useful for manual testing and scripting.
- **`udprecv` tool** (`cmd/udprecv`): CLI utility to listen on a UDP address and print each received datagram with RFC3339Nano timestamp, source address, and byte count. Output is either quoted string (default) or hex dump (`-hex` flag). Configurable receive buffer via `-buf`.

### Changed

- Map marker clicks no longer open the station detail card; station `firstSeen` and `lastSeen` now appear directly in the hover tooltip.
- The local `MyCall` map marker hover now shows only callsign and device name, since station freshness metadata is not useful for the own marker.
- Web public/channel chat history now requests 7 days, while `DM_<CALLSIGN>` leaves the window unset so the backend's DM default applies.

### Fixed

- Outgoing message echo matching now accepts truncated numeric node sequence suffixes such as `{571`, preventing valid node echoes from being marked as failed.
- Public and channel chat sends now show a green cloud after the local node echo is observed, matching the existing failed-send indicator behavior.
- Web DM history requests no longer send a public/channel history window, allowing the backend's DM default window to apply.
- Direct-node hover and details keep showing `rssi`/`snr` after live `msg`/`pos` updates that omit those fields. Live freshness merges now preserve existing signal values instead of clearing them with `undefined`.
- Indirect `pos` packets now refresh `lastSeen` on every hop in the `via` chain. Signal values stay attached to direct packets and the last relay hop only.

---

## [0.4.2] - 2026-05-16

### Added

- First setup guide for MeshCom LAN deployment, including node IP discovery, ExtUDP destination configuration, firewall requirements for UDP `1799`, restart note, and startup examples.

### Fixed

- **Map — tooltip missing on standalone nodes**: hovering over a single node (not part of a cluster bubble) showed no tooltip. The `pointermove` handler only handled cluster features (which carry a `features[]` array); raw marker features (with a direct `position` property) were silently ignored. Both the hover tooltip and the click-to-select panel are now fixed to handle both feature types.

### Changed

- README quick start now links to the dedicated first setup guide and keeps the top-level setup overview concise.
- First setup guide now notes that public-IP deployments are possible but require extra routing and firewall care, and it clarifies when to bind the web UI to `0.0.0.0:8080` or a specific host IP.
- First setup guide now states that the MeshCom node must be connected to Wi-Fi before reading its IP or applying ExtUDP settings.

---

## [0.4.1] - 2026-05-16

### Fixed

- `crypto.randomUUID` not available in non-secure contexts (plain HTTP): SSE event ID generation now falls back to a `Date.now` + `Math.random` based ID when the Web Crypto API is unavailable.

---

## [0.4.0] - 2026-05-16

### Added

- **Map — node clustering**: stations closer than 30 px are grouped into a bubble showing the count; individual markers still visible for groups of 3 or fewer. Toggle button in the map controls; state persists across reloads.
- **Map — own callsign marker**: the local station (`MyCall`) is displayed as a red marker rendered above all others.
- **Map — label toggle**: button to show or hide callsign labels on markers; state persists across reloads.
- **Optional HTTP auth**: when `GOMESHCOM_AUTH_USERNAME` and `GOMESHCOM_AUTH_PASSWORD` are set, protected API and SSE endpoints require login and the web UI presents a sign-in modal. Successful login creates an HTTP-only session cookie.
- **NodeAddr auto-detection**: when `GOMESHCOM_NODE_ADDR` is not configured, the node address is inferred from the source of the first valid incoming UDP packet. Explicit configuration always takes priority and is never overridden.
- `POST /api/messages` returns `503 Service Unavailable` with `{"error":"node not yet detected"}` when no node address is configured and no UDP traffic has been received yet.

### Changed

- `GOMESHCOM_NODE_ADDR` defaults to empty — auto-detection is now the default behaviour.
- Maidenhead grid overlay defaults to off.
- Map: day/night zone overlay removed.

### Fixed

- DM conversations are now keyed on the interlocutor's callsign so both the sent and received sides of a thread appear as a single conversation. Previously, incoming messages (`dst=MyCall`) and outgoing messages (`src=MyCall`) landed in separate entries, one of which was labelled with the local callsign.
- The chat sidebar no longer creates DM entries for conversations between other stations that do not involve the local callsign.
- Duplicate chat records sharing the same message ID are suppressed both at read time (backend) and on live SSE updates (frontend).

---

## [0.3.0] - 2025-05-01

### Added

- **Web dashboard**: real-time map of heard stations with freshness colour coding — green (direct, ≤30 min), blue (relayed or direct >30 min, ≤1 h), gray (1–48 h); nodes silent for more than 48 h are hidden.
- **Map tooltips**: callsign, relative age, RSSI, SNR, altitude, battery, coordinates, and hardware device name (e.g. "T-Beam", "Heltec V3") when available.
- **Map controls**: Maidenhead grid overlay, marker label toggle, clustering toggle; all states persist in `localStorage`.
- **Chat panel**: broadcast channel, group channels, and direct messages. Per-conversation history loaded from disk on switch. Unread indicators (green dot + bold label) cleared on visit; read timestamps persisted in `localStorage`.
- **Message send**: send to broadcast, a channel, or a callsign with a loading indicator. Inline error banner on failure; duplicate-message notice on `429`.
- **ACK indicators**: LoRa acknowledgement (`✓✓`) and gateway acknowledgement (`☁️`) shown on outgoing messages, including group channel fan-out.
- **Delete / clear**: trash icon in the chat header deletes a channel or DM conversation (`DELETE /api/chat/{id}`); for broadcast it clears messages while keeping the entry. Modal confirmation prevents accidental deletes.
- **Persistent chat logs**: per-conversation JSONL files under `data/chat/` (`P_broadcast.jsonl`, `P_<channel>.jsonl`, `DM_<callsign>.jsonl`). Configurable history window (default 24 h, max 720 h).
- **Position store**: incoming `pos` packets are persisted to `data/nodes/positions.json` with relay-path (`via`) tracking. Freshness attribution propagated to the last relay hop for relayed packets.
- **SSE stream** (`GET /api/events`): snapshot on connect, configurable replay of recent packets (default 6 h), live events.
- **REST API**: `GET /api/chat/list`, `GET /api/chat/{id}?hours=N`, `DELETE /api/chat/{id}`, `GET /api/positions`, `GET /api/health`, `POST /api/messages`.
- **Single-binary deployment**: SvelteKit frontend embedded in the Go binary via `embed.FS`; SPA client-side routing fallback included.
- **Docker image**: multi-stage, distroless, multi-platform (linux/amd64, linux/arm64, linux/arm/v6); `/data` volume for runtime state.
- **Release pipeline**: GoReleaser producing binaries for Linux (amd64 / arm64 / armv6), macOS, Raspberry Pi, and Windows.

### Fixed

- MeshCom packet parsing handles `firmware`, `hw_id`, and `batt` fields sent as JSON numbers instead of strings.
- SSE `packet.received` events carry the `type` field so the frontend correctly routes `msg`, `pos`, and `tele` packets.

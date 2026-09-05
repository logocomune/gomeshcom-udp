# Statistics

The Statistics page (`/statistics`) shows hourly packet traffic received by the local node.

## Storage

Runtime statistics live in SQLite, in hourly UTC buckets. When a database is created from an existing installation, `data/stats/stats.json` is imported once. The JSON file is not updated afterwards.

Each bucket records received direct messages, delivered DM acknowledgements, public messages, telemetry, positions, parse errors, totals, channel counters, and five-kilometre distance buckets.

`total` contains DM, public, telemetry, and position packets; parse errors are tracked separately.

## API

```text
GET /api/stats?hours=N
```

`hours` defaults to `24` and is capped at `720`. The response contains UTC buckets in ascending order.

`GET /api/stats/dm` returns cumulative sent and acknowledged direct-message counters by destination callsign. A destination with a numeric SSID contributes to both its full callsign and base callsign totals.

## Retention

`GOMESHCOM_STORAGE_PURGE_INTERVAL` controls maintenance frequency. `GOMESHCOM_STORAGE_TELEMETRY_RETENTION` controls telemetry storage, while `GOMESHCOM_STORAGE_PUBLIC_CHAT_RETENTION`, `GOMESHCOM_STORAGE_RECEIVE_LOG_RETENTION`, and `GOMESHCOM_STORAGE_NODES_RETENTION` control their respective datasets. Hourly statistics use `GOMESHCOM_STATS_RETENTION_DAYS` (TOML `stats.retention_days`), default `30`. Expired buckets are pruned during the statistics flush cycle; `0` disables this pruning. Cumulative DM counters are separate from hourly bucket retention.

## Distance Buckets

Distance uses the Haversine great-circle distance between a sender position and the active station position. Buckets are five kilometres wide; distances above 100 km are grouped into `100+`. No distance is recorded until the active station has a known position.

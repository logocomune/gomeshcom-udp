# Documentation

The guides below describe current implemented behavior. Implementation checklists and pending manual acceptance checks live separately under `todo/`; they are not usage guides.

## Getting Started

- [First Setup](first-setup.md): configure a MeshCom node to send EXT UDP traffic to `gomeshcomd`.
- [Serial Transport](serial.md): firmware requirements, serial settings, DTR/RTS behavior, reconnects, and Docker device mapping.
- [NETConsole Transport](netconsole.md): TCP setup, HMAC authentication, reconnects, security, and cross-platform operation.
- [Nightly Builds](nightly-builds.md): image tags, supported architectures, retention, verification, and GHCR permissions.
- [Backend and Configuration](backend.md): configuration precedence, persistence, HTTP API, and operational behavior.
- [OpenAPI Contract](openapi.yaml): machine-readable HTTP API specification.

## Features

- [Progressive Web App](pwa.md): installation, secure-origin requirements, offline shell, and cache boundaries.
- [Graph View](graph.md): graph page and Map graph overlay behavior.
- [Statistics](statistics.md): hourly traffic aggregates and retention.
- [Chat Status Tracking](chat_status.md): unread state and conversation IDs.
- [Chat Text Actions](chat.md): callsign and web-link actions in chat messages.
- [IoT UDP Simulator](iot-simulator.md): local packet simulator flags and behavior.

## MeshCom Reference

- [UDP Message Format](message-udp.md)
- [Serial Message Format](message-serial.md)
- [Public Groups](groups.md)
- [Hardware IDs](hardware-ids.md)

External protocol details are documented only when they describe functionality this project currently supports. Refer to the upstream MeshCom firmware for device configuration and protocol authority.

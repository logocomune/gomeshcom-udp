# Progressive Web App

The goMeshCom web UI is an installable Progressive Web App (PWA). It can cache
its application shell so the interface can reopen when the server or network is
temporarily unavailable.

## Secure Origin Requirement

Browsers enable PWA installation and service workers only for secure origins:

- an HTTPS domain, such as `https://mesh.example.com`;
- `http://localhost` or `http://127.0.0.1`, with any port.

Opening `http://<lan-ip>:8080` from another device remains supported as a normal
website. Browsers do not enable PWA installation or offline caching for that
insecure LAN origin. If remote installation is required, expose the existing Go
HTTP server through an HTTPS reverse proxy or another TLS-terminating service.

## Installation

Visit goMeshCom from a supported secure origin, then use the browser's install
action. Depending on the platform, this can appear as **Install app**, **Add to
Home Screen**, or **Add to Dock**. goMeshCom does not display a custom install
prompt.

The first online visit installs and fills the application cache. Later launches
can reopen the cached shell while offline. A newly deployed service worker waits
until existing goMeshCom tabs close before taking control, avoiding an
unexpected reload during an active radio session.

## Offline Boundaries

Offline mode provides only the interface shell and bundled static assets. The
following resources always require the network and are never stored by the
service worker:

- REST API responses and authentication state;
- Server-Sent Events and live packet updates;
- messages, statistics, node positions, and remote map tiles;
- outgoing messages and other write operations.

When the backend is unavailable, the existing connection overlay reports the
disconnected state. goMeshCom does not queue messages for later transmission.

## Updates and Cache Policy

Versioned SvelteKit assets use cache-first loading. Page navigations try the
network first and fall back to the cached application shell. API requests,
non-GET requests, and cross-origin resources bypass the application cache.

The service worker removes only obsolete caches whose names start with
`gomeshcom-`. The server serves the service worker, manifest, index, and SPA
fallback with revalidation headers so deployments remain discoverable.

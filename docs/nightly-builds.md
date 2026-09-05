# Nightly Builds

Every push to the `develop` branch builds and publishes a multi-architecture
container image for `linux/amd64` and `linux/arm64`.

Use `ghcr.io/logocomune/gomeshcom:nightly` for the newest successful build.
Each build also receives an immutable `nightly-<commit-sha>` tag for rollback
and debugging.

The workflow cancels an older in-progress run when a newer `develop` push
arrives. After publishing, it retains the newest 14 immutable nightly image
versions and deletes older ones. The moving `nightly` tag is never deleted.

Verification installs dependencies from the committed lockfile and Chromium for
browser unit tests, then builds the web UI before Go tests so embedded
`internal/webui/dist` assets required by the Go binary are present.

The repository that runs the workflow needs `admin` access to the GHCR package
for cleanup. Images first published by this repository receive it
automatically. If cleanup returns `403`, grant the repository the `Admin` role
under the package's **Package settings** > **Manage Actions access**.

Git tags matching `v*` remain reserved for stable releases.

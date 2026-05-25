# iabridge

Lightweight self-hosted frontend for [archive.org](https://archive.org).  
Search the archive, browse item pages, and send downloads to a local qBittorrent instance or the `ia` CLI — all from a single, dependency-free Go binary.

**Features**
- Search across all mediatypes; audio results show cover thumbnails when enabled
- Item detail pages: sanitized HTML description, clickable topic/subject tags, creator search links
- One-click torrent send to qBittorrent (save path dropdown, no free-text input)
- `ia download` subprocess with live progress display
- Downloads history page: bulk clear from history or delete files from disk
- Config and downloads pages are LAN-only (`ALLOWED_CIDR`)

---

## Requirements

- Go 1.22+
- [internetarchive CLI](https://github.com/jjjake/internetarchive) (`pip install internetarchive`)
- qBittorrent with Web UI enabled (`Tools → Preferences → Web UI`)

---

## Installation

### 1. Build

```bash
# Local machine
go build -o iabridge ./cmd/iabridge

# ARM64 (e.g. Raspberry Pi 4)
GOOS=linux GOARCH=arm64 go build -o iabridge-arm64 ./cmd/iabridge
```

### 2. Configure

```bash
cp config.env.example config.env
$EDITOR config.env
chmod 600 config.env   # contains plaintext credentials
```

Minimum required fields:

```env
ALLOWED_CIDR=192.168.1.0/24          # your LAN subnet
QBITTORRENT_URL=http://localhost:8080
QBITTORRENT_USER=admin
QBITTORRENT_PASS=changeme
QBITTORRENT_SAVE_PATHS=/downloads    # comma-separated; each must exist on disk
```

See [config.env.example](config.env.example) for all options.

### 3. Run

```bash
CONFIG_PATH=/path/to/config.env ./iabridge
CONFIG_PATH=/tmp/config.env /usr/local/bin/iabridge
# Listening on :8090 by default
```

Open `http://localhost:8090` in your browser.

---

## Deployment (systemd)

Copy the binary and config to the server, then install the service:

```bash
# Copy binary
scp iabridge-arm64 user@your-server:/usr/local/bin/iabridge

# Copy and secure config
scp config.env user@your-server:/etc/iabridge/config.env
ssh user@your-server "chown nobody /etc/iabridge/config.env && chmod 600 /etc/iabridge/config.env"

# Install and start the service
sudo cp deploy/iabridge.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now iabridge
```

The service runs as `nobody` with a hardened systemd unit (`NoNewPrivileges`, `PrivateTmp`, `ProtectSystem=strict`).

**Do not expose port 8090 to the internet.** The config and downloads pages are protected by LAN-only middleware (`ALLOWED_CIDR`), but the search and item pages are not authenticated.

---

## Development

### Run locally

```bash
# Copy and edit the example config
cp config.env.example config.env

# Run directly (auto-reloads are not built in; restart manually after changes)
go run ./cmd/iabridge
```

### Build and test

```bash
go build ./...
go vet ./...
go test -race ./...
```

### Project layout

```
cmd/iabridge/        main package
internal/
  archive/           archive.org API client
  config/            config file loading and validation
  downloads/         ia subprocess management and history store
  handler/           HTTP handlers, embedded templates, SVG icons
    tmpl/            one HTML file per page
    static/icons/    mediatype SVG icons
  middleware/        LANOnly middleware
  qbittorrent/       qBittorrent Web UI API client
deploy/              systemd service unit
config.env.example   annotated configuration template
```

### Development phases

| Phase | Status | Scope |
|-------|--------|-------|
| 1 | ✅ Done | Landing page + search results |
| 2 | ✅ Done | Detail page, download buttons, downloads history, cover thumbnails in search |
| 3 | — | Detail page refinements for all other mediatypes |

Phase 3 extends the `/details/{identifier}` page with mediatype-specific layout and metadata. Each phase must be manually tested before starting the next.

# iabridge

> **This project is fully vibe coded.** It was built entirely with AI assistance and has not been audited. Use at your own risk.

Lightweight self-hosted frontend for [archive.org](https://archive.org). Search the archive, browse item pages, and send downloads to a local qBittorrent instance or the `ia` CLI — all from a single Go binary with no external dependencies. All archive.org API calls are proxied server-side.

---

## Features

- Search across all mediatypes; audio results show cover thumbnails when enabled
- Item detail pages: sanitized HTML description, clickable topic tags, creator search links
- One-click torrent send to qBittorrent (save path dropdown, no free-text input)
- `ia download` subprocess with live progress display
- Downloads history: bulk clear from history or delete files from disk
- Downloads page is LAN-only (`ALLOWED_CIDR`)

---

## Requirements

- Go 1.22+ *(build machine only)*
- [internetarchive CLI](https://github.com/jjjake/internetarchive) on the target host
- qBittorrent with Web UI enabled *(only needed for torrent sending)*

---

## Configuration

Config is read from a `config.env` file. Set `CONFIG_PATH` to override the location (default: `./config.env`).

| Key | Required | Default | Description |
|---|---|---|---|
| `ALLOWED_CIDR` | yes | — | LAN subnet allowed to access the downloads page (e.g. `192.168.1.0/24`) |
| `QBITTORRENT_URL` | yes | — | qBittorrent Web UI URL |
| `QBITTORRENT_USER` | yes | — | qBittorrent username |
| `QBITTORRENT_PASS` | yes | — | qBittorrent password |
| `QBITTORRENT_SAVE_PATHS` | yes | — | Comma-separated list of allowed save paths; each must be absolute and exist on disk |
| `PORT` | no | `8090` | HTTP port to listen on |
| `IA_BIN` | no | auto-detect | Absolute path to the `ia` binary |
| `SHOW_COVERS` | no | `false` | Load cover images for audio items |

See [config.env.example](config.env.example) for an annotated template.

---

## Deployment (systemd)

`deploy.sh` handles building and deploying to a remote Linux host over SSH. It uses `~/.ssh/config` for host resolution, so aliases work directly.

```bash
./deploy.sh nas              # alias defined in ~/.ssh/config
./deploy.sh 192.168.1.50
```

On each run it shows a pre-deploy status table (binary hash comparison, config state, service state) and asks for confirmation before making any changes.

**First-time setup** — the script will:
1. Detect target architecture (amd64/arm64) and cross-compile locally
2. Create `/etc/iabridge/` on the target if missing
3. Install the binary to `/usr/local/bin/iabridge`
4. Offer to deploy `config.env` (sets ownership and `chmod 600`)
5. Offer to install and enable the systemd service unit

The service runs as the SSH login user. The `User=` field in the unit is set automatically from `whoami` on the target.

**Subsequent runs** — updates the binary and restarts the service if it was active.

### `IA_BIN` and systemd PATH

Systemd runs services with a minimal environment. If `ia` is installed via pipx or in `~/.local/bin`, it will not be found automatically. Set the absolute path in `config.env`:

```env
IA_BIN=/home/youruser/.local/bin/ia
```

### Security

Do not expose port 8090 to the internet. The downloads page is protected by LAN-only middleware (`ALLOWED_CIDR`), but search and item detail pages are unauthenticated.

---

## Project layout

```
cmd/iabridge/        main package
internal/
  archive/           archive.org API client
  config/            config loading and validation
  downloads/         ia subprocess management and history
  handler/           HTTP handlers, embedded templates, static assets
    tmpl/            HTML templates (one per page)
    static/          CSS, favicon, mediatype SVG icons
  middleware/        LANOnly middleware
  qbittorrent/       qBittorrent Web UI client
config.env.example   annotated config template
iabridge.service     systemd unit (User= substituted by deploy.sh)
deploy.sh            build and deploy script
```

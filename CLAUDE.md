# iabridge

A self-hostable lightweight frontend for archive.org with one-click
torrent sending to a local qBittorrent instance, and fallback download
via the `ia` CLI tool.

Target audience: self-hosters. Zero assumptions about the host machine —
all config via config file and environment variables.

---

## What it does

- Search archive.org (all mediatypes)
  - Audio results show a cover thumbnail when `SHOW_COVERS=true` (lazy-loaded via server-side proxy); all other results show the mediatype icon
- Browse item detail pages without loading any media player
- Detail page shows:
  - Cover image (audio items only, if covers enabled in config)
  - Title, mediatype icon (served locally as SVG)
  - Creator, date, topics (each topic is a clickable `subject:"..."` search link), rendered HTML description (sanitized server-side)
  - Item size, addeddate, identifier, scanner, year
  - Collections, uploader
- Two download buttons on every detail page:
  - **"Send to qBittorrent"** — enabled when a `.torrent` exists, greyed out otherwise
  - **"Download with ia"** — always present, runs `ia download` as a subprocess
- Downloads page (LAN only): live progress display (last output line from `ia`), checkbox-based bulk clear (history only) or delete (history + files on disk)
- Config page (LAN only) to manage settings and saved paths

## What it explicitly does NOT do

- No Wayback Machine
- No landing page featured/top collections — landing page is search bar only
- No embedded media player of any kind (no audio, no video playback)
- No user accounts, no database

---

## Architecture

- Pure Go, stdlib only — `net/http`, `encoding/json`, `embed`, `os/exec`
- `go.mod` must have an empty `require` block — **NO external dependencies**
- Frontend: single embedded HTML/CSS/JS file per page (no framework, no bundler)
- All archive.org API calls proxied server-side (avoids CORS, hides client IP)
- Static assets (SVG icons) embedded via `//go:embed`

### Upstream APIs

| Purpose | URL |
|---|---|
| Search | `https://archive.org/advancedsearch.php?output=json` |
| Item detail | `https://archive.org/metadata/{identifier}` |
| Cover image | `https://archive.org/services/img/{identifier}` |

### Routes

| Route | Handler |
|---|---|
| `GET /` | Landing page — search bar only |
| `GET /search?q=...` | Search results page |
| `GET /details/{identifier}` | Item detail page |
| `GET /api/cover/{identifier}` | Cover image proxy (audio only, server-side) |
| `POST /api/qbittorrent/add` | Add torrent to qBittorrent |
| `POST /api/ia/download` | Run `ia download` subprocess |
| `GET /api/downloads` | List download history + live status (JSON) — LAN only |
| `POST /api/downloads/clear` | Remove entries from history (LAN only) |
| `POST /api/downloads/delete` | Remove entries from history + delete files from disk (LAN only) |
| `GET /downloads` | Downloads page (history + live state UI) — LAN only |
| `GET /config` | Config page (LAN only) |
| `POST /config` | Save config (LAN only) |
| `GET /static/*` | Embedded static assets (SVG icons) |

---

## Config

Config is read from `config.env` (path configurable via `CONFIG_PATH` env var,
defaults to `./config.env`). The config page writes back to this file.

### config.env format

```env
PORT=8090
QBITTORRENT_URL=http://localhost:8080
QBITTORRENT_USER=admin
QBITTORRENT_PASS=changeme
QBITTORRENT_SAVE_PATHS=/downloads,/media/music,/media/video
IA_BIN=/home/user/.local/bin/ia
SHOW_COVERS=false
```

### Config fields

| Key | Default | Description |
|---|---|---|
| `PORT` | `8090` | HTTP port to listen on |
| `ALLOWED_CIDR` | — | LAN subnet allowed to access `/config`, `/downloads`, and `/api/downloads` (e.g. `192.168.1.0/24`) |
| `QBITTORRENT_URL` | — | qBittorrent Web UI URL |
| `QBITTORRENT_USER` | — | qBittorrent username |
| `QBITTORRENT_PASS` | — | qBittorrent password |
| `QBITTORRENT_SAVE_PATHS` | — | Comma-separated list of allowed save paths |
| `IA_BIN` | auto-detected via `which ia` | Absolute path to `ia` CLI binary |
| `SHOW_COVERS` | `false` | Whether to load cover images (audio only) |

Config is validated at startup. Missing required fields produce a clear error message and halt.
`ALLOWED_CIDR` must be a valid CIDR notation string; invalid values halt startup.

After writing `config.env`, set restrictive permissions — it contains plaintext credentials:
```bash
chmod 600 config.env
```
The systemd service runs as `nobody`; ensure `nobody` owns the file (`chown nobody config.env`).

### Config page security

The `/config` route is protected by the `LANOnly` middleware.
It parses `ALLOWED_CIDR` at startup into a `net.IPNet` and uses `net.IPNet.Contains()`
to check every request's origin IP.

Rules:
- `RemoteAddr` is always extracted first
- `X-Forwarded-For` is trusted **only** when `RemoteAddr` is loopback (`127.0.0.1`/`::1`)
  or already within `ALLOWED_CIDR` — both indicate a known reverse proxy.
  An external client cannot forge `RemoteAddr`; ignoring XFF from untrusted sources
  prevents IP spoofing bypassing the LAN gate.
- Any request whose resolved IP is outside `ALLOWED_CIDR` receives `403 Forbidden`
- No authentication needed — the restriction is network-level
- **Never expose port 8090 directly to the internet**

### CSRF protection on POST /config

The config POST must reject cross-site requests. Since there are no sessions or tokens,
use the `Origin` / `Referer` header check pattern:

1. If the `Origin` header is present and does not match the server's own origin
   (`http://<host>:<PORT>`), return `403 Forbidden`.
2. If `Origin` is absent, check `Referer` — if present and not from the server's own
   origin, return `403 Forbidden`.
3. Requests with neither header (direct curl, same-origin browser form) are allowed.

This prevents a page on another LAN host from silently submitting the config form
when a user's browser visits it.

### Save path security

Two layers of protection against path traversal:

**At config save time** — when the user submits a new path via the config page:
1. `filepath.Clean()` the input
2. Confirm it is absolute (starts with `/`)
3. Confirm it exists on disk (`os.Stat()`)
4. Reject anything that fails any of these three checks

**At download time** — when a save path arrives in a POST body:
1. Look up the submitted value against the whitelist in `cfg.QbittorrentPaths` — exact string match only
2. Reject any value not in the whitelist, even if it looks valid
3. Never pass user input directly to subprocesses or qBittorrent API

### archive.org identifier security

Before any identifier touches a subprocess or upstream API call:
- Validate against the regexp `^[a-zA-Z0-9_.-]+$`
- Reject anything that does not match
- Pass each argument to `exec.Command` as a separate string — never via shell interpolation

The archive client (`GetItem`, `CoverURL`) validates identifiers internally as
defense-in-depth. Handlers must also validate at the boundary (URL path param, POST body)
before passing identifiers to any downstream function — do not rely solely on the client.

`ItemFile.Name` values from archive.org metadata are not identifiers and must be
URL-encoded with `url.PathEscape` before use in a constructed URL — never interpolated raw.

### ia subprocess security

```go
// CORRECT — arguments as separate strings, never shell-interpolated
exec.Command(cfg.IABin, "download", identifier, "--destdir", validatedPath)

// NEVER DO THIS
exec.Command("sh", "-c", "ia download "+userInput)
```

The subprocess must run with a sanitised environment. `ia` is a Python tool; without
explicit `cmd.Env`, it inherits `PYTHONPATH`, `PYTHONSTARTUP`, `PYTHONHOME`, and
`LD_PRELOAD` from the parent — any of which can redirect or inject code into the
Python runtime. Strip these before exec:

```go
cmd.Env = safeEnv() // filters PYTHONPATH, PYTHONSTARTUP, PYTHONHOME, LD_PRELOAD, LD_LIBRARY_PATH
```

### HTML rendering security

All HTML pages **must** use `html/template` (not `text/template`). The auto-escaping
prevents XSS from archive.org metadata (title, description, creator, etc.).

**Description field exception** — archive.org item descriptions contain HTML (headings,
links, paragraphs). These are sanitized server-side by `sanitizeHTML` in
`internal/handler/sanitize.go` before being passed as `template.HTML`:
- `<script>` and `<style>` blocks (and their content) are stripped entirely
- Dangerous tags (`<iframe>`, `<object>`, `<embed>`, `<form>`, `<base>`, etc.) are removed
- Event handler attributes (`on*=…`) are removed
- `javascript:` in `href`/`src`/`action` attributes is replaced with `#`
This is a regexp-based approach adequate for trusted content from archive.org; do not
reuse it for arbitrary user input.

`Store.Start()` accepts `title` and `mediatype` without internal validation — they come
from the handler, which gets them from the POST body. The handler must not pass raw
user-supplied strings here; it should derive these values from the already-fetched
`Item` metadata (trusted source), not from the request body directly.

### Upstream API security

All outbound HTTP calls use a shared client with a timeout:

```go
var httpClient = &http.Client{Timeout: 15 * time.Second}
```

Response bodies must be size-limited before JSON decoding to prevent memory exhaustion:

```go
const maxResponseBody = 10 << 20 // 10 MB
json.NewDecoder(io.LimitReader(resp.Body, maxResponseBody)).Decode(&dst)
```

Always check `resp.StatusCode` before decoding — a non-2xx response from archive.org
must be returned as an error, not silently decoded as empty data.

### QBITTORRENT_URL SSRF risk

`QBITTORRENT_URL` is admin-supplied via the config page. The `/api/qbittorrent/add`
handler proxies requests to this URL. A LAN user with config access could point it at
an internal service (router, NAS) and use the handler as an SSRF relay. Mitigations:

- Document the risk prominently in the config UI
- Optionally warn at startup if `QBITTORRENT_URL` is not `localhost` / `127.0.0.1`

### Per-download save path

On both download buttons, the user picks a save path from a dropdown
populated with `QBITTORRENT_SAVE_PATHS`. No free-text path input in the UI.

---

## UI — CSS tokens

archive.org uses a light theme with a dark header. Bootstrap 3 base.
These values were extracted via DevTools on archive.org:

```css
--ia-font:           "Helvetica Neue", Helvetica, Arial, sans-serif;
--ia-mono:           "Courier New", Courier, monospace;
--ia-font-size-base: 14px;

--ia-bg:             #ffffff;
--ia-text:           #2c2c2c;
--ia-header-bg:      #222222;
--ia-header-text:    #ffffff;

--ia-link:           #4b64ff;
--ia-link-hover:     #3a50d9;
--ia-btn-color:      #4b64ff;
--ia-btn-hover:      #3a50d9;

--ia-h1-size:        3rem;
--ia-text-muted:     #767676;
--ia-border:         #dddddd;
--ia-bg-secondary:   #f5f5f5;
--ia-metadata-size:  14px;
```

Notes:
- All headings use `color: inherit` — no specific heading color
- Mediatype icons served locally as SVG in `static/icons/`:
  `audio.svg`, `movies.svg`, `texts.svg`, `software.svg`, `image.svg`,
  `collection.svg`, `etree.svg`, `data.svg`, `web.svg`, `other.svg`
- Cover images shown for `mediatype == "audio"` and only when `SHOW_COVERS=true`
  — on the detail page as a full-width image, on the search results page as a 56×56 thumbnail (lazy-loaded, hidden on error)
- Search results fall back to the mediatype icon when covers are disabled or the item is not audio

---

## Development phases

**Do not scaffold ahead of the current phase.**
Each phase must be fully working and manually tested before starting the next.

| Phase | Status | Scope |
|---|---|---|
| 1 | ✅ Done | Landing page (search bar only) + search results page |
| 2 | ✅ Done | `/details/` page, both download buttons, downloads history, cover thumbnails in search |
| 3 | — | `/details/` refinements for all other mediatypes |

---

## Build

```bash
# Local build
go build -o iabridge ./cmd/iabridge

# Cross-compile for ARM64 (e.g. Raspberry Pi 4)
GOOS=linux GOARCH=arm64 go build -o iabridge-arm64 ./cmd/iabridge
```

## Deploy

```bash
scp iabridge-arm64 user@your-server:/usr/local/bin/iabridge
```

Example systemd service is in `deploy/iabridge.service`.

---

## Code style

- No global state
- Errors returned, not panicked
- No hardcoded URLs, credentials, or paths anywhere in the code
- Config validated at startup with clear error messages and early exit
- Handlers are thin — business logic in separate packages
- `os/exec` used only for `ia download` subprocess; always use absolute path from config

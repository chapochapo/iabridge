#!/usr/bin/env bash
set -euo pipefail

HOST=${1:?"Usage: deploy.sh <ssh-host>"}

# ── Architecture detection and service user ─────────────────────────────────
echo "Checking architecture platform on target…"
read -r SERVICE_USER ARCH <<< "$(ssh "$HOST" 'echo "$(whoami) $(uname -m)"')"
if [ "$SERVICE_USER" = "root" ]; then
  echo "WARNING: connected as root — the service will run as root."
fi
case "$ARCH" in
  x86_64)  GOARCH=amd64 ;;
  aarch64) GOARCH=arm64 ;;
  *)       echo "Unknown architecture: $ARCH"; exit 1 ;;
esac

# ── Build ───────────────────────────────────────────────────────────────────
echo "Building for $ARCH ($GOARCH)…"
GOOS=linux GOARCH=$GOARCH go build -o iabridge ./cmd/iabridge

# ── Local state ─────────────────────────────────────────────────────────────
LOCAL_BIN_HASH=$(sha256sum iabridge | awk '{print $1}')

LOCAL_CONFIG=no
LOCAL_CONFIG_HASH=none
if [ -f config.env ]; then
  LOCAL_CONFIG=yes
  LOCAL_CONFIG_HASH=$(sha256sum config.env | awk '{print $1}')
fi

# ── Remote state (single SSH call) ─────────────────────────────────────────
echo "Checking remote state on $HOST…"
REMOTE_INFO=$(ssh "$HOST" bash <<'ENDSSH'
BINARY=/usr/local/bin/iabridge
CFG_DIR=/etc/iabridge
CONFIG=/etc/iabridge/config.env
SVC=/etc/systemd/system/iabridge.service

if [ -f "$BINARY" ]; then
  R_BIN_EXISTS=yes; R_BIN_HASH=$(sha256sum "$BINARY" | awk '{print $1}')
else
  R_BIN_EXISTS=no; R_BIN_HASH=none
fi

[ -d "$CFG_DIR" ] && R_DIR_EXISTS=yes || R_DIR_EXISTS=no

if [ -f "$CONFIG" ]; then
  R_CFG_EXISTS=yes
  R_CFG_HASH=$(sha256sum "$CONFIG" | awk '{print $1}')
  R_CFG_PERMS=$(stat -c %a "$CONFIG")
else
  R_CFG_EXISTS=no; R_CFG_HASH=none; R_CFG_PERMS=none
fi

if [ -f "$SVC" ]; then
  R_SVC_INSTALLED=yes
  R_SVC_ENABLED=$(systemctl is-enabled iabridge 2>/dev/null || echo unknown)
  R_SVC_ACTIVE=$(systemctl is-active iabridge 2>/dev/null || echo unknown)
else
  R_SVC_INSTALLED=no; R_SVC_ENABLED=na; R_SVC_ACTIVE=na
fi

echo "$R_BIN_EXISTS $R_BIN_HASH $R_DIR_EXISTS $R_CFG_EXISTS $R_CFG_HASH $R_CFG_PERMS $R_SVC_INSTALLED $R_SVC_ENABLED $R_SVC_ACTIVE"
ENDSSH
)

read -r R_BIN_EXISTS R_BIN_HASH R_DIR_EXISTS R_CFG_EXISTS R_CFG_HASH \
        R_CFG_PERMS R_SVC_INSTALLED R_SVC_ENABLED R_SVC_ACTIVE \
  <<< "$REMOTE_INFO"

# ── Status display ──────────────────────────────────────────────────────────
echo ""
echo "Pre-deploy status on $HOST:"
echo ""

if [ "$R_BIN_EXISTS" = "yes" ]; then
  if [ "$LOCAL_BIN_HASH" = "$R_BIN_HASH" ]; then
    BIN_NOTE="installed — same as local build (no change)"
  else
    BIN_NOTE="installed — differs from local build (will be updated)"
  fi
else
  BIN_NOTE="not installed (will be installed)"
fi
printf "  %-26s %s\n" "Binary:" "$BIN_NOTE"

if [ "$R_DIR_EXISTS" = "yes" ]; then
  DIR_NOTE="exists"
else
  DIR_NOTE="missing (will be created automatically)"
fi
printf "  %-26s %s\n" "Config directory:" "$DIR_NOTE"

if [ "$LOCAL_CONFIG" = "no" ]; then
  CFG_NOTE="no local config.env found — skipping"
elif [ "$R_CFG_EXISTS" = "no" ]; then
  CFG_NOTE="not on target — you will be asked to deploy"
elif [ "$LOCAL_CONFIG_HASH" = "$R_CFG_HASH" ]; then
  CFG_NOTE="up to date"
  [ "$R_CFG_PERMS" != "600" ] && CFG_NOTE="$CFG_NOTE (WARNING: permissions $R_CFG_PERMS ≠ 600)"
else
  CFG_NOTE="differs from local config.env — you will be asked to overwrite"
  [ "$R_CFG_PERMS" != "600" ] && CFG_NOTE="$CFG_NOTE (WARNING: permissions $R_CFG_PERMS ≠ 600)"
fi
printf "  %-26s %s\n" "Config file:" "$CFG_NOTE"

if [ "$R_SVC_INSTALLED" = "yes" ]; then
  SVC_NOTE="installed ($R_SVC_ENABLED, $R_SVC_ACTIVE)"
  [ "$R_SVC_ACTIVE" = "active" ] && SVC_NOTE="$SVC_NOTE — will be restarted after binary update"
else
  SVC_NOTE="not installed — you will be asked to install"
fi
printf "  %-26s %s\n" "Systemd service:" "$SVC_NOTE"

echo ""
read -rp "Proceed with deploy to $HOST? [y/N] " CONFIRM
case "$CONFIRM" in
  [yY]|[yY][eE][sS]) ;;
  *) echo "Aborted."; exit 0 ;;
esac
echo ""

# ── Create config directory ─────────────────────────────────────────────────
if [ "$R_DIR_EXISTS" = "no" ]; then
  echo "Creating /etc/iabridge on $HOST…"
  ssh -t "$HOST" "sudo mkdir -p /etc/iabridge && sudo chown $SERVICE_USER /etc/iabridge"
fi

# ── Deploy binary ───────────────────────────────────────────────────────────
echo "Uploading binary…"
scp iabridge "$HOST":/tmp/iabridge
echo "Installing binary to /usr/local/bin/iabridge…"
ssh -t "$HOST" "sudo mv /tmp/iabridge /usr/local/bin/iabridge && sudo chmod +x /usr/local/bin/iabridge"

# ── Deploy config ───────────────────────────────────────────────────────────
DEPLOY_CONFIG=no
if [ "$LOCAL_CONFIG" = "yes" ]; then
  if [ "$R_CFG_EXISTS" = "no" ]; then
    echo ""
    echo "Config file is missing on $HOST at /etc/iabridge/config.env."
    echo "  The service cannot start without it."
    echo "  Deploying will copy local config.env, set owner to $SERVICE_USER, and permissions to 600."
    echo "  Skipping means the service will fail to start until a config is placed manually."
    read -rp "  Deploy config.env? [y/N] " CFG_CONFIRM
    case "$CFG_CONFIRM" in
      [yY]|[yY][eE][sS]) DEPLOY_CONFIG=yes ;;
      *) echo "  Skipping config. The service will not start without it." ;;
    esac
  elif [ "$LOCAL_CONFIG_HASH" != "$R_CFG_HASH" ]; then
    echo ""
    echo "Config on $HOST differs from local config.env."
    echo "  Deploying will overwrite the remote config; the service will load new settings on next start."
    echo "  Skipping keeps the existing remote config unchanged."
    read -rp "  Overwrite remote config with local config.env? [y/N] " CFG_CONFIRM
    case "$CFG_CONFIRM" in
      [yY]|[yY][eE][sS]) DEPLOY_CONFIG=yes ;;
      *) echo "  Keeping existing remote config." ;;
    esac
  fi
fi

if [ "$DEPLOY_CONFIG" = "yes" ]; then
  echo "Uploading config.env…"
  scp config.env "$HOST":/tmp/iabridge_config.env
  echo "Installing to /etc/iabridge/config.env (owner: $SERVICE_USER, permissions: 600)…"
  ssh -t "$HOST" "sudo mv /tmp/iabridge_config.env /etc/iabridge/config.env && sudo chown $SERVICE_USER /etc/iabridge/config.env && sudo chmod 600 /etc/iabridge/config.env"
fi

# ── Systemd service ─────────────────────────────────────────────────────────
if [ "$R_SVC_INSTALLED" = "no" ]; then
  echo ""
  echo "The systemd service is not installed on $HOST."
  echo "  Installing registers iabridge as a system service that starts automatically on boot"
  echo "  and launches it immediately via 'systemctl enable --now'."
  echo "  Skipping means the binary is deployed but will not run until started manually."
  read -rp "  Install and enable the systemd service? [y/N] " SVC_CONFIRM
  case "$SVC_CONFIRM" in
    [yY]|[yY][eE][sS])
      echo "Uploading service unit…"
      scp iabridge.service "$HOST":/tmp/iabridge.service
      echo "Installing service unit (User=$SERVICE_USER), enabling, and starting…"
      ssh -t "$HOST" "sed 's/^User=.*/User=$SERVICE_USER/' /tmp/iabridge.service | sudo tee /etc/systemd/system/iabridge.service > /dev/null && sudo systemctl daemon-reload && sudo systemctl enable --now iabridge"
      echo "Service installed and running."
      ;;
    *)
      echo "Skipping service installation."
      ;;
  esac
elif [ "$R_SVC_ACTIVE" = "active" ]; then
  echo "Restarting iabridge to load the new binary…"
  ssh -t "$HOST" "sudo systemctl restart iabridge"
  echo "Service restarted."
fi

echo ""
echo "Done."
#!/usr/bin/env bash
# GitWebUI 1-Line Universal Installer (Linux & macOS)
# Author: Giang3DLab / Giang Sam
set -e

REPO="giangsamne/git-webui"
APP="git-webui"
INSTALL_DIR="/usr/local/bin"

echo "=========================================================="
echo "   ⚡ GitWebUI - Universal 1-Line Installer              "
echo "=========================================================="

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64|amd64)
    ARCH="amd64"
    ;;
  aarch64|arm64)
    ARCH="arm64"
    ;;
  *)
    echo "❌ Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

case "$OS" in
  linux)
    OS="linux"
    ;;
  darwin)
    OS="darwin"
    ;;
  *)
    echo "❌ Unsupported operating system: $OS"
    exit 1
    ;;
esac

echo "🔍 Detected Platform: $OS ($ARCH)"

DOWNLOAD_URL="https://github.com/$REPO/releases/latest/download/${APP}-${OS}-${ARCH}.tar.gz"
TMP_DIR="$(mktemp -d)"

echo "📥 Downloading latest binary from $DOWNLOAD_URL..."
if curl -fsSL -o "$TMP_DIR/pkg.tar.gz" "$DOWNLOAD_URL"; then
  tar -xzf "$TMP_DIR/pkg.tar.gz" -C "$TMP_DIR"
  BIN_FILE="$TMP_DIR/${APP}-${OS}-${ARCH}"
else
  echo "⚠️ Release asset download fallback to direct compilation if Go is available..."
  if command -v go >/dev/null 2>&1; then
    go install "github.com/$REPO@latest"
    echo "✅ Installed via go install"
    exit 0
  else
    echo "❌ Failed to download binary. Please check release at https://github.com/$REPO/releases"
    exit 1
  fi
fi

# Elevate if needed
if [ -w "$INSTALL_DIR" ]; then
  mv "$BIN_FILE" "$INSTALL_DIR/$APP"
  chmod +x "$INSTALL_DIR/$APP"
else
  echo "🔒 Sudo required to install to $INSTALL_DIR/$APP"
  sudo mv "$BIN_FILE" "$INSTALL_DIR/$APP"
  sudo chmod +x "$INSTALL_DIR/$APP"
fi

rm -rf "$TMP_DIR"
echo "✅ GitWebUI successfully installed to $INSTALL_DIR/$APP"

# Systemd autostart setup for Linux
if [ "$OS" = "linux" ] && command -v systemctl >/dev/null 2>&1; then
  read -p "❓ Do you want to enable and start GitWebUI as a systemd service? (Y/n): " -n 1 -r
  echo
  if [[ $REPLY =~ ^[Yy]$ ]] || [ -z "$REPLY" ]; then
    CURRENT_USER="$USER"
    SERVICE_FILE="/etc/systemd/system/${APP}.service"
    sudo bash -c "cat <<EOF > $SERVICE_FILE
[Unit]
Description=GitWebUI - Homelab Git Web UI & Auto Backup Service
After=network.target

[Service]
Type=simple
User=$CURRENT_USER
WorkingDirectory=$HOME
ExecStart=$INSTALL_DIR/$APP
Restart=always
RestartSec=3
Environment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
Environment=HOME=$HOME

[Install]
WantedBy=multi-user.target
EOF"
    sudo systemctl daemon-reload
    sudo systemctl enable --now "${APP}.service"
    echo "🚀 GitWebUI service started on port 18090!"
  fi
fi

echo "=========================================================="
echo " 🎉 Installation Complete!                                "
echo " 🌐 Web Dashboard: http://localhost:18090                 "
echo "=========================================================="

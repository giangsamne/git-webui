<div align="center">

# ⚡ GitWebUI

**Ultra-lightweight, zero-dependency bare-metal Git Web UI & Realtime Auto-Backup daemon for Homelab & Developers.**

[![Release](https://img.shields.io/badge/Release-v1.0.0-blue.svg?logo=github)](https://github.com/giangsamne/git-webui/releases)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8.svg?logo=go&logoColor=white)](https://golang.org)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-brightgreen.svg)](https://github.com/giangsamne/git-webui)
[![Dependencies](https://img.shields.io/badge/dependencies-0%25%20(Pure%20Native%20Binary)-success.svg)](https://github.com/giangsamne/git-webui)
[![Memory Footprint](https://img.shields.io/badge/RAM-%3C%204MB%20RSS-blueviolet.svg)](https://github.com/giangsamne/git-webui)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

[English](#-english) • [Tiếng Việt](#-tiếng-việt)

</div>

---

### ⚡ 1-Line Universal Auto-Installer

Run a single command in your terminal or PowerShell — automatically detects your OS and architecture (Apple Silicon / Intel / ARM64 / x86_64), then installs and runs GitWebUI:

#### 🐧 Linux & 🍏 macOS (Terminal):
```bash
curl -fsSL https://raw.githubusercontent.com/giangsamne/git-webui/main/install.sh | bash
```

#### 🪟 Windows (PowerShell as Administrator or User):
```powershell
irm https://raw.githubusercontent.com/giangsamne/git-webui/main/install.ps1 | iex
```

---

## 🌟 Key Features

* **⚡ Ultra Low Resource Consumption:** Pure Go native binary. Runs on **~2.2MB to 4MB RAM**, CPU sits at **0.0%** in idle mode.
* **🛡️ Zero Dependencies:** Single standalone binary with embedded modern Web UI. No Python, no Node.js, no Docker, no external web server needed.
* **⏱️ Realtime & Scheduled Auto-Backup:**
  * **File Watcher (`inotify`):** Automatically detects modified/created files with debouncing and commits/pushes to GitHub immediately.
  * **Interval Scheduler:** Backup every 1 min, 5 min, 15 min, 30 min, 1 hour, or daily.
* **🔄 1-Click Rollback / Restore:** Time-machine style restore of any past commit in your Git history. Linear, safe, and pushes the restored state to remote.
* **🧹 1-Click Discard Changes:** Wipe away unwanted local draft edits and bring your working tree back to clean state.
* **📁 Visual Repository Manager:** Add any local folder, auto `git init`, auto-create GitHub repositories (Private/Public) via GitHub CLI.
* **🌐 Cross-Platform:** Native support for Linux (x86_64, ARM64/Raspberry Pi), macOS (Apple Silicon M1-M4 & Intel), and Windows.

---

## 🖥️ Web Dashboard (Port 18090)

Once launched, access the dashboard at:
* Local: `http://localhost:18090`
* LAN / Tailscale: `http://<your-homelab-ip>:18090`

---

## 🚀 Manual Download & Build

### Pre-compiled Binaries:
Download the pre-compiled binary for your system from [GitHub Releases](https://github.com/giangsamne/git-webui/releases).

| Platform | Architecture | File |
| :--- | :--- | :--- |
| **Linux** | AMD64 (x86_64) | `git-webui-linux-amd64.tar.gz` |
| **Linux** | ARM64 (Raspberry Pi / VPS) | `git-webui-linux-arm64.tar.gz` |
| **macOS** | Apple Silicon (M1/M2/M3/M4) | `git-webui-darwin-arm64.tar.gz` |
| **macOS** | Intel | `git-webui-darwin-amd64.tar.gz` |
| **Windows** | AMD64 (x64) | `git-webui-windows-amd64.zip` |

### Building from Source:
```bash
git clone https://github.com/giangsamne/git-webui.git
cd git-webui
go build -ldflags="-s -w" -o git-webui main.go
./git-webui
```

---

## 🛠️ Systemd Service (Linux Homelab)

To run GitWebUI 24/7 as a background service:
```bash
sudo bash -c 'cat <<EOF > /etc/systemd/system/git-webui.service
[Unit]
Description=GitWebUI - Homelab Git Web UI & Auto Backup Service
After=network.target

[Service]
Type=simple
User='$USER'
WorkingDirectory='$HOME'
ExecStart=/usr/local/bin/git-webui
Restart=always
RestartSec=3
Environment=HOME='$HOME'

[Install]
WantedBy=multi-user.target
EOF'

sudo systemctl daemon-reload
sudo systemctl enable --now git-webui.service
```

---

## 🇻🇳 Tiếng Việt

**GitWebUI** là công cụ quản lý Git qua Web UI và tự động sao lưu dự án siêu nhẹ viết hoàn toàn bằng Go Native:
* **Siêu nhẹ:** Ăn chưa tới **4MB RAM**, CPU **0%**.
* **Khôi phục 1-Click (Rollback):** Quay ngược thời gian về bất kỳ bản sao lưu cũ nào chỉ bằng 1 nút bấm.
* **Tự động sao lưu lên GitHub:** Nhận diện file vừa sửa đổi là tự động commit và đẩy lên GitHub ngay lập tức.
* **Hỗ trợ đầy đủ:** Linux (Homelab/Ubuntu/Pi), macOS (MacBook M1/M2/M3/M4) và Windows.

---

## 📜 License

Distributed under the MIT License. Author: **Giang3DLab / Giang Sam** ([@giangsamne](https://github.com/giangsamne)).

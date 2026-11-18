<div align="center">

<img src="docs/icon.png" alt="NetPilot Logo" width="84" />

# NetPilot

**Multi-adapter network control for Windows, macOS, and Linux**

Use Wi-Fi, Ethernet, and phone tethering together, or keep a backup link ready when your primary connection drops.

[![Latest Release](https://img.shields.io/github/v/release/j1seki/netpilot?style=flat-square&color=10b981&label=Download)](https://github.com/j1seki/netpilot/releases/latest)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-38bdf8?style=flat-square)](#%EF%B8%8F-downloads--installation)
[![License: MIT](https://img.shields.io/badge/License-MIT-zinc?style=flat-square)](LICENSE)

</div>

---

## NetPilot in action

![NetPilot desktop app: live dashboard, process icons, tethering, and TTL preview](docs/netpilot-demo.gif)

- **Systems** shows adapter traffic, active processes, and app routing rules. **Metrics** shows usage history.
- **Speed** distributes connections across healthy adapters. **Backup** switches to a secondary link when the primary fails.
- **Mobile** puts tethering profiles and link tests alongside **TTL & Hop Limit**. Preview supported desktop packet-header changes before applying them, or restore the original settings.

Download and upload speeds, idle and loaded latency, and bufferbloat measure link performance. Neither those numbers nor desktop OS read-back verify how iOS, Android, or a carrier classifies tethered traffic. See [tethering validation](docs/tethering-validation.md) for the measured baseline and test steps.

---

## ⬇️ Downloads & Installation

Grab the latest prebuilt package for your operating system from the **[Releases Page](https://github.com/j1seki/netpilot/releases/latest)**:

| Operating System | Package | Notes |
| :--- | :--- | :--- |
| **🪟 Windows (10 / 11)** | **`netpilot-setup.exe`** / **`netpilot-windows-amd64-portable.zip`** | Single-binary NetPilot GUI + net-merge engine (auto-extracts `wintun.dll`). |
| **🍎 macOS (Universal)** | **`netpilot-macos-universal.dmg`** | Native Universal binary for Apple Silicon (`M1/M2/M3/M4`) & Intel Macs. |
| **🐧 Linux x86-64** | **`netpilot-linux-amd64.AppImage`** | Portable single-file AppImage for Ubuntu, Debian, Fedora, Arch, Mint, etc. |
| **🐧 Linux ARM64** | **`netpilot-linux-arm64.AppImage`** | Native aarch64 AppImage for ARM servers, Apple Silicon VMs (OrbStack/UTM), and Raspberry Pi-class devices. |

### Running the Linux AppImage

Make the AppImage executable and run it directly — no installation required:

```bash
# x86-64
chmod +x netpilot-linux-amd64.AppImage
./netpilot-linux-amd64.AppImage

# ARM64 (aarch64)
chmod +x netpilot-linux-arm64.AppImage
./netpilot-linux-arm64.AppImage
```

> **Linux WebKitGTK Note:** Most desktop Linux distributions include WebKit2GTK preinstalled. On minimal installations, install the WebKit2GTK 4.1 runtime if needed:
> - **Ubuntu / Debian / Mint:** `sudo apt update && sudo apt install -y libwebkit2gtk-4.1-0`
> - **Fedora / RHEL:** `sudo dnf install webkit2gtk4.1`
> - **Arch Linux:** `sudo pacman -S webkit2gtk-4.1`

---

## 🛠️ Building from Source

NetPilot uses **Go** for the backend/engine and **React + Vite + `typescript-go` (`tsgo`)** for the frontend.

```bash
# 1. Clone the repository
git clone https://github.com/j1seki/netpilot.git
cd netpilot

# 2. Sync the latest upstream net-merge source (optional, already synced in repo)
bash scripts/sync-net-merge.sh

# 3. Build frontend assets (typechecked with typescript-go / tsgo)
cd frontend
npm ci
npm run build
cd ..

# 4. Build the single NetPilot binary (GUI + net-merge CLI + tray in one file)
#    On Linux add -tags gtk3; on Windows add -ldflags "-H=windowsgui" to hide the console.
go build -trimpath -ldflags "-s -w" -o dist/netpilot ./cmd/netpilot

# The net-merge subcommands are built in: `dist/netpilot version`, `list`, `run`, etc.
# `internal/netmerge/VERSION` is embedded via go:embed; keep it in sync with the engine.

# 5. Run the test suite
go test ./internal/... ./tests/common ./tests/scenarios
```

---

## 📄 License

Licensed under the [MIT License](LICENSE). See [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md) for third-party attributions including [net-merge](https://github.com/net-merge/net-merge) and Wintun.

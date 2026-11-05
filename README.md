# NetPilot

**Multi-network manager and bandwidth aggregator desktop app.**

NetPilot is a cross-platform desktop control center for **Windows, macOS, and Linux**. It provides a polished GUI for combining network connections, per-app routing, failover, live analytics, and a TTL/hop-limit manager.

---

## ⚡ Required: net-merge Engine

NetPilot uses the high-performance **[net-merge](https://github.com/net-merge/net-merge)** engine to handle network adapter bonding, multi-path routing, and TUN interfaces.

1. Download the latest `net-merge` binary for your platform from the official repository:
   👉 **[Download net-merge from Releases](https://github.com/net-merge/net-merge/releases)**
2. Place the executable (`net-merge.exe` on Windows, or `net-merge` on macOS/Linux) in the **same folder** as NetPilot (or add it to your system PATH).
3. Start NetPilot — it will automatically detect and manage the engine.

---

## 📥 Downloads

Get the latest release for your platform from the **[Releases](https://github.com/j1seki/netpilot/releases)** page:

| OS | Download | Description |
|---|---|---|
| **Windows** | `netpilot-setup.exe` | Windows installer (Inno Setup) |
| **macOS** | `netpilot-macos-universal.dmg` | Universal DMG (Intel & Apple Silicon) |
| **Linux** | `netpilot-linux-amd64.AppImage` | Standalone Linux AppImage |

### Linux Requirements

NetPilot's UI is built with WebKitGTK. Modern desktop environments (Ubuntu, Mint, Fedora, Arch) usually have this preinstalled. If NetPilot does not start on a minimal distribution, install the WebKit runtime:

- **Ubuntu / Debian / Mint**:
  ```bash
  sudo apt update && sudo apt install -y libwebkit2gtk-4.1-0
  ```
- **Fedora / RHEL**:
  ```bash
  sudo dnf install webkit2gtk4.1
  ```
- **Arch Linux**:
  ```bash
  sudo pacman -S webkit2gtk-4.1
  ```

---

## ✨ Features

- **Combine Wi-Fi, Ethernet & phone hotspots**: Spread downloads, games, and browsing across all active adapters.
- **Per-App Routing**: Direct specific applications to specific network links with bandwidth limits.
- **Automatic Failover**: Fast detection of disconnected links with automatic rerouting.
- **Real-Time Analytics**: Monitor per-adapter throughput, active sockets, and monthly usage history.
- **TTL / Hop-Limit Manager**: Fine-tune IPv4/IPv6 packet hop limits for VPNs and network bypass.
- **Scenario Simulation**: Deterministic test suites to debug routing decisions.

---

## 🛠️ Development

```bash
# Clone the repository
git clone https://github.com/j1seki/netpilot.git
cd netpilot

# Build frontend assets
cd frontend
npm ci && npm run build
cd ..

# Build NetPilot GUI
go build -o dist/netpilot ./cmd/netpilot

# Run tests
go test ./tests/common ./tests/scenarios
```

---

## License

MIT License. See [LICENSE](LICENSE) and [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md).

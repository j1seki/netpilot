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

- **Systems** shows adapter traffic, active processes, and app routing rules. **Metrics** lets you choose units for usage and transfer rates.
- **Speed** distributes connections across healthy adapters. **Backup** switches to a secondary link when the primary fails.
- **Mobile** groups Wi-Fi hotspot and USB tether settings with connection profiles, link tests, and **TTL & Hop Limit**. Load a profile to compare it with current settings, edit the draft, then apply or restore previous values.

Download and upload speeds, idle and loaded latency, and bufferbloat measure link performance. Neither those numbers nor desktop OS read-back verify how iOS, Android, or a carrier classifies tethered traffic. See [tethering validation](docs/tethering-validation.md) for the measured baseline and test steps.

### Application routing and process inspection

Create and edit policies from **Systems → Application Routing Policies** or **Rules**. A policy can match an application name, executable path, or destination, then select a detected network adapter, optional fallback, bandwidth limit, traffic quota, and custom DNS. Inspect a running process to see matching policies. The live TCP route check reports the adapter and source address used by its probe.

System-wide application policies require the built-in NetPilot engine and elevated permissions. On Windows, run NetPilot as administrator. With the SOCKS5 proxy, an application must be configured to use NetPilot's local proxy. The separate upstream engine can run a system-wide tunnel, but it does not apply NetPilot's application policies. A routing rule affects new connections, so reconnect an application after changing its policy. Custom DNS works when NetPilot can see the hostname before it is resolved. Operating-system resolvers, encrypted DNS, VPN routes, and application caches can affect DNS behavior.

Speed mode distributes connections across available links. Backup mode keeps new connections on the selected primary link and uses a backup when the primary is unavailable. An application's explicit adapter choice takes precedence for that application's new connections. The chosen adapter must have an address for the connection's IP family and pass the configured reachability checks. Per-adapter and per-application bandwidth limits both apply. A reached application quota blocks new connections that match the policy.

### Mobile and connection profiles

The Mobile page shows saved settings next to a draft of desired values. Select the PC network adapter carrying the phone's hotspot connection, then edit its tunnel MTU, IPv6 behavior, scheduling, and reachability probes. USB tether adapters may be identified automatically. A phone connected over Wi-Fi is not always distinguishable from another access point, so select the PC's Wi-Fi adapter manually when needed. Choosing the hotspot adapter records and tests that link. It does not direct all NetPilot traffic through it.

Suggested profiles load settings into the draft. Review or customize them before selecting **Apply changes**. **Discard edits**, **Load defaults**, and **Restore previous** have separate actions. MTU, IPv6, scheduling, and probes affect the whole NetPilot tunnel. Changing them while the separate upstream engine is running can require restarting that engine. Applying a saved full connection profile can also change routing mode.

The **Diagnostics** page checks implemented app, routing, notification, mobile, and desktop features. Use **Skip disruptive tests** to omit tests that can change connectivity. A live hotspot check is optional and accepts the PC adapter to test. Without a selected adapter, NetPilot may still detect a USB tether or cellular link. Standard Wi-Fi hotspot connections generally require manual selection.

Notifications are managed in **Settings**. NetPilot can show an in-app notification and submit desktop banners when the operating system permits them. Use **Test notification** to check the current delivery result, and open system notification settings if banners are disabled.

---

## ⬇️ Downloads & Installation

Grab the latest prebuilt package for your operating system from the **[Releases Page](https://github.com/j1seki/netpilot/releases/latest)**. The Latest release is always the newest stable build; prereleases are listed under **[all releases](https://github.com/j1seki/netpilot/releases)**:

| Operating System | Package | Notes |
| :--- | :--- | :--- |
| **🪟 Windows (10 / 11)** | **`netpilot-<version>-installer.exe`** / **`netpilot-<version>-windows-amd64-portable.zip`** | For the portable package, extract the ZIP and open `netpilot.exe`. |
| **🍎 macOS (Universal)** | **`netpilot-<version>-macos-universal.dmg`** | Universal binary for Apple Silicon and Intel Macs. |
| **🐧 Linux x86-64** | **`netpilot-<version>-linux-amd64.AppImage`** | Portable single-file AppImage for desktop Linux. |
| **🐧 Linux ARM64** | **`netpilot-<version>-linux-arm64.AppImage`** | Native aarch64 AppImage for Linux ARM64 desktops and VMs. |

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

### Choosing a net-merge engine

**Settings → Net-Merge Engine** lets you select the bundled official release, download a newer or older official version, or switch back to NetPilot's integrated engine. Stop merging and the SOCKS proxy before switching. The integrated engine supports Backup Failover and per-app routing; official downloads use Speed mode. Release builds bundle the latest stable upstream net-merge separately from the integrated engine.

### Verifying an application rule

Save the rule in **Application Routing** or **Rules**, then open **Active Processes → Inspect → Routing rules**. The live test reports the outbound adapter and source IP after a TCP connection succeeds. Observed rule matches appear when the running application opens fresh connections through the in-app tunnel or proxy. System-wide rule enforcement requires the desktop's in-app NetPilot engine. On Windows, launch NetPilot as administrator for that mode.

On macOS, install NetPilot in Applications. If macOS blocks a copy you downloaded from the official release page, use **Privacy & Security → Open Anyway**, or the DMG's **Open NetPilot.command** helper to verify the bundle and remove quarantine from that copy after confirming.

### Automatic updates

NetPilot checks GitHub for a newer build on startup and every six hours, and can download, verify and
install it without leaving the app. Open **Settings → Automatic Updates** and pick a channel:

| Channel | Receives | GitHub meaning |
| :--- | :--- | :--- |
| **Stable** | Finished releases without a prerelease suffix | Normal releases and the Latest badge |
| **Beta** | Prereleases and finished stable releases | GitHub prereleases plus normal releases |

Versions are compared as full semver, so `vX.Y.Z-beta.10` is newer than `vX.Y.Z-beta.9` and a stable
`vX.Y.Z` supersedes its own betas. New installations follow the channel of the build they came from,
and the choice is remembered in the per-user configuration directory (`%AppData%\netpilot` on
Windows, `~/Library/Application Support/netpilot` on macOS, `~/.config/netpilot` on Linux).

Every release publishes a `checksums.txt`; the updater verifies the downloaded package's SHA-256 (or
GitHub's asset digest when an older release has no checksum file) before installing. One-click
install is supported by the Windows installer and portable ZIP, the macOS `.app` bundle when it lives
in a writable location such as `/Applications`, and the Linux AppImage. Other setups get a link to the
release page instead. Beta builds are always GitHub prereleases, so the **Latest** release and the
badge on the Releases page stay on the newest stable version.

---

## 🛠️ Building from Source

NetPilot uses **Go** for the backend/engine and **React + Vite + `typescript-go` (`tsgo`)** for the frontend.
Use the Go version in `go.mod` and Node.js 22 or later. The Linux release build needs the GTK 3 and
WebKitGTK 4.1 development packages (`libgtk-3-dev` and `libwebkit2gtk-4.1-dev` on Debian and Ubuntu).

```bash
git clone https://github.com/j1seki/netpilot.git
cd netpilot

# Build the frontend and optionally bundle the official engine.
(cd frontend && npm ci && npm run build)
go run scripts/bundle-net-merge.go

# Build and launch the Linux desktop app.
go build -tags gtk3 -trimpath -o dist/netpilot ./cmd/netpilot
./dist/netpilot

go test -tags gtk3 ./...
```

Use `./dist/netpilot -mode dry-run` for inspection and previews. Choose Live Mode in Settings to
start forwarding or change host network settings. The built-in `version`, `list`, and `run` commands
also work directly from a terminal.

On macOS and Windows, omit `-tags gtk3`. On Windows, add `-ldflags "-H=windowsgui"` to build
without a console window. The Windows release bundle supplies the Wintun driver.

---

## 📄 License

Licensed under the [MIT License](LICENSE). See [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md) for third-party attributions including [net-merge](https://github.com/net-merge/net-merge) and Wintun.

# NetPilot

**Use all your network connections at once — in one download.**

NetPilot is a desktop app for **Windows, macOS and Linux** that bundles the
[net-merge](https://github.com/net-merge/net-merge) connection-balancing engine under a
polished interface, plus a management console with per-app routing, failover,
analytics and a TTL / hop-limit manager.

> You no longer need two repos. Get NetPilot here and it always ships with the latest
> net-merge engine — releases are synced automatically.

## Features

- **Combine Wi-Fi, Ethernet and phone tethers** — every new connection spreads across your
  links, so downloads, games and browsers get the speed of every connection added up.
- **Per-app routing** — send specific apps over specific adapters, with bandwidth limits and
  data quotas.
- **Failover** — a link that dies is detected fast and its connections move to the next link.
- **Analytics** — live throughput per adapter, monthly usage history, per-app stats.
- **TTL / hop-limit manager** — bump the IPv4/IPv6 TTL of every hop on a path (great for
  game servers and VPNs), with inspect / apply / restore and an event history.
- **Deterministic scenarios & platform test suites** — built-in simulation and tests you can
  run from the CLI or the UI.
- **Cross-platform** — Windows (installer + portable zip), macOS (universal binary) and
  Linux (`.deb`, `.rpm`, `.apk`, `.tar.gz`).

## Download

| OS | Install |
|---|---|
| Windows | [`netpilot-setup-*.exe`](https://github.com/j1seki/netpilot/releases) (Inno Setup installer) or `netpilot-*-portable.zip` |
| macOS | `netpilot-*-macos-universal.tar.gz` (Intel + Apple Silicon) |
| Linux | `.deb` (Debian/Ubuntu), `.rpm` (Fedora/openSUSE), `.apk` (Alpine), `.pkg.tar.zst` (Arch), or the plain tarball |

The **latest** copies (`netpilot-latest-*`) always point at the newest release.

## How the engine stays in sync

NetPilot embeds the net-merge engine. Whenever the
[net-merge/net-merge](https://github.com/net-merge/net-merge) project publishes a release,
this repository is updated automatically:

1. A **sync** workflow fetches the tagged source of the new net-merge release.
2. The engine packages (`internal/{engine,icon,logx,nics,platform,state,tray,tun,update}`)
  are replaced and their imports rewritten from `net-merge/` to `netpilot/`.
3. A **release** workflow rebuilds NetPilot for every desktop OS and publishes the packages,
  with the net-merge changelog section folded into the release notes.

So you always get the newest engine without visiting a second repo.

## Development

```sh
git clone https://github.com/j1seki/netpilot
cd netpilot

# Frontend assets
cd frontend && npm ci && npm run build && cd ..

# Build for the current platform
go build -o dist/netpilot ./cmd/netpilot

# Run the test suites
go test ./tests/common ./tests/scenarios
```

### Release flow

- `scripts/sync-net-merge.sh <tag>` — pull a tagged net-merge release into the engine packages.
- `scripts/build.ps1 <version>` — Windows: exes, wintun, installer, portable zip.
- `scripts/build-linux.sh <version> dist` — Linux tarball + `.deb`/`.rpm`/`.apk`/Arch packages.
- Tag a version (`v0.1.0`) to trigger the full cross-platform release.

## License

MIT. The net-merge engine is MIT; see [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md).
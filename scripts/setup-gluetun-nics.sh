#!/usr/bin/env bash
# Create real temporary TUN adapters glu1..glu5, each backed by one Gluetun HTTP proxy
# (see /home/x/gluetun_proxies.md).  Traffic sent into gluN is forwarded by tun2socks
# through proxy N, so every adapter really has its own public egress IP:
#
#   glu1 192.168.1.151:18881 -> 92.119.177.24
#   glu2 192.168.1.151:18882 -> 138.199.42.140
#   glu3 192.168.1.151:18883 -> 146.70.183.206
#   glu4 192.168.1.151:18884 -> 45.149.173.225
#   glu5 192.168.1.151:18885 -> 2.56.189.75
#
# Usage:
#   sudo ./scripts/setup-gluetun-nics.sh setup
#   ./scripts/setup-gluetun-nics.sh probe
#   sudo ./scripts/setup-gluetun-nics.sh teardown
#
# Requires: Linux, /dev/net/tun, root (or CAP_NET_ADMIN), and the tun2socks module
# already present in go.mod (github.com/xjasonlyu/tun2socks/v2).
set -euo pipefail

ACTION=${1:-setup}
PROXY_HOST=${GLUETUN_HOST:-192.168.1.151}
PROXY_BASE_PORT=${GLUETUN_BASE_PORT:-18881}
BIN=${TUN2SOCKS_BIN:-/tmp/netpilot-tun2socks}
RUN_DIR=${RUN_DIR:-/tmp/netpilot-gluetun}
IFACE_PREFIX=${IFACE_PREFIX:-glu}
BASE_ADDR=${BASE_ADDR:-10.10.0}
ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

need_root() {
  if [ "$(id -u)" -ne 0 ]; then
    echo "error: $1 needs root (try: sudo $0 $1)" >&2
    exit 1
  fi
}

find_go() {
  if [ -n "${GO_BIN:-}" ] && [ -x "$GO_BIN" ]; then
    echo "$GO_BIN"; return
  fi
  if command -v go >/dev/null 2>&1; then
    command -v go; return
  fi
  # `sudo` resets PATH; fall back to common install locations.
  for candidate in /usr/local/go/bin/go /usr/bin/go /opt/go*/bin/go /home/*/go/bin/go; do
    [ -x "$candidate" ] && { echo "$candidate"; return; }
  done
  echo ""
}

build_tun2socks() {
  if [ -x "$BIN" ]; then
    return
  fi
  local gobin dir
  gobin=$(find_go)
  if [ -z "$gobin" ]; then
    echo "error: Go toolchain not found; set GO_BIN=/path/to/go and retry" >&2
    exit 1
  fi
  echo "Building tun2socks helper -> $BIN (using $gobin)"
  dir=$(cd "$ROOT_DIR" && run_as_builder "$gobin" list -m -f '{{.Dir}}' github.com/xjasonlyu/tun2socks/v2)
  if [ -z "$dir" ]; then
    echo "error: cannot locate tun2socks module in $ROOT_DIR" >&2
    exit 1
  fi
  run_as_builder bash -c "cd '$dir' && '$gobin' build -o '$BIN' ."
}

# When the script is run through sudo, build with the invoking user's Go module
# cache instead of root's (which is usually empty).
run_as_builder() {
  if [ "$(id -u)" -eq 0 ] && [ -n "${SUDO_USER:-}" ] && [ "${SUDO_USER}" != root ]; then
    sudo -u "$SUDO_USER" env "PATH=$(dirname "$1"):$PATH" "$@"
  else
    "$@"
  fi
}

start_one() {
  local i=$1
  local iface="${IFACE_PREFIX}${i}"
  local port=$((PROXY_BASE_PORT + i - 1))
  local addr="${BASE_ADDR}.$((i + 1))"
  local table=$((1000 + i))

  # Remove leftovers from an earlier run.
  pkill -f "tun2socks --device tun://${iface} " 2>/dev/null || true
  ip link del "$iface" 2>/dev/null || true
  ip rule del from "$addr" lookup "$table" 2>/dev/null || true

  mkdir -p "$RUN_DIR"
  echo "== $iface -> http://$PROXY_HOST:$port ($addr)"
  nohup "$BIN" --device "tun://${iface}" --proxy "http://${PROXY_HOST}:${port}" \
    --loglevel warn >"$RUN_DIR/${iface}.log" 2>&1 </dev/null &
  echo $! >"$RUN_DIR/${iface}.pid"

  # tun2socks creates the device; configure it once it exists.
  for _ in $(seq 1 50); do
    ip link show "$iface" >/dev/null 2>&1 && break
    sleep 0.1
  done
  ip link show "$iface" >/dev/null 2>&1 || {
    echo "error: $iface was not created; see $RUN_DIR/${iface}.log" >&2
    return 1
  }

  ip addr add "${addr}/32" dev "$iface" 2>/dev/null || true
  ip link set "$iface" up
  # Route traffic sourced from the adapter address into its TUN.
  ip rule add from "$addr" lookup "$table" 2>/dev/null || true
  ip route add default dev "$iface" table "$table" 2>/dev/null || true
}

stop_one() {
  local i=$1
  local iface="${IFACE_PREFIX}${i}"
  local addr="${BASE_ADDR}.$((i + 1))"
  local table=$((1000 + i))
  if [ -f "$RUN_DIR/${iface}.pid" ]; then
    kill "$(cat "$RUN_DIR/${iface}.pid")" 2>/dev/null || true
    rm -f "$RUN_DIR/${iface}.pid"
  fi
  # Fallback for runs whose PID file was lost.
  pkill -f "tun2socks --device tun://${iface} " 2>/dev/null || true
  ip rule del from "$addr" lookup "$table" 2>/dev/null || true
  ip route flush table "$table" 2>/dev/null || true
  ip link set "$iface" down 2>/dev/null || true
  ip link del "$iface" 2>/dev/null || true
  echo "removed $iface"
}

case "$ACTION" in
  setup)
    need_root setup
    build_tun2socks
    for i in 1 2 3 4 5; do start_one "$i"; done
    echo
    ip -br addr show | grep -E "${IFACE_PREFIX}[1-5]" || true
    echo
    echo "NetPilot discovery:"
    echo "  go run ./cmd/netpilot list -include $(seq -s, -f "${IFACE_PREFIX}%g" 1 5)"
    echo "  /tmp/netpilot -headless   # then: curl http://127.0.0.1:4545/api/interfaces"
    ;;
  teardown|down|delete)
    need_root teardown
    for i in 1 2 3 4 5; do stop_one "$i"; done
    ;;
  probe)
    fail=0
    for i in 1 2 3 4 5; do
      addr="${BASE_ADDR}.$((i + 1))"
      ip=$(curl -s -m 12 --interface "$addr" http://ifconfig.me || true)
      echo "${IFACE_PREFIX}${i} ($addr) -> ${ip:-<no answer>}"
      [ -n "$ip" ] || fail=1
    done
    exit "$fail"
    ;;
  *)
    echo "usage: $0 [setup|probe|teardown]" >&2
    exit 2
    ;;
esac

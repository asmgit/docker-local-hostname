#!/usr/bin/env bash
# docker-local-hostname installer. Sets up local *.ldev domains for multi-project docker dev on macOS:
#   1. docker-mac-net-connect — WireGuard tunnel so the host can reach container IPs.
#   2. docker-local-hostname daemon       — keeps /etc/hosts in sync with *.ldev containers.
# Idempotent: safe to re-run.
set -euo pipefail

DOMAIN="${DOCKER_LOCAL_HOSTNAME_DOMAIN:-.ldev}"
BIN=/usr/local/bin/docker-local-hostname
PLIST=/Library/LaunchDaemons/com.docker.local-hostname.plist
LABEL=com.docker.local-hostname
SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

log() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(uname)" = "Darwin" ] || die "docker-local-hostname is macOS-only (it relies on Docker Desktop networking)."
command -v docker >/dev/null || die "docker not found — install Docker Desktop first."
command -v brew   >/dev/null || die "Homebrew not found — install from https://brew.sh first."
[[ "$DOMAIN" =~ ^\.[a-z0-9]([a-z0-9-]*[a-z0-9])?$ ]] || die "invalid DOCKER_LOCAL_HOSTNAME_DOMAIN '$DOMAIN' (e.g. .ldev or .test)"

log "Installing docker-mac-net-connect (host <-> container tunnel)…"
if ! brew list docker-mac-net-connect >/dev/null 2>&1; then
  brew install chipmk/tap/docker-mac-net-connect
fi
sudo brew services start chipmk/tap/docker-mac-net-connect

log "Installing docker-local-hostname daemon (domain: ${DOMAIN})…"
sudo install -d -m 0755 "$(dirname "$BIN")"
sudo install -m 0755 "$SRC_DIR/bin/docker-local-hostname" "$BIN"

tmp_plist="$(mktemp)"
sed "s#__DOMAIN__#${DOMAIN}#g" "$SRC_DIR/launchd/com.docker.local-hostname.plist" > "$tmp_plist"
sudo install -m 0644 "$tmp_plist" "$PLIST"
rm -f "$tmp_plist"

# Reload, tolerating the bootout->bootstrap race (bootout is not synchronous).
sudo launchctl bootout "system/$LABEL" 2>/dev/null || true
for _ in 1 2 3 4 5 6 7 8 9 10; do
  sudo launchctl print "system/$LABEL" >/dev/null 2>&1 || break
  sleep 0.3
done
sudo launchctl bootstrap system "$PLIST" 2>/dev/null \
  || { sleep 1; sudo launchctl bootstrap system "$PLIST"; }

log "Done."
cat <<EOF

Verify:
  docker compose -f "$SRC_DIR/examples/project_1/compose.yaml" up -d
  curl http://project_1${DOMAIN}
  grep -A4 'BEGIN DOCKER-LOCAL-HOSTNAME' /etc/hosts

A project just needs a 'hostname: <name>${DOMAIN}' on its service and no host ports.
Daemon log: /var/log/docker-local-hostname.log
EOF

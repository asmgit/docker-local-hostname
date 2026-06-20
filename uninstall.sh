#!/usr/bin/env bash
# Remove the docker-local-hostname daemon and its /etc/hosts block. Leaves
# docker-mac-net-connect installed (remove it with: brew uninstall docker-mac-net-connect).
set -euo pipefail

BIN=/usr/local/bin/docker-local-hostname
PLIST=/Library/LaunchDaemons/com.docker.local-hostname.plist
LABEL=com.docker.local-hostname
BEGIN="# BEGIN DOCKER_LOCAL_HOSTNAME"
END="# END DOCKER_LOCAL_HOSTNAME"

log() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }

log "Stopping and removing docker-local-hostname daemon…"
sudo launchctl bootout "system/$LABEL" 2>/dev/null || true
sudo rm -f "$PLIST" "$BIN" /var/log/docker-local-hostname.log

log "Removing the managed block from /etc/hosts (atomically)…"
tmp="$(sudo mktemp /etc/hosts.ldev.XXXXXX)"
awk -v b="$BEGIN" -v e="$END" '$0==b{s=1;next} $0==e{s=0;next} !s{print}' /etc/hosts | sudo tee "$tmp" >/dev/null
sudo chmod 644 "$tmp"
sudo mv -f "$tmp" /etc/hosts
sudo rm -f /etc/hosts.ldev.bak
sudo dscacheutil -flushcache 2>/dev/null || true
sudo killall -HUP mDNSResponder 2>/dev/null || true

log "Done. (docker-mac-net-connect left installed.)"

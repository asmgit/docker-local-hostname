#!/usr/bin/env bash
# Remove the ldev-hosts daemon and its /etc/hosts block. Leaves
# docker-mac-net-connect installed (remove it with: brew uninstall docker-mac-net-connect).
set -euo pipefail

BIN=/usr/local/bin/ldev-hosts
PLIST=/Library/LaunchDaemons/com.ldev.hosts.plist
LABEL=com.ldev.hosts
BEGIN="# BEGIN LDEV CONTAINERS"
END="# END LDEV CONTAINERS"

log() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }

log "Stopping and removing ldev-hosts daemon…"
sudo launchctl bootout "system/$LABEL" 2>/dev/null || true
sudo rm -f "$PLIST" "$BIN" /var/log/ldev-hosts.log

log "Removing the managed block from /etc/hosts (atomically)…"
tmp="$(sudo mktemp /etc/hosts.ldev.XXXXXX)"
awk -v b="$BEGIN" -v e="$END" '$0==b{s=1;next} $0==e{s=0;next} !s{print}' /etc/hosts | sudo tee "$tmp" >/dev/null
sudo chmod 644 "$tmp"
sudo mv -f "$tmp" /etc/hosts
sudo rm -f /etc/hosts.ldev.bak
sudo dscacheutil -flushcache 2>/dev/null || true
sudo killall -HUP mDNSResponder 2>/dev/null || true

log "Done. (docker-mac-net-connect left installed.)"

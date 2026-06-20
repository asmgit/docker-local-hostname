# Docker Container Access by hostname on Mac

Connect directly to Docker containers via container hostname.

Based on [Docker Mac Net Connect](https://github.com/chipmk/docker-mac-net-connect)
and [this Stack Overflow answer](https://stackoverflow.com/a/63656003).

## Features

- Each container can be reached by **hostname** from the host macOS. Default domain: `*.ldev` (configurable).
- Works for **any TCP** — HTTP *and* databases (Postgres, MySQL, Redis…), not just web.
- **No published host ports** — multiple projects can reuse the same internal ports (`80`, `5432`, `3306`) without conflict.
- Updates automatically as containers start/stop. No DNS server: it keeps a managed block in `/etc/hosts` and resolves in ~1s.

A container opts in just by setting its `hostname` and publishing no ports.

## Installation

Requires macOS, Docker Desktop and Homebrew.

```bash
brew install asmgit/tap/docker-local-hostname
sudo brew services start asmgit/tap/docker-local-hostname
```

Runs as root (it creates the WireGuard tunnel and edits `/etc/hosts`). It **includes**
the docker-mac-net-connect tunnel — if you already run the upstream one, stop it first:

```bash
sudo brew services stop chipmk/tap/docker-mac-net-connect
```

## Example

```yaml
# compose.yaml — nothing published to the host
services:
  web:
    image: traefik/whoami
    hostname: app.ldev
  db:
    image: postgres:18-alpine
    hostname: db.app.ldev
    environment:
      POSTGRES_PASSWORD: secret
```

```bash
docker compose up -d

curl http://app.ldev                # -> the web container
psql -h db.app.ldev -U postgres     # password: secret
```

Copy it for another project with different hostnames (`app2.ldev`, `db.app2.ldev`):
the same internal ports won't collide, because nothing is bound to the host.

Runnable two-project demo: [`examples/`](examples/). Why it's built this way: [SPEC.md](SPEC.md).

## Uninstall

```bash
sudo brew services stop asmgit/tap/docker-local-hostname
brew uninstall asmgit/tap/docker-local-hostname
```

If `uninstall` reports "Permission denied" on root-owned paths (the service ran as
root), remove them first:

```bash
sudo rm -rf /opt/homebrew/Cellar/docker-local-hostname \
            /opt/homebrew/opt/docker-local-hostname \
            /opt/homebrew/var/homebrew/linked/docker-local-hostname
```

## Related projects

The hard part this adds is reaching **databases** by name on a shared port from
macOS Docker Desktop — others stop at HTTP.

| Project | What it does | Where it stops |
|---|---|---|
| [docker-mac-net-connect](https://github.com/chipmk/docker-mac-net-connect) | WireGuard tunnel that makes container IPs reachable from the Mac — **this builds on it**. | Reachability only; no name resolution. |
| [Portless](https://github.com/vercel-labs/portless) | Stable `*.localhost` URLs via an HTTPS reverse proxy. | **HTTP/HTTPS only** — not databases/TCP. |
| [dockportless](https://github.com/mazrean/dockportless) | Pretty URLs on one port with TLS-SNI routing. | TCP only via **TLS-SNI**; standard `psql`/`mysql` (STARTTLS) don't route by name. |
| [docker-hoster](https://github.com/dvddarias/docker-hoster) | Syncs `/etc/hosts` with container names via docker events — **the same idea**. | A container, so on Docker Desktop it can't reach `172.x` or flush the macOS DNS cache. |
| Reverse proxy (Traefik / nginx-proxy) | Routes HTTP by the `Host` header. | Can't route databases by name (no hostname in the raw TCP stream). |

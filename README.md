# docker-local-hostname — reach multi-project Docker by stable local hostnames on macOS

Run many Docker Compose projects at once and reach each one **by name** from your
Mac — `http://project_admin.ldev`, `db.project_admin.ldev:5432`, `http://project_mono.ldev`, …
— with **no published host ports**. Because nothing is bound to the host, every
project can use the **same internal ports** (`80`, `5432`, `3306`); they never
collide. Install once, then it just works.

```
curl http://project_admin.ldev      ->  project_admin's web container
psql -h db.project_admin.ldev       ->  project_admin's postgres
curl http://project_mono.ldev       ->  project_mono's web container   (also on :80, no conflict)
psql -h db.project_mono.ldev        ->  project_mono's postgres        (also on :5432, no conflict)
```

## The problem

Running several Compose projects locally, the usual pain is **host ports**.
If `project_admin` publishes `5432:5432`, `project_mono` can't — you start juggling
`5433`, `5434`, remembering which port is which, and rewriting `.env` files.
A reverse proxy fixes HTTP (it routes by the `Host` header), but **databases
can't be routed by name** — the MySQL/Postgres wire protocol carries no hostname
(TLS/SNI only comes up *after* a plaintext greeting), so a proxy can't tell two
databases apart on one `IP:port`. See [SPEC.md](SPEC.md) for the full analysis.

`docker-local-hostname` takes a different route: give **each container its own IP**,
reachable from the Mac, and resolve each project's hostname to its IP. No host
ports, no reverse proxy, no per-project port bookkeeping. Routing is by
**name → IP**, so ports are free to repeat. The domain is configurable (examples
below use `.ldev`).

## How it works

It's **one binary** (a fork of [docker-mac-net-connect](https://github.com/chipmk/docker-mac-net-connect)
with a built-in `/etc/hosts` sync), run as a single root service:

```
 Mac:  curl http://project_admin.ldev
   |  /etc/hosts:  project_admin.ldev -> 172.20.0.3
   v
   docker-local-hostname (one root service):
     • WireGuard tunnel       -> the Mac can reach container 172.x IPs
     • watches docker events  -> writes the *.ldev hosts, flushes the DNS cache
   v
   project_admin's web container  --(service name 'db')-->  project_admin's db container
```

There is **no DNS server** and **no `/etc/resolver`**: `/etc/hosts` is consulted
before DNS and isn't subject to negative-cache stalls, so a container that comes
back up resolves again in about a second.

## Requirements

macOS with **Docker Desktop**.

## Install

```bash
brew install asmgit/tap/docker-local-hostname
sudo brew services start asmgit/tap/docker-local-hostname
```

It runs as **root** (it creates the WireGuard tunnel and edits `/etc/hosts`).
It **includes** the docker-mac-net-connect tunnel, so don't run both — if you have
the upstream tunnel running, stop it first:

```bash
sudo brew services stop chipmk/tap/docker-mac-net-connect
```

## Use it

Give each service a `hostname` ending in `.ldev` and **publish nothing**:

```yaml
# project_admin/compose.yaml
name: project_admin
services:
  web:
    image: traefik/whoami
    hostname: project_admin.ldev
  db:
    image: postgres:18-alpine
    hostname: db.project_admin.ldev
    environment: { POSTGRES_PASSWORD: secret }
    volumes: [ "dbdata:/var/lib/postgresql" ]
volumes: { dbdata: {} }
```

```bash
docker compose -f examples/project_admin/compose.yaml up -d
docker compose -f examples/project_mono/compose.yaml up -d

curl http://project_admin.ldev          # -> project_admin web
curl http://project_mono.ldev           # -> project_mono web   (also :80, no conflict)
psql -h db.project_admin.ldev -U postgres
psql -h db.project_mono.ldev -U postgres
```

No IP is ever written into a project file; only the **name** lives in `compose.yaml`.

### Add another project

Copy a project, change the `hostname`s (`project_shop.ldev`, `db.project_shop.ldev`),
keep the same internal ports, publish nothing. Nothing else to configure.

## Verify

```bash
grep -A6 'BEGIN DOCKER_LOCAL_HOSTNAME' /etc/hosts   # the managed block
sudo brew services list                             # service running?
tail /opt/homebrew/var/log/docker-local-hostname.log
```

## Configuration

The domain defaults to `.ldev`, configurable via the `DOCKER_LOCAL_HOSTNAME_DOMAIN`
environment variable in the service plist
(`/Library/LaunchDaemons/homebrew.mxcl.docker-local-hostname.plist`) — edit it and
`sudo brew services restart asmgit/tap/docker-local-hostname`. Use a domain the OS
doesn't special-case (`.ldev`/`.test` are fine; **not** `.local`, which is mDNS).

## Uninstall

```bash
sudo brew services stop asmgit/tap/docker-local-hostname
brew uninstall asmgit/tap/docker-local-hostname
```

## Source & build

The binary is built from a fork of docker-mac-net-connect that adds a
`hostsmanager` package (the `/etc/hosts` sync) — a small additive change hooked
into `main` with one line.

- Source: <https://github.com/asmgit/docker-mac-net-connect/tree/docker-local-hostname>
- Formula: <https://github.com/asmgit/homebrew-tap>

## Related projects

Other tools solve part of this. The hard part `docker-local-hostname` adds is
reaching **databases** by name on a shared port from macOS Docker Desktop.

| Project | What it does | Where it stops (for this use case) |
|---|---|---|
| [docker-mac-net-connect](https://github.com/chipmk/docker-mac-net-connect) | WireGuard tunnel that makes container IPs reachable from the Mac — **this project builds on it**. | Reachability only; no name resolution. |
| [Portless](https://github.com/vercel-labs/portless) ([portless.sh](https://portless.sh)) | Stable `*.localhost` URLs via an HTTPS reverse proxy with auto-assigned ports. | **HTTP/HTTPS only** — not databases/TCP. |
| [dockportless](https://github.com/mazrean/dockportless) | Portless-style for any Compose tool; pretty URLs on one port with TLS-SNI routing. | TCP only via **TLS-SNI**; standard `psql`/`mysql` (STARTTLS) still don't route by name. |
| [docker-hoster](https://github.com/dvddarias/docker-hoster) | Keeps `/etc/hosts` in sync with container names via docker events — **the same core idea**. | Runs as a container, so on Docker Desktop it can't reach `172.x` IPs or flush the macOS DNS cache. |
| Reverse proxy (Traefik / nginx-proxy) | Routes HTTP by the `Host` header. | Can't route databases by name (no hostname in the raw TCP stream). |

## Credits & references

- [**docker-mac-net-connect**](https://github.com/chipmk/docker-mac-net-connect) by chipmk — the WireGuard host↔container tunnel this builds on (MIT).
- The **`/etc/hosts`-from-docker-events** approach is prior art: this [Stack Overflow answer](https://stackoverflow.com/a/63656003) (bash + docker events) and [docker-hoster](https://github.com/dvddarias/docker-hoster). `docker-local-hostname` pairs the idea with the tunnel **and a host-side DNS flush** so it works for **databases** on macOS, not just HTTP.
- Pretty local-URL inspiration: [Portless](https://github.com/vercel-labs/portless).

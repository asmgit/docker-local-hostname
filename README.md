# docker-local-hostname — reach multi-project Docker by stable local hostnames on macOS

Run many Docker Compose projects at once and reach each one **by name** from your
Mac — `http://project_admin.ldev`, `db.project_admin.ldev:5432`, `http://project_mono.ldev`, …
— with **no published host ports**. Because nothing is bound to the host, every
project can use the **same internal ports** (`80`, `5432`, `3306`); they never
collide. Install once, then it just works.

```
curl http://project_admin.ldev      ->  project_admin's web container
psql -h db.project_admin.ldev       ->  project_admin's postgres
curl http://project_mono.ldev      ->  project_mono's web container   (also on :80, no conflict)
psql -h db.project_mono.ldev       ->  project_mono's postgres        (also on :5432, no conflict)
```

## The problem

Running several Compose projects locally, the usual pain is **host ports**.
If `project_admin` publishes `5432:5432`, `project_mono` can't — you start juggling
`5433`, `5434`, remembering which port is which, and rewriting `.env` files.
A reverse proxy fixes HTTP (it routes by the `Host` header), but **databases
can't be routed by name** — the MySQL/Postgres wire protocol carries no hostname
(TLS/SNI only comes up *after* a plaintext greeting), so a proxy can't tell two
databases apart on one `IP:port`. See [SPEC.md](SPEC.md) for the full analysis.

`docker-local-hostname` takes a different route: give **each container its own IP**, reachable
from the Mac, and resolve each project's hostname to its IP. No host ports, no reverse
proxy, no per-project port bookkeeping. Routing is by **name → IP**, so ports are
free to repeat. The domain is configurable (examples below use `.ldev`).

## How it works

```
 Mac:  curl http://project_admin.ldev
   |  /etc/hosts:  project_admin.ldev -> 172.20.0.3   (kept in sync by the docker-local-hostname daemon)
   v
   docker-mac-net-connect routes 172.x from the Mac into the Docker VM
   v
   project_admin's web container  --(by service name 'db')-->  project_admin's db container
```

Two small pieces, both installed by `install.sh`:

| Component | Role |
|---|---|
| [`docker-mac-net-connect`](https://github.com/chipmk/docker-mac-net-connect) | A WireGuard tunnel so the Mac can reach container IPs (`172.x`). Docker Desktop normally hides them. |
| `docker-local-hostname` (this repo) | A tiny launchd daemon that watches Docker events and keeps a managed block in `/etc/hosts` mapping every `*.ldev` container hostname to its IP, flushing the DNS cache on change. |

There is **no DNS server** and **no `/etc/resolver`**: `/etc/hosts` is consulted
before DNS and isn't subject to negative-cache stalls, so a container that comes
back up resolves again in about a second.

## Requirements

- macOS with **Docker Desktop**
- **Homebrew** (for `docker-mac-net-connect`)

## Install

```bash
git clone https://github.com/asmgit/docker-local-hostname.git
cd docker-local-hostname
./install.sh
```

`install.sh` is idempotent. It installs and starts `docker-mac-net-connect`,
installs the `docker-local-hostname` daemon, and asks for `sudo` (the daemon edits
`/etc/hosts` and runs as root, which is required to flush the DNS cache).

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
curl http://project_mono.ldev          # -> project_mono web   (also :80, no conflict)
psql -h db.project_admin.ldev -U postgres
psql -h db.project_mono.ldev -U postgres
```

That's it — the daemon notices the containers and updates `/etc/hosts`. No IP
is ever written into a project file; only the **name** lives in `compose.yaml`.

### Add another project

Copy a project, change the `hostname`s (`project_shop.ldev`, `db.project_shop.ldev`),
keep the same internal ports, publish nothing. Nothing else to configure.

## Verify

```bash
grep -A6 'BEGIN DOCKER_LOCAL_HOSTNAME' /etc/hosts     # the managed block
cat /var/log/docker-local-hostname.log          # daemon activity
```

## Configuration

The domain defaults to `.ldev`. To use another (e.g. `.test`):

```bash
DOCKER_LOCAL_HOSTNAME_DOMAIN=.test ./install.sh
```

## Troubleshooting

- **A name doesn't resolve / `curl: could not resolve host`.** Check the block
  in `/etc/hosts` and `/var/log/docker-local-hostname.log`. Make sure the container's
  `hostname` actually ends in your domain.
- **Name resolves but the connection hangs/refuses.** That's the tunnel, not
  DNS. Confirm `docker-mac-net-connect` is running:
  `sudo brew services list | grep docker-mac-net-connect`, and that
  `curl http://<container-ip>` works.
- **Nothing updates after `up`.** Restart the daemon:
  `sudo launchctl kickstart -k system/com.docker.local-hostname`.

## Uninstall

```bash
./uninstall.sh
# and, if you also want the tunnel gone:
brew uninstall docker-mac-net-connect
```

## A more integrated option (single binary)

`docker-local-hostname` is intentionally a small shell daemon layered on top of the
upstream tunnel. The same logic can be folded **into** `docker-mac-net-connect`
so one binary does both the tunnel and the `/etc/hosts` sync — see
[`fork/`](fork/) for that variant and the rationale.

## Why not just …?

Short version (full version in [SPEC.md](SPEC.md)):

| Approach | Verdict on macOS |
|---|---|
| Different host ports per project | Works, but you manage a port map forever; not "same ports". |
| Reverse proxy (Traefik) by name | Great for HTTP; **cannot** route databases by name (no hostname in the TCP/SNI stream). |
| dnsmasq / custom DNS | Resolves names, but on Docker Desktop the container IPs it returns aren't reachable, and unknown names negative-cache. |
| macvlan | The clean Linux answer; on Docker Desktop `parent=eth0` is the VM, not your LAN — doesn't expose containers. |
| **docker-mac-net-connect + /etc/hosts (this)** | Each container gets a reachable IP; names map to IPs in `/etc/hosts`; ports are free to repeat. |

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

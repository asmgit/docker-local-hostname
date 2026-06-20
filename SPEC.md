# Specification & design notes

This document states the problem precisely, records why the obvious solutions
fail on macOS Docker Desktop, and explains the chosen design and its trade-offs.
It is the "why" behind the few small files in this repo.

## 1. Problem statement

Local development with **multiple Docker Compose projects running at the same
time** on macOS. Requirements:

1. **Reach each project from the Mac by a stable name**, for both HTTP and
   databases — e.g. `http://project_admin.ldev`, `db.project_admin.ldev:5432`.
2. **No per-project host-port bookkeeping.** Projects should be free to use the
   **same internal ports** (`80`, `5432`, `3306`) without colliding.
3. **No hard-coded IP addresses** in project files — a project declares a
   *name*, not an address.
4. **Run two or more projects simultaneously.**
5. **Install once; it keeps working** across reboots and `compose up/down`.

Access is from the **developer's own Mac** (localhost), not from other machines
on the network. (LAN access is possible but out of scope — it needs either a
router static route or an extra IP per project; see §6.)

## 2. Environment constraints (the hard part)

On **Docker Desktop for Mac**, containers run inside a Linux VM. Two facts drive
everything:

- **Container IPs (`172.x`) are not reachable from the Mac.** Unlike Docker on
  Linux, there is no `docker0` bridge on the host; you can only reach containers
  through *published ports*.
- **Two services cannot share one `host-IP:port`.** So "both databases on
  `:5432`/`:3306`" is impossible if both publish to the same host IP.

## 3. Why the obvious approaches don't satisfy the spec

### 3.1 Different host ports
`project_admin` db on `5432`, `project_mono` db on `5433`, … Works and is simple, but
violates requirement 2 (same ports) and means a growing port map to remember.

### 3.2 Reverse proxy (Traefik/nginx) routing by name
A proxy routes **HTTP** by the `Host` header — perfect for `http://project_admin.ldev`.
But it **cannot route databases by name**. The MySQL/Postgres/MariaDB wire
protocol begins with a **plaintext server greeting**; TLS (and therefore SNI, the
only place a hostname appears in a TCP stream) is negotiated *afterwards*. A
TCP/SNI router never sees a hostname at connect time. Traefik's own docs confirm
it: a TCP router can only use a real domain in `HostSNI` **with TLS**, and for
non-TLS TCP the only rule is `HostSNI(`*`)` — a catch-all, one backend per port.
Community attempts to put `HostSNI(`db.example.com`)` in front of MySQL fail with
"Lost connection". **Conclusion: two databases on one `IP:port` cannot be split
by name by any proxy.** They need different IPs or different ports.

### 3.3 dnsmasq / a custom DNS server
A DNS server (dnsmasq, dns-proxy-server, CoreDNS) can resolve `*.ldev`. But:
- It must resolve to an IP that is **reachable**. Container `172.x` IPs are not
  reachable from the Mac (§2), so resolving to them doesn't help by itself.
- Pointing all names at one host IP brings back the database-collision problem.
- `dns-proxy-server` auto-discovers container IPs — but those are the
  unreachable `172.x`, so on Docker Desktop it only helps for HTTP-via-proxy, not
  database-by-name.

### 3.4 macvlan
On Linux, `docker network create -d macvlan -o parent=eth0` gives each container
a **real LAN IP** — the clean answer. On Docker Desktop, `eth0` is the **VM's**
interface behind NAT, not your Mac's NIC, so macvlan IPs are not on your network
and aren't reachable. macvlan is a Linux-host solution, not a Docker-Desktop one.

### 3.5 Custom `/etc/resolver` + DNS server
You can point `*.ldev` at a local DNS server via `/etc/resolver/ldev`. This
works, but adds a moving part (the DNS server) and inherits **DNS negative
caching**: when a name doesn't resolve (container down), macOS caches the
failure. If the DNS server forwards unknown `.ldev` upstream, the cached negative
carries the **root servers' SOA with an 86400-second (24h) TTL** — the name then
stays "not found" for a very long time after the container returns, unless you
flush the cache. Even after making the DNS server authoritative (short TTL) the
floor is macOS's own negative cache (~30–75s). DNS is simply the wrong layer for
something that changes this often.

## 4. The chosen solution

Two requirements force the shape:

- "Reach databases by name on the same port" ⇒ **each project/container needs its
  own reachable IP** (no proxy can substitute).
- "No host ports, names not addresses, fast updates" ⇒ resolve names to those
  IPs in a way that updates instantly and doesn't negative-cache.

So:

1. **[`docker-mac-net-connect`](https://github.com/chipmk/docker-mac-net-connect)**
   creates a WireGuard tunnel between the Mac and the Docker VM and adds routes
   for the Docker subnets. Now the Mac can reach any container by its `172.x` IP
   on any port. This removes constraint §2 (unreachable IPs). Each container
   already has its own IP, so identical ports across projects never collide.

2. **`docker-local-hostname`** (this repo) maps names to those IPs using **`/etc/hosts`**,
   not DNS:
   - It watches Docker `container` events over the API socket.
   - On any change it lists running containers, takes each one whose
     `Config.Hostname` ends in the domain (default `.ldev`), and writes
     `IP hostname` lines into a delimited block in `/etc/hosts`.
   - When the set changes it flushes the macOS DNS cache
     (`dscacheutil -flushcache; killall -HUP mDNSResponder`).

`/etc/hosts` is checked **before** DNS and has no TTL/negative-cache machinery,
so a name appears the instant the container starts and disappears when it stops.
Measured recovery after `down`/`up`: **~1 second**, with no manual flush.

### Resolution path

```
project_admin.ldev
  -> /etc/hosts        (docker-local-hostname wrote: 172.20.0.3 project_admin.ldev)
  -> 172.20.0.3        (route via docker-mac-net-connect's utun)
  -> container :80 / :5432 / :3306
```

### Why each piece is minimal

- No DNS server, no `/etc/resolver`: `/etc/hosts` wins and never stalls.
- No host ports: reachability is by container IP, so ports may repeat freely.
- No IPs in project files: a project declares `hostname: name.ldev`; the daemon
  discovers the IP at runtime. Container restarts get new IPs — the daemon just
  rewrites the block.

## 5. Design decisions & trade-offs

- **`/etc/hosts` vs DNS.** `/etc/hosts` is authoritative, immediate, and immune
  to negative caching. The cost is that it's a global system file — `docker-local-hostname`
  only ever touches its own `# BEGIN/END DOCKER_LOCAL_HOSTNAME` block and leaves all
  other lines untouched.
- **Flush on change.** macOS's `mDNSResponder` does not always re-read
  `/etc/hosts` immediately; a flush guarantees ~1s pickup. It only fires when the
  `.ldev` set actually changes, not on every unrelated container event.
- **Root daemon.** Editing `/etc/hosts` and flushing the DNS cache both require
  root, so the daemon runs from `/Library/LaunchDaemons` (boot scope).
- **Name from `Config.Hostname`.** Set `hostname:` on the service. (A Docker
  *network alias* is what makes a name resolvable *between containers*; the
  *hostname* is what this host-side daemon reads. They are different mechanisms —
  see the note in §7.)
- **Depends on a third-party tunnel.** `docker-mac-net-connect` is the only thing
  that can make `172.x` reachable on Docker Desktop without per-project IP hacks.
  The §8 fork folds it in so there is a single binary.

## 6. Out of scope: LAN access

Reaching these names from *other* machines needs the container subnet routed to
your Mac (a static route on the router, plus IP forwarding) or a real per-project
IP. On a typical ISP router that can't do static routes, this isn't practical;
`docker-local-hostname` targets the developer's own machine.

## 7. Note: hostname vs network alias

Inside Docker, containers resolve each other by **service name**, **container
name**, or **network alias** via the embedded DNS at `127.0.0.11`. They do *not*
resolve each other by `hostname`. The host-side `docker-local-hostname` daemon, however,
reads the container's `Config.Hostname` from the Docker API — so for `docker-local-hostname` you
set `hostname:`. If you also want container-to-container resolution by the same
`.ldev` name, add it as a network `alias` too.

## 8. Optimization: fold it into one binary

`docker-mac-net-connect` already runs as root, already holds a Docker client, and
already watches Docker events. The natural simplification is to add the
`/etc/hosts` sync **inside it**, so a single `brew install` + one service does
everything. The [`fork/`](fork/) directory contains that integration (a small Go
`hostsmanager` plus a hook in `main.go`) and notes on building it. The shell
daemon in this repo remains the zero-build, easy-to-audit default.

## 9. Lessons (measured)

| Stage | Recovery after down/up | Why |
|---|---|---|
| DNS server forwarding unknown `.ldev` upstream | ~77 s (then stuck pending flush) | root SOA negative TTL 86400 |
| DNS server made authoritative (`remote: off`) | ~30 s | macOS default negative cache, plus a dead resolver still in the chain |
| **`/etc/hosts` + flush-on-change (this repo)** | **~1 s** | hosts file beats DNS, no negative caching |

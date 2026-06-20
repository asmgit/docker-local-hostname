# Single-binary variant (fork of docker-mac-net-connect)

The default `ldev` setup is a tiny shell daemon (`../bin/ldev-hosts`) layered on
top of the upstream tunnel — zero build, easy to audit. This directory documents
the **more integrated** option: folding the `/etc/hosts` sync **into**
`docker-mac-net-connect` so a single binary does both the tunnel and name
resolution.

**Live fork:** https://github.com/asmgit/docker-mac-net-connect/tree/ldev-hosts

## What the fork changes

It's a small, additive change (the upstream already runs as root, already holds
a Docker client, and already watches Docker events):

- adds [`hostsmanager.go`](hostsmanager.go) (a `hostsmanager` package), and
- adds one line to `main.go`, reusing the existing Docker client:

  ```go
  // after: ctx := context.Background()
  go hostsmanager.Run(ctx, cli)
  ```

`hostsmanager` watches container `start`/`die`/`destroy` events, lists running
containers, takes each one whose `Config.Hostname` ends in `LDEV_DOMAIN`
(default `.ldev`), writes `IP hostname` lines into the `# BEGIN/END LDEV
CONTAINERS` block in `/etc/hosts`, and flushes the macOS DNS cache when the set
changes.

## Build & run

```bash
git clone -b ldev-hosts https://github.com/asmgit/docker-mac-net-connect
cd docker-mac-net-connect
go build -o docker-mac-net-connect .
sudo LDEV_DOMAIN=.ldev ./docker-mac-net-connect
```

(Or package it as a Homebrew formula / launchd daemon the same way upstream is.)

## Which should I use?

| | shell daemon (default) | single binary (this fork) |
|---|---|---|
| Build step | none | `go build` |
| Moving parts | upstream brew tool + 1 shell daemon | one binary |
| Auditability | trivial (read one script) | read the Go diff |
| Recommendation | start here | adopt if you want one process |

Both deliver identical behavior (~1s name resolution after `up`, no host ports,
duplicate ports across projects).

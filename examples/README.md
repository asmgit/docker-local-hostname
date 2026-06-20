# Examples

Two minimal projects that prove the core idea: **identical internal ports, no
host ports, reached by name.** Both use off-the-shelf images, so there's nothing
to build.

```bash
docker compose -f project_admin/compose.yaml up -d
docker compose -f project_mono/compose.yaml up -d
```

Both `web` services listen on `:80` and both `db` services on `:5432`, with
**no** ports published to the host — yet they don't conflict, because each name
resolves to its own container IP:

```bash
curl http://project_admin.ldev        # traefik/whoami prints Host: project_admin.ldev
curl http://project_mono.ldev        # traefik/whoami prints Host: project_mono.ldev

psql -h db.project_admin.ldev -U postgres   # password: secret
psql -h db.project_mono.ldev -U postgres   # password: secret
```

Check what the daemon wrote:

```bash
grep -A6 'BEGIN DOCKER_LOCAL_HOSTNAME' /etc/hosts
```

Tear down:

```bash
docker compose -f project_admin/compose.yaml down
docker compose -f project_mono/compose.yaml down
```

The names vanish from `/etc/hosts` within a second of `down`, and reappear within
a second of `up` — no manual DNS flush needed.

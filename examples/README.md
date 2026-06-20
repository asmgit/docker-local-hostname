# Examples

Two minimal projects that prove the core idea: **identical internal ports, no
host ports, reached by name.** Both use off-the-shelf images, so there's nothing
to build.

```bash
docker compose -f project_1/compose.yaml up -d
docker compose -f project_2/compose.yaml up -d
```

Both `web` services listen on `:80` and both `db` services on `:5432`, with
**no** ports published to the host — yet they don't conflict, because each name
resolves to its own container IP:

```bash
curl http://project_1.ldev        # traefik/whoami prints Host: project_1.ldev
curl http://project_2.ldev        # traefik/whoami prints Host: project_2.ldev

psql -h db.project_1.ldev -U postgres   # password: secret
psql -h db.project_2.ldev -U postgres   # password: secret
```

Check what the daemon wrote:

```bash
grep -A6 'BEGIN DOCKER-LOCAL-HOSTNAME' /etc/hosts
```

Tear down:

```bash
docker compose -f project_1/compose.yaml down
docker compose -f project_2/compose.yaml down
```

The names vanish from `/etc/hosts` within a second of `down`, and reappear within
a second of `up` — no manual DNS flush needed.

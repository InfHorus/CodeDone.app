# Shell / DevOps

Use this skill when the task touches shell scripts, deployment, Docker, Linux services, reverse proxies, CI/CD, environment variables, logs, permissions, ports, or production runtime behavior.

Keep changes operationally safe. Prefer diagnosis before rewrites.

## Activation

Use for:

- `.sh`, `.bash`, `.zsh`, `.ps1`, Dockerfile, `docker-compose.yml`, GitHub Actions, GitLab CI, systemd units, Nginx/Apache configs, deployment scripts, cron jobs, SSH/SCP workflows.
- Linux service debugging, ports/processes, permissions, environment variables, logs, TLS/reverse proxy, containers, CI failures, build pipelines, and production startup issues.

Do not use for:

- Normal application code unless the issue is deployment/runtime/tooling related.

## Core Rule

DevOps code changes can break live systems. Be explicit, reversible, and conservative. Diagnose first, then change the smallest thing that explains the failure.

## 80/20 Workflow

1. Identify runtime: local, CI, container, VPS, systemd, serverless, or managed platform.
2. Read existing configs before writing new ones.
3. Preserve ports, domains, paths, user/group, environment, and secrets flow.
4. Make commands copy-pasteable and idempotent when possible.
5. Avoid destructive commands unless explicitly required.
6. Show verification commands after changes.
7. For services, check logs and process/port state, not only “active” status.

## Shell Script Standards

For Bash scripts, start with safe defaults when appropriate:

```bash
#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'
```

Use quotes around variables:

```bash
cp "$src" "$dest"
```

Prefer arrays for command arguments:

```bash
args=(--port "$PORT" --host 0.0.0.0)
uvicorn app:app "${args[@]}"
```

Avoid:

```bash
rm -rf $DIR       # unquoted, dangerous
for f in $(ls)    # breaks on spaces
```

Better:

```bash
[[ -n "${DIR:-}" && -d "$DIR" ]] && rm -rf -- "$DIR"
find . -type f -name '*.log' -print0 | while IFS= read -r -d '' file; do
  echo "$file"
done
```

## Environment Variables and Secrets

Never hardcode secrets in scripts, Dockerfiles, CI YAML, or logs.

Use:

- `.env.example` for names without values.
- CI secret stores for tokens.
- systemd `EnvironmentFile=` or platform secret management.
- Runtime injection rather than baking secrets into images.

Do not print full tokens. Mask them or print only presence:

```bash
: "${API_TOKEN:?API_TOKEN is required}"
echo "API_TOKEN is set"
```

## Docker

Good Dockerfiles are small, reproducible, and do not run as root unless necessary.

Node example pattern:

```dockerfile
FROM node:22-alpine AS deps
WORKDIR /app
COPY package*.json ./
RUN npm ci

FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production
COPY --from=deps /app/node_modules ./node_modules
COPY . .
USER node
CMD ["node", "server.js"]
```

Python example pattern:

```dockerfile
FROM python:3.12-slim
WORKDIR /app
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
CMD ["python", "-m", "app"]
```

Common Docker mistakes:

- Copying the whole repo before installing deps, destroying layer cache.
- Missing `.dockerignore`.
- Running dev servers in production images.
- Baking `.env` or credentials into the image.
- Assuming localhost inside a container means the host.

## docker-compose

Use service names for inter-container networking:

```yaml
services:
  api:
    build: .
    environment:
      DATABASE_URL: postgres://app:app@db:5432/app
    depends_on:
      - db
  db:
    image: postgres:16
```

`depends_on` does not guarantee the database is ready. Add healthchecks or app retry logic for real readiness.

## systemd

A service should define working directory, user, environment, restart behavior, and the actual process.

```ini
[Unit]
Description=Example API
After=network-online.target
Wants=network-online.target

[Service]
WorkingDirectory=/opt/example
ExecStart=/opt/example/venv/bin/python -m uvicorn app:app --host 127.0.0.1 --port 8000
Restart=always
RestartSec=3
User=example
EnvironmentFile=-/etc/example/example.env

[Install]
WantedBy=multi-user.target
```

Useful commands:

```bash
sudo systemctl daemon-reload
sudo systemctl restart example.service
sudo systemctl status example.service --no-pager
sudo journalctl -u example.service -n 100 --no-pager
sudo journalctl -u example.service -f
```

Do not trust `active (running)` alone. Check logs and port binding.

## Ports, Processes, and Logs

Common diagnosis commands:

```bash
ss -lntp | grep ':8000'
ps -p <pid> -o pid,stat,etimes,%cpu,%mem,cmd
curl -i http://127.0.0.1:8000/health
journalctl -u service-name -n 200 --no-pager
```

For container logs:

```bash
docker compose ps
docker compose logs -f api
docker compose exec api sh
```

## Nginx / Apache Reverse Proxy

Reverse proxy configs should preserve host/protocol headers and target the correct local upstream.

Nginx pattern:

```nginx
location / {
  proxy_pass http://127.0.0.1:8000;
  proxy_http_version 1.1;
  proxy_set_header Host $host;
  proxy_set_header X-Real-IP $remote_addr;
  proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  proxy_set_header X-Forwarded-Proto $scheme;
}
```

Apache pattern:

```apache
ProxyPreserveHost On
ProxyPass / http://127.0.0.1:8000/
ProxyPassReverse / http://127.0.0.1:8000/
```

Validate before reload:

```bash
sudo nginx -t && sudo systemctl reload nginx
sudo apachectl configtest && sudo systemctl reload apache2
```

## CI/CD

Before editing CI, identify:

- package manager and lockfile.
- language/runtime version.
- cache strategy.
- test/build commands.
- deployment side effects.
- required secrets.

GitHub Actions pattern:

```yaml
name: CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 22
          cache: npm
      - run: npm ci
      - run: npm run typecheck --if-present
      - run: npm test --if-present
```

Avoid changing CI to “pass” by removing tests or ignoring failures unless explicitly requested.

## Permissions

Prefer least privilege:

- Services should not run as root unless necessary.
- Writeable directories should be narrow.
- Executable scripts need `chmod +x`.
- Web servers need access only to served files and required sockets.

Common fixes:

```bash
sudo chown -R appuser:appuser /opt/app
chmod +x scripts/deploy.sh
```

Avoid broad `chmod -R 777`.

## PowerShell Notes

Use PowerShell for Windows-native automation when the repo uses `.ps1` or Windows deployment.

Good habits:

```powershell
$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest
```

Quote paths and use `Join-Path` for path composition.

## Common AI Mistakes

Avoid these:

- Replacing the project’s package manager: `pnpm` repo suddenly using `npm`, or Poetry repo using raw `pip`.
- Writing commands that only work from the wrong working directory.
- Using `sudo` inside scripts that should run in CI/container.
- Restarting services without `daemon-reload` after changing unit files.
- Ignoring firewall, reverse proxy, and app bind address mismatch.
- Exposing internal service ports publicly when they should bind to `127.0.0.1`.
- Logging secrets while debugging.
- Assuming Linux commands work on Windows or vice versa.
- Using destructive cleanup commands without guards.
- Adding Docker but not `.dockerignore`.

## Verification Checklist

Use what applies:

```bash
# Shell
bash -n script.sh
shellcheck script.sh

# Docker
docker build .
docker compose config
docker compose up --build

# systemd
sudo systemctl daemon-reload
sudo systemctl status service --no-pager
sudo journalctl -u service -n 100 --no-pager

# Reverse proxy
sudo nginx -t
sudo apachectl configtest
curl -i https://domain.example/health

# Ports
ss -lntp
curl -i http://127.0.0.1:PORT/
```

If a command is unavailable, do not invent results. State what could not be verified.

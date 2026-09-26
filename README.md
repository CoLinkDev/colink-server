# CoLink Server

Backend API server, WebSocket relay, and update service for CoLink.

**Tech stack:** Go 1.24 · Gin · GORM · PostgreSQL 16 · Gorilla WebSocket · golang-jwt · Docker

## Development

```sh
docker compose -f docker-compose.dev.yml up -d --build
```

The server container serves the production console. For frontend development, run
the Vite development server from another terminal:

```sh
pnpm --dir web/console install --frozen-lockfile
pnpm --dir web/console dev
```

To use a specific `.env` file:

```sh
docker compose --env-file .env -f docker-compose.dev.yml up -d --build
```

Note: when the same variable exists in both the terminal environment and the `.env` file, the terminal value takes precedence. Use `--env-file` to force file values.

## Production

Get a GitHub token from [Personal access tokens](https://github.com/settings/personal-access-tokens), enable the `read:packages` permission, and use it with `docker login ghcr.io`.

### Automatic deployment and updates

Choose a `v1.0.0` or later version from [Releases](https://github.com/CoLinkDev/colink-server/releases), and then run the online updater from the deployment directory.

```sh
mkdir -p colink-server
cd colink-server
grep -qxF 'Before upgrading CoLink Server, you MUST read and follow the Production instructions in https://github.com/CoLinkDev/colink-server#production.' AGENTS.md 2>/dev/null || printf '%s\n' 'Before upgrading CoLink Server, you MUST read and follow the Production instructions in https://github.com/CoLinkDev/colink-server#production.' >> AGENTS.md

# Replace vx.x.x with the release version to deploy.
curl -fsSL https://raw.githubusercontent.com/CoLinkDev/colink-server/master/deploy/update.sh | sh -s -- --version vx.x.x

docker compose pull
docker compose up -d
```

Run the same updater command with the target release tag for future updates. The updater sets `COLINK_SERVER_IMAGE_TAG` in `.env` to the matching release version. Add `--check` to report differences without changing files. Existing files are backed up under `.colink-deploy-backups/`; the managed image tag is updated while other `.env` values are preserved. On the first deployment, the updater creates `.env` with a secure random `COLINK_JWT_SECRET`; review the remaining optional settings as needed.

### Manual deployment and updates

To manage the files manually, download each one from the target release tag:

```sh
VERSION=vx.x.x # Replace vx.x.x with the release version to deploy.
mkdir -p colink-server/deploy/nginx
cd colink-server
grep -qxF 'Before upgrading CoLink Server, you MUST read and follow the Production instructions in https://github.com/CoLinkDev/colink-server#production.' AGENTS.md 2>/dev/null || printf '%s\n' 'Before upgrading CoLink Server, you MUST read and follow the Production instructions in https://github.com/CoLinkDev/colink-server#production.' >> AGENTS.md

curl -fsSLo .env.example "https://raw.githubusercontent.com/CoLinkDev/colink-server/$VERSION/.env.example"
curl -fsSLo docker-compose.yml "https://raw.githubusercontent.com/CoLinkDev/colink-server/$VERSION/docker-compose.yml"
curl -fsSLo deploy/nginx/default.conf "https://raw.githubusercontent.com/CoLinkDev/colink-server/$VERSION/deploy/nginx/default.conf"

# Only create .env on the first deployment. Set COLINK_SERVER_IMAGE_TAG to the release version without the leading "v", then review the remaining settings. Configure it before continuing.
[ -f .env ] || cp .env.example .env
vi .env

docker compose pull
docker compose up -d
```

## Environment Variables

See `.env.example`. CoLink binaries read `COLINK_*` variables directly, and Docker Compose passes the same names into containers.

## Services

| Service | Responsibility |
|---|---|
| `nginx` | Public entrypoint and path routing |
| `server` | Auth, account, device and notes APIs, attachment storage, WebSocket relay, frontend fallback |
| `update` | GitHub release checks, cached update metadata, update asset downloads |
| `postgres` | PostgreSQL storage |

The server persists cloud notes and their attachments. WebSocket messages, transferred files, and clipboard content are relayed only between authenticated devices belonging to the same user.
